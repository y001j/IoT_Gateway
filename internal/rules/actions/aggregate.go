package actions

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// AggregateHandler Aggregate动作处理器 - 使用增量统计优化
type AggregateHandler struct {
	manager *AggregateManager
}

// NewAggregateHandler 创建Aggregate处理器
func NewAggregateHandler() *AggregateHandler {
	return &AggregateHandler{
		manager: NewAggregateManager(),
	}
}

// Name 返回处理器名称
func (h *AggregateHandler) Name() string {
	return "aggregate"
}

// Execute 执行聚合动作 - 使用增量统计优化
func (h *AggregateHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	// 首先验证配置
	if err := h.Validate(config); err != nil {
		return &rules.ActionResult{
			Type:     "aggregate",
			Success:  false,
			Error:    fmt.Sprintf("配置验证失败: %v", err),
			Duration: 0,
		}, err
	}
	
	// 解析配置
	aggregateConfig, err := h.parseConfig(config)
	if err != nil {
		return &rules.ActionResult{
			Type:     "aggregate",
			Success:  false,
			Error:    fmt.Sprintf("解析配置失败: %v", err),
			Duration: 0,
		}, err
	}

	// 使用新的聚合管理器处理
	return h.manager.ProcessPoint(rule, point, aggregateConfig)
}

