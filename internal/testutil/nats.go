// Package testutil 提供单元测试使用的辅助工具（仅在测试中引用）。
package testutil

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// StartNATSWithMQTT 启动同时开启 MQTT 协议的内嵌 NATS 服务器（用作测试 MQTT broker），
// 返回 NATS 客户端和 MQTT 地址（tcp://127.0.0.1:port）。
func StartNATSWithMQTT(t testing.TB) (*nats.Conn, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	opts := &server.Options{
		ServerName:         "test-mqtt",
		Host:               "127.0.0.1",
		Port:               -1,
		NoLog:              true,
		NoSigs:             true,
		JetStream:          true,
		StoreDir:           t.TempDir(),
		JetStreamMaxMemory: 64 << 20,
		JetStreamMaxStore:  1 << 30,
	}
	opts.MQTT.Host = "127.0.0.1"
	opts.MQTT.Port = port
	_, nc := startWithOptions(t, opts)
	return nc, fmt.Sprintf("tcp://127.0.0.1:%d", port)
}

// StartNATS 在随机端口启动一个内嵌 NATS 服务器（开启 JetStream），
// 返回已连接的客户端。测试结束时自动关闭。
func StartNATS(t testing.TB) (*server.Server, *nats.Conn) {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		NoLog:     true,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  t.TempDir(),
		// 显式设置上限，避免服务器在 darwin 上探测系统内存
		// （该探测代码在 -race 的 checkptr 检查下会崩溃）
		JetStreamMaxMemory: 64 << 20,
		JetStreamMaxStore:  1 << 30,
	}
	return startWithOptions(t, opts)
}

func startWithOptions(t testing.TB, opts *server.Options) (*server.Server, *nats.Conn) {
	t.Helper()
	s, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("创建NATS服务器失败: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		t.Fatalf("NATS服务器启动超时")
	}
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		s.Shutdown()
		t.Fatalf("连接NATS失败: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		s.Shutdown()
		s.WaitForShutdown()
	})
	return s, nc
}

// WaitFor 轮询等待条件成立，超时则测试失败
func WaitFor(t testing.TB, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", msg)
}
