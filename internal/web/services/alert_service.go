package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// AlertService 告警服务接口
type AlertService interface {
	GetAlerts(req *models.AlertListRequest) ([]models.Alert, int, error)
	GetAlert(id string) (*models.Alert, error)
	CreateAlert(alert *models.AlertCreateRequest) (*models.Alert, error)
	UpdateAlert(id string, req *models.AlertUpdateRequest) (*models.Alert, error)
	DeleteAlert(id string) error
	AcknowledgeAlert(id string, userID string, comment string) error
	ResolveAlert(id string, userID string, comment string) error
	GetAlertStats() (*models.AlertStats, error)
	GetAlertRules() ([]models.AlertRule, error)
	CreateAlertRule(rule *models.AlertRuleCreateRequest) (*models.AlertRule, error)
	UpdateAlertRule(id string, rule *models.AlertRuleUpdateRequest) (*models.AlertRule, error)
	DeleteAlertRule(id string) error
	TestAlertRule(id string, data map[string]interface{}) (*models.AlertRuleTestResponse, error)
	GetNotificationChannels() ([]models.NotificationChannel, error)
	CreateNotificationChannel(channel *models.NotificationChannelCreateRequest) (*models.NotificationChannel, error)
	UpdateNotificationChannel(id string, channel *models.NotificationChannelUpdateRequest) (*models.NotificationChannel, error)
	DeleteNotificationChannel(id string) error
	TestNotificationChannel(id string) error
}

// alertService 告警服务实现
type alertService struct {
	alerts               map[string]*models.Alert
	alertRules           map[string]*models.AlertRule
	notificationChannels map[string]*models.NotificationChannel
	mu                   sync.RWMutex  // 添加读写锁保护
	alertSubscriber      *AlertSubscriber  // NATS告警订阅器
	idCounter            int64            // ID计数器，确保唯一性
}

// NewAlertService 创建告警服务
func NewAlertService() AlertService {
	service := &alertService{
		alerts:               make(map[string]*models.Alert),
		alertRules:           make(map[string]*models.AlertRule),
		notificationChannels: make(map[string]*models.NotificationChannel),
	}

	// 初始化一些默认数据
	service.initializeDefaultData()

	return service
}

// NewAlertServiceWithNATS 创建带NATS连接的告警服务
func NewAlertServiceWithNATS(natsConn *nats.Conn) AlertService {
	service := &alertService{
		alerts:               make(map[string]*models.Alert),
		alertRules:           make(map[string]*models.AlertRule),
		notificationChannels: make(map[string]*models.NotificationChannel),
		alertSubscriber:      NewAlertSubscriber(natsConn),
	}

	// 初始化一些默认数据
	service.initializeDefaultData()

	// 启动NATS订阅器
	if err := service.alertSubscriber.Start(); err != nil {
		log.Error().Err(err).Msg("Failed to start alert subscriber")
	}

	return service
}

