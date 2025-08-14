package rules

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
	"github.com/y001j/iot-gateway/internal/model"
)

// RuleManager defines the interface for managing rules.
type RuleManager interface {
	LoadRules() error
	GetRule(id string) (*Rule, error)
	ListRules() []*Rule
	GetEnabledRules() []*Rule
	SaveRule(rule *Rule) error
	DeleteRule(id string) error
	EnableRule(id string) error
	DisableRule(id string) error
	GetIndex() *Index
	WatchChanges() (<-chan RuleChangeEvent, error)
	Close() error
	GetStats() map[string]interface{}
}

// Manager 规则管理器
type Manager struct {
	rulesDir         string
	rules            map[string]*Rule
	ruleIndex        *Index
	watcher          *fsnotify.Watcher
	changesChan      chan RuleChangeEvent
	mu               sync.RWMutex
}

// NewManager 创建规则管理器
func NewManager(rulesDir string) *Manager {
	return &Manager{
		rulesDir:    rulesDir,
		rules:       make(map[string]*Rule),
		ruleIndex:   NewIndex(),
		changesChan: make(chan RuleChangeEvent, 100),
	}
}

// LoadRules 加载所有规则
func (m *Manager) LoadRules() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 确保规则目录存在
	if err := os.MkdirAll(m.rulesDir, 0755); err != nil {
		return fmt.Errorf("创建规则目录失败: %w", err)
	}

	// 清空现有规则
	m.rules = make(map[string]*Rule)
	m.ruleIndex = NewIndex()

	// 扫描规则文件
	err := filepath.Walk(m.rulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理 .json 和 .yaml/.yml 文件
		ext := filepath.Ext(path)
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			return nil
		}

		rules, err := m.loadRuleFile(path)
		if err != nil {
			log.Error().Err(err).Str("file", path).Msg("加载规则文件失败")
			return nil // 继续处理其他文件
		}

		for _, rule := range rules {
			if err := m.addRule(rule); err != nil {
				log.Error().Err(err).Str("rule_id", rule.ID).Msg("添加规则失败")
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("扫描规则目录失败: %w", err)
	}

	log.Info().Int("count", len(m.rules)).Str("dir", m.rulesDir).Msg("规则加载完成")
	// 调试：输出所有加载的规则ID和名称
	for id, rule := range m.rules {
		log.Debug().Str("rule_id", id).Str("name", rule.Name).Int("priority", rule.Priority).Msg("已加载的规则详情")
	}
	return nil
}

