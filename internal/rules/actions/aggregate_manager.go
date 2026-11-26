package actions

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// AggregateConfig 聚合配置
type AggregateConfig struct {
	WindowSize       int                    `json:"window_size"`
	WindowType       string                 `json:"window_type"`     // "count" or "time"
	WindowDuration   time.Duration          `json:"window_duration"` // 时间窗口大小
	Alignment        string                 `json:"alignment"`       // "none", "calendar" (Phase 2)
	Functions        []string               `json:"functions"`
	GroupBy          []string               `json:"group_by"`
	Output           map[string]interface{} `json:"output"`
	TTL              time.Duration          `json:"ttl"`
	UpperLimit       *float64               `json:"upper_limit,omitempty"`
	LowerLimit       *float64               `json:"lower_limit,omitempty"`
	OutlierThreshold *float64               `json:"outlier_threshold,omitempty"`
}

// AggregateState 聚合状态
type AggregateState struct {
	GroupKey   string
	WindowSize int
	Stats      *IncrementalStats
	LastAccess time.Time
	mu         sync.RWMutex

	ttl         time.Duration // 闲置过期时间（至少覆盖两个时间窗口）
	fingerprint string        // 创建时的窗口配置指纹，配置变化时状态需要重建
}

// AggregateManager 聚合状态管理器
type AggregateManager struct {
	states      map[string]*AggregateState
	mu          sync.RWMutex
	defaultTTL  time.Duration
	cleanupTick time.Duration
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	maxStates   int   // 最大状态数量限制
	maxMemory   int64 // 最大内存使用限制（字节）
	currentMem  int64 // 当前内存使用估算

	// 持久化（EnablePersistence 开启）：定期保存变化的状态，重启后恢复
	persister AggregateStatePersister
	lastSave  time.Time
	removed   []string // 已清理、待从持久化存储删除的状态键
}

// NewAggregateManager 创建聚合管理器
func NewAggregateManager() *AggregateManager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &AggregateManager{
		states:      make(map[string]*AggregateState),
		defaultTTL:  10 * time.Minute,
		cleanupTick: 1 * time.Minute,
		ctx:         ctx,
		cancel:      cancel,
		maxStates:   10000,             // 最大状态数量限制
		maxMemory:   100 * 1024 * 1024, // 100MB内存限制
		currentMem:  0,
	}

	// 启动清理协程
	manager.wg.Add(1)
	go manager.cleanupLoop()

	return manager
}

