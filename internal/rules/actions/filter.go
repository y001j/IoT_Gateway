package actions

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// FilterHandler Filter动作处理器
type FilterHandler struct {
	duplicateCache  map[string]DuplicateEntry
	statisticsCache map[string]*StatisticsWindow
	changeRateCache map[string]*ChangeRateEntry
	consecutiveCache map[string]*ConsecutiveEntry
	mu              sync.RWMutex
}

// DuplicateEntry 重复数据缓存条目
type DuplicateEntry struct {
	Value     interface{}
	Timestamp time.Time
}

// StatisticsWindow 统计窗口数据
type StatisticsWindow struct {
	Values    []float64
	WindowSize int
	Mean      float64
	StdDev    float64
	LastUpdate time.Time
}

// ChangeRateEntry 变化率缓存条目
type ChangeRateEntry struct {
	LastValue     interface{}
	LastTimestamp time.Time
}

// ConsecutiveEntry 连续异常缓存条目
type ConsecutiveEntry struct {
	ConsecutiveCount int
	LastCheckTime    time.Time
	IsAbnormal       bool
}

// NewFilterHandler 创建Filter处理器
func NewFilterHandler() *FilterHandler {
	handler := &FilterHandler{
		duplicateCache:   make(map[string]DuplicateEntry),
		statisticsCache:  make(map[string]*StatisticsWindow),
		changeRateCache:  make(map[string]*ChangeRateEntry),
		consecutiveCache: make(map[string]*ConsecutiveEntry),
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
	Type       string                 `json:"type"`       // duplicate, range, rate_limit, pattern, null, threshold, time_window, quality, change_rate, statistical_anomaly, consecutive
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
	case "quality":
		return h.qualityFilter(point, config)
	case "change_rate":
		return h.changeRateFilter(point, config)
	case "statistical_anomaly":
		return h.statisticalAnomalyFilter(point, config)
	case "consecutive":
		return h.consecutiveFilter(point, config)
	default:
		return false, "", fmt.Errorf("不支持的过滤类型: %s", config.Type)
	}
}

