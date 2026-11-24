package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestMQTTSinkPublishesNormalizedValues(t *testing.T) {
	// 内嵌 NATS 开启 MQTT 协议作为测试 broker：MQTT 主题 iot/a/b 对应 NATS 主题 iot.a.b
	nc, broker := testutil.StartNATSWithMQTT(t)
	sub, err := nc.SubscribeSync("iot.>")
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	s := NewMQTTSink()
	cfg := fmt.Sprintf(`{"name":"m","type":"mqtt","buffer_size":100,"tags":{"site":"A"},
		"params":{"broker":%q,"client_id":"gw-test","topic_tpl":"iot/%%s/%%s","qos":1,"password":"secret-pass"}}`, broker)
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if err := s.Healthy(); err != nil {
		t.Fatalf("连接后应健康: %v", err)
	}

	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := model.NewPoint("speed", "plc1", int64(1500), model.TypeInt)
	p.Timestamp = ts
	if err := s.Publish([]model.Point{p}); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("broker 未收到消息: %v", err)
	}
	if msg.Subject != "iot.plc1.speed" {
		t.Fatalf("主题 = %s", msg.Subject)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("payload 不是 JSON: %s", msg.Data)
	}
	// 回归：int64 以前被发布为 0
	if payload["value"].(float64) != 1500 {
		t.Fatalf("value = %v", payload["value"])
	}
	tags, _ := payload["tags"].(map[string]interface{})
	if tags["site"] != "A" {
		t.Fatalf("应附加配置的标签: %v", payload)
	}
}

func TestMQTTSinkStartDoesNotHangWhenBrokerDown(t *testing.T) {
	s := NewMQTTSink()
	s.startTimeout = 300 * time.Millisecond
	if err := s.Init(json.RawMessage(`{"name":"m","type":"mqtt","params":{"broker":"tcp://127.0.0.1:1","client_id":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- s.Start(context.Background()) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("broker 不可达时 Start 无限阻塞")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("Start 等待时间过长: %v", time.Since(start))
	}
	if s.Healthy() == nil {
		t.Fatalf("未连接时不应健康")
	}
	s.Stop()
}

func TestMQTTSinkPublishBeforeStart(t *testing.T) {
	s := NewMQTTSink()
	if err := s.Publish([]model.Point{{Key: "k"}}); err == nil {
		t.Fatalf("未启动时 Publish 应报错")
	}
}

func TestMQTTWriteBatchWaitsForAck(t *testing.T) {
	nc, broker := testutil.StartNATSWithMQTT(t)
	sub, _ := nc.SubscribeSync("gw.>")
	nc.Flush()
	s := NewMQTTSink()
	cfg := fmt.Sprintf(`{"name":"m","type":"mqtt","params":{"broker":%q,"client_id":"wb","topic_tpl":"gw/%%s/%%s","qos":1}}`, broker)
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteBatch(context.Background(), []model.Point{model.NewPoint("k", "d", 1, model.TypeInt)}); err == nil {
		t.Fatalf("未连接时 WriteBatch 应报错")
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	batch := []model.Point{model.NewPoint("a", "d", 1, model.TypeInt), model.NewPoint("b", "d", 2, model.TypeInt)}
	if err := s.WriteBatch(context.Background(), batch); err != nil {
		t.Fatalf("同步发布失败: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := sub.NextMsg(3 * time.Second); err != nil {
			t.Fatalf("broker 未收到第 %d 条: %v", i, err)
		}
	}
}
