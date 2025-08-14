package services

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
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
		ID:          managerRule.ID,    // String ID from rule manager
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
		ID:          fmt.Sprintf("rule-%d", time.Now().UnixNano()), // Generate a unique ID
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

// ValidateRule 验证规则
func (s *ruleService) ValidateRule(rule *models.Rule) (*models.RuleValidationResponseExtended, error) {
	response := &models.RuleValidationResponseExtended{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}

	// 基本字段验证
	if rule.Name == "" {
		response.Valid = false
		response.Errors = append(response.Errors, "规则名称不能为空")
	}

	if rule.Priority < 0 || rule.Priority > 100 {
		response.Valid = false
		response.Errors = append(response.Errors, "规则优先级必须在0-100之间")
	}

	// 验证条件
	if rule.Conditions == nil {
		response.Valid = false
		response.Errors = append(response.Errors, "规则条件不能为空")
	} else {
		if err := s.validateCondition(rule.Conditions); err != nil {
			response.Valid = false
			response.Errors = append(response.Errors, "条件验证失败: "+err.Error())
		}
	}

	// 验证动作
	if len(rule.Actions) == 0 {
		response.Valid = false
		response.Errors = append(response.Errors, "规则必须至少包含一个动作")
	} else {
		for i, action := range rule.Actions {
			if err := s.validateAction(&action); err != nil {
				response.Valid = false
				response.Errors = append(response.Errors, fmt.Sprintf("动作 %d 验证失败: %s", i+1, err.Error()))
			}
		}
	}

	// 添加一些警告
	if rule.Priority > 90 {
		response.Warnings = append(response.Warnings, "高优先级规则可能影响系统性能")
	}

	return response, nil
}

// validateCondition 验证条件
func (s *ruleService) validateCondition(condition *models.RuleCondition) error {
	if condition.Type == "" {
		return fmt.Errorf("条件类型不能为空")
	}

	switch condition.Type {
	case "simple":
		if condition.Field == "" {
			return fmt.Errorf("简单条件必须指定字段")
		}
		if condition.Operator == "" {
			return fmt.Errorf("简单条件必须指定操作符")
		}
		if condition.Value == nil {
			return fmt.Errorf("简单条件必须指定值")
		}
	case "and", "or":
		if len(condition.And) < 2 && len(condition.Or) < 2 {
			return fmt.Errorf("复合条件至少需要2个子条件")
		}
		subConditions := condition.And
		if condition.Type == "or" {
			subConditions = condition.Or
		}
		for i, subCondition := range subConditions {
			if err := s.validateCondition(&subCondition); err != nil {
				return fmt.Errorf("子条件 %d: %s", i+1, err.Error())
			}
		}
	case "expression":
		if condition.Expression == "" {
			return fmt.Errorf("表达式条件必须指定表达式")
		}
		// 这里可以添加表达式语法验证
	default:
		return fmt.Errorf("不支持的条件类型: %s", condition.Type)
	}

	return nil
}

// validateAction 验证动作
func (s *ruleService) validateAction(action *models.RuleAction) error {
	if action.Type == "" {
		return fmt.Errorf("动作类型不能为空")
	}

	switch action.Type {
	case "alert":
		if action.Config == nil {
			return fmt.Errorf("告警动作必须指定配置")
		}
		// 验证告警配置
		if message, ok := action.Config["message"]; !ok || message == "" {
			return fmt.Errorf("告警动作必须指定消息")
		}
	case "transform":
		if action.Config == nil {
			return fmt.Errorf("转换动作必须指定配置")
		}
	case "filter", "aggregate", "forward":
		if action.Config == nil {
			return fmt.Errorf("%s动作必须指定配置", action.Type)
		}
	default:
		return fmt.Errorf("不支持的动作类型: %s", action.Type)
	}

	return nil
}

// TestRule 测试规则
func (s *ruleService) TestRule(req *models.RuleTestRequestExtended) (*models.RuleTestResponseExtended, error) {
	response := &models.RuleTestResponseExtended{
		Success:    true,
		Results:    []models.RuleTestResult{},
		Errors:     []string{},
		ExecutedAt: time.Now(),
	}

	// 首先验证规则
	validation, err := s.ValidateRule(&req.Rule)
	if err != nil {
		response.Success = false
		response.Errors = append(response.Errors, "规则验证失败: "+err.Error())
		return response, nil
	}

	if !validation.Valid {
		response.Success = false
		response.Errors = append(response.Errors, validation.Errors...)
		return response, nil
	}

	// 模拟测试每个测试数据
	for i, testData := range req.TestData {
		result := models.RuleTestResult{
			TestCaseIndex: i,
			Input:         testData,
			Matched:       false,
			Actions:       []string{},
			Error:         "",
		}

		// 简化的条件匹配逻辑（实际应该使用规则引擎）
		matched, err := s.evaluateCondition(req.Rule.Conditions, testData)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Matched = matched
			if matched {
				// 如果匹配，记录会执行的动作
				for _, action := range req.Rule.Actions {
					result.Actions = append(result.Actions, fmt.Sprintf("%s: %v", action.Type, action.Config))
				}
			}
		}

		response.Results = append(response.Results, result)
	}

	return response, nil
}

