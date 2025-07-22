package rules

import (
	"sort"
	"sync"

	"github.com/y001j/iot-gateway/internal/model"
)

// Index 规则索引
type Index struct {
	deviceIndex   map[string][]*Rule // 按设备ID索引
	keyIndex      map[string][]*Rule // 按数据key索引
	priorityIndex []*Rule            // 按优先级排序
	typeIndex     map[string][]*Rule // 按数据类型索引
	allRules      []*Rule            // 所有启用的规则
	mu            sync.RWMutex
}

// NewIndex 创建新的索引
func NewIndex() *Index {
	return &Index{
		deviceIndex: make(map[string][]*Rule),
		keyIndex:    make(map[string][]*Rule),
		typeIndex:   make(map[string][]*Rule),
		allRules:    make([]*Rule, 0),
	}
}

// AddRule 添加规则到索引
func (idx *Index) AddRule(rule *Rule) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 移除旧的规则（如果存在）
	idx.removeRuleFromIndex(rule.ID)

	// 只索引启用的规则
	if !rule.Enabled {
		return
	}

	// 添加到所有规则列表
	idx.allRules = append(idx.allRules, rule)

	// 分析规则条件，建立索引
	idx.analyzeAndIndex(rule)

	// 重新排序优先级索引
	idx.rebuildPriorityIndex()
}

// RemoveRule 从索引中移除规则
func (idx *Index) RemoveRule(rule *Rule) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.removeRuleFromIndex(rule.ID)
	idx.rebuildPriorityIndex()
}

// removeRuleFromIndex 从索引中移除规则（内部方法，不加锁）
func (idx *Index) removeRuleFromIndex(ruleID string) {
	// 从所有规则列表中移除
	for i, rule := range idx.allRules {
		if rule.ID == ruleID {
			idx.allRules = append(idx.allRules[:i], idx.allRules[i+1:]...)
			break
		}
	}

	// 从设备索引中移除
	for deviceID, rules := range idx.deviceIndex {
		for i, rule := range rules {
			if rule.ID == ruleID {
				idx.deviceIndex[deviceID] = append(rules[:i], rules[i+1:]...)
				if len(idx.deviceIndex[deviceID]) == 0 {
					delete(idx.deviceIndex, deviceID)
				}
				break
			}
		}
	}

	// 从key索引中移除
	for key, rules := range idx.keyIndex {
		for i, rule := range rules {
			if rule.ID == ruleID {
				idx.keyIndex[key] = append(rules[:i], rules[i+1:]...)
				if len(idx.keyIndex[key]) == 0 {
					delete(idx.keyIndex, key)
				}
				break
			}
		}
	}

	// 从类型索引中移除
	for dataType, rules := range idx.typeIndex {
		for i, rule := range rules {
			if rule.ID == ruleID {
				idx.typeIndex[dataType] = append(rules[:i], rules[i+1:]...)
				if len(idx.typeIndex[dataType]) == 0 {
					delete(idx.typeIndex, dataType)
				}
				break
			}
		}
	}
}

// analyzeAndIndex 分析规则条件并建立索引
func (idx *Index) analyzeAndIndex(rule *Rule) {
	// 分析条件中的字段
	fields := idx.extractFields(rule.Conditions)

	// 建立设备ID索引
	if deviceIDs, exists := fields["device_id"]; exists {
		for _, deviceID := range deviceIDs {
			if deviceID == "*" || deviceID == "" {
				continue // 通配符或空值不建立索引
			}
			idx.deviceIndex[deviceID] = append(idx.deviceIndex[deviceID], rule)
		}
	} else {
		// 没有指定设备ID的规则，添加到通配符索引
		idx.deviceIndex["*"] = append(idx.deviceIndex["*"], rule)
	}

	// 建立key索引
	if keys, exists := fields["key"]; exists {
		for _, key := range keys {
			if key == "*" || key == "" {
				continue // 通配符或空值不建立索引
			}
			idx.keyIndex[key] = append(idx.keyIndex[key], rule)
		}
	} else {
		// 没有指定key的规则，添加到通配符索引
		idx.keyIndex["*"] = append(idx.keyIndex["*"], rule)
	}

	// 建立类型索引
	if types, exists := fields["type"]; exists {
		for _, dataType := range types {
			if dataType == "*" || dataType == "" {
				continue // 通配符或空值不建立索引
			}
			idx.typeIndex[dataType] = append(idx.typeIndex[dataType], rule)
		}
	} else {
		// 没有指定类型的规则，添加到通配符索引
		idx.typeIndex["*"] = append(idx.typeIndex["*"], rule)
	}
}

// extractFields 提取条件中的字段值
func (idx *Index) extractFields(condition *Condition) map[string][]string {
	fields := make(map[string][]string)
	idx.extractFieldsRecursive(condition, fields)
	return fields
}

