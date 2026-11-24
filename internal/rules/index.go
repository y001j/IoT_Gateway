package rules

import (
	"sort"
	"sync"

	"github.com/y001j/iot-gateway/internal/model"
)

// Index 规则索引：根据数据点的 device_id / key / type 快速筛选候选规则。
//
// 索引只使用"必然成立"的约束：从根条件出发、只经过 AND 节点能到达的
// `field eq "常量"` 条件。OR、NOT 分支以及表达式条件里的约束不能保证一定成立，
// 不参与索引（视为该维度不受约束）。
//
// 一个规则是数据点的候选规则，当且仅当在 device_id、key、type 三个维度上，
// 规则要么不受约束，要么数据点的取值落在规则约束的取值集合内（取交集）。
//
// 以前的实现对三个维度取并集，并把任何缺少设备条件的规则放进设备通配符桶，
// 几乎所有规则都会成为每个数据点的候选规则，索引形同虚设；而且会把 OR/NOT
// 分支里的取值当作约束，可能错误地排除本应命中的规则。
type Index struct {
	mu sync.RWMutex

	// 规则按优先级排序（高优先级在前，同优先级按 ID 稳定排序）
	priorityIndex []*Rule
	allRules      []*Rule
	constraints   map[string]ruleConstraints // rule ID -> 约束

	// 设备维度的倒排索引：deviceIndex[device] 为约束了该设备的规则，
	// deviceAny 为设备维度不受约束的规则
	deviceIndex map[string][]*Rule
	deviceAny   []*Rule

	// 统计用
	keyIndex  map[string][]*Rule
	typeIndex map[string][]*Rule
}

// ruleConstraints 规则在各维度上的约束；nil 表示该维度不受约束
type ruleConstraints struct {
	devices map[string]struct{}
	keys    map[string]struct{}
	types   map[string]struct{}
}

// NewIndex 创建新的索引
func NewIndex() *Index {
	return &Index{
		constraints: make(map[string]ruleConstraints),
		deviceIndex: make(map[string][]*Rule),
		keyIndex:    make(map[string][]*Rule),
		typeIndex:   make(map[string][]*Rule),
	}
}

// AddRule 添加或更新规则（禁用的规则会从索引中移除）
func (idx *Index) AddRule(rule *Rule) {
	if rule == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.removeRuleLocked(rule.ID)
	if !rule.Enabled {
		idx.rebuildLocked()
		return
	}
	idx.allRules = append(idx.allRules, rule)
	idx.constraints[rule.ID] = analyzeConstraints(rule.Conditions)
	idx.rebuildLocked()
}

// RemoveRule 从索引中移除规则
func (idx *Index) RemoveRule(rule *Rule) {
	if rule == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeRuleLocked(rule.ID)
	idx.rebuildLocked()
}

func (idx *Index) removeRuleLocked(ruleID string) {
	for i, r := range idx.allRules {
		if r.ID == ruleID {
			idx.allRules = append(idx.allRules[:i:i], idx.allRules[i+1:]...)
			break
		}
	}
	delete(idx.constraints, ruleID)
}

// rebuildLocked 根据 allRules 重建派生索引
func (idx *Index) rebuildLocked() {
	idx.priorityIndex = make([]*Rule, len(idx.allRules))
	copy(idx.priorityIndex, idx.allRules)
	sortByPriority(idx.priorityIndex)

	idx.deviceIndex = make(map[string][]*Rule)
	idx.keyIndex = make(map[string][]*Rule)
	idx.typeIndex = make(map[string][]*Rule)
	idx.deviceAny = nil

	for _, r := range idx.priorityIndex {
		c := idx.constraints[r.ID]
		if c.devices == nil {
			idx.deviceAny = append(idx.deviceAny, r)
		} else {
			for d := range c.devices {
				idx.deviceIndex[d] = append(idx.deviceIndex[d], r)
			}
		}
		for k := range c.keys {
			idx.keyIndex[k] = append(idx.keyIndex[k], r)
		}
		for t := range c.types {
			idx.typeIndex[t] = append(idx.typeIndex[t], r)
		}
	}
}