// GetOrCreateState 获取或创建聚合状态
func (m *AggregateManager) GetOrCreateState(stateKey string, windowSize int, config ...*AggregateConfig) *AggregateState {
	var cfg *AggregateConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	fingerprint := configFingerprint(windowSize, cfg)

	m.mu.RLock()
	if state, exists := m.states[stateKey]; exists && state.fingerprint == fingerprint {
		state.mu.Lock()
		state.LastAccess = time.Now()
		state.mu.Unlock()
		m.mu.RUnlock()
		return state
	}
	m.mu.RUnlock()

	// 需要创建新状态
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if state, exists := m.states[stateKey]; exists {
		if state.fingerprint == fingerprint {
			state.mu.Lock()
			state.LastAccess = time.Now()
			state.mu.Unlock()
			return state
		}
		// 规则的窗口配置变了（热加载修改了规则）：旧状态按旧窗口累积，继续使用会混合两种配置的数据
		log.Info().Str("state_key", stateKey).Msg("聚合窗口配置已变化，重建聚合状态")
		delete(m.states, stateKey)
		m.currentMem -= m.estimateStateSize(state.WindowSize)
	}

	// 检查资源限制
	if len(m.states) >= m.maxStates {
		// 强制清理过期状态
		m.forceCleanup()

		// 如果清理后仍然超限，拒绝创建新状态
		if len(m.states) >= m.maxStates {
			log.Warn().Int("current_states", len(m.states)).Int("max_states", m.maxStates).Msg("状态数量超限，拒绝创建新状态")
			// 返回一个临时状态，不保存到管理器中
			return &AggregateState{
				GroupKey:   stateKey,
				WindowSize: windowSize,
				Stats:      NewIncrementalStats(windowSize),
				LastAccess: time.Now(),
			}
		}
	}

	// 估算新状态的内存使用
	estimatedSize := m.estimateStateSize(windowSize)
	if m.currentMem+estimatedSize > m.maxMemory {
		// 强制清理以释放内存
		m.forceCleanup()

		// 如果清理后仍然超限，拒绝创建新状态
		if m.currentMem+estimatedSize > m.maxMemory {
			log.Warn().Int64("current_memory", m.currentMem).Int64("max_memory", m.maxMemory).Msg("内存使用超限，拒绝创建新状态")
			// 返回一个临时状态
			return &AggregateState{
				GroupKey:   stateKey,
				WindowSize: windowSize,
				Stats:      NewIncrementalStats(windowSize),
				LastAccess: time.Now(),
			}
		}
	}

	// 创建新状态
	var stats *IncrementalStats

	// 如果提供了配置，使用带配置的构造函数
	if len(config) > 0 && config[0] != nil {
		aggConfig := config[0]

		// 使用新的窗口配置构造函数
		stats = NewIncrementalStatsWithWindow(
			windowSize,
			aggConfig.WindowType,
			aggConfig.WindowDuration,
			aggConfig.Alignment,
		)

		// 应用阈值配置 (直接从AggregateConfig使用)
		if aggConfig.UpperLimit != nil {
			stats.upperLimit = aggConfig.UpperLimit
		}
		if aggConfig.LowerLimit != nil {
			stats.lowerLimit = aggConfig.LowerLimit
		}
		if aggConfig.OutlierThreshold != nil {
			stats.outlierThreshold = *aggConfig.OutlierThreshold
		}

		// 兼容从output字段读取阈值配置（旧格式）
		if output := aggConfig.Output; output != nil {
			if upperLimit, exists := output["upper_limit"]; exists {
				if val, ok := upperLimit.(float64); ok && stats.upperLimit == nil {
					stats.upperLimit = &val
				}
			}
			if lowerLimit, exists := output["lower_limit"]; exists {
				if val, ok := lowerLimit.(float64); ok && stats.lowerLimit == nil {
					stats.lowerLimit = &val
				}
			}
			if outlierThreshold, exists := output["outlier_threshold"]; exists {
				if val, ok := outlierThreshold.(float64); ok && val > 0 && aggConfig.OutlierThreshold == nil {
					stats.outlierThreshold = val
				}
			}
		}
	} else {
		stats = NewIncrementalStats(windowSize)
	}

	state := &AggregateState{
		GroupKey:    stateKey,
		WindowSize:  windowSize,
		Stats:       stats,
		LastAccess:  time.Now(),
		ttl:         stateTTL(m.defaultTTL, cfg),
		fingerprint: fingerprint,
	}

	m.states[stateKey] = state
	m.currentMem += estimatedSize
	log.Debug().Str("state_key", stateKey).Int("window_size", windowSize).Msg("创建新的聚合状态")

	return state
}