// loadRuleFile 加载单个规则文件
func (m *Manager) loadRuleFile(filePath string) ([]*Rule, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	ext := filepath.Ext(filePath)
	var rules []*Rule

	if ext == ".json" {
		// 先尝试解析单个规则（最常见的情况）
		var singleRule Rule
		if err := json.Unmarshal(data, &singleRule); err == nil && singleRule.ID != "" {
			// 成功解析为单个规则且ID不为空
			log.Debug().Str("file", filePath).Str("rule_id", singleRule.ID).Msg("成功解析单个规则")
			rules = []*Rule{&singleRule}
		} else {
			// 尝试解析为直接的规则数组格式 [{"id": "..."}, ...]
			var ruleArray []*Rule
			if err2 := json.Unmarshal(data, &ruleArray); err2 == nil && len(ruleArray) > 0 {
				// 验证数组中的规则是否有效
				validRules := []*Rule{}
				for _, rule := range ruleArray {
					if rule != nil && rule.ID != "" {
						validRules = append(validRules, rule)
					}
				}
				if len(validRules) > 0 {
					log.Debug().Str("file", filePath).Int("total", len(ruleArray)).Int("valid", len(validRules)).Msg("成功解析直接规则数组")
					rules = validRules
				} else {
					log.Debug().Str("file", filePath).Int("total", len(ruleArray)).Msg("直接规则数组解析成功但没有有效规则")
				}
			} else {
				// 尝试解析为包装规则数组的对象格式 {"rules": [...]}
				var ruleData struct {
					Rules []*Rule `json:"rules"`
				}
				if err3 := json.Unmarshal(data, &ruleData); err3 != nil || len(ruleData.Rules) == 0 {
					log.Debug().Str("file", filePath).Err(err).Err(err2).Err(err3).Msg("JSON解析失败，尝试三种格式都失败")
					return nil, fmt.Errorf("解析JSON规则失败: 单规则格式错误(%v), 数组格式错误(%v), 对象格式错误(%v)", err, err2, err3)
				}
				log.Debug().Str("file", filePath).Int("count", len(ruleData.Rules)).Msg("成功解析包装规则数组")
				rules = ruleData.Rules
			}
		}
	} else {
		// YAML格式
		var ruleData struct {
			Rules []*Rule `yaml:"rules"`
		}
		if err := yaml.Unmarshal(data, &ruleData); err != nil {
			// 尝试解析单个规则
			var singleRule Rule
			if err2 := yaml.Unmarshal(data, &singleRule); err2 != nil {
				return nil, fmt.Errorf("解析YAML规则失败: %w", err)
			}
			rules = []*Rule{&singleRule}
		} else {
			rules = ruleData.Rules
		}
	}

	// 验证和初始化规则
	for _, rule := range rules {
		if err := m.validateRule(rule); err != nil {
			log.Error().Err(err).Str("rule_id", rule.ID).Msg("规则验证失败")
			continue
		}
		m.initializeRule(rule)
	}

	return rules, nil
}

// validateRule 验证规则
func (m *Manager) validateRule(rule *Rule) error {
	if rule.ID == "" {
		return fmt.Errorf("规则ID不能为空")
	}
	if rule.Name == "" {
		return fmt.Errorf("规则名称不能为空")
	}
	if rule.Conditions == nil {
		return fmt.Errorf("规则条件不能为空")
	}
	if len(rule.Actions) == 0 {
		return fmt.Errorf("规则动作不能为空")
	}

	// 验证条件
	if err := m.validateCondition(rule.Conditions); err != nil {
		return fmt.Errorf("条件验证失败: %w", err)
	}

	// 验证动作
	for i, action := range rule.Actions {
		if action.Type == "" {
			return fmt.Errorf("动作[%d]类型不能为空", i)
		}
	}

	return nil
}

// validateCondition 验证条件
func (m *Manager) validateCondition(condition *Condition) error {
	if condition == nil {
		return fmt.Errorf("条件不能为空")
	}

	// 检查条件类型
	switch condition.Type {
	case "", "simple":
		// 简单条件必须有字段和操作符
		if condition.Field == "" && len(condition.And) == 0 && len(condition.Or) == 0 && condition.Not == nil {
			return fmt.Errorf("简单条件必须指定字段或逻辑操作")
		}
		if condition.Field != "" && condition.Operator == "" {
			return fmt.Errorf("指定字段时必须指定操作符")
		}
	case "and":
		if len(condition.And) < 2 {
			return fmt.Errorf("AND条件必须至少包含2个子条件")
		}
	case "or":
		if len(condition.Or) < 2 {
			return fmt.Errorf("OR条件必须至少包含2个子条件")
		}
	case "expression":
		if condition.Expression == "" {
			return fmt.Errorf("表达式条件必须指定expression")
		}
	case "lua":
		if condition.Script == "" {
			return fmt.Errorf("Lua条件必须指定script")
		}
	default:
		return fmt.Errorf("不支持的条件类型: %s", condition.Type)
	}

	// 递归验证嵌套条件
	for _, subCondition := range condition.And {
		if err := m.validateCondition(subCondition); err != nil {
			return err
		}
	}
	for _, subCondition := range condition.Or {
		if err := m.validateCondition(subCondition); err != nil {
			return err
		}
	}
	if condition.Not != nil {
		if err := m.validateCondition(condition.Not); err != nil {
			return err
		}
	}

	return nil
}

