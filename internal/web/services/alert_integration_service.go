package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// AlertIntegrationService 告警集成服务 - 连接规则引擎和告警管理
type AlertIntegrationService struct {
	alertService         AlertService
	notificationService  NotificationService
	natsConn            *nats.Conn
	ruleManager         rules.RuleManager
	
	// 内部状态管理
	activeAlerts        map[string]*models.Alert  // 活跃告警映射
	alertThrottleMap    map[string]time.Time      // 告警节流映射
	autoResolveTimers   map[string]*time.Timer    // 自动解决定时器
	mu                  sync.RWMutex
	
	// 订阅管理
	subscriptions       []*nats.Subscription
	ctx                 context.Context
	cancel              context.CancelFunc
}

// RuleEngineAlert 来自规则引擎的告警消息
type RuleEngineAlert struct {
	ID                  string            `json:"id"`
	RuleID              string            `json:"rule_id"`
	RuleName            string            `json:"rule_name"`
	Level               string            `json:"level"`
	Message             string            `json:"message"`
	DeviceID            string            `json:"device_id,omitempty"`
	Key                 string            `json:"key,omitempty"`
	Value               interface{}       `json:"value,omitempty"`
	Tags                map[string]string `json:"tags,omitempty"`
	Timestamp           time.Time         `json:"timestamp"`
	Throttle            time.Duration     `json:"throttle"`
	NotificationChannels []string         `json:"notification_channels,omitempty"`
	AutoResolve         bool              `json:"auto_resolve,omitempty"`
	ResolveTimeout      time.Duration     `json:"resolve_timeout,omitempty"`
	Priority            int               `json:"priority,omitempty"`
}

// NotificationService 通知服务接口
type NotificationService interface {
	SendNotification(channelID string, alert *models.Alert) error
	TestChannel(channelID string) error
	GetChannels() ([]models.NotificationChannel, error)
}

// NewAlertIntegrationService 创建告警集成服务
func NewAlertIntegrationService(
	alertService AlertService,
	notificationService NotificationService,
	natsConn *nats.Conn,
	ruleManager rules.RuleManager,
) *AlertIntegrationService {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &AlertIntegrationService{
		alertService:        alertService,
		notificationService: notificationService,
		natsConn:           natsConn,
		ruleManager:        ruleManager,
		activeAlerts:       make(map[string]*models.Alert),
		alertThrottleMap:   make(map[string]time.Time),
		autoResolveTimers:  make(map[string]*time.Timer),
		subscriptions:      make([]*nats.Subscription, 0),
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start 启动集成服务
func (s *AlertIntegrationService) Start() error {
	log.Info().Msg("启动告警集成服务")
	
	// 订阅规则引擎告警事件
	if err := s.subscribeToRuleEngineAlerts(); err != nil {
		return fmt.Errorf("订阅规则引擎告警失败: %w", err)
	}
	
	// 启动清理协程
	go s.cleanupRoutine()
	
	log.Info().Msg("告警集成服务启动成功")
	return nil
}

// Stop 停止集成服务
func (s *AlertIntegrationService) Stop() error {
	log.Info().Msg("停止告警集成服务")
	
	// 取消上下文
	s.cancel()
	
	// 取消所有订阅
	for _, sub := range s.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			log.Warn().Err(err).Msg("取消NATS订阅失败")
		}
	}
	
	// 停止所有自动解决定时器
	s.mu.Lock()
	for alertID, timer := range s.autoResolveTimers {
		timer.Stop()
		delete(s.autoResolveTimers, alertID)
	}
	s.mu.Unlock()
	
	log.Info().Msg("告警集成服务停止成功")
	return nil
}

// subscribeToRuleEngineAlerts 订阅规则引擎告警
func (s *AlertIntegrationService) subscribeToRuleEngineAlerts() error {
	// 只订阅通用告警主题，避免重复接收
	subjects := []string{
		"iot.alerts.triggered",
	}
	
	for _, subject := range subjects {
		sub, err := s.natsConn.Subscribe(subject, s.handleRuleEngineAlert)
		if err != nil {
			return fmt.Errorf("订阅主题 %s 失败: %w", subject, err)
		}
		s.subscriptions = append(s.subscriptions, sub)
		log.Debug().Str("subject", subject).Msg("订阅告警主题成功")
	}
	
	return nil
}

