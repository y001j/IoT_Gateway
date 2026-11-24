package websocket

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

func TestConvertValue(t *testing.T) {
	s := NewWebSocketSink()
	if v := s.convertValue(model.Point{Type: model.TypeInt, Value: int64(5)}, PointConfig{}); v.(int64) != 5 {
		t.Fatalf("int64 = %v（以前被转换为 0）", v)
	}
	if v := s.convertValue(model.Point{Type: model.TypeInt, Value: int64(5)}, PointConfig{Transform: "scale", ScaleFactor: 0.5}); v.(float64) != 2.5 {
		t.Fatalf("scale = %v", v)
	}
	loc := &model.LocationData{Latitude: 1, Longitude: 2}
	if v := s.convertValue(model.Point{Type: model.TypeLocation, Value: loc}, PointConfig{}); v != loc {
		t.Fatalf("复合类型应保留原结构，而不是格式化成字符串")
	}
}

func TestDefaultOriginPolicy(t *testing.T) {
	s := NewWebSocketSink()
	cfg := `{"name":"ws","type":"websocket","params":{"address":"127.0.0.1:0","path":"/ws"}}`
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	same := httptest.NewRequest("GET", "http://gw:8092/ws", nil)
	same.Host = "gw:8092"
	same.Header.Set("Origin", "http://gw:8092")
	if !s.upgrader.CheckOrigin(same) {
		t.Fatalf("同源连接应允许")
	}
	evil := httptest.NewRequest("GET", "http://gw:8092/ws", nil)
	evil.Host = "gw:8092"
	evil.Header.Set("Origin", "http://evil.com")
	if s.upgrader.CheckOrigin(evil) {
		t.Fatalf("未配置 allow_origins 时不应允许任意跨域连接")
	}
}