// initializeRule 初始化规则
func (m *Manager) initializeRule(rule *Rule) {
	now := time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now

	if rule.Version == 0 {
		rule.Version = 1
	}

	if rule.Tags == nil {
		rule.Tags = make(map[string]string)
	}
}

// addRule 添加规则
func (m *Manager) addRule(rule *Rule) error {
	// 检查重复ID
	if existingRule, exists := m.rules[rule.ID]; exists {
		log.Warn().
			Str("rule_id", rule.ID).
			Str("existing_name", existingRule.Name).
			Str("new_name", rule.Name).
			Msg("规则ID重复，将覆盖现有规则")
	}

	m.rules[rule.ID] = rule
	m.ruleIndex.AddRule(rule)

	log.Debug().
		Str("rule_id", rule.ID).
		Str("name", rule.Name).
		Bool("enabled", rule.Enabled).
		Int("priority", rule.Priority).
		Msg("规则添加成功")

	return nil
}

// GetRule 获取规则
func (m *Manager) GetRule(id string) (*Rule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rule, exists := m.rules[id]
	if !exists {
		return nil, fmt.Errorf("规则不存在: %s", id)
	}

	return rule, nil
}

// ListRules 列出所有规则
func (m *Manager) ListRules() []*Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rules := make([]*Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		rules = append(rules, rule)
	}

	// 按优先级 + 规则名称排序
	sort.Slice(rules, func(i, j int) bool {
		// 首先按优先级排序（降序）
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		// 优先级相同时按规则名称排序（升序）
		return rules[i].Name < rules[j].Name
	})

	return rules
}

// GetEnabledRules 获取启用的规则
func (m *Manager) GetEnabledRules() []*Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var rules []*Rule
	for _, rule := range m.rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}

	// 按优先级 + 规则名称排序
	sort.Slice(rules, func(i, j int) bool {
		// 首先按优先级排序（降序）
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		// 优先级相同时按规则名称排序（升序）
		return rules[i].Name < rules[j].Name
	})

	return rules
}

// SaveRule 保存规则
func (m *Manager) SaveRule(rule *Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.validateRule(rule); err != nil {
		return fmt.Errorf("规则验证失败: %w", err)
	}

	// 更新版本和时间
	if existingRule, exists := m.rules[rule.ID]; exists {
		rule.Version = existingRule.Version + 1
		rule.CreatedAt = existingRule.CreatedAt
	} else {
		rule.Version = 1
		rule.CreatedAt = time.Now()
	}
	rule.UpdatedAt = time.Now()

	// 智能文件管理：清理数组文件中的重复规则
	if err := m.cleanupDuplicateInArrayFiles(rule.ID); err != nil {
		log.Warn().Err(err).Str("rule_id", rule.ID).Msg("清理数组文件中的重复规则失败")
	}

	// 先保存到独立文件
	filePath := filepath.Join(m.rulesDir, fmt.Sprintf("%s.json", rule.ID))
	if err := m.saveRuleToFile(rule, filePath); err != nil {
		return fmt.Errorf("保存规则文件失败: %w", err)
	}

	// 确保内存状态同步：强制更新内存中的规则
	m.rules[rule.ID] = rule
	m.ruleIndex.AddRule(rule) // AddRule内部会处理重复规则的覆盖

	// 发送变更事件
	select {
	case m.changesChan <- RuleChangeEvent{Type: "update", Rule: rule}:
	default:
		log.Warn().Str("rule_id", rule.ID).Msg("规则变更事件队列已满")
	}

	log.Info().Str("rule_id", rule.ID).Str("name", rule.Name).Int("version", rule.Version).
		Time("updated_at", rule.UpdatedAt).Msg("规则保存成功")
	return nil
}

// saveRuleToFile 保存规则到文件
func (m *Manager) saveRuleToFile(rule *Rule, filePath string) error {
	data, err := json.MarshalIndent(rule, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化规则失败: %w", err)
	}

	return ioutil.WriteFile(filePath, data, 0644)
}