// handleRuleEngineAlert 处理来自规则引擎的告警
func (s *AlertIntegrationService) handleRuleEngineAlert(msg *nats.Msg) {
	var ruleAlert RuleEngineAlert
	if err := json.Unmarshal(msg.Data, &ruleAlert); err != nil {
		log.Error().Err(err).Msg("解析规则引擎告警消息失败")
		return
	}
	
	log.Info().
		Str("alert_id", ruleAlert.ID).
		Str("rule_id", ruleAlert.RuleID).
		Str("level", ruleAlert.Level).
		Msg("收到规则引擎告警")
	
	// 检查节流
	if s.shouldThrottle(&ruleAlert) {
		log.Debug().
			Str("alert_id", ruleAlert.ID).
			Str("rule_id", ruleAlert.RuleID).
			Msg("告警被节流跳过")
		return
	}
	
	// 转换为告警服务格式并创建
	alert, err := s.convertAndCreateAlert(&ruleAlert)
	if err != nil {
		log.Error().
			Err(err).
			Str("alert_id", ruleAlert.ID).
			Msg("创建告警失败")
		return
	}
	
	// 记录节流时间
	s.recordThrottle(&ruleAlert)
	
	// 发送通知
	go s.sendNotifications(alert, ruleAlert.NotificationChannels)
	
	// 设置自动解决（如果启用）
	if ruleAlert.AutoResolve && ruleAlert.ResolveTimeout > 0 {
		s.scheduleAutoResolve(alert.ID, ruleAlert.ResolveTimeout)
	}
	
	// 发布告警事件到前端
	s.publishAlertToFrontend(alert)
}

// convertAndCreateAlert 转换并创建告警
func (s *AlertIntegrationService) convertAndCreateAlert(ruleAlert *RuleEngineAlert) (*models.Alert, error) {
	// 构建额外数据
	data := map[string]interface{}{
		"rule_id":   ruleAlert.RuleID,
		"rule_name": ruleAlert.RuleName,
		"device_id": ruleAlert.DeviceID,
		"key":       ruleAlert.Key,
		"value":     ruleAlert.Value,
		"priority":  ruleAlert.Priority,
	}
	
	// 添加标签到数据中
	if ruleAlert.Tags != nil {
		for k, v := range ruleAlert.Tags {
			data["tag_"+k] = v
		}
	}
	
	// 创建告警请求
	createReq := &models.AlertCreateRequest{
		Title:       s.generateAlertTitle(ruleAlert),
		Description: ruleAlert.Message,
		Level:       ruleAlert.Level,
		Source:      "rule-engine",
		Data:        data,
	}
	
	// 创建告警
	alert, err := s.alertService.CreateAlert(createReq)
	if err != nil {
		return nil, fmt.Errorf("创建告警失败: %w", err)
	}
	
	// 存储到活跃告警映射
	s.mu.Lock()
	s.activeAlerts[alert.ID] = alert
	s.mu.Unlock()
	
	log.Info().
		Str("alert_id", alert.ID).
		Str("rule_id", ruleAlert.RuleID).
		Str("level", ruleAlert.Level).
		Str("title", alert.Title).
		Msg("告警创建成功")
	
	return alert, nil
}

// generateAlertTitle 生成告警标题
func (s *AlertIntegrationService) generateAlertTitle(ruleAlert *RuleEngineAlert) string {
	if ruleAlert.DeviceID != "" && ruleAlert.Key != "" {
		return fmt.Sprintf("%s - %s:%s", ruleAlert.RuleName, ruleAlert.DeviceID, ruleAlert.Key)
	} else if ruleAlert.DeviceID != "" {
		return fmt.Sprintf("%s - %s", ruleAlert.RuleName, ruleAlert.DeviceID)
	}
	return ruleAlert.RuleName
}

