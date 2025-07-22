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

// AggregateConfig 聚合配置
type AggregateConfig struct {
	WindowSize  int                    `json:"window_size"`
	Functions   []string               `json:"functions"`
	GroupBy     []string               `json:"group_by"`
	Output      map[string]interface{} `json:"output"`
	TTL         time.Duration          `json:"ttl"`
}

// AggregateState 聚合状态
type AggregateState struct {
	GroupKey    string
	WindowSize  int
	Stats       *IncrementalStats
	LastAccess  time.Time
	mu          sync.RWMutex
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
	}
	
	// 启动清理协程
	manager.wg.Add(1)
	go manager.cleanupLoop()
	
	return manager
}

// GetOrCreateState 获取或创建聚合状态
func (m *AggregateManager) GetOrCreateState(stateKey string, windowSize int) *AggregateState {
	m.mu.RLock()
	if state, exists := m.states[stateKey]; exists {
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
		state.mu.Lock()
		state.LastAccess = time.Now()
		state.mu.Unlock()
		return state
	}
	
	// 创建新状态
	state := &AggregateState{
		GroupKey:   stateKey,
		WindowSize: windowSize,
		Stats:      NewIncrementalStats(windowSize),
		LastAccess: time.Now(),
	}
	
	m.states[stateKey] = state
	log.Debug().Str("state_key", stateKey).Int("window_size", windowSize).Msg("创建新的聚合状态")
	
	return state
}

// ProcessPoint 处理数据点
func (m *AggregateManager) ProcessPoint(rule *rules.Rule, point model.Point, config *AggregateConfig) (*rules.ActionResult, error) {
	start := time.Now()
	
	// 生成状态键
	stateKey := m.generateStateKey(rule.ID, point, config.GroupBy)
	
	// 获取或创建聚合状态
	state := m.GetOrCreateState(stateKey, config.WindowSize)
	
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
	if config.WindowSize > 0 {
		// 滑动窗口模式：当窗口满时输出
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
			if point.Tags != nil {
				if tagValue, exists := point.Tags[field]; exists {
					groupBy[field] = tagValue
				}
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
			if point.Tags != nil {
				if tagValue, exists := point.Tags[field]; exists {
					keyParts = append(keyParts, tagValue)
				} else {
					keyParts = append(keyParts, "unknown")
				}
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
	
	for key, state := range m.states {
		state.mu.RLock()
		age := now.Sub(state.LastAccess)
		state.mu.RUnlock()
		
		if age > m.defaultTTL {
			expiredKeys = append(expiredKeys, key)
		}
	}
	
	// 删除过期状态
	for _, key := range expiredKeys {
		delete(m.states, key)
		log.Debug().Str("state_key", key).Msg("清理过期聚合状态")
	}
	
	if len(expiredKeys) > 0 {
		log.Info().Int("cleaned", len(expiredKeys)).Int("remaining", len(m.states)).Msg("聚合状态清理完成")
	}
}

// GetStats 获取管理器统计信息
func (m *AggregateManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"total_states": len(m.states),
		"default_ttl":  m.defaultTTL.String(),
		"cleanup_tick": m.cleanupTick.String(),
	}
}

// Close 关闭管理器
func (m *AggregateManager) Close() {
	m.cancel()
	m.wg.Wait()
}

// extractNumericValue 提取数值
func extractNumericValue(value interface{}) (float64, error) {
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
	case uint:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("不支持的数值类型: %T", value)
	}
}