package actions

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// OptimizedAggregateHandler 优化版聚合处理器
// 使用分片管理器实现超高性能聚合处理
type OptimizedAggregateHandler struct {
	shardedManager *ShardedAggregateManager
	enabled        bool
}

// NewOptimizedAggregateHandler 创建优化版聚合处理器
func NewOptimizedAggregateHandler() *OptimizedAggregateHandler {
	return &OptimizedAggregateHandler{
		shardedManager: NewShardedAggregateManager(),
		enabled:        true,
	}
}

// Name 返回处理器名称
func (h *OptimizedAggregateHandler) Name() string {
	return "optimized_aggregate"
}

// Execute 执行优化版聚合动作
func (h *OptimizedAggregateHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	if !h.enabled {
		return h.fallbackToOriginal(ctx, point, rule, config)
	}

	start := time.Now()
	
	// 验证配置
	if err := h.Validate(config); err != nil {
		return &rules.ActionResult{
			Type:     "aggregate",
			Success:  false,
			Error:    fmt.Sprintf("配置验证失败: %v", err),
			Duration: time.Since(start),
		}, err
	}
	
	// 解析配置
	aggregateConfig, err := h.parseOptimizedConfig(config)
	if err != nil {
		return &rules.ActionResult{
			Type:     "aggregate",
			Success:  false,
			Error:    fmt.Sprintf("解析配置失败: %v", err),
			Duration: time.Since(start),
		}, err
	}

	// 使用分片管理器处理
	return h.shardedManager.ProcessPoint(rule, point, aggregateConfig)
}

// parseOptimizedConfig 解析优化版聚合配置
func (h *OptimizedAggregateHandler) parseOptimizedConfig(config map[string]interface{}) (*AggregateConfig, error) {
	aggregateConfig := &AggregateConfig{
		TTL: 10 * time.Minute, // 默认TTL
	}

	// 解析窗口大小 - 支持多种格式以保持兼容性
	windowSize := h.parseWindowSize(config)
	aggregateConfig.WindowSize = windowSize

	// 解析函数列表
	functions := h.parseFunctions(config)
	aggregateConfig.Functions = functions

	// 解析分组字段
	groupBy := h.parseGroupBy(config)
	aggregateConfig.GroupBy = groupBy

	// 解析阈值配置
	h.parseThresholds(config, aggregateConfig)

	// 解析输出配置
	if outputVal, ok := config["output"]; ok {
		if output, ok := outputVal.(map[string]interface{}); ok {
			aggregateConfig.Output = output
		}
	}

	return aggregateConfig, nil
}

// parseWindowSize 解析窗口大小
func (h *OptimizedAggregateHandler) parseWindowSize(config map[string]interface{}) int {
	// 优先使用window_size
	if windowSizeVal, ok := config["window_size"]; ok {
		return h.convertToInt(windowSizeVal)
	}
	
	// 兼容UI配置中的size字段
	if sizeVal, ok := config["size"]; ok {
		return h.convertToInt(sizeVal)
	}
	
	// 兼容旧配置中的count字段
	if countVal, ok := config["count"]; ok {
		return h.convertToInt(countVal)
	}
	
	// 默认值：滑动窗口模式
	return 10
}

// parseFunctions 解析函数列表
func (h *OptimizedAggregateHandler) parseFunctions(config map[string]interface{}) []string {
	if functionsVal, ok := config["functions"]; ok {
		if functions, ok := functionsVal.([]interface{}); ok {
			result := make([]string, 0, len(functions))
			for _, fn := range functions {
				if fnStr, ok := fn.(string); ok {
					result = append(result, fnStr)
				}
			}
			return result
		} else if functionsStr, ok := functionsVal.([]string); ok {
			return functionsStr
		} else if functionStr, ok := functionsVal.(string); ok {
			return []string{functionStr}
		}
	}
	
	// 默认使用平均值
	return []string{"avg"}
}

