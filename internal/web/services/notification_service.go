package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
)

// notificationService 通知服务实现
type notificationService struct {
	alertService AlertService
	httpClient   *http.Client
}

// NewNotificationService 创建通知服务
func NewNotificationService(alertService AlertService) NotificationService {
	return &notificationService{
		alertService: alertService,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendNotification 发送通知
func (s *notificationService) SendNotification(channelID string, alert *models.Alert) error {
	// 获取通知渠道配置
	channels, err := s.alertService.GetNotificationChannels()
	if err != nil {
		return fmt.Errorf("获取通知渠道失败: %w", err)
	}

	var channel *models.NotificationChannel
	for _, ch := range channels {
		if ch.ID == channelID {
			channel = &ch
			break
		}
	}

	if channel == nil {
		return fmt.Errorf("通知渠道 %s 不存在", channelID)
	}

	if !channel.Enabled {
		return fmt.Errorf("通知渠道 %s 已禁用", channelID)
	}

	// 根据渠道类型发送通知
	switch channel.Type {
	case "email":
		return s.sendEmailNotification(channel, alert)
	case "webhook":
		return s.sendWebhookNotification(channel, alert)
	case "sms":
		return s.sendSMSNotification(channel, alert)
	case "slack":
		return s.sendSlackNotification(channel, alert)
	case "dingtalk":
		return s.sendDingTalkNotification(channel, alert)
	default:
		return fmt.Errorf("不支持的通知渠道类型: %s", channel.Type)
	}
}

// sendEmailNotification 发送邮件通知
func (s *notificationService) sendEmailNotification(channel *models.NotificationChannel, alert *models.Alert) error {
	config := channel.Config

	// 解析配置
	smtpHost, _ := config["smtp_host"].(string)
	smtpPort, _ := config["smtp_port"].(float64)
	username, _ := config["username"].(string)
	password, _ := config["password"].(string)
	from, _ := config["from"].(string)
	
	recipients, ok := config["recipients"].([]interface{})
	if !ok {
		return fmt.Errorf("邮件收件人配置错误")
	}

	// 转换收件人列表
	var toList []string
	for _, recipient := range recipients {
		if email, ok := recipient.(string); ok {
			toList = append(toList, email)
		}
	}

	if len(toList) == 0 {
		return fmt.Errorf("没有有效的邮件收件人")
	}

	// 构建邮件内容
	subject := fmt.Sprintf("[%s] %s", strings.ToUpper(alert.Level), alert.Title)
	body := s.buildEmailBody(alert)

	// 构建邮件消息
	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n",
		from,
		strings.Join(toList, ","),
		subject,
		body,
	))

	// 发送邮件
	auth := smtp.PlainAuth("", username, password, smtpHost)
	addr := fmt.Sprintf("%s:%.0f", smtpHost, smtpPort)

	if err := smtp.SendMail(addr, auth, from, toList, msg); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}

	log.Info().
		Str("channel_id", channel.ID).
		Str("alert_id", alert.ID).
		Strs("recipients", toList).
		Msg("邮件通知发送成功")

	return nil
}

// sendWebhookNotification 发送Webhook通知
func (s *notificationService) sendWebhookNotification(channel *models.NotificationChannel, alert *models.Alert) error {
	config := channel.Config

	url, ok := config["url"].(string)
	if !ok || url == "" {
		return fmt.Errorf("Webhook URL未配置")
	}

	method, _ := config["method"].(string)
	if method == "" {
		method = "POST"
	}

	// 构建请求数据
	payload := map[string]interface{}{
		"alert_id":    alert.ID,
		"title":       alert.Title,
		"description": alert.Description,
		"level":       alert.Level,
		"status":      alert.Status,
		"source":      alert.Source,
		"data":        alert.Data,
		"created_at":  alert.CreatedAt,
		"updated_at":  alert.UpdatedAt,
		"webhook_meta": map[string]interface{}{
			"channel_id":   channel.ID,
			"channel_name": channel.Name,
			"sent_at":      time.Now(),
		},
	}

	// 序列化数据
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化Webhook数据失败: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("创建Webhook请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "IoT-Gateway-Alert-Service/1.0")

	// 添加自定义头
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送Webhook请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook响应错误: %d %s", resp.StatusCode, resp.Status)
	}

	log.Info().
		Str("channel_id", channel.ID).
		Str("alert_id", alert.ID).
		Str("url", url).
		Int("status_code", resp.StatusCode).
		Msg("Webhook通知发送成功")

	return nil
}