// shouldThrottle 检查是否应该节流
func (s *AlertIntegrationService) shouldThrottle(ruleAlert *RuleEngineAlert) bool {
	if ruleAlert.Throttle <= 0 {
		return false
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 生成节流键
	throttleKey := fmt.Sprintf("%s:%s:%s", ruleAlert.RuleID, ruleAlert.DeviceID, ruleAlert.Key)
	
	if lastTime, exists := s.alertThrottleMap[throttleKey]; exists {
		return time.Since(lastTime) < ruleAlert.Throttle
	}
	
	return false
}

// recordThrottle 记录节流时间
func (s *AlertIntegrationService) recordThrottle(ruleAlert *RuleEngineAlert) {
	if ruleAlert.Throttle <= 0 {
		return
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	throttleKey := fmt.Sprintf("%s:%s:%s", ruleAlert.RuleID, ruleAlert.DeviceID, ruleAlert.Key)
	s.alertThrottleMap[throttleKey] = time.Now()
}

// sendNotifications 发送通知
func (s *AlertIntegrationService) sendNotifications(alert *models.Alert, channelIDs []string) {
	if s.notificationService == nil {
		log.Warn().Msg("通知服务未配置，跳过通知发送")
		return
	}
	
	if len(channelIDs) == 0 {
		log.Debug().Str("alert_id", alert.ID).Msg("未配置通知渠道")
		return
	}
	
	for _, channelID := range channelIDs {
		if err := s.notificationService.SendNotification(channelID, alert); err != nil {
			log.Error().
				Err(err).
				Str("alert_id", alert.ID).
				Str("channel_id", channelID).
				Msg("发送通知失败")
		} else {
			log.Debug().
				Str("alert_id", alert.ID).
				Str("channel_id", channelID).
				Msg("通知发送成功")
		}
	}
}

// scheduleAutoResolve 安排自动解决
func (s *AlertIntegrationService) scheduleAutoResolve(alertID string, timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 如果已经有定时器，先停止
	if existingTimer, exists := s.autoResolveTimers[alertID]; exists {
		existingTimer.Stop()
	}
	
	// 创建新的定时器
	timer := time.AfterFunc(timeout, func() {
		if err := s.autoResolveAlert(alertID); err != nil {
			log.Error().
				Err(err).
				Str("alert_id", alertID).
				Msg("自动解决告警失败")
		}
	})
	
	s.autoResolveTimers[alertID] = timer
	
	log.Debug().
		Str("alert_id", alertID).
		Dur("timeout", timeout).
		Msg("设置自动解决定时器")
}

// autoResolveAlert 自动解决告警
func (s *AlertIntegrationService) autoResolveAlert(alertID string) error {
	log.Info().Str("alert_id", alertID).Msg("自动解决告警")
	
	// 解决告警
	if err := s.alertService.ResolveAlert(alertID, "system", "自动解决：超时未恢复"); err != nil {
		return fmt.Errorf("自动解决告警失败: %w", err)
	}
	
	// 清理状态
	s.mu.Lock()
	delete(s.activeAlerts, alertID)
	delete(s.autoResolveTimers, alertID)
	s.mu.Unlock()
	
	// 发布解决事件到前端
	s.publishAlertResolvedToFrontend(alertID)
	
	return nil
}

// publishAlertToFrontend 发布告警到前端
func (s *AlertIntegrationService) publishAlertToFrontend(alert *models.Alert) {
	alertData := map[string]interface{}{
		"type":      "alert_created",
		"alert_id":  alert.ID,
		"alert":     alert,
		"timestamp": time.Now(),
	}
	
	data, _ := json.Marshal(alertData)
	if err := s.natsConn.Publish("iot.frontend.alerts", data); err != nil {
		log.Warn().Err(err).Msg("发布告警到前端失败")
	}
}

// publishAlertResolvedToFrontend 发布告警解决到前端
func (s *AlertIntegrationService) publishAlertResolvedToFrontend(alertID string) {
	alertData := map[string]interface{}{
		"type":      "alert_resolved",
		"alert_id":  alertID,
		"timestamp": time.Now(),
	}
	
	data, _ := json.Marshal(alertData)
	if err := s.natsConn.Publish("iot.frontend.alerts", data); err != nil {
		log.Warn().Err(err).Msg("发布告警解决到前端失败")
	}
}

// cleanupRoutine 清理协程
func (s *AlertIntegrationService) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup 清理过期数据
func (s *AlertIntegrationService) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	now := time.Now()
	
	// 清理节流映射中的过期记录
	for key, lastTime := range s.alertThrottleMap {
		if now.Sub(lastTime) > time.Hour {
			delete(s.alertThrottleMap, key)
		}
	}
	
	log.Debug().
		Int("throttle_entries", len(s.alertThrottleMap)).
		Int("active_alerts", len(s.activeAlerts)).
		Int("auto_resolve_timers", len(s.autoResolveTimers)).
		Msg("告警集成服务状态清理完成")
}

// GetStats 获取统计信息
func (s *AlertIntegrationService) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return map[string]interface{}{
		"active_alerts":        len(s.activeAlerts),
		"throttle_entries":     len(s.alertThrottleMap),
		"auto_resolve_timers":  len(s.autoResolveTimers),
		"nats_subscriptions":   len(s.subscriptions),
	}
}