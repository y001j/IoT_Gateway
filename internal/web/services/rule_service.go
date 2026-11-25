package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/web/models"

	"github.com/y001j/iot-gateway/internal/rules"
)

// RuleService 规则服务接口
type RuleService interface {
	GetRules(req *models.RuleListRequest) ([]models.Rule, int, error)
	GetRule(id string) (*models.Rule, error)
	CreateRule(rule *models.RuleCreateRequest) (*models.Rule, error)
	UpdateRule(id string, rule *models.RuleUpdateRequest) (*models.Rule, error)
	DeleteRule(id string) error
	EnableRule(id string) error
	DisableRule(id string) error
	ValidateRule(rule *models.Rule) (*models.RuleValidationResponseExtended, error)
	TestRule(req *models.RuleTestRequestExtended) (*models.RuleTestResponseExtended, error)
	GetRuleStats(id string) (*models.RuleStatsExtended, error)
	GetRuleExecutionHistory(id string, req *models.RuleHistoryRequest) ([]models.RuleExecution, int, error)
	GetRuleTemplates() ([]models.RuleTemplate, error)
	CreateRuleFromTemplate(templateID string, req *models.RuleFromTemplateRequest) (*models.Rule, error)
}

// ruleService 规则服务实现
type ruleService struct {
	manager rules.RuleManager
}

// NewRuleService 创建规则服务
func NewRuleService(manager rules.RuleManager) (RuleService, error) {
	if manager == nil {
		return nil, fmt.Errorf("rule manager is required")
	}
	return &ruleService{manager: manager}, nil
}

// convertToWebRule converts a manager rule to a web model rule.
func convertToWebRule(managerRule *rules.Rule) models.Rule {
	return models.Rule{
		ID:          managerRule.ID, // String ID from rule manager
		Name:        managerRule.Name,
		Description: managerRule.Description,
		Enabled:     managerRule.Enabled,
		Priority:    managerRule.Priority,
		Version:     managerRule.Version,
		DataType:    managerRule.DataType, // 传递数据类型字段
		Conditions:  convertCondition(managerRule.Conditions),
		Actions:     convertActions(managerRule.Actions),
		Tags:        managerRule.Tags,
		CreatedAt:   managerRule.CreatedAt,
		UpdatedAt:   managerRule.UpdatedAt,
	}
}

// convertCondition converts rules.Condition to models.RuleCondition
func convertCondition(cond *rules.Condition) *models.RuleCondition {
	if cond == nil {
		return nil
	}

	webCond := &models.RuleCondition{
		Type:       cond.Type,
		Field:      cond.Field,
		Operator:   cond.Operator,
		Value:      cond.Value,
		Expression: cond.Expression,
		Script:     cond.Script,
	}

	// Convert And conditions
	if len(cond.And) > 0 {
		webCond.And = make([]models.RuleCondition, len(cond.And))
		for i, andCond := range cond.And {
			if converted := convertCondition(andCond); converted != nil {
				webCond.And[i] = *converted
			}
		}
	}

	// Convert Or conditions
	if len(cond.Or) > 0 {
		webCond.Or = make([]models.RuleCondition, len(cond.Or))
		for i, orCond := range cond.Or {
			if converted := convertCondition(orCond); converted != nil {
				webCond.Or[i] = *converted
			}
		}
	}

	// Convert Not condition
	if cond.Not != nil {
		webCond.Not = convertCondition(cond.Not)
	}

	return webCond
}

// convertActions converts []rules.Action to []models.RuleAction
func convertActions(actions []rules.Action) []models.RuleAction {
	if len(actions) == 0 {
		return []models.RuleAction{}
	}

	webActions := make([]models.RuleAction, len(actions))
	for i, action := range actions {
		webActions[i] = models.RuleAction{
			Type:    action.Type,
			Config:  action.Config,
			Async:   action.Async,
			Timeout: action.Timeout.String(),
			Retry:   action.Retry,
		}
	}

	return webActions
}

