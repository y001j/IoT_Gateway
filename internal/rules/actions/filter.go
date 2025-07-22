package actions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// FilterHandler Filter动作处理器
type FilterHandler struct {
	duplicateCache map[string]DuplicateEntry
	mu             sync.RWMutex
}

// DuplicateEntry 重复数据缓存条目
type DuplicateEntry struct {
	Value     interface{}
	Timestamp time.Time
}

// NewFilterHandler 创建Filter处理器
func NewFilterHandler() *FilterHandler {
	handler := &FilterHandler{
		duplicateCache: make(map[string]DuplicateEntry),
	}

	// 启动清理协程
	go handler.cleanupCache()

	return handler
}

// Name 返回处理器名称
func (h *FilterHandler) Name() string {
	return "filter"
}

// Execute 执行过滤动作
func (h *FilterHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	start := time.Now()

	// 解析配置
	filterConfig, err := h.parseConfig(config)
	if err != nil {
		return &rules.ActionResult{
			Type:     "filter",
			Success:  false,
			Error:    fmt.Sprintf("解析配置失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// 执行过滤
	shouldFilter, reason, err := h.shouldFilter(point, filterConfig)
	if err != nil {
		return &rules.ActionResult{
			Type:     "filter",
			Success:  false,
			Error:    fmt.Sprintf("过滤检查失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// 记录过滤结果
	if shouldFilter {
		log.Debug().
			Str("rule_id", rule.ID).
			Str("device_id", point.DeviceID).
			Str("key", point.Key).
			Interface("value", point.Value).
			Str("reason", reason).
			Str("filter_type", filterConfig.Type).
			Msg("数据被过滤")
	}

	return &rules.ActionResult{
		Type:     "filter",
		Success:  true,
		Duration: time.Since(start),
		Output: map[string]interface{}{
			"filtered":    shouldFilter,
			"reason":      reason,
			"filter_type": filterConfig.Type,
			"point":       point,
		},
	}, nil
}

// FilterConfig 过滤配置
type FilterConfig struct {
	Type       string                 `json:"type"`       // duplicate, range, rate_limit, pattern, custom
	Parameters map[string]interface{} `json:"parameters"` // 过滤参数
	Action     string                 `json:"action"`     // drop, pass, modify
	TTL        time.Duration          `json:"ttl"`        // 缓存生存时间
}

// parseConfig 解析配置
func (h *FilterHandler) parseConfig(config map[string]interface{}) (*FilterConfig, error) {
	filterConfig := &FilterConfig{
		Type:       "duplicate",
		Parameters: make(map[string]interface{}),
		Action:     "drop",
		TTL:        5 * time.Minute,
	}

	// 解析过滤类型
	if filterType, ok := config["type"].(string); ok {
		filterConfig.Type = filterType
	}

	// 解析参数
	if parameters, ok := config["parameters"].(map[string]interface{}); ok {
		filterConfig.Parameters = parameters
	}

	// 解析动作
	if action, ok := config["action"].(string); ok {
		filterConfig.Action = action
	}

	// 解析TTL
	if ttlStr, ok := config["ttl"].(string); ok {
		if duration, err := time.ParseDuration(ttlStr); err == nil {
			filterConfig.TTL = duration
		}
	}

	return filterConfig, nil
}

// shouldFilter 检查是否应该过滤
func (h *FilterHandler) shouldFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	switch config.Type {
	case "duplicate":
		return h.duplicateFilter(point, config)
	case "range":
		return h.rangeFilter(point, config)
	case "rate_limit":
		return h.rateLimitFilter(point, config)
	case "pattern":
		return h.patternFilter(point, config)
	case "null":
		return h.nullFilter(point, config)
	case "threshold":
		return h.thresholdFilter(point, config)
	case "time_window":
		return h.timeWindowFilter(point, config)
	default:
		return false, "", fmt.Errorf("不支持的过滤类型: %s", config.Type)
	}
}

// duplicateFilter 重复数据过滤
func (h *FilterHandler) duplicateFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 生成缓存键
	cacheKey := fmt.Sprintf("dup:%s:%s", point.DeviceID, point.Key)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查是否存在重复数据
	if entry, exists := h.duplicateCache[cacheKey]; exists {
		// 检查值是否相同
		if h.valuesEqual(entry.Value, point.Value) {
			// 检查时间间隔
			if time.Since(entry.Timestamp) < config.TTL {
				return true, "重复数据", nil
			}
		}
	}

	// 更新缓存
	h.duplicateCache[cacheKey] = DuplicateEntry{
		Value:     point.Value,
		Timestamp: time.Now(),
	}

	return false, "", nil
}

// rangeFilter 范围过滤
func (h *FilterHandler) rangeFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	min, hasMin := config.Parameters["min"]
	max, hasMax := config.Parameters["max"]

	if !hasMin && !hasMax {
		return false, "", fmt.Errorf("范围过滤需要配置min或max参数")
	}

	// 转换为数字进行比较
	value, err := h.toFloat64(point.Value)
	if err != nil {
		return false, "", fmt.Errorf("无法转换为数字进行范围比较: %w", err)
	}

	if hasMin {
		if minVal, err := h.toFloat64(min); err == nil {
			if value < minVal {
				return true, fmt.Sprintf("值 %.2f 小于最小值 %.2f", value, minVal), nil
			}
		}
	}

	if hasMax {
		if maxVal, err := h.toFloat64(max); err == nil {
			if value > maxVal {
				return true, fmt.Sprintf("值 %.2f 大于最大值 %.2f", value, maxVal), nil
			}
		}
	}

	return false, "", nil
}

// rateLimitFilter 速率限制过滤
func (h *FilterHandler) rateLimitFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 获取速率限制参数
	maxRate, ok := config.Parameters["max_rate"].(float64)
	if !ok {
		return false, "", fmt.Errorf("速率限制需要配置max_rate参数")
	}

	windowStr, ok := config.Parameters["window"].(string)
	if !ok {
		windowStr = "1m"
	}

	window, err := time.ParseDuration(windowStr)
	if err != nil {
		return false, "", fmt.Errorf("无效的时间窗口: %w", err)
	}

	// 生成缓存键
	cacheKey := fmt.Sprintf("rate:%s:%s", point.DeviceID, point.Key)

	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()

	// 检查上次记录时间
	if entry, exists := h.duplicateCache[cacheKey]; exists {
		timeSince := now.Sub(entry.Timestamp)
		if timeSince < window {
			// 计算当前速率
			currentRate := 1.0 / timeSince.Seconds() * 60 // 转换为每分钟
			if currentRate > maxRate {
				return true, fmt.Sprintf("速率 %.2f/min 超过限制 %.2f/min", currentRate, maxRate), nil
			}
		}
	}

	// 更新时间戳
	h.duplicateCache[cacheKey] = DuplicateEntry{
		Value:     nil,
		Timestamp: now,
	}

	return false, "", nil
}

// patternFilter 模式过滤
func (h *FilterHandler) patternFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	pattern, ok := config.Parameters["pattern"].(string)
	if !ok {
		return false, "", fmt.Errorf("模式过滤需要配置pattern参数")
	}

	field, ok := config.Parameters["field"].(string)
	if !ok {
		field = "value"
	}

	// 获取要匹配的值
	var valueToMatch string
	switch field {
	case "device_id":
		valueToMatch = point.DeviceID
	case "key":
		valueToMatch = point.Key
	case "value":
		valueToMatch = fmt.Sprintf("%v", point.Value)
	default:
		if point.Tags != nil {
			if tagValue, exists := point.Tags[field]; exists {
				valueToMatch = tagValue
			}
		}
	}

	// 简单的模式匹配（可以扩展为正则表达式）
	matched := h.simplePatternMatch(valueToMatch, pattern)

	if matched {
		return true, fmt.Sprintf("值 '%s' 匹配模式 '%s'", valueToMatch, pattern), nil
	}

	return false, "", nil
}

// nullFilter 空值过滤
func (h *FilterHandler) nullFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	if point.Value == nil {
		return true, "值为空", nil
	}

	// 检查字符串是否为空
	if str, ok := point.Value.(string); ok && str == "" {
		return true, "字符串值为空", nil
	}

	return false, "", nil
}