// DeleteRule 删除规则
func (m *Manager) DeleteRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, exists := m.rules[id]
	if !exists {
		return fmt.Errorf("规则不存在: %s", id)
	}

	// 从内存中删除
	delete(m.rules, id)
	m.ruleIndex.RemoveRule(rule)

	// 删除文件
	filePath := filepath.Join(m.rulesDir, fmt.Sprintf("%s.json", id))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Error().Err(err).Str("file", filePath).Msg("删除规则文件失败")
	}

	// 发送变更事件
	select {
	case m.changesChan <- RuleChangeEvent{Type: "delete", Rule: rule}:
	default:
		log.Warn().Str("rule_id", id).Msg("规则变更事件队列已满")
	}

	log.Info().Str("rule_id", id).Str("name", rule.Name).Msg("规则删除成功")
	return nil
}

// EnableRule 启用规则
func (m *Manager) EnableRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, exists := m.rules[id]
	if !exists {
		return fmt.Errorf("规则不存在: %s", id)
	}

	if !rule.Enabled {
		rule.Enabled = true
		rule.UpdatedAt = time.Now()
		rule.Version++

		// 更新索引
		m.ruleIndex.AddRule(rule)

		// 保存到文件
		filePath := filepath.Join(m.rulesDir, fmt.Sprintf("%s.json", rule.ID))
		if err := m.saveRuleToFile(rule, filePath); err != nil {
			return fmt.Errorf("保存规则文件失败: %w", err)
		}

		// 发送变更事件
		select {
		case m.changesChan <- RuleChangeEvent{Type: "update", Rule: rule}:
		default:
			log.Warn().Str("rule_id", rule.ID).Msg("规则变更事件队列已满")
		}

		log.Info().Str("rule_id", id).Str("name", rule.Name).Msg("规则已启用")
	}

	return nil
}

// DisableRule 禁用规则
func (m *Manager) DisableRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, exists := m.rules[id]
	if !exists {
		return fmt.Errorf("规则不存在: %s", id)
	}

	if rule.Enabled {
		rule.Enabled = false
		rule.UpdatedAt = time.Now()
		rule.Version++

		// 更新索引
		m.ruleIndex.AddRule(rule)

		// 保存到文件
		filePath := filepath.Join(m.rulesDir, fmt.Sprintf("%s.json", rule.ID))
		if err := m.saveRuleToFile(rule, filePath); err != nil {
			return fmt.Errorf("保存规则文件失败: %w", err)
		}

		// 发送变更事件
		select {
		case m.changesChan <- RuleChangeEvent{Type: "update", Rule: rule}:
		default:
			log.Warn().Str("rule_id", rule.ID).Msg("规则变更事件队列已满")
		}

		log.Info().Str("rule_id", id).Str("name", rule.Name).Msg("规则已禁用")
	}

	return nil
}

// GetIndex 获取规则索引
func (m *Manager) GetIndex() *Index {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ruleIndex
}

// WatchChanges 监控规则变化
func (m *Manager) WatchChanges() (<-chan RuleChangeEvent, error) {
	var err error
	m.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("创建文件监控器失败: %w", err)
	}

	// 监控规则目录
	if err := m.watcher.Add(m.rulesDir); err != nil {
		return nil, fmt.Errorf("添加目录监控失败: %w", err)
	}

	// 启动文件监控协程
	go m.watchFileChanges()

	return m.changesChan, nil
}

