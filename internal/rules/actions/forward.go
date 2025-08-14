package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// ForwardHandler Forward动作处理器 (简化版本 - 只支持NATS转发)
type ForwardHandler struct {
	natsConn *nats.Conn
}

// NewForwardHandler 创建Forward处理器
func NewForwardHandler(natsConn *nats.Conn) *ForwardHandler {
	return &ForwardHandler{
		natsConn: natsConn,
	}
}

// Name 返回处理器名称
func (h *ForwardHandler) Name() string {
	return "forward"
}

// Execute 执行转发动作 (简化版本 - 只支持NATS转发)
func (h *ForwardHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	start := time.Now()

	if h.natsConn == nil {
		return &rules.ActionResult{
			Type:     "forward",
			Success:  false,
			Error:    "NATS连接未初始化",
			Duration: time.Since(start),
		}, nil
	}

	// 获取目标主题
	subject, ok := config["subject"].(string)
	if !ok || subject == "" {
		subject = fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
	}

	// 准备转发数据
	forwardData := map[string]interface{}{
		"device_id": point.DeviceID,
		"key":       point.Key,
		"value":     point.Value,
		"type":      string(point.Type),
		"timestamp": point.Timestamp,
		"tags":      rules.SafeValueForJSON(point.GetTagsCopy()), // 使用安全的JSON转换
		"rule_info": map[string]interface{}{
			"rule_id":   rule.ID,
			"rule_name": rule.Name,
			"action":    "forward",
		},
		"processed_at": time.Now(),
	}

	// 序列化并发送
	jsonData, err := json.Marshal(forwardData)
	if err != nil {
		return &rules.ActionResult{
			Type:     "forward",
			Success:  false,
			Error:    fmt.Sprintf("序列化数据失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	if err := h.natsConn.Publish(subject, jsonData); err != nil {
		return &rules.ActionResult{
			Type:     "forward",
			Success:  false,
			Error:    fmt.Sprintf("发送NATS消息失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	log.Debug().
		Str("rule_id", rule.ID).
		Str("subject", subject).
		Int("bytes", len(jsonData)).
		Msg("Forward动作执行成功")

	return &rules.ActionResult{
		Type:     "forward",
		Success:  true,
		Duration: time.Since(start),
		Output: map[string]interface{}{
			"subject":    subject,
			"bytes_sent": len(jsonData),
		},
	}, nil
}