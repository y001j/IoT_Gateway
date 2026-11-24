package mqtt_sub

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestMQTTSubAdapterReceivesAndParses(t *testing.T) {
	nc, broker := testutil.StartNATSWithMQTT(t)
	a := &MQTTSubAdapter{}
	cfg := fmt.Sprintf(`{"name":"sub","type":"mqtt_sub","broker":%q,"client_id":"sub-test","topics":[
		{"topic":"sensors/temp","key":"temperature","type":"float","path":"data.temp","device_id":"room1","qos":1},
		{"topic":"sensors/state","key":"running","type":"bool","path":"on","device_id":"room1"}]}`, broker)
	if err := a.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	ch := make(chan model.Point, 10)
	if err := a.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // 等待订阅生效

	// 通过 NATS 发布，NATS 的 MQTT 网关会投递给 MQTT 订阅者（sensors.temp ↔ sensors/temp）
	nc.Publish("sensors.temp", []byte(`{"data":{"temp":23.5}}`))
	nc.Publish("sensors.state", []byte(`{"on":true}`))
	nc.Flush()

	got := map[string]model.Point{}
	deadline := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case p := <-ch:
			got[p.Key] = p
		case <-deadline:
			t.Fatalf("超时，已收到: %+v", got)
		}
	}
	if got["temperature"].Value != 23.5 || got["temperature"].DeviceID != "room1" || got["temperature"].Type != model.TypeFloat {
		t.Fatalf("温度解析错误: %+v", got["temperature"])
	}
	if got["running"].Value != true {
		t.Fatalf("布尔解析错误: %+v", got["running"])
	}

	// Stop 后可再次 Start（以前停止通道只在 Init 中创建，再次 Stop 会 panic）
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestMQTTSubConfigValidation(t *testing.T) {
	a := &MQTTSubAdapter{}
	if err := a.Init(json.RawMessage(`{"name":"x","type":"mqtt_sub","broker":"tcp://127.0.0.1:1883","topics":[]}`)); err == nil {
		t.Fatalf("没有订阅主题时应报错")
	}
	if err := a.Init(json.RawMessage(`{"name":"x","type":"mqtt_sub","topics":[{"topic":"a","key":"k"}]}`)); err == nil {
		t.Fatalf("缺少 broker 时应报错")
	}
}