// ProcessPoint 处理数据点
func (m *AggregateManager) ProcessPoint(rule *rules.Rule, point model.Point, config *AggregateConfig) (*rules.ActionResult, error) {
	start := time.Now()

	// 生成状态键
	stateKey := m.generateStateKey(rule.ID, point, config.GroupBy)

	// 获取或创建聚合状态
	state := m.GetOrCreateState(stateKey, config.WindowSize, config)

	// 提取数值
	value, err := extractNumericValue(point.Value)
	if err != nil {
		return &rules.ActionResult{
			Type:     "aggregate",
			Success:  false,
			Error:    fmt.Sprintf("无法提取数值: %v", err),
			Duration: time.Since(start),
		}, err
	}

	// 添加值到统计计算器
	state.Stats.AddValue(value)

	// 检查是否需要输出结果
	shouldOutput := false
	if config.WindowType == "time" {
		// 时间窗口模式：有数据就输出聚合结果
		shouldOutput = state.Stats.GetCount() > 0
	} else if config.WindowSize > 0 {
		// 数量滑动窗口模式：当窗口满时输出
		shouldOutput = state.Stats.GetCount() >= int64(config.WindowSize)
	} else {
		// 累积模式：每次都输出
		shouldOutput = true
	}

	// 创建输出映射
	outputMap := map[string]interface{}{
		"state_key": stateKey,
		"count":     state.Stats.GetCount(),
	}

	if shouldOutput {
		// 计算聚合结果
		aggregateResult := m.calculateAggregateResult(state, config, point)
		outputMap["aggregated"] = true
		outputMap["aggregate_result"] = aggregateResult

		log.Debug().
			Str("state_key", stateKey).
			Int64("count", state.Stats.GetCount()).
			Interface("functions", aggregateResult.Functions).
			Msg("聚合计算完成")
	}

	result := &rules.ActionResult{
		Type:     "aggregate",
		Success:  true,
		Duration: time.Since(start),
		Output:   outputMap,
	}

	return result, nil
}

// calculateAggregateResult 计算聚合结果
func (m *AggregateManager) calculateAggregateResult(state *AggregateState, config *AggregateConfig, point model.Point) *rules.AggregateResult {
	stats := state.Stats.GetStats()
	functions := make(map[string]interface{})

	// 计算请求的函数
	for _, function := range config.Functions {
		if value, exists := stats[function]; exists {
			functions[function] = value
		} else {
			log.Warn().Str("function", function).Msg("不支持的聚合函数")
		}
	}

	// 如果没有指定函数，默认计算平均值
	if len(functions) == 0 {
		functions["avg"] = stats["avg"]
	}

	// 构建分组信息
	groupBy := make(map[string]string)
	for _, field := range config.GroupBy {
		switch field {
		case "device_id":
			groupBy[field] = point.DeviceID
		case "key":
			groupBy[field] = point.Key
		case "type":
			groupBy[field] = string(point.Type)
		default:
			// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
			if tagValue, exists := point.GetTag(field); exists {
				groupBy[field] = tagValue
			}
		}
	}

	return &rules.AggregateResult{
		DeviceID:  point.DeviceID,
		Key:       point.Key,
		Window:    fmt.Sprintf("window_size:%d", state.WindowSize),
		GroupBy:   groupBy,
		Functions: functions,
		StartTime: state.Stats.GetLastUpdateTime(),
		EndTime:   time.Now(),
		Count:     state.Stats.GetCount(),
		Timestamp: time.Now(),
	}
}

// generateStateKey 生成状态键
func (m *AggregateManager) generateStateKey(ruleID string, point model.Point, groupBy []string) string {
	if len(groupBy) == 0 {
		return fmt.Sprintf("%s:default", ruleID)
	}

	var keyParts []string
	keyParts = append(keyParts, ruleID)

	for _, field := range groupBy {
		switch field {
		case "device_id":
			keyParts = append(keyParts, point.DeviceID)
		case "key":
			keyParts = append(keyParts, point.Key)
		case "type":
			keyParts = append(keyParts, string(point.Type))
		default:
			// 尝试从标签中获取
			// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
			if tagValue, exists := point.GetTag(field); exists {
				keyParts = append(keyParts, tagValue)
			} else {
				keyParts = append(keyParts, "unknown")
			}
		}
	}

	result := ""
	for i, part := range keyParts {
		if i > 0 {
			result += ":"
		}
		result += part
	}
	return result
}

// cleanupLoop 清理过期状态
func (m *AggregateManager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupExpiredStates()
		}
	}
}