// watchFileChanges 监控文件变化
func (m *Manager) watchFileChanges() {
	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}

			// 只处理规则文件
			ext := filepath.Ext(event.Name)
			if ext != ".json" && ext != ".yaml" && ext != ".yml" {
				continue
			}

			log.Debug().Str("file", event.Name).Str("op", event.Op.String()).Msg("检测到文件变化")

			// 延迟处理，避免文件正在写入
			time.Sleep(100 * time.Millisecond)

			switch {
			case event.Op&fsnotify.Write == fsnotify.Write:
				m.handleFileUpdate(event.Name)
			case event.Op&fsnotify.Create == fsnotify.Create:
				m.handleFileCreate(event.Name)
			case event.Op&fsnotify.Remove == fsnotify.Remove:
				m.handleFileDelete(event.Name)
			}

		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("文件监控错误")
		}
	}
}

// handleFileUpdate 处理文件更新
func (m *Manager) handleFileUpdate(filePath string) {
	rules, err := m.loadRuleFile(filePath)
	if err != nil {
		log.Error().Err(err).Str("file", filePath).Msg("重新加载规则文件失败")
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, rule := range rules {
		if err := m.validateRule(rule); err != nil {
			log.Error().Err(err).Str("rule_id", rule.ID).Msg("规则验证失败")
			continue
		}

		m.initializeRule(rule)
		m.rules[rule.ID] = rule
		m.ruleIndex.AddRule(rule)

		// 发送变更事件
		select {
		case m.changesChan <- RuleChangeEvent{Type: "update", Rule: rule}:
		default:
			log.Warn().Str("rule_id", rule.ID).Msg("规则变更事件队列已满")
		}
	}

	log.Info().Str("file", filePath).Int("count", len(rules)).Msg("规则文件重新加载完成")
}

// handleFileCreate 处理文件创建
func (m *Manager) handleFileCreate(filePath string) {
	m.handleFileUpdate(filePath) // 创建和更新处理逻辑相同
}

// handleFileDelete 处理文件删除
func (m *Manager) handleFileDelete(filePath string) {
	// 从文件名推断规则ID
	fileName := filepath.Base(filePath)
	ext := filepath.Ext(fileName)
	ruleID := fileName[:len(fileName)-len(ext)]

	m.mu.Lock()
	defer m.mu.Unlock()

	if rule, exists := m.rules[ruleID]; exists {
		delete(m.rules, ruleID)
		m.ruleIndex.RemoveRule(rule)

		// 发送变更事件
		select {
		case m.changesChan <- RuleChangeEvent{Type: "delete", Rule: rule}:
		default:
			log.Warn().Str("rule_id", ruleID).Msg("规则变更事件队列已满")
		}

		log.Info().Str("rule_id", ruleID).Str("file", filePath).Msg("规则文件删除，已移除规则")
	}
}

// Close 关闭管理器
func (m *Manager) Close() error {
	if m.watcher != nil {
		return m.watcher.Close()
	}
	return nil
}

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalRules := len(m.rules)
	enabledRules := 0
	disabledRules := 0

	for _, rule := range m.rules {
		if rule.Enabled {
			enabledRules++
		} else {
			disabledRules++
		}
	}

	return map[string]interface{}{
		"total_rules":    totalRules,
		"enabled_rules":  enabledRules,
		"disabled_rules": disabledRules,
		"rules_dir":      m.rulesDir,
	}
}

// cleanupDuplicateInArrayFiles 清理数组文件中的重复规则
func (m *Manager) cleanupDuplicateInArrayFiles(ruleID string) error {
	// 扫描所有JSON文件
	err := filepath.Walk(m.rulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理 JSON 文件
		if filepath.Ext(path) != ".json" {
			return nil
		}

		// 跳过目标规则的独立文件
		expectedIndividualFile := filepath.Join(m.rulesDir, fmt.Sprintf("%s.json", ruleID))
		if path == expectedIndividualFile {
			return nil
		}

		// 读取文件内容
		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Error().Err(err).Str("file", path).Msg("读取文件失败")
			return nil // 继续处理其他文件
		}

		// 检查是否是数组文件
		if !m.isArrayFile(data) {
			return nil
		}

		// 尝试移除重复规则
		modified, newData, err := m.removeRuleFromArrayFile(data, ruleID)
		if err != nil {
			log.Error().Err(err).Str("file", path).Str("rule_id", ruleID).Msg("处理数组文件失败")
			return nil
		}

		// 如果有修改，写回文件
		if modified {
			if len(newData) == 0 || string(newData) == "[]" {
				// 如果数组为空，删除文件
				if err := os.Remove(path); err != nil {
					log.Error().Err(err).Str("file", path).Msg("删除空数组文件失败")
				} else {
					log.Info().Str("file", path).Str("rule_id", ruleID).Msg("删除空数组文件")
				}
			} else {
				// 写回更新的内容
				if err := ioutil.WriteFile(path, newData, 0644); err != nil {
					log.Error().Err(err).Str("file", path).Msg("写回数组文件失败")
				} else {
					log.Info().Str("file", path).Str("rule_id", ruleID).Msg("从数组文件中移除重复规则")
				}
			}
		}

		return nil
	})

	return err
}