func sortByPriority(rules []*Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
}

// analyzeConstraints 提取条件中必然成立的 device_id / key / type 等值约束
func analyzeConstraints(cond *Condition) ruleConstraints {
	var c ruleConstraints
	collectConstraints(cond, &c)
	return c
}

func addConstraint(set *map[string]struct{}, v string) {
	// 同一维度出现多个 AND 等值约束时（例如 key=a AND key=b），规则不可能命中；
	// 这里保守地取并集，由求值器做最终判断。
	if *set == nil {
		*set = make(map[string]struct{})
	}
	(*set)[v] = struct{}{}
}

func collectConstraints(cond *Condition, c *ruleConstraints) {
	if cond == nil {
		return
	}
	// 与 Evaluator.Evaluate 的分派顺序保持一致：And 优先，其次 Or / Not，再按 Type
	if cond.And != nil {
		for _, sub := range cond.And {
			collectConstraints(sub, c)
		}
		return
	}
	if cond.Or != nil || cond.Not != nil {
		return
	}
	if cond.Type != "" && cond.Type != "simple" {
		return
	}
	op := cond.Operator
	if op != "" && op != "eq" {
		return
	}
	v, ok := cond.Value.(string)
	if !ok || v == "" || v == "*" {
		return
	}
	switch cond.Field {
	case "device_id":
		addConstraint(&c.devices, v)
	case "key":
		addConstraint(&c.keys, v)
	case "type":
		addConstraint(&c.types, v)
	}
}

func allows(set map[string]struct{}, v string) bool {
	if set == nil {
		return true
	}
	_, ok := set[v]
	return ok
}

// Match 返回数据点的候选规则（按优先级排序）
func (idx *Index) Match(point model.Point) []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	specific := idx.deviceIndex[point.DeviceID]
	result := make([]*Rule, 0, len(specific)+len(idx.deviceAny))
	dataType := string(point.Type)
	for _, list := range [][]*Rule{specific, idx.deviceAny} {
		for _, r := range list {
			c := idx.constraints[r.ID]
			if allows(c.keys, point.Key) && allows(c.types, dataType) {
				result = append(result, r)
			}
		}
	}
	if len(specific) > 0 && len(idx.deviceAny) > 0 {
		sortByPriority(result)
	}
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

// GetRulesByDevice 获取可能作用于指定设备的规则
func (idx *Index) GetRulesByDevice(deviceID string) []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	result := append([]*Rule{}, idx.deviceIndex[deviceID]...)
	result = append(result, idx.deviceAny...)
	sortByPriority(result)
	return result
}

// GetRulesByKey 获取可能作用于指定 key 的规则
func (idx *Index) GetRulesByKey(key string) []*Rule {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var result []*Rule
	for _, r := range idx.priorityIndex {
		if allows(idx.constraints[r.ID].keys, key) {
			result = append(result, r)
		}
	}
	return result
}

// GetStats 获取索引统计信息
func (idx *Index) GetStats() map[string]interface{} {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	avg := func(m map[string][]*Rule) float64 {
		if len(m) == 0 {
			return 0
		}
		total := 0
		for _, rules := range m {
			total += len(rules)
		}
		return float64(total) / float64(len(m))
	}
	return map[string]interface{}{
		"total_rules":           len(idx.allRules),
		"device_index_count":    len(idx.deviceIndex),
		"device_wildcard_rules": len(idx.deviceAny),
		"key_index_count":       len(idx.keyIndex),
		"type_index_count":      len(idx.typeIndex),
		"avg_rules_per_device":  avg(idx.deviceIndex),
		"avg_rules_per_key":     avg(idx.keyIndex),
	}
}

// Clear 清空索引
func (idx *Index) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.allRules = nil
	idx.constraints = make(map[string]ruleConstraints)
	idx.rebuildLocked()
}