func (s *ruleService) GetRules(req *models.RuleListRequest) ([]models.Rule, int, error) {
	if s.manager == nil {
		log.Error().Msg("RuleService: manager为nil")
		return []models.Rule{}, 0, fmt.Errorf("rule manager is nil")
	}

	managerRules := s.manager.ListRules()
	log.Info().Int("rule_count", len(managerRules)).Msg("RuleService: 获取规则列表")

	webRules := make([]models.Rule, 0, len(managerRules))
	for _, r := range managerRules {
		log.Info().Str("rule_id", r.ID).Str("rule_name", r.Name).Msg("RuleService: 转换规则")
		// TODO: Implement filtering based on req.Type, req.Status, etc.
		webRules = append(webRules, convertToWebRule(r))
	}

	sort.Slice(webRules, func(i, j int) bool {
		return webRules[i].Priority > webRules[j].Priority
	})

	total := len(webRules)
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize
	if start >= total {
		return []models.Rule{}, total, nil
	}
	if end > total {
		end = total
	}

	return webRules[start:end], total, nil
}

func (s *ruleService) GetRule(id string) (*models.Rule, error) {
	managerRule, err := s.manager.GetRule(id)
	if err != nil {
		return nil, err
	}
	webRule := convertToWebRule(managerRule)
	return &webRule, nil
}