// initializeDefaultData 初始化默认数据
func (s *alertService) initializeDefaultData() {
	// 创建少量示例告警（演示用途）
	alerts := []*models.Alert{
		{
			ID:          "demo-alert-001",
			Title:       "系统启动完成",
			Description: "IoT网关系统已成功启动，等待真实告警数据...",
			Level:       "info",
			Status:      "resolved",
			Source:      "system",
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			UpdatedAt:   time.Now().Add(-1 * time.Hour),
			ResolvedAt:  &[]time.Time{time.Now().Add(-1 * time.Hour)}[0],
			ResolvedBy:  "system",
			Data: map[string]interface{}{
				"startup_time": time.Now().Add(-1 * time.Hour),
				"version":      "1.0.0",
				"demo":         true,
			},
		},
	}

	for _, alert := range alerts {
		s.alerts[alert.ID] = alert
	}

	// 创建一些示例告警规则
	alertRules := []*models.AlertRule{
		{
			ID:          "rule-001",
			Name:        "CPU使用率监控",
			Description: "当CPU使用率超过阈值时触发告警",
			Enabled:     true,
			Level:       "warning",
			Condition: &models.AlertCondition{
				Type:     "threshold",
				Field:    "cpu_usage",
				Operator: "gt",
				Value:    80.0,
			},
			NotificationChannels: []string{"channel-001", "channel-002"},
			CreatedAt:           time.Now().Add(-24 * time.Hour),
			UpdatedAt:           time.Now().Add(-24 * time.Hour),
		},
		{
			ID:          "rule-002",
			Name:        "设备离线检测",
			Description: "当设备超过5分钟未上报数据时触发告警",
			Enabled:     true,
			Level:       "critical",
			Condition: &models.AlertCondition{
				Type:     "absence",
				Field:    "last_seen",
				Operator: "older_than",
				Value:    "5m",
			},
			NotificationChannels: []string{"channel-001"},
			CreatedAt:           time.Now().Add(-24 * time.Hour),
			UpdatedAt:           time.Now().Add(-24 * time.Hour),
		},
	}

	for _, rule := range alertRules {
		s.alertRules[rule.ID] = rule
	}

	// 创建一些示例通知渠道
	channels := []*models.NotificationChannel{
		{
			ID:          "channel-001",
			Name:        "邮件通知",
			Type:        "email",
			Enabled:     true,
			Config: map[string]interface{}{
				"recipients": []string{"admin@example.com", "ops@example.com"},
				"smtp_host":  "smtp.example.com",
				"smtp_port":  587,
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			ID:          "channel-002",
			Name:        "Webhook通知",
			Type:        "webhook",
			Enabled:     true,
			Config: map[string]interface{}{
				"url":    "https://hooks.example.com/alerts",
				"method": "POST",
				"headers": map[string]string{
					"Authorization": "Bearer token123",
				},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-24 * time.Hour),
		},
	}

	for _, channel := range channels {
		s.notificationChannels[channel.ID] = channel
	}
}

// GetAlerts 获取告警列表
func (s *alertService) GetAlerts(req *models.AlertListRequest) ([]models.Alert, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	alerts := make([]models.Alert, 0)
	
	// 合并本地告警和规则引擎告警
	allAlerts := make(map[string]*models.Alert)
	
	// 添加本地告警
	for id, alert := range s.alerts {
		allAlerts[id] = alert
	}
	
	// 添加规则引擎告警
	if s.alertSubscriber != nil {
		ruleAlerts := s.alertSubscriber.GetAlertStore().GetAlerts()
		for _, alert := range ruleAlerts {
			allAlerts[alert.ID] = alert
		}
	}
	
	// 应用过滤器
	for _, alert := range allAlerts {
		if req.Level != "" && alert.Level != req.Level {
			continue
		}
		
		if req.Status != "" && alert.Status != req.Status {
			continue
		}
		
		if req.Source != "" && alert.Source != req.Source {
			continue
		}
		
		if req.Search != "" {
			searchTerm := strings.ToLower(req.Search)
			if !strings.Contains(strings.ToLower(alert.Title), searchTerm) &&
				!strings.Contains(strings.ToLower(alert.Description), searchTerm) {
				continue
			}
		}
		
		// 时间范围过滤
		if !req.StartTime.IsZero() && alert.CreatedAt.Before(req.StartTime) {
			continue
		}
		
		if !req.EndTime.IsZero() && alert.CreatedAt.After(req.EndTime) {
			continue
		}
		
		alerts = append(alerts, *alert)
	}
	
	// 排序
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].CreatedAt.After(alerts[j].CreatedAt)
	})
	
	// 分页
	total := len(alerts)
	start := (req.Page - 1) * req.PageSize
	end := start + req.PageSize
	
	if start >= total {
		return []models.Alert{}, total, nil
	}
	if end > total {
		end = total
	}
	
	return alerts[start:end], total, nil
}

// GetAlert 获取单个告警
func (s *alertService) GetAlert(id string) (*models.Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 先从本地告警查找
	if alert, exists := s.alerts[id]; exists {
		return alert, nil
	}
	
	// 再从规则引擎告警查找
	if s.alertSubscriber != nil {
		if alert, exists := s.alertSubscriber.GetAlertStore().GetAlert(id); exists {
			return alert, nil
		}
	}
	
	return nil, fmt.Errorf("告警未找到: %s", id)
}