// sendSMSNotification 发送短信通知（示例实现）
func (s *notificationService) sendSMSNotification(channel *models.NotificationChannel, alert *models.Alert) error {
	config := channel.Config

	// 这里是示例实现，实际应该集成具体的短信服务提供商
	apiKey, _ := config["api_key"].(string)
	apiSecret, _ := config["api_secret"].(string)
	serviceURL, _ := config["service_url"].(string)
	
	phoneNumbers, ok := config["phone_numbers"].([]interface{})
	if !ok {
		return fmt.Errorf("短信收件人配置错误")
	}

	// 构建短信内容
	message := fmt.Sprintf("[%s] %s: %s", strings.ToUpper(alert.Level), alert.Title, alert.Description)
	if len(message) > 160 {
		message = message[:157] + "..."
	}

	// 发送短信（示例实现）
	for _, phoneNumber := range phoneNumbers {
		if phone, ok := phoneNumber.(string); ok {
			payload := map[string]interface{}{
				"api_key":      apiKey,
				"api_secret":   apiSecret,
				"phone_number": phone,
				"message":      message,
			}

			data, _ := json.Marshal(payload)
			req, err := http.NewRequest("POST", serviceURL, bytes.NewBuffer(data))
			if err != nil {
				log.Error().Err(err).Str("phone", phone).Msg("创建短信请求失败")
				continue
			}

			req.Header.Set("Content-Type", "application/json")
			
			resp, err := s.httpClient.Do(req)
			if err != nil {
				log.Error().Err(err).Str("phone", phone).Msg("发送短信失败")
				continue
			}
			resp.Body.Close()

			log.Info().
				Str("channel_id", channel.ID).
				Str("alert_id", alert.ID).
				Str("phone", phone).
				Msg("短信通知发送成功")
		}
	}

	return nil
}

// sendSlackNotification 发送Slack通知
func (s *notificationService) sendSlackNotification(channel *models.NotificationChannel, alert *models.Alert) error {
	config := channel.Config

	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("Slack Webhook URL未配置")
	}

	// 根据告警级别设置颜色
	color := s.getAlertColor(alert.Level)

	// 构建Slack消息
	slackPayload := map[string]interface{}{
		"text": fmt.Sprintf("告警: %s", alert.Title),
		"attachments": []map[string]interface{}{
			{
				"color":      color,
				"title":      alert.Title,
				"text":       alert.Description,
				"fields": []map[string]interface{}{
					{"title": "级别", "value": strings.ToUpper(alert.Level), "short": true},
					{"title": "来源", "value": alert.Source, "short": true},
					{"title": "状态", "value": alert.Status, "short": true},
					{"title": "时间", "value": alert.CreatedAt.Format("2006-01-02 15:04:05"), "short": true},
				},
				"footer": "IoT Gateway Alert System",
				"ts":     alert.CreatedAt.Unix(),
			},
		},
	}

	// 序列化数据
	data, err := json.Marshal(slackPayload)
	if err != nil {
		return fmt.Errorf("序列化Slack数据失败: %w", err)
	}

	// 发送请求
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("创建Slack请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送Slack请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Slack响应错误: %d", resp.StatusCode)
	}

	log.Info().
		Str("channel_id", channel.ID).
		Str("alert_id", alert.ID).
		Msg("Slack通知发送成功")

	return nil
}

