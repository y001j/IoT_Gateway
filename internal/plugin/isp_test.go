package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestISPMessageRoundTrip(t *testing.T) {
	cfgMsg, err := NewConfigMessage("c1", ConfigPayload{Mode: "tcp", Address: "1.2.3.4:502", IntervalMS: 1000,
		Registers: []RegisterConfig{{Key: "t", Address: 1, Quantity: 2, Type: "float32", Scale: 0.1}}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := cfgMsg.ToJSON()
	back, err := FromJSON(data)
	if err != nil || back.Type != MessageTypeConfig || back.ID != "c1" {
		t.Fatalf("配置消息往返失败: %v %+v", err, back)
	}
	cp, err := back.ParseConfigPayload()
	if err != nil || cp.Address != "1.2.3.4:502" || cp.Registers[0].Scale != 0.1 {
		t.Fatalf("配置载荷解析错误: %+v", cp)
	}

	dm, _ := NewDataMessage([]DataPoint{{Key: "t", Value: 21.5, Type: "float", Tags: map[string]string{"a": "b"}}})
	dp, err := dm.ParseDataPayload()
	if err != nil || len(dp.Points) != 1 || dp.Points[0].Value.(float64) != 21.5 {
		t.Fatalf("数据载荷解析错误: %+v", dp)
	}
	if v, ok := dp.Points[0].GetTag("a"); !ok || v != "b" {
		t.Fatalf("标签读取失败")
	}
	dp.Points[0].AddTag("c", "d")
	if dp.Points[0].GetTagsSafe()["c"] != "d" {
		t.Fatalf("AddTag 失败")
	}

	rm, _ := NewResponseMessage("r1", false, "boom", map[string]int{"x": 1})
	rp, err := rm.ParseResponsePayload()
	if err != nil || rp.Success || rp.Error != "boom" {
		t.Fatalf("响应载荷解析错误: %+v", rp)
	}

	mm, _ := NewMetricsMessage(MetricsPayload{DataPointsCollected: 10, ErrorsCount: 1})
	mp, err := mm.ParseMetricsPayload()
	if err != nil || mp.DataPointsCollected != 10 {
		t.Fatalf("指标载荷解析错误: %+v", mp)
	}
	for _, m := range []*ISPMessage{NewStatusMessage("s"), NewHeartbeatMessage(), NewMetricsRequestMessage("m")} {
		if m.Timestamp == 0 || m.Type == "" {
			t.Fatalf("消息缺少类型或时间戳: %+v", m)
		}
	}
	if _, err := FromJSON([]byte("{bad")); err == nil {
		t.Fatalf("非法 JSON 应报错")
	}
}

// fakeSidecar 模拟 sidecar：对 CONFIG 请求回复 RESPONSE，按需推送数据、心跳，并可主动断开
type fakeSidecar struct {
	ln         net.Listener
	mu         sync.Mutex
	conns      []net.Conn
	heartbeats int32
}

func newFakeSidecar(t *testing.T) *fakeSidecar {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSidecar{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, c)
			f.mu.Unlock()
			go f.serve(c)
		}
	}()
	t.Cleanup(func() { ln.Close(); f.closeAll() })
	return f
}

func (f *fakeSidecar) serve(c net.Conn) {
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		msg, err := FromJSON(line)
		if err != nil {
			continue
		}
		switch msg.Type {
		case MessageTypeConfig:
			resp, _ := NewResponseMessage(msg.ID, true, "", map[string]string{"ok": "1"})
			f.send(c, resp)
		case MessageTypeHeartbeat:
			atomic.AddInt32(&f.heartbeats, 1)
		case MessageTypeStatus:
			resp, _ := NewResponseMessage(msg.ID, true, "", StatusData{Name: "fake", Running: true, Connected: true, Health: "healthy"})
			f.send(c, resp)
		case MessageTypeMetrics:
			resp, _ := NewResponseMessage(msg.ID, true, "", map[string]interface{}{"data_points_collected": 42, "errors_count": 1, "connection_uptime": 60, "last_error": "x", "average_response_time": 3.5})
			f.send(c, resp)
		}
	}
}