// CreateAlert 创建告警
func (s *alertService) CreateAlert(req *models.AlertCreateRequest) (*models.Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 使用计数器确保ID唯一性，避免纳秒时间戳冲突
	s.idCounter++
	alert := &models.Alert{
		ID:          fmt.Sprintf("alert-%d-%d", time.Now().UnixNano(), s.idCounter),
		Title:       req.Title,
		Description: req.Description,
		Level:       req.Level,
		Status:      "active",
		Source:      req.Source,
		Data:        req.Data,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	s.alerts[alert.ID] = alert
	log.Info().
		Str("alert_id", alert.ID).
		Str("level", alert.Level).
		Str("source", alert.Source).
		Str("title", alert.Title).
		Msg("Alert created successfully")
	
	return alert, nil
}

// UpdateAlert 更新告警
func (s *alertService) UpdateAlert(id string, req *models.AlertUpdateRequest) (*models.Alert, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	alert, exists := s.alerts[id]
	if !exists {
		return nil, fmt.Errorf("告警未找到: %s", id)
	}
	
	if req.Title != "" {
		alert.Title = req.Title
	}
	if req.Description != "" {
		alert.Description = req.Description
	}
	if req.Level != "" {
		alert.Level = req.Level
	}
	if req.Status != "" {
		alert.Status = req.Status
	}
	if req.Data != nil {
		alert.Data = req.Data
	}
	
	alert.UpdatedAt = time.Now()
	return alert, nil
}

// DeleteAlert 删除告警
func (s *alertService) DeleteAlert(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, exists := s.alerts[id]; !exists {
		return fmt.Errorf("告警未找到: %s", id)
	}
	delete(s.alerts, id)
	return nil
}

// AcknowledgeAlert 确认告警
func (s *alertService) AcknowledgeAlert(id string, userID string, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 先检查本地告警
	if alert, exists := s.alerts[id]; exists {
		now := time.Now()
		alert.Status = "acknowledged"
		alert.AcknowledgedAt = &now
		alert.AcknowledgedBy = userID
		alert.AcknowledgedComment = comment
		alert.UpdatedAt = now
		return nil
	}
	
	// 检查规则引擎告警
	if s.alertSubscriber != nil {
		if alert, exists := s.alertSubscriber.GetAlertStore().GetAlert(id); exists {
			now := time.Now()
			alert.Status = "acknowledged"
			alert.AcknowledgedAt = &now
			alert.AcknowledgedBy = userID
			alert.AcknowledgedComment = comment
			alert.UpdatedAt = now
			s.alertSubscriber.GetAlertStore().UpdateAlert(alert)
			return nil
		}
	}
	
	return fmt.Errorf("告警未找到: %s", id)
}

// ResolveAlert 解决告警
func (s *alertService) ResolveAlert(id string, userID string, comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 先检查本地告警
	if alert, exists := s.alerts[id]; exists {
		now := time.Now()
		alert.Status = "resolved"
		alert.ResolvedAt = &now
		alert.ResolvedBy = userID
		alert.ResolvedComment = comment
		alert.UpdatedAt = now
		return nil
	}
	
	// 检查规则引擎告警
	if s.alertSubscriber != nil {
		if alert, exists := s.alertSubscriber.GetAlertStore().GetAlert(id); exists {
			now := time.Now()
			alert.Status = "resolved"
			alert.ResolvedAt = &now
			alert.ResolvedBy = userID
			alert.ResolvedComment = comment
			alert.UpdatedAt = now
			s.alertSubscriber.GetAlertStore().UpdateAlert(alert)
			return nil
		}
	}
	
	return fmt.Errorf("告警未找到: %s", id)
}

// GetAlertStats 获取告警统计（修复版本，提高准确性）
func (s *alertService) GetAlertStats() (*models.AlertStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := &models.AlertStats{
		Total:        0,
		Active:       0,
		Acknowledged: 0,
		Resolved:     0,
		ByLevel: map[string]int{
			"debug":    0,
			"info":     0,
			"warning":  0,
			"error":    0,
			"critical": 0,
		},
		BySource: make(map[string]int),
		RecentTrends: []models.AlertTrend{},
	}
	
	// 合并本地告警和规则引擎告警
	allAlerts := make(map[string]*models.Alert)
	
	// 添加本地告警（带调试日志）
	localCount := 0
	for id, alert := range s.alerts {
		allAlerts[id] = alert
		localCount++
	}
	
	// 添加规则引擎告警（带调试日志和错误处理）
	ruleEngineCount := 0
	if s.alertSubscriber != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Msg("Panic occurred while getting rule engine alerts")
				}
			}()
			
			ruleAlerts := s.alertSubscriber.GetAlertStore().GetAlerts()
			for _, alert := range ruleAlerts {
				// 避免重复ID的告警被覆盖
				if _, exists := allAlerts[alert.ID]; !exists {
					allAlerts[alert.ID] = alert
					ruleEngineCount++
				} else {
					log.Warn().
						Str("alert_id", alert.ID).
						Msg("Duplicate alert ID found, skipping rule engine alert")
				}
			}
		}()
	} else {
		log.Debug().Msg("Alert subscriber is nil, rule engine alerts not included in stats")
	}
	
	log.Debug().
		Int("local_alerts", localCount).
		Int("rule_engine_alerts", ruleEngineCount).
		Int("total_unique_alerts", len(allAlerts)).
		Msg("Alert stats data sources")
	
	// 统计总数和状态
	for id, alert := range allAlerts {
		stats.Total++
		
		// 状态统计
		switch alert.Status {
		case "active":
			stats.Active++
		case "acknowledged":
			stats.Acknowledged++
		case "resolved":
			stats.Resolved++
		default:
			log.Warn().
				Str("alert_id", id).
				Str("unknown_status", alert.Status).
				Msg("Unknown alert status encountered")
			stats.Active++ // 默认作为活跃告警处理
		}
		
		// 按级别统计（改进处理，支持所有级别）
		if _, exists := stats.ByLevel[alert.Level]; exists {
			stats.ByLevel[alert.Level]++
		} else {
			// 处理未预定义的级别
			stats.ByLevel[alert.Level] = 1
			log.Debug().
				Str("alert_id", id).
				Str("new_level", alert.Level).
				Msg("New alert level encountered")
		}
		
		// 按来源统计
		stats.BySource[alert.Source]++
		
		// 调试日志：记录每个告警的详细信息
		log.Debug().
			Str("alert_id", id).
			Str("level", alert.Level).
			Str("status", alert.Status).
			Str("source", alert.Source).
			Str("title", alert.Title).
			Msg("Alert included in statistics")
	}
	
	// 模拟最近趋势数据（最近7天）
	for i := 6; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i)
		stats.RecentTrends = append(stats.RecentTrends, models.AlertTrend{
			Date:  date,
			Count: 5 + i*2, // 模拟数据
		})
	}
	
	// 记录最终统计结果
	log.Info().
		Int("total", stats.Total).
		Int("active", stats.Active).
		Int("acknowledged", stats.Acknowledged).
		Int("resolved", stats.Resolved).
		Interface("by_level", stats.ByLevel).
		Interface("by_source", stats.BySource).
		Msg("Alert statistics calculated")
	
	return stats, nil
}