// parseConfig 解析聚合配置
func (h *AggregateHandler) parseConfig(config map[string]interface{}) (*AggregateConfig, error) {
	aggregateConfig := &AggregateConfig{
		TTL:        10 * time.Minute, // 默认TTL
		WindowType: "count",          // 默认数量窗口
		Alignment:  "none",           // 默认不对齐
	}

	// 解析窗口大小 (优先使用window_size，兼容UI的size字段和旧的count字段)
	if windowSizeVal, ok := config["window_size"]; ok {
		if windowSize, ok := windowSizeVal.(int); ok {
			aggregateConfig.WindowSize = windowSize
		} else if windowSizeStr, ok := windowSizeVal.(string); ok {
			if windowSize, err := strconv.Atoi(windowSizeStr); err == nil {
				aggregateConfig.WindowSize = windowSize
			}
		} else if windowSizeFloat, ok := windowSizeVal.(float64); ok {
			aggregateConfig.WindowSize = int(windowSizeFloat)
		}
	}

	// 兼容UI配置中的size字段
	if sizeVal, ok := config["size"]; ok && aggregateConfig.WindowSize == 0 {
		if size, ok := sizeVal.(int); ok {
			aggregateConfig.WindowSize = size
		} else if sizeFloat, ok := sizeVal.(float64); ok {
			aggregateConfig.WindowSize = int(sizeFloat)
		} else if sizeStr, ok := sizeVal.(string); ok {
			if size, err := strconv.Atoi(sizeStr); err == nil {
				aggregateConfig.WindowSize = size
			}
		}
	}

	// 兼容旧配置中的count字段
	if countVal, ok := config["count"]; ok && aggregateConfig.WindowSize == 0 {
		if count, ok := countVal.(int); ok {
			aggregateConfig.WindowSize = count
		} else if countFloat, ok := countVal.(float64); ok {
			aggregateConfig.WindowSize = int(countFloat)
		}
	}

	// 解析窗口类型和时间窗口配置
	if windowTypeVal, ok := config["window_type"]; ok {
		if windowType, ok := windowTypeVal.(string); ok {
			aggregateConfig.WindowType = windowType
			
			// 如果是时间窗口，解析时间参数
			if windowType == "time" {
				if windowVal, ok := config["window"]; ok {
					if windowStr, ok := windowVal.(string); ok {
						if duration, err := time.ParseDuration(windowStr); err == nil {
							aggregateConfig.WindowDuration = duration
							// 时间窗口模式下，WindowSize用于限制最大缓存数据点数
							if aggregateConfig.WindowSize == 0 {
								aggregateConfig.WindowSize = 1000 // 默认最大缓存1000个点
							}
						} else {
							return nil, fmt.Errorf("无效的时间窗口格式: %s", windowStr)
						}
					} else {
						return nil, fmt.Errorf("时间窗口参数必须为字符串格式，如'1m', '30s'")
					}
				} else {
					return nil, fmt.Errorf("时间窗口模式下必须配置window参数")
				}
			}
		}
	}

	// 解析对齐方式 (Phase 2)
	if alignmentVal, ok := config["alignment"]; ok {
		if alignment, ok := alignmentVal.(string); ok {
			if alignment == "calendar" || alignment == "none" {
				aggregateConfig.Alignment = alignment
			}
		}
	}

	// 如果都没设置，使用默认值为10（滑动窗口模式）
	// 设置为0时表示累积模式，设置为>0时表示滑动窗口模式
	if aggregateConfig.WindowSize == 0 && aggregateConfig.WindowType == "count" {
		// 检查是否有明确的配置，如果没有任何窗口配置，则使用默认值10
		if _, hasWindowSize := config["window_size"]; !hasWindowSize {
			if _, hasSize := config["size"]; !hasSize {
				if _, hasCount := config["count"]; !hasCount {
					aggregateConfig.WindowSize = 10 // 默认滑动窗口大小
				}
			}
		}
	}

	// 解析函数列表
	if functionsVal, ok := config["functions"]; ok {
		if functions, ok := functionsVal.([]interface{}); ok {
			for _, fn := range functions {
				if fnStr, ok := fn.(string); ok {
					aggregateConfig.Functions = append(aggregateConfig.Functions, fnStr)
				}
			}
		} else if functionsStr, ok := functionsVal.([]string); ok {
			aggregateConfig.Functions = functionsStr
		} else if functionStr, ok := functionsVal.(string); ok {
			aggregateConfig.Functions = []string{functionStr}
		}
	}

	// 如果没有指定函数，默认使用平均值
	if len(aggregateConfig.Functions) == 0 {
		aggregateConfig.Functions = []string{"avg"}
	}

	// 解析分组字段
	if groupByVal, ok := config["group_by"]; ok {
		if groupBy, ok := groupByVal.([]interface{}); ok {
			for _, field := range groupBy {
				if fieldStr, ok := field.(string); ok {
					aggregateConfig.GroupBy = append(aggregateConfig.GroupBy, fieldStr)
				}
			}
		} else if groupByStr, ok := groupByVal.([]string); ok {
			aggregateConfig.GroupBy = groupByStr
		} else if groupByStr, ok := groupByVal.(string); ok {
			aggregateConfig.GroupBy = []string{groupByStr}
		}
	}

	// 解析输出配置
	if outputVal, ok := config["output"]; ok {
		if output, ok := outputVal.(map[string]interface{}); ok {
			aggregateConfig.Output = output
		}
	}

	// 兼容UI配置中的output_key字段
	if outputKeyVal, ok := config["output_key"]; ok {
		if outputKey, ok := outputKeyVal.(string); ok {
			if aggregateConfig.Output == nil {
				aggregateConfig.Output = make(map[string]interface{})
			}
			aggregateConfig.Output["key"] = outputKey
		}
	}

	// 兼容UI配置中的forward字段
	if forwardVal, ok := config["forward"]; ok {
		if forward, ok := forwardVal.(bool); ok {
			if aggregateConfig.Output == nil {
				aggregateConfig.Output = make(map[string]interface{})
			}
			aggregateConfig.Output["forward"] = forward
		}
	}

	// 解析TTL
	if ttlVal, ok := config["ttl"]; ok {
		if ttlStr, ok := ttlVal.(string); ok {
			if duration, err := time.ParseDuration(ttlStr); err == nil {
				aggregateConfig.TTL = duration
			}
		} else if ttlFloat, ok := ttlVal.(float64); ok {
			aggregateConfig.TTL = time.Duration(ttlFloat) * time.Second
		}
	}

	// 解析阈值配置 (Phase 2增强功能)
	if upperLimitVal, ok := config["upper_limit"]; ok {
		if upperLimit, ok := upperLimitVal.(float64); ok {
			aggregateConfig.UpperLimit = &upperLimit
		}
	}
	
	if lowerLimitVal, ok := config["lower_limit"]; ok {
		if lowerLimit, ok := lowerLimitVal.(float64); ok {
			aggregateConfig.LowerLimit = &lowerLimit
		}
	}
	
	if outlierThresholdVal, ok := config["outlier_threshold"]; ok {
		if outlierThreshold, ok := outlierThresholdVal.(float64); ok && outlierThreshold > 0 {
			aggregateConfig.OutlierThreshold = &outlierThreshold
		}
	}

	return aggregateConfig, nil
}

