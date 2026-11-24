package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

func TestISPAdapterProxyEndToEnd(t *testing.T) {
	sc := newFakeSidecar(t)
	p, err := NewISPAdapterProxy("modbus-sidecar", sc.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Init(json.RawMessage(`{bad`)); err == nil {
		t.Fatalf("非法 JSON 配置应报错")
	}
	cfg := `{"plugin_config":{"protocol":"tcp","host":"10.0.0.1","port":502,"interval_ms":1000,
		"registers":[{"key":"temp","address":1,"type":"float32","device_id":1}]}}`
	if err := p.Init(json.RawMessage(cfg)); err != nil {
		t.Fatalf("Init（发送配置并等待 sidecar 确认）失败: %v", err)
	}
	ch := make(chan model.Point, 10)
	if err := p.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	if err := p.Start(context.Background(), ch); err == nil {
		t.Fatalf("重复启动应报错")
	}

	dm, _ := NewDataMessage([]DataPoint{
		{Key: "temp", Source: "plc1", Value: 21.5, Type: "float32", Tags: map[string]string{"unit": "C"}},
		{Key: "count", Source: "plc1", Value: 7.0, Type: "uint64", Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()},
		{Key: "weird", Source: "plc1", Value: 1, Type: "complex128"},
	})
	time.Sleep(50 * time.Millisecond)
	sc.broadcast(dm)

	got := map[string]model.Point{}
	deadline := time.After(3 * time.Second)
	for len(got) < 2 {
		select {
		case pt := <-ch:
			got[pt.Key] = pt
		case <-deadline:
			t.Fatalf("超时，已收到 %v", got)
		}
	}
	temp := got["temp"]
	if temp.Type != model.TypeFloat || temp.DeviceID != "plc1" || temp.Value != 21.5 {
		t.Fatalf("temp 转换错误: %+v", temp)
	}
	if v, _ := temp.GetTag("unit"); v != "C" {
		t.Fatalf("标签丢失")
	}
	if time.Since(temp.Timestamp) > time.Minute {
		t.Fatalf("缺少时间戳时应使用当前时间，而不是 1970 年: %v", temp.Timestamp)
	}
	cnt := got["count"]
	if cnt.Type != model.TypeInt || cnt.Timestamp.Year() != 2026 {
		t.Fatalf("uint64 数据点（以前会被丢弃）转换错误: %+v", cnt)
	}
	select {
	case pt := <-ch:
		t.Fatalf("未知类型的数据点应被丢弃: %+v", pt)
	case <-time.After(100 * time.Millisecond):
	}

	st, err := p.Status()
	if err != nil || st["connected"] != true || st["remote_status"] == nil {
		t.Fatalf("状态查询失败: %v %v", err, st)
	}
	m, err := p.GetMetrics()
	if err != nil {
		t.Fatalf("指标查询失败: %v", err)
	}
	b, _ := json.Marshal(m)
	if !json.Valid(b) || string(b) == "null" {
		t.Fatalf("指标为空")
	}
	var mm map[string]interface{}
	json.Unmarshal(b, &mm)
	if mm["data_points_collected"].(float64) != 42 {
		t.Fatalf("指标转换错误: %s", b)
	}

	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(); err != nil {
		t.Fatalf("重复停止应安全: %v", err)
	}
	p.Close()
}

func TestISPAdapterProxyInitFailsWithoutSidecar(t *testing.T) {
	p, _ := NewISPAdapterProxy("x", "127.0.0.1:1")
	if err := p.Init(json.RawMessage(`{}`)); err == nil {
		t.Fatalf("sidecar 不可达时 Init 应报错")
	}
}