// GetAlertRules 获取告警规则列表
func (s *alertService) GetAlertRules() ([]models.AlertRule, error) {
	rules := make([]models.AlertRule, 0, len(s.alertRules))
	for _, rule := range s.alertRules {
		rules = append(rules, *rule)
	}
	
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].CreatedAt.After(rules[j].CreatedAt)
	})
	
	return rules, nil
}

// CreateAlertRule 创建告警规则
func (s *alertService) CreateAlertRule(req *models.AlertRuleCreateRequest) (*models.AlertRule, error) {
	rule := &models.AlertRule{
		ID:                   fmt.Sprintf("rule-%d", time.Now().UnixNano()),
		Name:                 req.Name,
		Description:          req.Description,
		Enabled:              req.Enabled,
		Level:                req.Level,
		Condition:            req.Condition,
		NotificationChannels: req.NotificationChannels,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	
	s.alertRules[rule.ID] = rule
	return rule, nil
}

// UpdateAlertRule 更新告警规则
func (s *alertService) UpdateAlertRule(id string, req *models.AlertRuleUpdateRequest) (*models.AlertRule, error) {
	rule, exists := s.alertRules[id]
	if !exists {
		return nil, fmt.Errorf("告警规则未找到: %s", id)
	}
	
	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.Description != "" {
		rule.Description = req.Description
	}
	rule.Enabled = req.Enabled
	if req.Level != "" {
		rule.Level = req.Level
	}
	if req.Condition != nil {
		rule.Condition = req.Condition
	}
	if req.NotificationChannels != nil {
		rule.NotificationChannels = req.NotificationChannels
	}
	
	rule.UpdatedAt = time.Now()
	return rule, nil
}

// DeleteAlertRule 删除告警规则
func (s *alertService) DeleteAlertRule(id string) error {
	if _, exists := s.alertRules[id]; !exists {
		return fmt.Errorf("告警规则未找到: %s", id)
	}
	delete(s.alertRules, id)
	return nil
}

// TestAlertRule 测试告警规则
func (s *alertService) TestAlertRule(id string, data map[string]interface{}) (*models.AlertRuleTestResponse, error) {
	rule, exists := s.alertRules[id]
	if !exists {
		return nil, fmt.Errorf("告警规则未找到: %s", id)
	}
	
	response := &models.AlertRuleTestResponse{
		RuleID:    id,
		Triggered: false,
		Message:   "",
		TestedAt:  time.Now(),
	}
	
	// 简化的规则测试逻辑
	condition := rule.Condition
	if fieldValue, exists := data[condition.Field]; exists {
		switch condition.Operator {
		case "gt":
			if fv, ok := fieldValue.(float64); ok {
				if cv, ok := condition.Value.(float64); ok {
					if fv > cv {
						response.Triggered = true
						response.Message = fmt.Sprintf("字段 %s 的值 %.2f 大于阈值 %.2f", condition.Field, fv, cv)
					}
				}
			}
		case "lt":
			if fv, ok := fieldValue.(float64); ok {
				if cv, ok := condition.Value.(float64); ok {
					if fv < cv {
						response.Triggered = true
						response.Message = fmt.Sprintf("字段 %s 的值 %.2f 小于阈值 %.2f", condition.Field, fv, cv)
					}
				}
			}
		case "eq":
			if fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", condition.Value) {
				response.Triggered = true
				response.Message = fmt.Sprintf("字段 %s 的值 %v 等于 %v", condition.Field, fieldValue, condition.Value)
			}
		}
	}
	
	if !response.Triggered {
		response.Message = "规则条件未满足，不会触发告警"
	}
	
	return response, nil
}