// duplicateFilter 重复数据过滤
func (h *FilterHandler) duplicateFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 获取时间窗口参数
	var timeWindow time.Duration = config.TTL // 默认使用配置的TTL
	
	// 如果参数中有window设置，优先使用
	if windowStr, ok := config.Parameters["window"].(string); ok {
		if duration, err := time.ParseDuration(windowStr); err == nil {
			timeWindow = duration
		}
	}
	
	// 如果都没有设置，使用默认值
	if timeWindow == 0 {
		timeWindow = 60 * time.Second // 默认60秒
	}

	// 生成基于值的缓存键，这样每个不同值都有独立的缓存条目
	valueStr := fmt.Sprintf("%v", point.Value)
	cacheKey := fmt.Sprintf("dup:%s:%s:%s", point.DeviceID, point.Key, valueStr)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 检查是否存在重复数据
	if entry, exists := h.duplicateCache[cacheKey]; exists {
		// 检查时间间隔（基于数据点时间戳，不是当前系统时间）
		timeDiff := point.Timestamp.Sub(entry.Timestamp)
		if timeDiff < timeWindow && timeDiff >= 0 {
			return true, "重复数据", nil
		}
	}

	// 更新缓存（使用数据点的时间戳）
	h.duplicateCache[cacheKey] = DuplicateEntry{
		Value:     point.Value,
		Timestamp: point.Timestamp,
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
		// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
		if tagValue, exists := point.GetTag(field); exists {
			valueToMatch = tagValue
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

// qualityFilter 质量过滤 - 过滤设备质量码异常的数据
func (h *FilterHandler) qualityFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 获取允许的质量码列表，默认只允许0（正常）
	allowedQuality, ok := config.Parameters["allowed_quality"].([]interface{})
	if !ok {
		// 默认只允许质量码为0的数据通过
		if point.Quality != 0 {
			return true, fmt.Sprintf("数据质量异常: %d", point.Quality), nil
		}
		return false, "", nil
	}

	// 检查质量码是否在允许列表中
	for _, allowed := range allowedQuality {
		// 支持多种类型的转换
		var allowedInt int
		switch v := allowed.(type) {
		case int:
			allowedInt = v
		case float64:
			allowedInt = int(v)
		case int32:
			allowedInt = int(v)
		case int64:
			allowedInt = int(v)
		default:
			continue // 跳过无法转换的类型
		}
		
		if allowedInt == point.Quality {
			return false, "", nil
		}
	}

	return true, fmt.Sprintf("数据质量异常: %d", point.Quality), nil
}

// changeRateFilter 变化率过滤 - 过滤变化过快的数据
func (h *FilterHandler) changeRateFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 获取最大变化率参数
	maxChangeRate, ok := config.Parameters["max_change_rate"].(float64)
	if !ok {
		return false, "", fmt.Errorf("变化率过滤需要配置max_change_rate参数")
	}

	// 获取时间窗口参数
	timeWindowStr, ok := config.Parameters["time_window"].(string)
	if !ok {
		timeWindowStr = "10s" // 默认10秒窗口
	}

	timeWindow, err := time.ParseDuration(timeWindowStr)
	if err != nil {
		return false, "", fmt.Errorf("无效的时间窗口: %w", err)
	}

	// 生成缓存键
	cacheKey := fmt.Sprintf("chg:%s:%s", point.DeviceID, point.Key)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 转换当前值为数字
	currentValue, err := h.toFloat64(point.Value)
	if err != nil {
		// 非数字值不进行变化率检查
		return false, "", nil
	}

	// 检查是否有历史数据
	if entry, exists := h.changeRateCache[cacheKey]; exists {
		timeDiff := point.Timestamp.Sub(entry.LastTimestamp)
		
		// 只在时间窗口内检查变化率，且确保时间差大于0
		if timeDiff <= timeWindow && timeDiff > 0 {
			if lastValue, err := h.toFloat64(entry.LastValue); err == nil {
				// 计算变化率：变化量/时间差
				valueDiff := math.Abs(currentValue - lastValue)
				changeRate := valueDiff / timeDiff.Seconds()
				
				if changeRate > maxChangeRate {
					// 过滤掉变化率过快的数据，不更新缓存，保持原有基准值
					return true, fmt.Sprintf("变化率过快: %.2f/s 超过限制 %.2f/s", changeRate, maxChangeRate), nil
				}
			}
		}
	}

	// 更新缓存为当前正常值
	h.changeRateCache[cacheKey] = &ChangeRateEntry{
		LastValue:     point.Value,
		LastTimestamp: point.Timestamp,
	}

	return false, "", nil
}

// statisticalAnomalyFilter 统计异常检测过滤 - 基于移动统计检测异常
func (h *FilterHandler) statisticalAnomalyFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 获取配置参数
	windowSize, ok := config.Parameters["window_size"].(float64)
	if !ok {
		windowSize = 20 // 默认窗口大小
	}

	stdThreshold, ok := config.Parameters["std_threshold"].(float64)
	if !ok {
		stdThreshold = 2.0 // 默认2个标准差
	}

	minSamples, ok := config.Parameters["min_samples"].(float64)
	if !ok {
		minSamples = 5 // 最少需要5个样本才开始检测
	}

	// 转换数值
	currentValue, err := h.toFloat64(point.Value)
	if err != nil {
		// 非数字值不进行统计分析
		return false, "", nil
	}

	// 生成缓存键
	cacheKey := fmt.Sprintf("stat:%s:%s", point.DeviceID, point.Key)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 获取或创建统计窗口
	window, exists := h.statisticsCache[cacheKey]
	if !exists {
		window = &StatisticsWindow{
			Values:     make([]float64, 0, int(windowSize)),
			WindowSize: int(windowSize),
			LastUpdate: time.Now(),
		}
		h.statisticsCache[cacheKey] = window
	}

	// 检查是否有足够的样本进行异常检测（基于现有数据，不包含当前值）
	if len(window.Values) >= int(minSamples) {
		// 先用现有数据计算统计值
		h.updateStatistics(window)

		// 检查当前值是否为异常
		deviation := math.Abs(currentValue - window.Mean)
		if window.StdDev > 0 && deviation > stdThreshold*window.StdDev {
			// 异常值，不添加到窗口中，但记录检测
			return true, fmt.Sprintf("统计异常: 偏离均值%.2f个标准差 (阈值:%.1f)", deviation/window.StdDev, stdThreshold), nil
		}
	}

	// 添加新值到窗口（只有正常值才添加）
	window.Values = append(window.Values, currentValue)
	if len(window.Values) > window.WindowSize {
		// 移除最旧的值
		window.Values = window.Values[1:]
	}
	window.LastUpdate = time.Now()

	return false, "", nil
}