func (f *fakeSidecar) send(c net.Conn, m *ISPMessage) {
	data, _ := m.ToJSON()
	c.Write(append(data, '\n'))
}

func (f *fakeSidecar) broadcast(m *ISPMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		f.send(c, m)
	}
}

func (f *fakeSidecar) closeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		c.Close()
	}
	f.conns = nil
}

func TestISPClientRequestDataAndHeartbeat(t *testing.T) {
	sc := newFakeSidecar(t)
	c := NewISPClient(sc.ln.Addr().String())
	if err := c.SendMessage(NewHeartbeatMessage()); err == nil {
		t.Fatalf("未连接时发送应报错")
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()

	cfgMsg, _ := NewConfigMessage("req-1", ConfigPayload{Mode: "tcp"})
	resp, err := c.SendRequest(cfgMsg, 2*time.Second)
	if err != nil || resp.ID != "req-1" {
		t.Fatalf("请求/响应失败: %v", err)
	}
	if _, err := c.SendRequest(&ISPMessage{Type: MessageTypeConfig}, time.Second); err == nil {
		t.Fatalf("没有 ID 的请求应报错")
	}

	var mu sync.Mutex
	var got []float64
	if err := c.Subscribe(func(m *ISPMessage) {
		p, _ := m.ParseDataPayload()
		mu.Lock()
		got = append(got, p.Points[0].Value.(float64))
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 50; i++ {
		dm, _ := NewDataMessage([]DataPoint{{Key: "k", Value: float64(i)}})
		sc.broadcast(dm)
	}
	// 超过 64KB 的大消息（以前 bufio.Scanner 会因 token too long 永久停止接收）
	big := strings.Repeat("x", 100*1024)
	bm, _ := NewDataMessage([]DataPoint{{Key: "big", Value: 1000.0, Tags: map[string]string{"blob": big}}})
	sc.broadcast(bm)
	sc.broadcast(NewHeartbeatMessage())

	testutil.WaitFor(t, 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 51
	}, "应收到 50 条数据和 1 条大消息")
	mu.Lock()
	for i := 0; i < 50; i++ {
		if got[i] != float64(i) {
			mu.Unlock()
			t.Fatalf("数据消息顺序错误: 第 %d 条为 %v", i, got[i])
		}
	}
	mu.Unlock()
	testutil.WaitFor(t, 2*time.Second, func() bool { return atomic.LoadInt32(&sc.heartbeats) >= 1 }, "应回复心跳")
}

func TestISPClientDetectsDisconnect(t *testing.T) {
	sc := newFakeSidecar(t)
	c := NewISPClient(sc.ln.Addr().String())
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	sc.closeAll() // sidecar 退出
	// 以前 EOF 后接收循环空转，连接状态一直显示为已连接
	testutil.WaitFor(t, 2*time.Second, func() bool { return !c.IsConnected() }, "sidecar 断开后客户端应标记为未连接")
	// 可以重新连接
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("重新连接失败: %v", err)
	}
	c.Disconnect()
	c.Disconnect()
}

func TestISPClientDisconnectDuringPendingRequest(t *testing.T) {
	// 一个只接受连接、从不回复的服务器
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				buf := make([]byte, 1024)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}()
		}
	}()
	c := NewISPClient(ln.Addr().String())
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		m, _ := NewConfigMessage("pending", ConfigPayload{})
		_, err := c.SendRequest(m, 5*time.Second)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	c.Disconnect() // 以前这里和 SendRequest 的 defer 会重复 close 通道而 panic
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("断开后等待中的请求应返回错误")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("断开后等待中的请求没有返回")
	}
}

var _ = json.Marshal