// isArrayFile 检查文件内容是否为数组格式
func (m *Manager) isArrayFile(data []byte) bool {
	// 尝试解析为数组
	var ruleArray []*Rule
	if err := json.Unmarshal(data, &ruleArray); err == nil && len(ruleArray) > 0 {
		return true
	}

	// 尝试解析为包装数组格式
	var ruleData struct {
		Rules []*Rule `json:"rules"`
	}
	if err := json.Unmarshal(data, &ruleData); err == nil && len(ruleData.Rules) > 0 {
		return true
	}

	return false
}

// removeRuleFromArrayFile 从数组文件中移除指定规则
func (m *Manager) removeRuleFromArrayFile(data []byte, ruleID string) (bool, []byte, error) {
	// 尝试直接数组格式
	var ruleArray []*Rule
	if err := json.Unmarshal(data, &ruleArray); err == nil {
		originalCount := len(ruleArray)
		filteredRules := []*Rule{}
		
		for _, rule := range ruleArray {
			if rule != nil && rule.ID != ruleID {
				filteredRules = append(filteredRules, rule)
			}
		}

		if len(filteredRules) != originalCount {
			// 有规则被移除
			if len(filteredRules) == 0 {
				return true, []byte("[]"), nil
			}
			newData, err := json.MarshalIndent(filteredRules, "", "  ")
			return true, newData, err
		}
		return false, data, nil
	}

	// 尝试包装数组格式
	var ruleData struct {
		Rules []*Rule `json:"rules"`
	}
	if err := json.Unmarshal(data, &ruleData); err == nil {
		originalCount := len(ruleData.Rules)
		filteredRules := []*Rule{}
		
		for _, rule := range ruleData.Rules {
			if rule != nil && rule.ID != ruleID {
				filteredRules = append(filteredRules, rule)
			}
		}

		if len(filteredRules) != originalCount {
			// 有规则被移除
			ruleData.Rules = filteredRules
			if len(filteredRules) == 0 {
				return true, []byte(`{"rules":[]}`), nil
			}
			newData, err := json.MarshalIndent(ruleData, "", "  ")
			return true, newData, err
		}
		return false, data, nil
	}

	return false, data, fmt.Errorf("无法解析为数组格式")
}

// EvaluateEnabledRules 评估所有启用的规则
func (m *Manager) EvaluateEnabledRules(point model.Point) (map[string]bool, map[string]error, error) {
	m.mu.RLock()
	enabledRules := m.GetEnabledRules()
	m.mu.RUnlock()
	
	if len(enabledRules) == 0 {
		return make(map[string]bool), make(map[string]error), nil
	}
	
	resultMap := make(map[string]bool)
	errorMap := make(map[string]error)
	evaluator := NewEvaluator()
	
	for _, rule := range enabledRules {
		if rule == nil {
			continue
		}
		
		result, err := evaluator.Evaluate(rule.Conditions, point)
		resultMap[rule.ID] = result
		if err != nil {
			errorMap[rule.ID] = err
		}
	}
	
	return resultMap, errorMap, nil
}
