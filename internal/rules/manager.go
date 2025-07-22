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
	rulesDir    string
	rules       map[string]*Rule
	ruleIndex   *Index
	watcher     *fsnotify.Watcher
	changesChan chan RuleChangeEvent
	mu          sync.RWMutex
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
		// JSON格式
		var ruleData struct {
			Rules []*Rule `json:"rules"`
		}
		if err := json.Unmarshal(data, &ruleData); err != nil {
			// 尝试解析单个规则
			var singleRule Rule
			if err2 := json.Unmarshal(data, &singleRule); err2 != nil {
				return nil, fmt.Errorf("解析JSON规则失败: %w", err)
			}
			rules = []*Rule{&singleRule}
		} else {
			rules = ruleData.Rules
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

	// 按优先级排序
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
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

	// 按优先级排序
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
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

	// 保存到内存
	m.rules[rule.ID] = rule
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

	log.Info().Str("rule_id", rule.ID).Str("name", rule.Name).Msg("规则保存成功")
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