// parseGroupBy 解析分组字段
func (h *OptimizedAggregateHandler) parseGroupBy(config map[string]interface{}) []string {
	if groupByVal, ok := config["group_by"]; ok {
		if groupBy, ok := groupByVal.([]interface{}); ok {
			result := make([]string, 0, len(groupBy))
			for _, field := range groupBy {
				if fieldStr, ok := field.(string); ok {
					result = append(result, fieldStr)
				}
			}
			return result
		} else if groupByStr, ok := groupByVal.([]string); ok {
			return groupByStr
		} else if groupByStr, ok := groupByVal.(string); ok {
			return []string{groupByStr}
		}
	}
	return []string{}
}

// parseThresholds 解析阈值配置
func (h *OptimizedAggregateHandler) parseThresholds(config map[string]interface{}, aggregateConfig *AggregateConfig) {
	if upperVal, ok := config["upper_limit"]; ok {
		if upper, ok := upperVal.(float64); ok {
			aggregateConfig.UpperLimit = &upper
		}
	}
	
	if lowerVal, ok := config["lower_limit"]; ok {
		if lower, ok := lowerVal.(float64); ok {
			aggregateConfig.LowerLimit = &lower
		}
	}
	
	if thresholdVal, ok := config["outlier_threshold"]; ok {
		if threshold, ok := thresholdVal.(float64); ok && threshold > 0 {
			aggregateConfig.OutlierThreshold = &threshold
		}
	}
}

// convertToInt 转换各种类型为int
func (h *OptimizedAggregateHandler) convertToInt(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		if intVal, err := strconv.Atoi(v); err == nil {
			return intVal
		}
	}
	return 0
}

// Validate 验证配置 - 高性能版本
func (h *OptimizedAggregateHandler) Validate(config map[string]interface{}) error {
	if config == nil {
		return fmt.Errorf("聚合配置不能为空")
	}

	// 验证函数列表
	if err := h.validateFunctions(config); err != nil {
		return err
	}

	// 验证窗口大小
	if err := h.validateWindowSize(config); err != nil {
		return err
	}

	// 验证阈值配置
	if err := h.validateThresholds(config); err != nil {
		return err
	}

	return nil
}

// validateFunctions 验证函数列表
func (h *OptimizedAggregateHandler) validateFunctions(config map[string]interface{}) error {
	supportedFunctions := map[string]bool{
		// 基础统计函数
		"count": true, "sum": true, "avg": true, "mean": true, "average": true,
		"min": true, "max": true, "stddev": true, "std": true, "variance": true,
		"first": true, "last": true, "median": true,
		
		// Phase 1 增强函数
		"p90": true, "p95": true, "p99": true,
		"null_rate": true, "completeness": true,
		"change": true, "change_rate": true, "outlier_count": true,
		
		// Phase 2 高级函数  
		"p25": true, "p50": true, "p75": true,
		"volatility": true, "cv": true,
		"above_count": true, "below_count": true, "in_range_count": true,
	}

	functions := h.parseFunctions(config)
	for _, function := range functions {
		if !supportedFunctions[function] {
			return fmt.Errorf("不支持的聚合函数: %s", function)
		}
	}

	return nil
}

// validateWindowSize 验证窗口大小
func (h *OptimizedAggregateHandler) validateWindowSize(config map[string]interface{}) error {
	windowSize := h.parseWindowSize(config)
	
	if windowSize < 0 {
		return fmt.Errorf("窗口大小不能为负数: %d", windowSize)
	}
	
	if windowSize > 1000000 {
		return fmt.Errorf("窗口大小过大: %d，最大允许1000000", windowSize)
	}
	
	return nil
}