// sendDingTalkNotification 发送钉钉通知
func (s *notificationService) sendDingTalkNotification(channel *models.NotificationChannel, alert *models.Alert) error {
	config := channel.Config

	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		return fmt.Errorf("钉钉Webhook URL未配置")
	}

	// 构建钉钉消息
	message := fmt.Sprintf("## %s\n\n**级别**: %s\n\n**描述**: %s\n\n**来源**: %s\n\n**时间**: %s",
		alert.Title,
		strings.ToUpper(alert.Level),
		alert.Description,
		alert.Source,
		alert.CreatedAt.Format("2006-01-02 15:04:05"),
	)

	dingTalkPayload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"title": fmt.Sprintf("告警: %s", alert.Title),
			"text":  message,
		},
	}

	// 序列化数据
	data, err := json.Marshal(dingTalkPayload)
	if err != nil {
		return fmt.Errorf("序列化钉钉数据失败: %w", err)
	}

	// 发送请求
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("创建钉钉请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送钉钉请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("钉钉响应错误: %d", resp.StatusCode)
	}

	log.Info().
		Str("channel_id", channel.ID).
		Str("alert_id", alert.ID).
		Msg("钉钉通知发送成功")

	return nil
}

// TestChannel 测试通知渠道
func (s *notificationService) TestChannel(channelID string) error {
	// 创建测试告警
	testAlert := &models.Alert{
		ID:          "test-alert-" + channelID,
		Title:       "测试告警",
		Description: "这是一条测试告警消息，用于验证通知渠道配置是否正确。",
		Level:       "info",
		Status:      "active",
		Source:      "notification-test",
		Data: map[string]interface{}{
			"test": true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	return s.SendNotification(channelID, testAlert)
}

// GetChannels 获取通知渠道列表
func (s *notificationService) GetChannels() ([]models.NotificationChannel, error) {
	return s.alertService.GetNotificationChannels()
}

// buildEmailBody 构建邮件正文
func (s *notificationService) buildEmailBody(alert *models.Alert) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>IoT Gateway 告警通知</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .alert-header { background-color: %s; color: white; padding: 15px; border-radius: 5px; }
        .alert-body { border: 1px solid #ddd; padding: 15px; margin-top: 10px; border-radius: 5px; }
        .alert-field { margin: 10px 0; }
        .alert-label { font-weight: bold; color: #333; }
        .alert-value { margin-left: 10px; }
        .footer { margin-top: 20px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="alert-header">
        <h2>IoT Gateway 告警通知</h2>
        <h3>%s</h3>
    </div>
    <div class="alert-body">
        <div class="alert-field">
            <span class="alert-label">级别:</span>
            <span class="alert-value">%s</span>
        </div>
        <div class="alert-field">
            <span class="alert-label">描述:</span>
            <span class="alert-value">%s</span>
        </div>
        <div class="alert-field">
            <span class="alert-label">来源:</span>
            <span class="alert-value">%s</span>
        </div>
        <div class="alert-field">
            <span class="alert-label">状态:</span>
            <span class="alert-value">%s</span>
        </div>
        <div class="alert-field">
            <span class="alert-label">创建时间:</span>
            <span class="alert-value">%s</span>
        </div>
        <div class="alert-field">
            <span class="alert-label">告警ID:</span>
            <span class="alert-value">%s</span>
        </div>
    </div>
    <div class="footer">
        <p>此邮件由 IoT Gateway 告警系统自动发送，请勿回复。</p>
    </div>
</body>
</html>`,
		s.getAlertColor(alert.Level),
		alert.Title,
		strings.ToUpper(alert.Level),
		alert.Description,
		alert.Source,
		alert.Status,
		alert.CreatedAt.Format("2006-01-02 15:04:05"),
		alert.ID,
	)
}

// getAlertColor 获取告警级别对应的颜色
func (s *notificationService) getAlertColor(level string) string {
	switch level {
	case "critical":
		return "#d73027"
	case "error":
		return "#fc8d59"
	case "warning":
		return "#fee08b"
	case "info":
		return "#91bfdb"
	default:
		return "#999999"
	}
}