package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// AlertSubscriber NATS告警订阅器
type AlertSubscriber struct {
	natsConn    *nats.Conn
	alertStore  *AlertStore
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	running     bool
	subs        []*nats.Subscription
}

// AlertStore 内存告警存储
type AlertStore struct {
	alerts map[string]*models.Alert
	mu     sync.RWMutex
}

// NewAlertStore 创建告警存储
func NewAlertStore() *AlertStore {
	return &AlertStore{
		alerts: make(map[string]*models.Alert),
	}
}

// AddAlert 添加告警
func (s *AlertStore) AddAlert(alert *models.Alert) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts[alert.ID] = alert
}

// GetAlert 获取告警
func (s *AlertStore) GetAlert(id string) (*models.Alert, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alert, exists := s.alerts[id]
	return alert, exists
}

// GetAlerts 获取所有告警
func (s *AlertStore) GetAlerts() []*models.Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	alerts := make([]*models.Alert, 0, len(s.alerts))
	for _, alert := range s.alerts {
		alerts = append(alerts, alert)
	}
	return alerts
}

// UpdateAlert 更新告警
func (s *AlertStore) UpdateAlert(alert *models.Alert) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts[alert.ID] = alert
}

// DeleteAlert 删除告警
func (s *AlertStore) DeleteAlert(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.alerts, id)
}

// GetAlertCount 获取告警数量
func (s *AlertStore) GetAlertCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.alerts)
}

// NewAlertSubscriber 创建告警订阅器
func NewAlertSubscriber(natsConn *nats.Conn) *AlertSubscriber {
	ctx, cancel := context.WithCancel(context.Background())
	return &AlertSubscriber{
		natsConn:   natsConn,
		alertStore: NewAlertStore(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start 启动订阅器
func (s *AlertSubscriber) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.running {
		return fmt.Errorf("alert subscriber is already running")
	}
	
	// 订阅规则引擎告警
	if err := s.subscribeToAlerts(); err != nil {
		return fmt.Errorf("failed to subscribe to alerts: %w", err)
	}
	
	s.running = true
	log.Info().Msg("Alert subscriber started")
	return nil
}

// Stop 停止订阅器
func (s *AlertSubscriber) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return nil
	}
	
	// 取消订阅
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			log.Error().Err(err).Msg("Failed to unsubscribe from NATS")
		}
	}
	s.subs = nil
	
	s.cancel()
	s.running = false
	log.Info().Msg("Alert subscriber stopped")
	return nil
}

// subscribeToAlerts 订阅告警消息
func (s *AlertSubscriber) subscribeToAlerts() error {
	// 禁用AlertSubscriber的告警创建，因为AlertIntegrationService已经处理了
	// 订阅所有级别的告警
	subjects := []string{
		// 暂时禁用，让AlertIntegrationService处理告警创建
		// "iot.alerts.triggered",
		// "iot.alerts.triggered.info",
		// "iot.alerts.triggered.warning",
		// "iot.alerts.triggered.error",
		// "iot.alerts.triggered.critical",
	}
	
	for _, subject := range subjects {
		sub, err := s.natsConn.Subscribe(subject, s.handleAlertMessage)
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
		log.Debug().Str("subject", subject).Msg("Subscribed to alert subject")
	}
	
	return nil
}

// handleAlertMessage 处理告警消息
func (s *AlertSubscriber) handleAlertMessage(msg *nats.Msg) {
	// 解析告警消息
	var alertData map[string]interface{}
	if err := json.Unmarshal(msg.Data, &alertData); err != nil {
		log.Error().Err(err).Str("subject", msg.Subject).Msg("Failed to parse alert message")
		return
	}
	
	// 转换为web模型
	alert := s.convertToWebAlert(alertData)
	if alert == nil {
		log.Error().Str("subject", msg.Subject).Msg("Failed to convert alert to web model")
		return
	}
	
	// 存储告警
	s.alertStore.AddAlert(alert)
	
	log.Info().
		Str("alert_id", alert.ID).
		Str("rule_id", alert.RuleID).
		Str("level", alert.Level).
		Str("device_id", alert.DeviceID).
		Str("key", alert.Key).
		Msg("Rule-based alert received and stored")
}

// convertToWebAlert 转换为Web告警模型
func (s *AlertSubscriber) convertToWebAlert(data map[string]interface{}) *models.Alert {
	alert := &models.Alert{
		Status:    "active",
		Source:    "rule_engine",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Data:      make(map[string]interface{}),
	}
	
	// 基本字段
	if id, ok := data["id"].(string); ok {
		alert.ID = id
	}
	if ruleID, ok := data["rule_id"].(string); ok {
		alert.RuleID = ruleID
	}
	if ruleName, ok := data["rule_name"].(string); ok {
		alert.RuleName = ruleName
		// 使用规则引擎提供的消息作为标题，而不是固定格式
		if message, ok := data["message"].(string); ok && message != "" {
			alert.Title = message
		} else {
			alert.Title = fmt.Sprintf("规则告警: %s", ruleName)
		}
	}
	if level, ok := data["level"].(string); ok {
		alert.Level = level
	}
	if message, ok := data["message"].(string); ok {
		alert.Description = message
	}
	if deviceID, ok := data["device_id"].(string); ok {
		alert.DeviceID = deviceID
	}
	if key, ok := data["key"].(string); ok {
		alert.Key = key
	}
	if value := data["value"]; value != nil {
		alert.Value = value
	}
	
	// 时间戳
	if timestampStr, ok := data["timestamp"].(string); ok {
		if timestamp, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			alert.CreatedAt = timestamp
			alert.UpdatedAt = timestamp
		}
	}
	
	// 标签
	if tags, ok := data["tags"].(map[string]interface{}); ok {
		alert.Tags = make(map[string]string)
		for k, v := range tags {
			if str, ok := v.(string); ok {
				alert.Tags[k] = str
			}
		}
	}
	
	// 通知渠道
	if channels, ok := data["notification_channels"].([]interface{}); ok {
		channelList := make([]string, 0, len(channels))
		for _, ch := range channels {
			if str, ok := ch.(string); ok {
				channelList = append(channelList, str)
			}
		}
		alert.NotificationChannels = channelList
	}
	
	// 优先级
	if priority, ok := data["priority"].(float64); ok {
		alert.Priority = int(priority)
	}
	
	// 自动解决
	if autoResolve, ok := data["auto_resolve"].(bool); ok {
		alert.AutoResolve = autoResolve
	}
	
	// 存储原始数据
	alert.Data["original_rule_data"] = data
	alert.Data["source_type"] = "rule_engine"
	
	return alert
}

// GetAlertStore 获取告警存储
func (s *AlertSubscriber) GetAlertStore() *AlertStore {
	return s.alertStore
}

// IsRunning 检查订阅器是否运行
func (s *AlertSubscriber) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetSubscriptionCount 获取订阅数量
func (s *AlertSubscriber) GetSubscriptionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subs)
}