// GetNotificationChannels 获取通知渠道列表
func (s *alertService) GetNotificationChannels() ([]models.NotificationChannel, error) {
	channels := make([]models.NotificationChannel, 0, len(s.notificationChannels))
	for _, channel := range s.notificationChannels {
		channels = append(channels, *channel)
	}
	
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].CreatedAt.After(channels[j].CreatedAt)
	})
	
	return channels, nil
}

// CreateNotificationChannel 创建通知渠道
func (s *alertService) CreateNotificationChannel(req *models.NotificationChannelCreateRequest) (*models.NotificationChannel, error) {
	channel := &models.NotificationChannel{
		ID:        fmt.Sprintf("channel-%d", time.Now().UnixNano()),
		Name:      req.Name,
		Type:      req.Type,
		Enabled:   req.Enabled,
		Config:    req.Config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	s.notificationChannels[channel.ID] = channel
	return channel, nil
}

// UpdateNotificationChannel 更新通知渠道
func (s *alertService) UpdateNotificationChannel(id string, req *models.NotificationChannelUpdateRequest) (*models.NotificationChannel, error) {
	channel, exists := s.notificationChannels[id]
	if !exists {
		return nil, fmt.Errorf("通知渠道未找到: %s", id)
	}
	
	if req.Name != "" {
		channel.Name = req.Name
	}
	if req.Type != "" {
		channel.Type = req.Type
	}
	channel.Enabled = req.Enabled
	if req.Config != nil {
		channel.Config = req.Config
	}
	
	channel.UpdatedAt = time.Now()
	return channel, nil
}

// DeleteNotificationChannel 删除通知渠道
func (s *alertService) DeleteNotificationChannel(id string) error {
	if _, exists := s.notificationChannels[id]; !exists {
		return fmt.Errorf("通知渠道未找到: %s", id)
	}
	delete(s.notificationChannels, id)
	return nil
}

// TestNotificationChannel 测试通知渠道
func (s *alertService) TestNotificationChannel(id string) error {
	channel, exists := s.notificationChannels[id]
	if !exists {
		return fmt.Errorf("通知渠道未找到: %s", id)
	}
	
	if !channel.Enabled {
		return fmt.Errorf("通知渠道已禁用")
	}
	
	// 这里应该实际发送测试通知
	// 当前只是模拟成功
	switch channel.Type {
	case "email":
		// 模拟邮件发送
		fmt.Printf("发送测试邮件到 %v\n", channel.Config["recipients"])
	case "webhook":
		// 模拟webhook调用
		fmt.Printf("发送测试webhook到 %v\n", channel.Config["url"])
	case "sms":
		// 模拟短信发送
		fmt.Printf("发送测试短信到 %v\n", channel.Config["phone_numbers"])
	default:
		return fmt.Errorf("不支持的通知类型: %s", channel.Type)
	}
	
	return nil
}