// validateThresholds 验证阈值配置
func (h *OptimizedAggregateHandler) validateThresholds(config map[string]interface{}) error {
	if upperVal, ok := config["upper_limit"]; ok {
		if _, ok := upperVal.(float64); !ok {
			return fmt.Errorf("上限阈值必须是数字")
		}
	}
	
	if lowerVal, ok := config["lower_limit"]; ok {
		if _, ok := lowerVal.(float64); !ok {
			return fmt.Errorf("下限阈值必须是数字")
		}
	}
	
	if thresholdVal, ok := config["outlier_threshold"]; ok {
		if threshold, ok := thresholdVal.(float64); !ok || threshold <= 0 {
			return fmt.Errorf("异常值阈值必须是正数")
		}
	}
	
	return nil
}

// fallbackToOriginal 回退到原始实现
func (h *OptimizedAggregateHandler) fallbackToOriginal(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	// 创建原始处理器并执行
	originalHandler := NewAggregateHandler()
	return originalHandler.Execute(ctx, point, rule, config)
}

// Close 关闭处理器
func (h *OptimizedAggregateHandler) Close() {
	if h.shardedManager != nil {
		h.shardedManager.Close()
	}
}

// GetMetrics 获取性能指标
func (h *OptimizedAggregateHandler) GetMetrics() map[string]interface{} {
	if h.shardedManager != nil {
		return h.shardedManager.GetMetrics()
	}
	return map[string]interface{}{}
}

// EnableOptimization 启用优化
func (h *OptimizedAggregateHandler) EnableOptimization(enable bool) {
	h.enabled = enable
	
	if enable {
		log.Info().Msg("已启用高性能聚合引擎")
	} else {
		log.Info().Msg("已禁用高性能聚合引擎，回退到原始实现")
	}
}

// IsOptimizationEnabled 检查是否启用优化
func (h *OptimizedAggregateHandler) IsOptimizationEnabled() bool {
	return h.enabled
}

// AggregateConfig is now defined in aggregate_manager.go with threshold support

// 性能配置选项
type PerformanceConfig struct {
	EnableBatchProcessing bool          `json:"enable_batch_processing"`
	BatchSize            int           `json:"batch_size"`
	FlushInterval        time.Duration `json:"flush_interval"`
	NumShards            int           `json:"num_shards"`
}

// GetPerformanceConfig 获取性能配置
func (h *OptimizedAggregateHandler) GetPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		EnableBatchProcessing: true,
		BatchSize:            500,
		FlushInterval:        10 * time.Millisecond,
		NumShards:            int(h.shardedManager.numShards),
	}
}

// UpdatePerformanceConfig 更新性能配置
func (h *OptimizedAggregateHandler) UpdatePerformanceConfig(config *PerformanceConfig) {
	if config.BatchSize > 0 {
		h.shardedManager.batchProcessor.batchSize = int32(config.BatchSize)
	}
	
	if config.FlushInterval > 0 {
		h.shardedManager.batchProcessor.flushInterval = config.FlushInterval
	}
	
	log.Info().
		Interface("config", config).
		Msg("已更新聚合引擎性能配置")
}

// GetSupportedFunctions 获取支持的函数列表
func (h *OptimizedAggregateHandler) GetSupportedFunctions() []string {
	return []string{
		// 基础统计函数 (13个)
		"count", "sum", "avg", "mean", "average", "min", "max", 
		"stddev", "std", "variance", "first", "last", "median",
		
		// Phase 1 增强函数 (8个)
		"p90", "p95", "p99", "null_rate", "completeness",
		"change", "change_rate", "outlier_count",
		
		// Phase 2 高级函数 (7个)
		"p25", "p50", "p75", "volatility", "cv",
		"above_count", "below_count", "in_range_count",
	}
}

// 初始化函数 - 注册优化聚合处理器工厂
func init() {
	// 导入rules包并设置工厂函数
	rules.SetOptimizedAggregateHandlerFactory(func() rules.OptimizedAggregateHandler {
		return NewOptimizedAggregateHandler()
	})
	
	// 检查环境变量来决定是否启用优化
	if enableOpt := os.Getenv("IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE"); enableOpt == "true" {
		log.Info().Msg("环境变量检测：启用高性能聚合引擎工厂已注册")
	}
}