// thresholdFilter 阈值过滤
func (h *FilterHandler) thresholdFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	threshold, ok := config.Parameters["threshold"]
	if !ok {
		return false, "", fmt.Errorf("阈值过滤需要配置threshold参数")
	}

	operator, ok := config.Parameters["operator"].(string)
	if !ok {
		operator = "gt" // 默认为大于
	}

	// 转换为数字进行比较
	value, err := h.toFloat64(point.Value)
	if err != nil {
		return false, "", fmt.Errorf("无法转换为数字进行阈值比较: %w", err)
	}

	thresholdVal, err := h.toFloat64(threshold)
	if err != nil {
		return false, "", fmt.Errorf("无法转换阈值为数字: %w", err)
	}

	var shouldFilter bool
	var reason string

	switch operator {
	case "gt":
		shouldFilter = value > thresholdVal
		reason = fmt.Sprintf("值 %.2f 大于阈值 %.2f", value, thresholdVal)
	case "lt":
		shouldFilter = value < thresholdVal
		reason = fmt.Sprintf("值 %.2f 小于阈值 %.2f", value, thresholdVal)
	case "eq":
		shouldFilter = value == thresholdVal
		reason = fmt.Sprintf("值 %.2f 等于阈值 %.2f", value, thresholdVal)
	case "ne":
		shouldFilter = value != thresholdVal
		reason = fmt.Sprintf("值 %.2f 不等于阈值 %.2f", value, thresholdVal)
	case "gte":
		shouldFilter = value >= thresholdVal
		reason = fmt.Sprintf("值 %.2f 大于等于阈值 %.2f", value, thresholdVal)
	case "lte":
		shouldFilter = value <= thresholdVal
		reason = fmt.Sprintf("值 %.2f 小于等于阈值 %.2f", value, thresholdVal)
	default:
		return false, "", fmt.Errorf("不支持的比较操作符: %s", operator)
	}

	return shouldFilter, reason, nil
}