// cleanupExpiredStates 清理过期状态
func (m *AggregateManager) cleanupExpiredStates() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var expiredKeys []string
	var freedMemory int64

	for key, state := range m.states {
		state.mu.RLock()
		age := now.Sub(state.LastAccess)
		windowSize := state.WindowSize
		ttl := state.ttlOr(m.defaultTTL)
		state.mu.RUnlock()

		if age > ttl {
			expiredKeys = append(expiredKeys, key)
			freedMemory += m.estimateStateSize(windowSize)
		}
	}

	// 删除过期状态
	for _, key := range expiredKeys {
		delete(m.states, key)
		log.Debug().Str("state_key", key).Msg("清理过期聚合状态")
	}
	if m.persister != nil {
		m.removed = append(m.removed, expiredKeys...)
	}

	// 更新内存使用估算
	m.currentMem -= freedMemory
	if m.currentMem < 0 {
		m.currentMem = 0
	}

	if len(expiredKeys) > 0 {
		log.Info().Int("cleaned", len(expiredKeys)).Int("remaining", len(m.states)).Int64("freed_memory", freedMemory).Msg("聚合状态清理完成")
	}
}

// forceCleanup 强制清理过期状态（在写锁内调用）
func (m *AggregateManager) forceCleanup() {
	now := time.Now()
	var expiredKeys []string
	var freedMemory int64

	for key, state := range m.states {
		state.mu.RLock()
		age := now.Sub(state.LastAccess)
		windowSize := state.WindowSize
		state.mu.RUnlock()

		// 使用更激进的清理策略
		if age > state.ttlOr(m.defaultTTL)/2 {
			expiredKeys = append(expiredKeys, key)
			freedMemory += m.estimateStateSize(windowSize)
		}
	}

	// 删除过期状态
	for _, key := range expiredKeys {
		delete(m.states, key)
	}
	if m.persister != nil {
		m.removed = append(m.removed, expiredKeys...)
	}

	// 更新内存使用估算
	m.currentMem -= freedMemory
	if m.currentMem < 0 {
		m.currentMem = 0
	}

	log.Info().Int("force_cleaned", len(expiredKeys)).Int("remaining", len(m.states)).Int64("freed_memory", freedMemory).Msg("强制清理聚合状态")
}

// estimateStateSize 估算状态的内存使用
func (m *AggregateManager) estimateStateSize(windowSize int) int64 {
	baseSize := int64(200) // 基本结构体大小
	if windowSize > 0 {
		// 滑动窗口模式：windowSize * 8字节（float64）
		return baseSize + int64(windowSize)*8
	}
	// 累积模式：固定大小
	return baseSize
}

// GetStats 获取管理器统计信息
func (m *AggregateManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memoryUsagePercent := float64(m.currentMem) / float64(m.maxMemory) * 100
	stateUsagePercent := float64(len(m.states)) / float64(m.maxStates) * 100

	return map[string]interface{}{
		"total_states":         len(m.states),
		"max_states":           m.maxStates,
		"state_usage_percent":  stateUsagePercent,
		"current_memory":       m.currentMem,
		"max_memory":           m.maxMemory,
		"memory_usage_percent": memoryUsagePercent,
		"default_ttl":          m.defaultTTL.String(),
		"cleanup_tick":         m.cleanupTick.String(),
	}
}

// Close 关闭管理器
func (m *AggregateManager) Close() {
	m.closeWithFinalSave()
}

// ttlOr 返回状态的闲置过期时间（没有设置时使用默认值）
func (s *AggregateState) ttlOr(def time.Duration) time.Duration {
	if s.ttl > 0 {
		return s.ttl
	}
	return def
}

// extractNumericValue 提取数值，支持复杂数据类型
func extractNumericValue(value interface{}) (float64, error) {
	if value == nil {
		return 0, fmt.Errorf("数值不能为nil")
	}

	var result float64

	switch v := value.(type) {
	case float64:
		result = v
	case float32:
		result = float64(v)
	case int:
		result = float64(v)
	case int32:
		result = float64(v)
	case int64:
		result = float64(v)
	case uint:
		result = float64(v)
	case uint32:
		result = float64(v)
	case uint64:
		result = float64(v)
	case string:
		// 尝试字符串到数字的转换
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			result = parsed
		} else {
			return 0, fmt.Errorf("无法将字符串 '%s' 转换为数值: %v", v, err)
		}
	case map[string]interface{}:
		// 处理复杂数据类型
		if val, ok := extractFromComplexType(v); ok {
			result = val
		} else {
			return 0, fmt.Errorf("复杂数据类型中没有有效的数值")
		}
	case model.CompositeData:
		// 类型化的复合数据（适配器在进程内直接产生）
		val, ok := extractFromCompositeData(v)
		if !ok {
			return 0, fmt.Errorf("复杂数据类型中没有有效的数值")
		}
		result = val
	default:
		return 0, fmt.Errorf("不支持的数值类型: %T", value)
	}

	// 边界检查
	if math.IsNaN(result) {
		return 0, fmt.Errorf("数值为NaN")
	}

	if math.IsInf(result, 0) {
		return 0, fmt.Errorf("数值为无穷大")
	}

	return result, nil
}

