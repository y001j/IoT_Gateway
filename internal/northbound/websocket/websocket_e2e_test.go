package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/y001j/iot-gateway/internal/model"
)

func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestWebSocketSinkBroadcasts(t *testing.T) {
	port := freePort(t)
	s := NewWebSocketSink()
	cfg := fmt.Sprintf(`{"name":"ws","type":"websocket","params":{"address":"127.0.0.1:%d","path":"/ws",
		"points":{"temp":{"topic":"custom/temp","transform":"scale","scale_factor":10}}}}`, port)
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	var conn *websocket.Conn
	var err error
	for i := 0; i < 50; i++ {
		conn, _, err = websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("连接 WebSocket 失败: %v", err)
	}
	defer conn.Close()

	// 跨域连接默认被拒绝
	hdr := http.Header{"Origin": {"http://evil.com"}}
	if _, _, err := websocket.DefaultDialer.Dial(url, hdr); err == nil {
		t.Fatalf("跨域连接应被拒绝")
	}

	time.Sleep(50 * time.Millisecond) // 等待注册
	p := model.NewPoint("temp", "dev.1", int64(2), model.TypeInt)
	if err := s.Publish([]model.Point{p, model.NewPoint("hum", "dev.1", 55.0, model.TypeFloat)}); err != nil {
		t.Fatal(err)
	}
	got := map[string]WebSocketMessage{}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for len(got) < 2 {
		var msg WebSocketMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("读取消息失败: %v (已收到 %v)", err, got)
		}
		got[msg.Key] = msg
	}
	if got["temp"].Topic != "custom/temp" || got["temp"].Value.(float64) != 20 {
		t.Fatalf("点位配置（主题/缩放）未生效: %+v", got["temp"])
	}
	if got["hum"].Topic != model.DataSubject("dev.1", "hum") {
		t.Fatalf("默认主题 = %q", got["hum"].Topic)
	}
}

func TestWebSocketSinkHealthy(t *testing.T) {
	s := NewWebSocketSink()
	if err := s.Healthy(); err == nil {
		t.Fatal("未运行时应不健康")
	}
	cfg := fmt.Sprintf(`{"name":"ws","type":"websocket","params":{"address":"127.0.0.1:%d","path":"/ws"}}`, freePort(t))
	if err := s.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Healthy(); err != nil {
		t.Fatalf("应健康: %v", err)
	}
	_ = s.Stop()
	if err := s.Healthy(); err == nil {
		t.Fatal("停止后应不健康")
	}
}