func (s *ruleService) CreateRule(req *models.RuleCreateRequest) (*models.Rule, error) {
	// Convert web condition to manager condition
	managerCondition, err := convertToManagerCondition(req.Conditions)
	if err != nil {
		return nil, fmt.Errorf("invalid conditions: %w", err)
	}

	// Convert web actions to manager actions
	managerActions, err := convertToManagerActions(req.Actions)
	if err != nil {
		return nil, fmt.Errorf("invalid actions: %w", err)
	}

	newRule := &rules.Rule{
		ID:          newID("rule"),
		Name:        req.Name,
		Description: req.Description,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
		Version:     1,
		Conditions:  managerCondition,
		Actions:     managerActions,
		Tags:        req.Tags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.manager.SaveRule(newRule); err != nil {
		return nil, err
	}

	webRule := convertToWebRule(newRule)
	return &webRule, nil
}

func (s *ruleService) UpdateRule(id string, req *models.RuleUpdateRequest) (*models.Rule, error) {
	managerRule, err := s.manager.GetRule(id)
	if err != nil {
		return nil, err
	}

	// Apply updates from req
	if req.Name != "" {
		managerRule.Name = req.Name
	}
	if req.Description != "" {
		managerRule.Description = req.Description
	}
	if req.Priority > 0 {
		managerRule.Priority = req.Priority
	}
	if req.Enabled != nil {
		managerRule.Enabled = *req.Enabled
	}
	if req.Tags != nil {
		managerRule.Tags = req.Tags
	}

	// Update conditions if provided
	if req.Conditions != nil {
		// Convert web condition to manager condition
		managerCondition, err := convertToManagerCondition(req.Conditions)
		if err != nil {
			return nil, fmt.Errorf("invalid conditions: %w", err)
		}
		managerRule.Conditions = managerCondition
	}

	// Update actions if provided
	if req.Actions != nil && len(req.Actions) > 0 {
		// Convert web actions to manager actions
		managerActions, err := convertToManagerActions(req.Actions)
		if err != nil {
			return nil, fmt.Errorf("invalid actions: %w", err)
		}
		managerRule.Actions = managerActions
	}

	// Update version and timestamp
	managerRule.Version++
	managerRule.UpdatedAt = time.Now()

	if err := s.manager.SaveRule(managerRule); err != nil {
		return nil, err
	}

	webRule := convertToWebRule(managerRule)
	return &webRule, nil
}

func (s *ruleService) DeleteRule(id string) error {
	return s.manager.DeleteRule(id)
}

func (s *ruleService) EnableRule(id string) error {
	return s.manager.EnableRule(id)
}

func (s *ruleService) DisableRule(id string) error {
	return s.manager.DisableRule(id)
}

// ErrRuleStatsUnavailable 规则级执行统计尚未实现
var ErrRuleStatsUnavailable = errors.New("规则执行统计和历史暂不支持，请使用网关 /metrics 中的规则引擎指标")

// supportedActionTypes 规则引擎内置的动作类型
var supportedActionTypes = map[string]bool{"alert": true, "transform": true, "filter": true, "aggregate": true, "forward": true}

// ValidateRule 验证规则：使用规则引擎自身的校验（rules.ValidateRule），保证与加载/求值语义一致。
// 以前这里另写了一套校验，要求条件必须写明 type，导致通过本接口创建的规则（不带 type）测试时必然失败。
func (s *ruleService) ValidateRule(rule *models.Rule) (*models.RuleValidationResponseExtended, error) {
	response := &models.RuleValidationResponseExtended{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}
	fail := func(msg string) {
		response.Valid = false
		response.Errors = append(response.Errors, msg)
	}

	if rule.Priority < 0 || rule.Priority > 100 {
		fail("规则优先级必须在0-100之间")
	}
	cond, err := convertToManagerCondition(rule.Conditions)
	if err != nil {
		fail("条件转换失败: " + err.Error())
	}
	actions, err := convertToManagerActions(rule.Actions)
	if err != nil {
		fail("动作转换失败: " + err.Error())
	}
	id := rule.ID
	if id == "" {
		id = "validate" // 新建规则尚未分配 ID
	}
	if err := rules.ValidateRule(&rules.Rule{ID: id, Name: rule.Name, Conditions: cond, Actions: actions}); err != nil {
		fail(err.Error())
	}
	for i, a := range rule.Actions {
		if a.Type != "" && !supportedActionTypes[a.Type] {
			fail(fmt.Sprintf("动作 %d: 不支持的动作类型 %s", i+1, a.Type))
		}
		if a.Type == "alert" {
			if msg, _ := a.Config["message"].(string); msg == "" {
				response.Warnings = append(response.Warnings, fmt.Sprintf("动作 %d: 告警未指定 message，将使用默认消息", i+1))
			}
		}
	}
	if rule.Priority > 90 {
		response.Warnings = append(response.Warnings, "高优先级规则可能影响系统性能")
	}
	return response, nil
}

// testPoint 把测试数据转换为数据点：device_id、key、value、type、quality 对应点的字段，
// tags 对象和其余字段作为标签
func testPoint(data map[string]interface{}) model.Point {
	p := model.Point{Timestamp: time.Now()}
	p.DeviceID, _ = data["device_id"].(string)
	p.Key, _ = data["key"].(string)
	p.Value = data["value"]
	if t, ok := data["type"].(string); ok {
		p.Type = model.DataType(t)
	}
	if q, ok := data["quality"].(float64); ok {
		p.Quality = int(q)
	}
	if tags, ok := data["tags"].(map[string]interface{}); ok {
		for k, v := range tags {
			p.AddTag(k, fmt.Sprint(v))
		}
	}
	for k, v := range data {
		switch k {
		case "device_id", "key", "value", "type", "quality", "tags", "timestamp":
		default:
			p.AddTag(k, fmt.Sprint(v))
		}
	}
	return p
}

// TestRule 用规则引擎的求值器对测试数据求值（以前是另写的简化逻辑，只支持少数操作符和 float64）
func (s *ruleService) TestRule(req *models.RuleTestRequestExtended) (*models.RuleTestResponseExtended, error) {
	response := &models.RuleTestResponseExtended{
		Success:    true,
		Results:    []models.RuleTestResult{},
		Errors:     []string{},
		ExecutedAt: time.Now(),
	}
	validation, _ := s.ValidateRule(&req.Rule)
	if !validation.Valid {
		response.Success = false
		response.Errors = append(response.Errors, validation.Errors...)
		return response, nil
	}
	cond, _ := convertToManagerCondition(req.Rule.Conditions)
	evaluator := rules.NewEvaluator()
	for i, testData := range req.TestData {
		result := models.RuleTestResult{TestCaseIndex: i, Input: testData, Actions: []string{}}
		matched, err := evaluator.Evaluate(cond, testPoint(testData))
		if err != nil {
			result.Error = err.Error()
		} else if matched {
			result.Matched = true
			for _, action := range req.Rule.Actions {
				result.Actions = append(result.Actions, fmt.Sprintf("%s: %v", action.Type, action.Config))
			}
		}
		response.Results = append(response.Results, result)
	}
	return response, nil
}

// GetRuleStats 获取规则统计信息。
// 以前返回 rand 生成的随机数冒充真实统计；规则引擎目前没有逐条规则的执行计数，如实返回不支持。
func (s *ruleService) GetRuleStats(id string) (*models.RuleStatsExtended, error) {
	if _, err := s.manager.GetRule(id); err != nil {
		return nil, err
	}
	return nil, ErrRuleStatsUnavailable
}

// GetRuleExecutionHistory 获取规则执行历史（以前返回随机生成的假数据）
func (s *ruleService) GetRuleExecutionHistory(id string, req *models.RuleHistoryRequest) ([]models.RuleExecution, int, error) {
	if _, err := s.manager.GetRule(id); err != nil {
		return nil, 0, err
	}
	return nil, 0, ErrRuleStatsUnavailable
}

// GetRuleTemplates 获取规则模板
func (s *ruleService) GetRuleTemplates() ([]models.RuleTemplate, error) {
	templates := []models.RuleTemplate{
		{
			ID:          "temp-alert",
			Name:        "温度告警",
			Description: "当温度超过阈值时发送告警",
			Category:    "环境监控",
			Tags:        []string{"温度", "告警", "传感器"},
			Template: map[string]interface{}{
				"name":        "温度告警规则",
				"description": "当温度超过设定阈值时触发告警",
				"conditions": map[string]interface{}{
					"type":     "simple",
					"field":    "temperature",
					"operator": "gt",
					"value":    30.0,
				},
				"actions": []map[string]interface{}{
					{
						"type": "alert",
						"config": map[string]interface{}{
							"message": "温度过高: {{temperature}}°C",
							"level":   "warning",
						},
					},
				},
			},
		},
		{
			ID:          "data-filter",
			Name:        "数据过滤",
			Description: "过滤指定设备的数据",
			Category:    "数据处理",
			Tags:        []string{"过滤", "数据处理"},
			Template: map[string]interface{}{
				"name":        "数据过滤规则",
				"description": "过滤特定设备的数据",
				"conditions": map[string]interface{}{
					"type":     "simple",
					"field":    "device_id",
					"operator": "eq",
					"value":    "sensor-001",
				},
				"actions": []map[string]interface{}{
					{
						"type": "filter",
						"config": map[string]interface{}{
							"action": "pass",
						},
					},
				},
			},
		},
		{
			ID:          "data-transform",
			Name:        "数据转换",
			Description: "转换数据格式或单位",
			Category:    "数据处理",
			Tags:        []string{"转换", "数据处理", "单位"},
			Template: map[string]interface{}{
				"name":        "数据转换规则",
				"description": "将摄氏度转换为华氏度",
				"conditions": map[string]interface{}{
					"type":     "simple",
					"field":    "unit",
					"operator": "eq",
					"value":    "celsius",
				},
				"actions": []map[string]interface{}{
					{
						"type": "transform",
						"config": map[string]interface{}{
							"field":   "temperature",
							"formula": "{{temperature}} * 9/5 + 32",
							"unit":    "fahrenheit",
						},
					},
				},
			},
		},
	}

	return templates, nil
}

// CreateRuleFromTemplate 从模板创建规则
func (s *ruleService) CreateRuleFromTemplate(templateID string, req *models.RuleFromTemplateRequest) (*models.Rule, error) {
	templates, err := s.GetRuleTemplates()
	if err != nil {
		return nil, err
	}

	var template *models.RuleTemplate
	for _, t := range templates {
		if t.ID == templateID {
			template = &t
			break
		}
	}

	if template == nil {
		return nil, fmt.Errorf("模板未找到: %s", templateID)
	}

	// 将模板转换为规则
	templateData, err := json.Marshal(template.Template)
	if err != nil {
		return nil, fmt.Errorf("模板序列化失败: %w", err)
	}

	var rule models.Rule
	if err := json.Unmarshal(templateData, &rule); err != nil {
		return nil, fmt.Errorf("模板反序列化失败: %w", err)
	}

	// 应用用户的自定义参数
	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.Description != "" {
		rule.Description = req.Description
	}
	rule.Enabled = req.Enabled

	// 应用参数替换
	if len(req.Parameters) > 0 {
		ruleData, err := json.Marshal(rule)
		if err != nil {
			return nil, fmt.Errorf("规则序列化失败: %w", err)
		}

		ruleStr := string(ruleData)
		for key, value := range req.Parameters {
			placeholder := fmt.Sprintf("{{%s}}", key)
			ruleStr = strings.ReplaceAll(ruleStr, placeholder, fmt.Sprintf("%v", value))
		}

		if err := json.Unmarshal([]byte(ruleStr), &rule); err != nil {
			return nil, fmt.Errorf("参数替换后反序列化失败: %w", err)
		}
	}

	// 创建规则
	createReq := &models.RuleCreateRequest{
		Name:        rule.Name,
		Description: rule.Description,
		Enabled:     rule.Enabled,
		Priority:    rule.Priority,
		Conditions:  rule.Conditions,
		Actions:     rule.Actions,
	}

	return s.CreateRule(createReq)
}

// convertToManagerCondition converts models.RuleCondition to rules.Condition
func convertToManagerCondition(webCond *models.RuleCondition) (*rules.Condition, error) {
	if webCond == nil {
		return nil, nil
	}

	managerCond := &rules.Condition{
		Type:       webCond.Type,
		Field:      webCond.Field,
		Operator:   webCond.Operator,
		Value:      webCond.Value,
		Expression: webCond.Expression,
		Script:     webCond.Script,
	}

	// Convert And conditions
	if len(webCond.And) > 0 {
		managerCond.And = make([]*rules.Condition, len(webCond.And))
		for i, andCond := range webCond.And {
			converted, err := convertToManagerCondition(&andCond)
			if err != nil {
				return nil, fmt.Errorf("failed to convert and condition %d: %w", i, err)
			}
			managerCond.And[i] = converted
		}
	}

	// Convert Or conditions
	if len(webCond.Or) > 0 {
		managerCond.Or = make([]*rules.Condition, len(webCond.Or))
		for i, orCond := range webCond.Or {
			converted, err := convertToManagerCondition(&orCond)
			if err != nil {
				return nil, fmt.Errorf("failed to convert or condition %d: %w", i, err)
			}
			managerCond.Or[i] = converted
		}
	}

	// Convert Not condition
	if webCond.Not != nil {
		converted, err := convertToManagerCondition(webCond.Not)
		if err != nil {
			return nil, fmt.Errorf("failed to convert not condition: %w", err)
		}
		managerCond.Not = converted
	}

	return managerCond, nil
}

// convertToManagerActions converts []models.RuleAction to []rules.Action
func convertToManagerActions(webActions []models.RuleAction) ([]rules.Action, error) {
	if len(webActions) == 0 {
		return []rules.Action{}, nil
	}

	managerActions := make([]rules.Action, len(webActions))
	for i, webAction := range webActions {
		timeout, err := time.ParseDuration(webAction.Timeout)
		if err != nil && webAction.Timeout != "" {
			return nil, fmt.Errorf("invalid timeout for action %d: %w", i, err)
		}

		managerActions[i] = rules.Action{
			Type:    webAction.Type,
			Config:  webAction.Config,
			Async:   webAction.Async,
			Timeout: timeout,
			Retry:   webAction.Retry,
		}
	}

	return managerActions, nil
}