// evaluateCondition 简化的条件评估
func (s *ruleService) evaluateCondition(condition *models.RuleCondition, data map[string]interface{}) (bool, error) {
	switch condition.Type {
	case "simple":
		fieldValue, exists := data[condition.Field]
		if !exists {
			return false, nil
		}

		switch condition.Operator {
		case "eq", "equals":
			return fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", condition.Value), nil
		case "gt":
			if fv, ok := fieldValue.(float64); ok {
				if cv, ok := condition.Value.(float64); ok {
					return fv > cv, nil
				}
			}
			return false, fmt.Errorf("gt操作符需要数值类型")
		case "lt":
			if fv, ok := fieldValue.(float64); ok {
				if cv, ok := condition.Value.(float64); ok {
					return fv < cv, nil
				}
			}
			return false, fmt.Errorf("lt操作符需要数值类型")
		case "contains":
			return strings.Contains(fmt.Sprintf("%v", fieldValue), fmt.Sprintf("%v", condition.Value)), nil
		}
	case "and":
		for _, subCondition := range condition.And {
			matched, err := s.evaluateCondition(&subCondition, data)
			if err != nil {
				return false, err
			}
			if !matched {
				return false, nil
			}
		}
		return true, nil
	case "or":
		for _, subCondition := range condition.Or {
			matched, err := s.evaluateCondition(&subCondition, data)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}

	return false, fmt.Errorf("不支持的条件类型: %s", condition.Type)
}

// GetRuleStats 获取规则统计信息
func (s *ruleService) GetRuleStats(id string) (*models.RuleStatsExtended, error) {
	rule, err := s.manager.GetRule(id)
	if err != nil {
		return nil, err
	}

	// 模拟统计数据（实际应该从规则引擎获取）
	stats := &models.RuleStatsExtended{
		RuleID:              id,
		RuleName:            rule.Name,
		ExecutionCount:      rand.Intn(1000) + 100,
		SuccessCount:        rand.Intn(900) + 90,
		ErrorCount:          rand.Intn(10),
		AverageExecutionTime: time.Duration(rand.Intn(100)+10) * time.Millisecond,
		LastExecuted:        time.Now().Add(-time.Duration(rand.Intn(60)) * time.Minute),
		Status:              "active",
		TriggerRate:         float64(rand.Intn(50)+10) / 100.0, // 10-60%
		ActionsTriggered: map[string]int{
			"alert":     rand.Intn(50),
			"transform": rand.Intn(30),
			"forward":   rand.Intn(20),
		},
	}

	if !rule.Enabled {
		stats.Status = "disabled"
		stats.TriggerRate = 0
	}

	return stats, nil
}

// GetRuleExecutionHistory 获取规则执行历史
func (s *ruleService) GetRuleExecutionHistory(id string, req *models.RuleHistoryRequest) ([]models.RuleExecution, int, error) {
	// 模拟执行历史数据
	executions := make([]models.RuleExecution, 0)
	
	// 生成一些模拟数据
	for i := 0; i < 50; i++ {
		execution := models.RuleExecution{
			ID:          fmt.Sprintf("exec-%d", i),
			RuleID:      id,
			ExecutedAt:  time.Now().Add(-time.Duration(i*10) * time.Minute),
			Success:     rand.Float32() > 0.1, // 90% 成功率
			Duration:    time.Duration(rand.Intn(100)+10) * time.Millisecond,
			InputData:   map[string]interface{}{"device_id": "sensor-001", "temperature": rand.Float64()*40 + 10},
			OutputData:  map[string]interface{}{"processed": true},
			Error:       "",
		}

		if !execution.Success {
			execution.Error = "处理失败: 连接超时"
		}

		// 应用时间过滤
		if !req.StartTime.IsZero() && execution.ExecutedAt.Before(req.StartTime) {
			continue
		}
		if !req.EndTime.IsZero() && execution.ExecutedAt.After(req.EndTime) {
			continue
		}

		// 应用状态过滤
		if req.Status != "" {
			if (req.Status == "success" && !execution.Success) ||
				(req.Status == "error" && execution.Success) {
				continue
			}
		}

		executions = append(executions, execution)
	}

	// 排序
	sort.Slice(executions, func(i, j int) bool {
		return executions[i].ExecutedAt.After(executions[j].ExecutedAt)
	})

	// 分页
	total := len(executions)
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize

	if start >= total {
		return []models.RuleExecution{}, total, nil
	}
	if end > total {
		end = total
	}

	return executions[start:end], total, nil
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