// Close 关闭处理器
func (h *AggregateHandler) Close() {
	if h.manager != nil {
		log.Info().Msg("关闭聚合处理器")
		h.manager.Close()
	}
}

// GetStats 获取处理器统计信息
func (h *AggregateHandler) GetStats() map[string]interface{} {
	if h.manager != nil {
		return h.manager.GetStats()
	}
	return map[string]interface{}{}
}

// Validate 验证配置
func (h *AggregateHandler) Validate(config map[string]interface{}) error {
	// 验证窗口类型和相关配置
	if windowTypeVal, ok := config["window_type"]; ok {
		if windowType, ok := windowTypeVal.(string); ok {
			if windowType == "time" {
				// 时间窗口必须有window参数
				if windowVal, ok := config["window"]; !ok {
					return fmt.Errorf("时间窗口模式下必须配置window参数")
				} else if windowStr, ok := windowVal.(string); !ok {
					return fmt.Errorf("window参数必须为字符串格式，如'1m', '30s'")
				} else {
					if _, err := time.ParseDuration(windowStr); err != nil {
						return fmt.Errorf("无效的时间窗口格式: %s", windowStr)
					}
				}
			} else if windowType != "count" {
				return fmt.Errorf("不支持的窗口类型: %s，仅支持'time'和'count'", windowType)
			}
		}
	}
	// 验证窗口大小
	if windowSizeVal, ok := config["window_size"]; ok {
		switch v := windowSizeVal.(type) {
		case int:
			if v < 0 {
				return fmt.Errorf("window_size不能为负数")
			}
			if v > 100000 { // 更严格的限制以防止资源耗尽
				return fmt.Errorf("window_size不能超过100000")
			}
		case float64:
			if v < 0 {
				return fmt.Errorf("window_size不能为负数")
			}
			if v > 100000 {
				return fmt.Errorf("window_size不能超过100000")
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("window_size不能为NaN或无穷大")
			}
		case string:
			if len(v) > 10 { // 防止超长字符串攻击
				return fmt.Errorf("window_size字符串长度不能超过10")
			}
			if size, err := strconv.Atoi(v); err != nil {
				return fmt.Errorf("window_size格式错误: %v", err)
			} else if size < 0 {
				return fmt.Errorf("window_size不能为负数")
			} else if size > 100000 {
				return fmt.Errorf("window_size不能超过100000")
			}
		default:
			return fmt.Errorf("window_size类型错误，应为数字")
		}
	}

	// 验证函数
	if functionsVal, ok := config["functions"]; ok {
		var functions []string
		switch v := functionsVal.(type) {
		case []interface{}:
			// 防止超大数组导致资源耗尽
			if len(v) > 30 {
				return fmt.Errorf("functions数组长度不能超过30")
			}
			for i, fn := range v {
				if i >= 30 { // 双重检查
					return fmt.Errorf("functions数组长度不能超过30")
				}
				if fnStr, ok := fn.(string); ok {
					if len(fnStr) > 50 { // 防止超长函数名
						return fmt.Errorf("函数名长度不能超过50个字符")
					}
					functions = append(functions, fnStr)
				} else {
					return fmt.Errorf("functions数组中包含非字符串元素")
				}
			}
		case []string:
			// 防止超大数组
			if len(v) > 30 {
				return fmt.Errorf("functions数组长度不能超过30")
			}
			for _, fnStr := range v {
				if len(fnStr) > 50 { // 防止超长函数名
					return fmt.Errorf("函数名长度不能超过50个字符")
				}
			}
			functions = v
		case string:
			if len(v) > 50 { // 防止超长函数名
				return fmt.Errorf("函数名长度不能超过50个字符")
			}
			functions = []string{v}
		default:
			return fmt.Errorf("functions类型错误")
		}

		// 验证支持的函数
		supportedFunctions := map[string]bool{
			// 原有基础函数
			"avg": true, "mean": true, "average": true,
			"sum": true, "count": true, "min": true, "max": true,
			"stddev": true, "std": true, "variance": true,
			"median": true, "first": true, "last": true,
			
			// Phase 1: 高价值新增函数
			"p90": true, "p95": true, "p99": true,
			"null_rate": true, "completeness": true,
			"change": true, "change_rate": true,
			"outlier_count": true,
			
			// Phase 2: 扩展函数
			"p25": true, "p50": true, "p75": true,  // 添加p50支持
			"volatility": true, "cv": true,
			"above_count": true, "below_count": true, "in_range_count": true,
		}

		// 防止空函数列表
		if len(functions) == 0 {
			return fmt.Errorf("functions不能为空")
		}

		for i, fn := range functions {
			// 额外的边界检查
			if i >= len(functions) {
				return fmt.Errorf("函数列表访问越界")
			}
			// 边界检查：防止空字符串或无效函数名
			if fn == "" {
				return fmt.Errorf("functions中包含空字符串")
			}
			if !supportedFunctions[fn] {
				return fmt.Errorf("不支持的聚合函数: %s", fn)
			}
		}
	}

	// 验证TTL - 加强安全性检查
	if ttlVal, ok := config["ttl"]; ok {
		switch v := ttlVal.(type) {
		case string:
			if len(v) > 20 { // 防止超长TTL字符串攻击
				return fmt.Errorf("TTL字符串长度不能超过20个字符")
			}
			
			duration, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("TTL格式错误: %v", err)
			}
			
			// 限制TTL的合理范围：1秒 - 24小时
			if duration < time.Second {
				return fmt.Errorf("TTL不能小于1秒")
			}
			if duration > 24*time.Hour {
				return fmt.Errorf("TTL不能超过24小时")
			}
			
		case float64:
			if v <= 0 {
				return fmt.Errorf("TTL必须大于0")
			}
			if v > 86400 { // 86400秒 = 24小时
				return fmt.Errorf("TTL不能超过24小时(86400秒)")
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("TTL不能为NaN或无穷大")
			}
			
		default:
			return fmt.Errorf("TTL类型错误，应为字符串或数字")
		}
	}

	// 验证阈值配置 (Phase 2增强功能)
	if upperLimitVal, ok := config["upper_limit"]; ok {
		switch v := upperLimitVal.(type) {
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("上限阈值不能为NaN或无穷大")
			}
		default:
			return fmt.Errorf("上限阈值类型错误，应为数字")
		}
	}
	
	if lowerLimitVal, ok := config["lower_limit"]; ok {
		switch v := lowerLimitVal.(type) {
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("下限阈值不能为NaN或无穷大")
			}
		default:
			return fmt.Errorf("下限阈值类型错误，应为数字")
		}
	}
	
	if outlierThresholdVal, ok := config["outlier_threshold"]; ok {
		switch v := outlierThresholdVal.(type) {
		case float64:
			if v <= 0 {
				return fmt.Errorf("异常值阈值必须大于0")
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("异常值阈值不能为NaN或无穷大")
			}
		default:
			return fmt.Errorf("异常值阈值类型错误，应为数字")
		}
	}
	
	// 验证阈值逻辑合理性
	if upperLimitVal, ok1 := config["upper_limit"]; ok1 {
		if lowerLimitVal, ok2 := config["lower_limit"]; ok2 {
			if upperLimit, ok1 := upperLimitVal.(float64); ok1 {
				if lowerLimit, ok2 := lowerLimitVal.(float64); ok2 {
					if upperLimit <= lowerLimit {
						return fmt.Errorf("上限阈值(%.2f)必须大于下限阈值(%.2f)", upperLimit, lowerLimit)
					}
				}
			}
		}
	}

	return nil
}