// timeWindowFilter 时间窗口过滤
func (h *FilterHandler) timeWindowFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	windowStr, ok := config.Parameters["window"].(string)
	if !ok {
		return false, "", fmt.Errorf("时间窗口过滤需要配置window参数")
	}

	window, err := time.ParseDuration(windowStr)
	if err != nil {
		return false, "", fmt.Errorf("无效的时间窗口: %w", err)
	}

	// 检查数据时间戳是否在窗口内
	now := time.Now()
	age := now.Sub(point.Timestamp)

	if age > window {
		return true, fmt.Sprintf("数据时间戳过旧，年龄 %v 超过窗口 %v", age, window), nil
	}

	return false, "", nil
}

// valuesEqual 比较两个值是否相等
func (h *FilterHandler) valuesEqual(a, b interface{}) bool {
	// 处理nil值
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// 尝试数字比较
	if numA, err := h.toFloat64(a); err == nil {
		if numB, err := h.toFloat64(b); err == nil {
			return numA == numB
		}
	}

	// 字符串比较
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

// simplePatternMatch 简单模式匹配
func (h *FilterHandler) simplePatternMatch(value, pattern string) bool {
	// 支持通配符 * 和 ?
	if pattern == "*" {
		return true
	}

	// 简单的前缀和后缀匹配
	if len(pattern) > 0 && pattern[0] == '*' {
		suffix := pattern[1:]
		return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
	}

	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}

	// 精确匹配
	return value == pattern
}

// toFloat64 转换为float64
func (h *FilterHandler) toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return 0, fmt.Errorf("字符串无法转换为数字")
	default:
		return 0, fmt.Errorf("无法转换为数字: %T", value)
	}
}

// cleanupCache 清理缓存
func (h *FilterHandler) cleanupCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for key, entry := range h.duplicateCache {
			// 清理超过1小时的缓存
			if now.Sub(entry.Timestamp) > time.Hour {
				delete(h.duplicateCache, key)
			}
		}
		h.mu.Unlock()
	}
}