// consecutiveFilter 连续异常过滤 - 只有连续N次异常才过滤
func (h *FilterHandler) consecutiveFilter(point model.Point, config *FilterConfig) (bool, string, error) {
	// 获取连续次数阈值
	consecutiveThreshold, ok := config.Parameters["consecutive_count"].(float64)
	if !ok {
		consecutiveThreshold = 3 // 默认连续3次
	}

	// 获取异常检测条件
	checkConfig := make(map[string]interface{})
	if innerConfig, ok := config.Parameters["inner_filter"].(map[string]interface{}); ok {
		checkConfig = innerConfig
	} else {
		return false, "", fmt.Errorf("连续异常过滤需要配置inner_filter参数")
	}

	// 创建内部过滤器配置
	innerFilterConfig := &FilterConfig{
		Type:       checkConfig["type"].(string),
		Parameters: checkConfig["parameters"].(map[string]interface{}),
		Action:     "drop",
		TTL:        config.TTL,
	}

	// 检查内部条件
	isAbnormal, reason, err := h.shouldFilter(point, innerFilterConfig)
	if err != nil {
		return false, "", err
	}

	// 生成缓存键
	cacheKey := fmt.Sprintf("cons:%s:%s", point.DeviceID, point.Key)

	h.mu.Lock()
	defer h.mu.Unlock()

	// 获取或创建连续计数器
	entry, exists := h.consecutiveCache[cacheKey]
	if !exists {
		entry = &ConsecutiveEntry{
			ConsecutiveCount: 0,
			LastCheckTime:    time.Now(),
			IsAbnormal:       false,
		}
		h.consecutiveCache[cacheKey] = entry
	}

	now := time.Now()
	
	if isAbnormal {
		entry.ConsecutiveCount++
		entry.IsAbnormal = true
	} else {
		entry.ConsecutiveCount = 0
		entry.IsAbnormal = false
	}
	entry.LastCheckTime = now

	// 检查是否达到连续异常阈值
	if entry.ConsecutiveCount >= int(consecutiveThreshold) {
		return true, fmt.Sprintf("连续%d次异常: %s", entry.ConsecutiveCount, reason), nil
	}

	return false, "", nil
}

// updateStatistics 更新统计窗口的统计值
func (h *FilterHandler) updateStatistics(window *StatisticsWindow) {
	n := len(window.Values)
	if n == 0 {
		return
	}

	// 计算均值
	sum := 0.0
	for _, v := range window.Values {
		sum += v
	}
	window.Mean = sum / float64(n)

	// 计算标准差
	if n > 1 {
		sumSquaredDiff := 0.0
		for _, v := range window.Values {
			diff := v - window.Mean
			sumSquaredDiff += diff * diff
		}
		window.StdDev = math.Sqrt(sumSquaredDiff / float64(n-1))
	} else {
		window.StdDev = 0
	}
}

// cleanupCache 清理缓存
func (h *FilterHandler) cleanupCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		
		// 清理重复数据缓存
		for key, entry := range h.duplicateCache {
			if now.Sub(entry.Timestamp) > time.Hour {
				delete(h.duplicateCache, key)
			}
		}
		
		// 清理统计缓存
		for key, window := range h.statisticsCache {
			if now.Sub(window.LastUpdate) > 2*time.Hour {
				delete(h.statisticsCache, key)
			}
		}
		
		// 清理变化率缓存
		for key, entry := range h.changeRateCache {
			if now.Sub(entry.LastTimestamp) > time.Hour {
				delete(h.changeRateCache, key)
			}
		}
		
		// 清理连续异常缓存
		for key, entry := range h.consecutiveCache {
			if now.Sub(entry.LastCheckTime) > time.Hour {
				delete(h.consecutiveCache, key)
			}
		}
		
		h.mu.Unlock()
	}
}