// extractFieldsRecursive 递归提取字段值
func (idx *Index) extractFieldsRecursive(condition *Condition, fields map[string][]string) {
	if condition == nil {
		return
	}

	// 处理简单条件
	if condition.Field != "" && condition.Operator == "eq" {
		if value, ok := condition.Value.(string); ok {
			fields[condition.Field] = append(fields[condition.Field], value)
		}
	}

	// 递归处理嵌套条件
	for _, subCondition := range condition.And {
		idx.extractFieldsRecursive(subCondition, fields)
	}
	for _, subCondition := range condition.Or {
		idx.extractFieldsRecursive(subCondition, fields)
	}
	if condition.Not != nil {
		idx.extractFieldsRecursive(condition.Not, fields)
	}
}

// rebuildPriorityIndex 重建优先级索引
func (idx *Index) rebuildPriorityIndex() {
	idx.priorityIndex = make([]*Rule, len(idx.allRules))
	copy(idx.priorityIndex, idx.allRules)

	// 按优先级排序（高优先级在前）
	sort.Slice(idx.priorityIndex, func(i, j int) bool {
		return idx.priorityIndex[i].Priority > idx.priorityIndex[j].Priority
	})
}

// Match 匹配数据点的规则
func (idx *Index) Match(point model.Point) []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	candidateRules := make(map[string]*Rule)

	// 1. 按设备ID匹配
	if rules, exists := idx.deviceIndex[point.DeviceID]; exists {
		for _, rule := range rules {
			candidateRules[rule.ID] = rule
		}
	}

	// 2. 匹配通配符设备规则
	if rules, exists := idx.deviceIndex["*"]; exists {
		for _, rule := range rules {
			candidateRules[rule.ID] = rule
		}
	}

	// 3. 按key匹配
	if rules, exists := idx.keyIndex[point.Key]; exists {
		for _, rule := range rules {
			candidateRules[rule.ID] = rule
		}
	}

	// 4. 匹配通配符key规则
	if rules, exists := idx.keyIndex["*"]; exists {
		for _, rule := range rules {
			candidateRules[rule.ID] = rule
		}
	}

	// 5. 按类型匹配
	dataType := string(point.Type)
	if rules, exists := idx.typeIndex[dataType]; exists {
		for _, rule := range rules {
			candidateRules[rule.ID] = rule
		}
	}

	// 6. 匹配通配符类型规则
	if rules, exists := idx.typeIndex["*"]; exists {
		for _, rule := range rules {
			candidateRules[rule.ID] = rule
		}
	}

	// 如果没有找到候选规则，返回所有规则（兜底策略）
	if len(candidateRules) == 0 {
		result := make([]*Rule, len(idx.priorityIndex))
		copy(result, idx.priorityIndex)
		return result
	}

	// 转换为切片并按优先级排序
	result := make([]*Rule, 0, len(candidateRules))
	for _, rule := range candidateRules {
		result = append(result, rule)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// GetAllRules 获取所有启用的规则（按优先级排序）
func (idx *Index) GetAllRules() []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]*Rule, len(idx.priorityIndex))
	copy(result, idx.priorityIndex)
	return result
}

// GetRulesByDevice 获取指定设备的规则
func (idx *Index) GetRulesByDevice(deviceID string) []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []*Rule

	// 获取特定设备的规则
	if rules, exists := idx.deviceIndex[deviceID]; exists {
		result = append(result, rules...)
	}

	// 获取通配符规则
	if rules, exists := idx.deviceIndex["*"]; exists {
		result = append(result, rules...)
	}

	// 按优先级排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// GetRulesByKey 获取指定key的规则
func (idx *Index) GetRulesByKey(key string) []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []*Rule

	// 获取特定key的规则
	if rules, exists := idx.keyIndex[key]; exists {
		result = append(result, rules...)
	}

	// 获取通配符规则
	if rules, exists := idx.keyIndex["*"]; exists {
		result = append(result, rules...)
	}

	// 按优先级排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// GetStats 获取索引统计信息
func (idx *Index) GetStats() map[string]interface{} {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	deviceCount := len(idx.deviceIndex)
	keyCount := len(idx.keyIndex)
	typeCount := len(idx.typeIndex)
	totalRules := len(idx.allRules)

	// 计算平均每个索引的规则数
	avgRulesPerDevice := 0.0
	if deviceCount > 0 {
		totalDeviceRules := 0
		for _, rules := range idx.deviceIndex {
			totalDeviceRules += len(rules)
		}
		avgRulesPerDevice = float64(totalDeviceRules) / float64(deviceCount)
	}

	avgRulesPerKey := 0.0
	if keyCount > 0 {
		totalKeyRules := 0
		for _, rules := range idx.keyIndex {
			totalKeyRules += len(rules)
		}
		avgRulesPerKey = float64(totalKeyRules) / float64(keyCount)
	}

	return map[string]interface{}{
		"total_rules":          totalRules,
		"device_index_count":   deviceCount,
		"key_index_count":      keyCount,
		"type_index_count":     typeCount,
		"avg_rules_per_device": avgRulesPerDevice,
		"avg_rules_per_key":    avgRulesPerKey,
	}
}

// Clear 清空索引
func (idx *Index) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.deviceIndex = make(map[string][]*Rule)
	idx.keyIndex = make(map[string][]*Rule)
	idx.typeIndex = make(map[string][]*Rule)
	idx.priorityIndex = make([]*Rule, 0)
	idx.allRules = make([]*Rule, 0)
}