// extractFromComplexType 从复杂数据类型中提取数值用于聚合
func extractFromComplexType(data map[string]interface{}) (float64, bool) {
	// 1. 数组数据类型 - 取第一个数值元素（元素可能是 []interface{} 或 []float64）
	if elements, ok := data["elements"]; ok {
		if elemArray := sliceElements(elements); len(elemArray) > 0 {
			if val, ok := convertToFloat64(elemArray[0]); ok {
				return val, true
			}
		}
	}

	// 2. 3D向量数据类型 - 使用幅值 (magnitude)，没有时按 x/y/z 计算
	if magnitude, ok := data["magnitude"]; ok {
		if val, ok := convertToFloat64(magnitude); ok {
			return val, true
		}
	}
	if x, okX := convertToFloat64(data["x"]); okX {
		y, okY := convertToFloat64(data["y"])
		z, okZ := convertToFloat64(data["z"])
		if okY && okZ {
			return math.Sqrt(x*x + y*y + z*z), true
		}
	}

	// 3. GPS位置数据类型 - 使用速度 (speed)
	if speed, ok := data["speed"]; ok {
		if val, ok := convertToFloat64(speed); ok {
			return val, true
		}
	}

	// 4. 颜色数据类型 - 使用亮度 (brightness / lightness)
	for _, field := range []string{"brightness", "lightness"} {
		if val, ok := convertToFloat64(data[field]); ok {
			return val, true
		}
	}

	// 5. 通用向量数据类型 - 使用平均值（values 可能是 []interface{} 或 []float64）
	if values, ok := data["values"]; ok {
		var sum float64
		var count int
		for _, v := range sliceElements(values) {
			if val, ok := convertToFloat64(v); ok {
				sum += val
				count++
			}
		}
		if count > 0 {
			return sum / float64(count), true
		}
	}

	// 6. 简单数值字段 - 检查常见的数值字段
	commonFields := []string{"value", "temperature", "humidity", "pressure", "vibration"}
	for _, field := range commonFields {
		if fieldValue, ok := data[field]; ok {
			if val, ok := convertToFloat64(fieldValue); ok {
				return val, true
			}
		}
	}

	return 0, false
}

// convertToFloat64 转换为float64的辅助函数
func convertToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

// extractFromCompositeData 从类型化的复合数据中提取用于聚合的数值，规则与 map 形式一致：
// 位置取速度、三维向量取模长、颜色取亮度、向量取平均值、数组取第一个元素
func extractFromCompositeData(data model.CompositeData) (float64, bool) {
	switch d := data.(type) {
	case *model.LocationData:
		return d.Speed, true
	case *model.Vector3D:
		return math.Sqrt(d.X*d.X + d.Y*d.Y + d.Z*d.Z), true
	case *model.ColorData:
		return (float64(d.R) + float64(d.G) + float64(d.B)) / 3 / 255 * 100, true
	case *model.VectorData:
		if len(d.Values) == 0 {
			return 0, false
		}
		var sum float64
		for _, v := range d.Values {
			sum += v
		}
		return sum / float64(len(d.Values)), true
	case *model.ArrayData:
		if len(d.Values) > 0 {
			return convertToFloat64(d.Values[0])
		}
	}
	return 0, false
}

// sliceElements 把任意切片（[]interface{}、[]float64 等）转换为 []interface{}
func sliceElements(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]interface{}, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out
}
