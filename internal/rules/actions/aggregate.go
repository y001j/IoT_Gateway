package actions

import (
	"context"
	"fmt"
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
		TTL: 10 * time.Minute, // 默认TTL
	}

	// 解析窗口大小 (优先使用window_size，兼容count字段)
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

	// 兼容旧配置中的count字段
	if countVal, ok := config["count"]; ok && aggregateConfig.WindowSize == 0 {
		if count, ok := countVal.(int); ok {
			aggregateConfig.WindowSize = count
		} else if countFloat, ok := countVal.(float64); ok {
			aggregateConfig.WindowSize = int(countFloat)
		}
	}

	// 如果都没设置，使用默认值
	if aggregateConfig.WindowSize == 0 {
		aggregateConfig.WindowSize = 10
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
	// 验证窗口大小
	if windowSizeVal, ok := config["window_size"]; ok {
		switch v := windowSizeVal.(type) {
		case int:
			if v <= 0 {
				return fmt.Errorf("window_size必须大于0")
			}
		case float64:
			if v <= 0 {
				return fmt.Errorf("window_size必须大于0")
			}
		case string:
			if size, err := strconv.Atoi(v); err != nil || size <= 0 {
				return fmt.Errorf("window_size格式错误或必须大于0")
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
			for _, fn := range v {
				if fnStr, ok := fn.(string); ok {
					functions = append(functions, fnStr)
				} else {
					return fmt.Errorf("functions数组中包含非字符串元素")
				}
			}
		case []string:
			functions = v
		case string:
			functions = []string{v}
		default:
			return fmt.Errorf("functions类型错误")
		}

		// 验证支持的函数
		supportedFunctions := map[string]bool{
			"avg": true, "mean": true, "average": true,
			"sum": true, "count": true, "min": true, "max": true,
			"stddev": true, "std": true, "variance": true,
			"median": true, "first": true, "last": true,
		}

		for _, fn := range functions {
			if !supportedFunctions[fn] {
				return fmt.Errorf("不支持的聚合函数: %s", fn)
			}
		}
	}

	// 验证TTL
	if ttlVal, ok := config["ttl"]; ok {
		switch v := ttlVal.(type) {
		case string:
			if _, err := time.ParseDuration(v); err != nil {
				return fmt.Errorf("TTL格式错误: %v", err)
			}
		case float64:
			if v <= 0 {
				return fmt.Errorf("TTL必须大于0")
			}
		default:
			return fmt.Errorf("TTL类型错误")
		}
	}

	return nil
}