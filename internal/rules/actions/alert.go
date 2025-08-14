package actions

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// AlertHandler Alert动作处理器
type AlertHandler struct {
	throttleMap map[string]time.Time
	mu          sync.RWMutex
	natsConn    interface{} // NATS连接，使用interface{}避免循环依赖
}

// NewAlertHandler 创建Alert处理器
func NewAlertHandler() *AlertHandler {
	handler := &AlertHandler{
		throttleMap: make(map[string]time.Time),
	}

	// 启动清理协程
	go handler.cleanupThrottleMap()

	return handler
}

// NewAlertHandlerWithNATS 创建带NATS连接的Alert处理器
func NewAlertHandlerWithNATS(natsConn interface{}) *AlertHandler {
	handler := &AlertHandler{
		throttleMap: make(map[string]time.Time),
		natsConn:    natsConn,
	}

	// 启动清理协程
	go handler.cleanupThrottleMap()

	return handler
}

// Name 返回处理器名称
func (h *AlertHandler) Name() string {
	return "alert"
}

// Execute 执行报警动作
func (h *AlertHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	start := time.Now()

	// 解析配置
	alertConfig, err := h.parseConfig(config)
	if err != nil {
		return &rules.ActionResult{
			Type:     "alert",
			Success:  false,
			Error:    fmt.Sprintf("解析配置失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// 创建报警消息
	alert := h.createAlert(point, rule, alertConfig)

	// 检查节流（原子操作，避免竞态条件）
	if h.checkAndRecordThrottle(alert) {
		return &rules.ActionResult{
			Type:     "alert",
			Success:  true,
			Error:    "报警被节流跳过",
			Duration: time.Since(start),
			Output:   map[string]interface{}{"throttled": true},
		}, nil
	}

	// 发送报警
	results := h.sendAlert(ctx, alert, alertConfig)

	// 发布告警到NATS（如果有连接）
	h.publishAlertToNATS(alert, alertConfig)

	// 统计结果
	successCount := 0
	var errors []string
	for channel, result := range results {
		if result.Success {
			successCount++
		} else {
			errors = append(errors, fmt.Sprintf("%s: %s", channel, result.Error))
		}
	}

	success := successCount > 0
	errorMsg := ""
	if len(errors) > 0 {
		errorMsg = strings.Join(errors, "; ")
	}

	return &rules.ActionResult{
		Type:     "alert",
		Success:  success,
		Error:    errorMsg,
		Duration: time.Since(start),
		Output: map[string]interface{}{
			"alert_id":       alert.ID,
			"channels_sent":  successCount,
			"channels_total": len(alertConfig.Channels),
			"results":        results,
			"message":        alert.Message, // 添加渲染后的消息内容
			"level":          alert.Level,
			"device_id":      alert.DeviceID,
			"key":            alert.Key,
			"value":          alert.Value,
		},
	}, nil
}

// AlertConfig 报警配置
type AlertConfig struct {
	Level      string                 `json:"level"`       // info, warning, error, critical
	Message    string                 `json:"message"`     // 报警消息模板
	Channels   []ChannelConfig        `json:"channels"`    // 通知渠道
	Throttle   time.Duration          `json:"throttle"`    // 节流时间
	Tags       map[string]string      `json:"tags"`        // 额外标签
	Template   map[string]interface{} `json:"template"`    // 消息模板参数
	RetryCount int                    `json:"retry_count"` // 重试次数
	RetryDelay time.Duration          `json:"retry_delay"` // 重试延迟
}

// ChannelConfig 通知渠道配置
type ChannelConfig struct {
	Type   string                 `json:"type"`   // console, webhook, email, sms
	Config map[string]interface{} `json:"config"` // 渠道特定配置
}

// ChannelResult 渠道发送结果
type ChannelResult struct {
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}

// parseConfig 解析配置
func (h *AlertHandler) parseConfig(config map[string]interface{}) (*AlertConfig, error) {
	alertConfig := &AlertConfig{
		Level:      "warning",
		Message:    "规则触发报警: {{.RuleName}}",
		Channels:   []ChannelConfig{},
		Throttle:   5 * time.Minute,
		Tags:       make(map[string]string),
		Template:   make(map[string]interface{}),
		RetryCount: 0,                    // 默认不重试
		RetryDelay: 100 * time.Millisecond, // 默认重试延迟
	}

	// 解析level
	if level, ok := config["level"].(string); ok {
		alertConfig.Level = level
	}

	// 解析message
	if message, ok := config["message"].(string); ok {
		alertConfig.Message = message
	}

	// 解析throttle (支持throttle和throttle_duration两种键名)
	if throttleStr, ok := config["throttle"].(string); ok {
		if duration, err := time.ParseDuration(throttleStr); err == nil {
			alertConfig.Throttle = duration
		}
	} else if throttleStr, ok := config["throttle_duration"].(string); ok {
		if duration, err := time.ParseDuration(throttleStr); err == nil {
			alertConfig.Throttle = duration
		}
	}

	// 解析channels
	if channelsData, ok := config["channels"]; ok {
		channelsBytes, _ := json.Marshal(channelsData)
		json.Unmarshal(channelsBytes, &alertConfig.Channels)
	}

	// 解析tags
	if tags, ok := config["tags"].(map[string]interface{}); ok {
		for k, v := range tags {
			if str, ok := v.(string); ok {
				alertConfig.Tags[k] = str
			}
		}
	}

	// 解析template
	if template, ok := config["template"].(map[string]interface{}); ok {
		alertConfig.Template = template
	}

	// 解析重试配置
	if retryCount, ok := config["retry_count"].(int); ok {
		alertConfig.RetryCount = retryCount
	} else if retryCount, ok := config["retry_count"].(float64); ok {
		alertConfig.RetryCount = int(retryCount)
	}

	if retryDelayStr, ok := config["retry_delay"].(string); ok {
		if delay, err := time.ParseDuration(retryDelayStr); err == nil {
			alertConfig.RetryDelay = delay
		}
	}

	// 如果没有配置渠道，默认添加console渠道
	if len(alertConfig.Channels) == 0 {
		alertConfig.Channels = []ChannelConfig{
			{
				Type:   "console",
				Config: map[string]interface{}{},
			},
		}
	}

	return alertConfig, nil
}

// createAlert 创建报警消息
func (h *AlertHandler) createAlert(point model.Point, rule *rules.Rule, config *AlertConfig) *rules.Alert {
	// 生成报警ID
	alertID := h.generateAlertID()

	// 解析消息模板
	message := h.parseMessageTemplate(config.Message, point, rule, config)

	// 合并标签
	tags := make(map[string]string)
	for k, v := range rule.Tags {
		tags[k] = v
	}
	for k, v := range config.Tags {
		tags[k] = v
	}
	// Go 1.24安全：使用GetTagsSafe获取标签
	pointTags := point.GetTagsSafe()
	for k, v := range pointTags {
		tags["point_"+k] = v
	}

	return &rules.Alert{
		ID:        alertID,
		RuleID:    rule.ID,
		RuleName:  rule.Name,
		Level:     config.Level,
		Message:   message,
		DeviceID:  point.DeviceID,
		Key:       point.Key,
		Value:     point.Value,
		Tags:      tags,
		Timestamp: time.Now(),
		Throttle:  config.Throttle,
	}
}

// parseMessageTemplate 解析消息模板，支持Go模板语法
func (h *AlertHandler) parseMessageTemplate(templateStr string, point model.Point, rule *rules.Rule, config *AlertConfig) string {
	// 准备模板数据
	templateData := map[string]interface{}{
		"RuleName":  rule.Name,
		"RuleID":    rule.ID,
		"DeviceID":  point.DeviceID,
		"Key":       point.Key,
		"Value":     point.Value,
		"Type":      string(point.Type),
		"Timestamp": point.Timestamp,
		"Level":     config.Level,
		"Tags":      make(map[string]interface{}),
	}

	// Go 1.24安全：添加point的tags，确保正确的映射结构
	pointTags := point.GetTagsSafe()
	if len(pointTags) > 0 {
		tagsMap := make(map[string]interface{})
		for key, value := range pointTags {
			tagsMap[key] = value
		}
		templateData["Tags"] = tagsMap
	}

	// 添加rule的tags
	if rule.Tags != nil {
		for key, value := range rule.Tags {
			templateData[key] = value
		}
	}

	// 添加config的template参数
	for key, value := range config.Template {
		templateData[key] = value
	}

	// 尝试使用Go模板引擎，添加常用函数
	tmpl, err := template.New("alert").Funcs(template.FuncMap{
		"gt": func(a, b interface{}) bool {
			// 大于比较，支持数值转换
			if aNum, ok := toFloat64(a); ok {
				if bNum, ok := toFloat64(b); ok {
					return aNum > bNum
				}
			}
			return false
		},
		"lt": func(a, b interface{}) bool {
			// 小于比较
			if aNum, ok := toFloat64(a); ok {
				if bNum, ok := toFloat64(b); ok {
					return aNum < bNum
				}
			}
			return false
		},
		"eq": func(a, b interface{}) bool {
			// 相等比较
			if aNum, ok := toFloat64(a); ok {
				if bNum, ok := toFloat64(b); ok {
					return aNum == bNum
				}
			}
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		},
	}).Parse(templateStr)
	
	if err != nil {
		// 如果Go模板解析失败，回退到简单字符串替换
		log.Warn().Err(err).Str("template", templateStr).Msg("Go模板解析失败，回退到简单替换")
		return h.parseMessageTemplateFallback(templateStr, point, rule, config)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, templateData)
	if err != nil {
		// 如果模板执行失败，回退到简单字符串替换
		log.Warn().Err(err).Str("template", templateStr).Interface("data", templateData).Msg("Go模板执行失败，回退到简单替换")
		return h.parseMessageTemplateFallback(templateStr, point, rule, config)
	}

	return buf.String()
}

// parseMessageTemplateFallback 简单字符串替换的回退方法
func (h *AlertHandler) parseMessageTemplateFallback(templateStr string, point model.Point, rule *rules.Rule, config *AlertConfig) string {
	message := templateStr

	// 替换基本变量
	replacements := map[string]string{
		"{{.RuleName}}":  rule.Name,
		"{{.RuleID}}":    rule.ID,
		"{{.DeviceID}}":  point.DeviceID,
		"{{.Key}}":       point.Key,
		"{{.Value}}":     fmt.Sprintf("%v", point.Value),
		"{{.Type}}":      string(point.Type),
		"{{.Timestamp}}": point.Timestamp.Format("2006-01-02 15:04:05"),
		"{{.Level}}":     config.Level,
	}

	for placeholder, value := range replacements {
		message = strings.ReplaceAll(message, placeholder, value)
	}

	// 替换模板参数
	for key, value := range config.Template {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		message = strings.ReplaceAll(message, placeholder, fmt.Sprintf("%v", value))
	}

	// Go 1.24安全：替换标签
	pointTags := point.GetTagsSafe()
	for key, value := range pointTags {
		placeholder := fmt.Sprintf("{{.Tags.%s}}", key)
		message = strings.ReplaceAll(message, placeholder, value)
	}

	return message
}

// toFloat64 辅助函数，尝试将值转换为float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

// generateAlertID 生成报警ID
func (h *AlertHandler) generateAlertID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// checkAndRecordThrottle 原子地检查节流并记录时间戳，避免竞态条件
func (h *AlertHandler) checkAndRecordThrottle(alert *rules.Alert) bool {
	if alert.Throttle <= 0 {
		return false
	}

	// 使用写锁确保检查和记录操作的原子性
	h.mu.Lock()
	defer h.mu.Unlock()

	// 生成节流键
	throttleKey := fmt.Sprintf("%s:%s:%s", alert.RuleID, alert.DeviceID, alert.Key)

	// 检查是否应该节流
	if lastTime, exists := h.throttleMap[throttleKey]; exists {
		if time.Since(lastTime) < alert.Throttle {
			// 仍在节流期内
			return true
		}
	}

	// 不在节流期内，记录当前时间戳并允许执行
	h.throttleMap[throttleKey] = time.Now()
	return false
}

// shouldThrottle 检查是否应该节流（保留方法以兼容性，但建议使用checkAndRecordThrottle）
func (h *AlertHandler) shouldThrottle(alert *rules.Alert) bool {
	if alert.Throttle <= 0 {
		return false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	// 生成节流键
	throttleKey := fmt.Sprintf("%s:%s:%s", alert.RuleID, alert.DeviceID, alert.Key)

	if lastTime, exists := h.throttleMap[throttleKey]; exists {
		return time.Since(lastTime) < alert.Throttle
	}

	return false
}

// recordThrottle 记录节流时间（保留方法以兼容性，但建议使用checkAndRecordThrottle）
func (h *AlertHandler) recordThrottle(alert *rules.Alert) {
	if alert.Throttle <= 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	throttleKey := fmt.Sprintf("%s:%s:%s", alert.RuleID, alert.DeviceID, alert.Key)
	h.throttleMap[throttleKey] = time.Now()
}

// sendAlert 发送报警，支持重试和故障转移
func (h *AlertHandler) sendAlert(ctx context.Context, alert *rules.Alert, config *AlertConfig) map[string]ChannelResult {
	results := make(map[string]ChannelResult)
	
	// 如果配置了重试，尝试逐个渠道发送，支持故障转移
	if config.RetryCount > 0 {
		return h.sendAlertWithRetry(ctx, alert, config)
	}

	// 没有重试配置，直接发送所有渠道
	for i, channel := range config.Channels {
		channelKey := fmt.Sprintf("%s_%d", channel.Type, i)
		start := time.Now()
		var err error

		switch channel.Type {
		case "console":
			err = h.sendConsoleAlert(alert, channel.Config)
		case "webhook":
			err = h.sendWebhookAlert(ctx, alert, channel.Config)
		case "email":
			err = h.sendEmailAlert(alert, channel.Config)
		case "sms":
			err = h.sendSMSAlert(alert, channel.Config)
		case "nats":
			err = h.sendNATSAlert(alert, channel.Config)
		default:
			err = fmt.Errorf("不支持的通知渠道: %s", channel.Type)
		}

		results[channelKey] = ChannelResult{
			Success: err == nil,
			Error: func() string {
				if err != nil {
					return err.Error()
				}
				return ""
			}(),
			Duration: time.Since(start),
		}
	}

	return results
}

// sendAlertWithRetry 带重试和故障转移的报警发送
func (h *AlertHandler) sendAlertWithRetry(ctx context.Context, alert *rules.Alert, config *AlertConfig) map[string]ChannelResult {
	results := make(map[string]ChannelResult)
	
	// 尝试每个渠道，如果失败则重试，最终使用故障转移
	for i, channel := range config.Channels {
		channelKey := fmt.Sprintf("%s_%d", channel.Type, i)
		success := false
		var lastErr error
		var totalDuration time.Duration
		
		// 重试逻辑
		for attempt := 0; attempt <= config.RetryCount; attempt++ {
			start := time.Now()
			var err error

			switch channel.Type {
			case "console":
				err = h.sendConsoleAlert(alert, channel.Config)
			case "webhook":
				err = h.sendWebhookAlert(ctx, alert, channel.Config)
			case "email":
				err = h.sendEmailAlert(alert, channel.Config)
			case "sms":
				err = h.sendSMSAlert(alert, channel.Config)
			case "nats":
				err = h.sendNATSAlert(alert, channel.Config)
			default:
				err = fmt.Errorf("不支持的通知渠道: %s", channel.Type)
			}
			
			duration := time.Since(start)
			totalDuration += duration
			
			if err == nil {
				success = true
				break
			}
			
			lastErr = err
			
			// 如果不是最后一次尝试，等待重试延迟
			if attempt < config.RetryCount {
				time.Sleep(config.RetryDelay)
			}
		}
		
		results[channelKey] = ChannelResult{
			Success: success,
			Error: func() string {
				if lastErr != nil {
					return lastErr.Error()
				}
				return ""
			}(),
			Duration: totalDuration,
		}
		
		// 如果某个渠道成功了，可以选择是否继续尝试其他渠道
		// 这里继续尝试所有渠道以获得完整结果
	}

	return results
}

// sendNATSAlert 发送NATS报警
func (h *AlertHandler) sendNATSAlert(alert *rules.Alert, config map[string]interface{}) error {
	if h.natsConn == nil {
		return fmt.Errorf("NATS连接未初始化")
	}

	subject, ok := config["subject"].(string)
	if !ok || subject == "" {
		subject = "alerts.default"
	}

	// 构建消息
	alertMsg := map[string]interface{}{
		"id":        alert.ID,
		"rule_id":   alert.RuleID,
		"rule_name": alert.RuleName,
		"level":     alert.Level,
		"message":   alert.Message,
		"device_id": alert.DeviceID,
		"key":       alert.Key,
		"value":     alert.Value,
		"tags":      alert.Tags,
		"timestamp": alert.Timestamp,
	}

	data, err := json.Marshal(alertMsg)
	if err != nil {
		return fmt.Errorf("序列化NATS消息失败: %w", err)
	}

	// 发布消息
	if publisher, ok := h.natsConn.(interface {
		Publish(string, []byte) error
	}); ok {
		return publisher.Publish(subject, data)
	}

	return fmt.Errorf("NATS连接不支持发布消息")
}

// sendConsoleAlert 发送控制台报警
func (h *AlertHandler) sendConsoleAlert(alert *rules.Alert, config map[string]interface{}) error {
	// 根据级别选择日志级别
	switch alert.Level {
	case "critical":
		log.Error().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	case "error":
		log.Error().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	case "warning":
		log.Warn().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	default:
		log.Info().
			Str("alert_id", alert.ID).
			Str("rule_id", alert.RuleID).
			Str("rule_name", alert.RuleName).
			Str("device_id", alert.DeviceID).
			Str("key", alert.Key).
			Interface("value", alert.Value).
			Interface("tags", alert.Tags).
			Msg(alert.Message)
	}

	return nil
}

// sendWebhookAlert 发送Webhook报警
func (h *AlertHandler) sendWebhookAlert(ctx context.Context, alert *rules.Alert, config map[string]interface{}) error {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("webhook URL未配置")
	}

	// 准备请求数据
	payload := map[string]interface{}{
		"alert_id":  alert.ID,
		"rule_id":   alert.RuleID,
		"rule_name": alert.RuleName,
		"level":     alert.Level,
		"message":   alert.Message,
		"device_id": alert.DeviceID,
		"key":       alert.Key,
		"value":     alert.Value,
		"tags":      alert.Tags,
		"timestamp": alert.Timestamp.Unix(),
	}

	// 序列化数据
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "IoT-Gateway-Rules-Engine")

	// 添加认证头（如果配置了）
	if token, ok := config["token"].(string); ok && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// 发送请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook响应错误: %d", resp.StatusCode)
	}

	log.Debug().
		Str("alert_id", alert.ID).
		Str("url", url).
		Int("status", resp.StatusCode).
		Msg("Webhook报警发送成功")

	return nil
}

// sendEmailAlert 发送邮件报警（占位符实现）
func (h *AlertHandler) sendEmailAlert(alert *rules.Alert, config map[string]interface{}) error {
	// 这里是邮件发送的占位符实现
	// 在实际实现中，应该集成SMTP客户端
	log.Info().
		Str("alert_id", alert.ID).
		Str("type", "email").
		Interface("config", config).
		Msg("邮件报警发送（占位符实现）")

	return nil
}

// sendSMSAlert 发送短信报警（占位符实现）
func (h *AlertHandler) sendSMSAlert(alert *rules.Alert, config map[string]interface{}) error {
	// 这里是短信发送的占位符实现
	// 在实际实现中，应该集成短信服务提供商API
	log.Info().
		Str("alert_id", alert.ID).
		Str("type", "sms").
		Interface("config", config).
		Msg("短信报警发送（占位符实现）")

	return nil
}

// publishAlertToNATS 发布告警到NATS
func (h *AlertHandler) publishAlertToNATS(alert *rules.Alert, config *AlertConfig) {
	if h.natsConn == nil {
		return
	}

	// 构建告警消息
	alertMsg := map[string]interface{}{
		"id":                    alert.ID,
		"rule_id":               alert.RuleID,
		"rule_name":             alert.RuleName,
		"level":                 alert.Level,
		"message":               alert.Message,
		"device_id":             alert.DeviceID,
		"key":                   alert.Key,
		"value":                 alert.Value,
		"tags":                  alert.Tags,
		"timestamp":             alert.Timestamp,
		"throttle":              alert.Throttle,
		"notification_channels": h.extractNotificationChannels(config),
		"auto_resolve":          false, // 默认不自动解决
		"priority":              5,     // 默认优先级
	}

	// 序列化消息
	data, err := json.Marshal(alertMsg)
	if err != nil {
		log.Error().Err(err).Str("alert_id", alert.ID).Msg("序列化告警消息失败")
		return
	}

	// 发布到不同主题
	subjects := []string{
		"iot.alerts.triggered",
		fmt.Sprintf("iot.alerts.triggered.%s", alert.Level),
	}

	// 使用反射调用Publish方法
	if publisher, ok := h.natsConn.(interface {
		Publish(string, []byte) error
	}); ok {
		for _, subject := range subjects {
			if err := publisher.Publish(subject, data); err != nil {
				log.Error().
					Err(err).
					Str("alert_id", alert.ID).
					Str("subject", subject).
					Msg("发布告警到NATS失败")
			} else {
				log.Info().
					Str("alert_id", alert.ID).
					Str("subject", subject).
					Msg("告警发布到NATS成功")
			}
		}
	}
}

// extractNotificationChannels 提取通知渠道ID
func (h *AlertHandler) extractNotificationChannels(config *AlertConfig) []string {
	var channels []string
	for _, channel := range config.Channels {
		// 这里简化处理，将渠道类型作为ID
		// 实际应该有更复杂的映射逻辑
		channels = append(channels, channel.Type)
	}
	return channels
}

// cleanupThrottleMap 清理节流映射
func (h *AlertHandler) cleanupThrottleMap() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for key, lastTime := range h.throttleMap {
			// 清理超过1小时的记录
			if now.Sub(lastTime) > time.Hour {
				delete(h.throttleMap, key)
			}
		}
		h.mu.Unlock()
	}
}
