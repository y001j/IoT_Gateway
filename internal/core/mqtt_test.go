package core

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestMQTTConfigFromViper(t *testing.T) {
	v := viper.New()
	cfg, err := mqttConfigFromViper(v)
	if err != nil || cfg.Enabled || cfg.Host != "127.0.0.1" || cfg.Port != 1883 {
		t.Fatalf("默认值: %+v %v", cfg, err)
	}
	v.Set("gateway.mqtt.port", 0) // 未启用时不校验
	if _, err := mqttConfigFromViper(v); err != nil {
		t.Fatal(err)
	}
	for name, set := range map[string]map[string]interface{}{
		"端口为 0":  {"gateway.mqtt.port": 0},
		"端口超出范围": {"gateway.mqtt.port": 70000},
		"只配置用户名": {"gateway.mqtt.username": "u"},
		"只配置密码":  {"gateway.mqtt.password": "p"},
	} {
		bad := viper.New()
		bad.Set("gateway.mqtt.enabled", true)
		for k, val := range set {
			bad.Set(k, val)
		}
		if _, err := mqttConfigFromViper(bad); err == nil {
			t.Errorf("%s 应报错", name)
		}
	}
	v = viper.New()
	v.Set("gateway.mqtt.enabled", true)
	v.Set("gateway.mqtt.password", "p")
	v.Set("gateway.mqtt.username", "u")
	v.Set("gateway.mqtt.host", "0.0.0.0")
	cfg, err = mqttConfigFromViper(v)
	if err != nil || cfg.addr() != "0.0.0.0:1883" {
		t.Fatalf("%+v %v", cfg, err)
	}
	for host, want := range map[string]bool{"127.0.0.1": true, "::1": true, "localhost": true, "0.0.0.0": false, "192.168.1.2": false, "x": false} {
		if isLoopbackHost(host) != want {
			t.Errorf("isLoopbackHost(%s) != %v", host, want)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startEmbeddedWithMQTT 按网关的方式（applyMQTTOptions）启动带 MQTT 的 NATS 服务器
func startEmbeddedWithMQTT(t *testing.T, cfg mqttConfig) *nats.Conn {
	t.Helper()
	// 显式设置 JetStream 上限：否则 nats-server 读取系统内存，其 macOS 实现在 -race（checkptr）下崩溃
	opts := &server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, JetStream: true, StoreDir: t.TempDir(),
		JetStreamMaxMemory: 64 << 20, JetStreamMaxStore: 1 << 30}
	if err := applyMQTTOptions(opts, cfg); err != nil {
		t.Fatal(err)
	}
	if opts.ServerName == "" || opts.MQTT.Port != cfg.Port {
		t.Fatalf("MQTT 选项未设置: %+v", opts.MQTT)
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	t.Cleanup(srv.Shutdown)
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("NATS 未就绪")
	}
	if err := waitMQTTListening(cfg, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func mqttClient(t *testing.T, cfg mqttConfig, user, pass string) (mqtt.Client, error) {
	t.Helper()
	o := mqtt.NewClientOptions().AddBroker("tcp://" + cfg.addr()).
		SetClientID(fmt.Sprintf("test-%d", time.Now().UnixNano())).SetConnectTimeout(3 * time.Second).SetAutoReconnect(false)
	if user != "" {
		o.SetUsername(user).SetPassword(pass)
	}
	c := mqtt.NewClient(o)
	tok := c.Connect()
	if !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		return nil, fmt.Errorf("连接失败: %v", tok.Error())
	}
	t.Cleanup(func() { c.Disconnect(100) })
	return c, nil
}

// MQTT 与 NATS 主题互通：MQTT 客户端能收到 iot.rules.> / iot.alerts.> 的消息，MQTT 发布的消息也进入 NATS
func TestEmbeddedMQTTBridgesNATS(t *testing.T) {
	cfg := mqttConfig{Enabled: true, Host: "127.0.0.1", Port: freePort(t)}
	nc := startEmbeddedWithMQTT(t, cfg)
	c, err := mqttClient(t, cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// NATS → MQTT（外部系统订阅规则引擎结果和告警）
	got := make(chan string, 4)
	if tok := c.Subscribe("iot/alerts/#", 1, func(_ mqtt.Client, m mqtt.Message) {
		got <- m.Topic() + " " + string(m.Payload())
	}); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("订阅失败: %v", tok.Error())
	}
	time.Sleep(100 * time.Millisecond)
	if err := nc.Publish("iot.alerts.triggered.warning", []byte(`{"level":"warning"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != `iot/alerts/triggered/warning {"level":"warning"}` {
			t.Fatalf("MQTT 收到的消息错误: %s", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MQTT 客户端没有收到 NATS 发布的告警")
	}

	// MQTT → NATS（设备或 mqtt 连接器发布）
	sub, _ := nc.SubscribeSync("iot.data.dev1.temp")
	nc.Flush()
	if tok := c.Publish("iot/data/dev1/temp", 1, false, `{"value":21.5}`); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("发布失败: %v", tok.Error())
	}
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil || string(msg.Data) != `{"value":21.5}` {
		t.Fatalf("NATS 没有收到 MQTT 发布的消息: %v %v", msg, err)
	}
}

func TestEmbeddedMQTTAuth(t *testing.T) {
	user, pass := "gw", fmt.Sprintf("p-%d", time.Now().UnixNano())
	cfg := mqttConfig{Enabled: true, Host: "127.0.0.1", Port: freePort(t), Username: user, Password: pass}
	startEmbeddedWithMQTT(t, cfg)
	if _, err := mqttClient(t, cfg, "", ""); err == nil {
		t.Fatal("没有凭据时应拒绝连接")
	}
	if _, err := mqttClient(t, cfg, user, "wrong"); err == nil {
		t.Fatal("密码错误时应拒绝连接")
	}
	if _, err := mqttClient(t, cfg, user, pass); err != nil {
		t.Fatalf("正确凭据应能连接: %v", err)
	}
}

// 端口被占用时跳过 MQTT（nats-server 监听失败会结束整个进程）；对外开放时也能设置选项
func TestApplyMQTTOptionsPortBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	cfg := mqttConfig{Enabled: true, Host: "127.0.0.1", Port: busy}
	if err := applyMQTTOptions(&server.Options{}, cfg); err == nil || !strings.Contains(err.Error(), "不可用") {
		t.Fatalf("端口被占用应报错: %v", err)
	}
	opts := &server.Options{ServerName: "keep"}
	open := mqttConfig{Enabled: true, Host: "0.0.0.0", Port: freePort(t)}
	if err := applyMQTTOptions(opts, open); err != nil || opts.ServerName != "keep" || opts.MQTT.Host != "0.0.0.0" {
		t.Fatalf("%+v %v", opts.MQTT, err)
	}
	// 0.0.0.0 时通过回环地址检查监听；端口没有监听时超时报错
	closed := mqttConfig{Host: "0.0.0.0", Port: freePort(t)}
	if err := waitMQTTListening(closed, 200*time.Millisecond); err == nil {
		t.Fatal("没有监听时应超时")
	}
}

func TestMQTTHealthComponent(t *testing.T) {
	rt := newTestRuntime(t)
	rt.started.Store(true)
	if _, ok := rt.HealthStatus().Components["mqtt"]; ok {
		t.Fatal("未启用时不应报告 mqtt 组件")
	}
	rt.mqttEnabled, rt.mqttAddr = true, "127.0.0.1:1883"
	if c := rt.HealthStatus().Components["mqtt"]; c.Status != "healthy" || c.Message != "监听 127.0.0.1:1883" {
		t.Fatalf("运行中: %+v", c)
	}
	rt.mqttAddr, rt.mqttProblem = "", "端口被占用"
	got := rt.HealthStatus()
	if got.Components["mqtt"].Status != "degraded" || got.Components["mqtt"].Message != "端口被占用" || got.Status != "degraded" {
		t.Fatalf("未启动: %+v", got)
	}
}

func writeMQTTRuntimeConfig(t *testing.T, natsURL string, mqttPort int) string {
	t.Helper()
	dir := t.TempDir()
	cfg := fmt.Sprintf(`
gateway:
  name: test
  log_level: error
  nats_url: %q
  http_port: %d
  plugins_dir: %q
  mqtt:
    enabled: true
    port: %d
storage:
  store_dir: %q
rule_engine:
  enabled: false
web_ui:
  enabled: false
`, natsURL, freePort(t), filepath.Join(dir, "plugins"), mqttPort, filepath.Join(dir, "js"))
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// 使用外部 NATS 时 gateway.mqtt 不生效，健康状态说明原因
func TestRuntimeMQTTWithExternalNATS(t *testing.T) {
	srv, _ := testutil.StartNATS(t)
	rt, err := NewRuntime(writeMQTTRuntimeConfig(t, srv.ClientURL(), freePort(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Bus.Close()
	rt.started.Store(true)
	c := rt.HealthStatus().Components["mqtt"]
	if c.Status != "degraded" || !strings.Contains(c.Message, "外部 NATS") {
		t.Fatalf("mqtt 组件: %+v", c)
	}
	if _, err := NewRuntime(writeMQTTRuntimeConfig(t, srv.ClientURL(), 70000)); err == nil {
		t.Fatal("非法端口应返回配置错误")
	}
}

// 内嵌模式启动 MQTT。4222 端口上已有 NATS 服务器（例如本机正在运行网关）时，网关会连接它而不启动内嵌服务器，
// 这时应说明 MQTT 没有启动
func TestRuntimeEmbeddedMQTT(t *testing.T) {
	if runtime.GOOS == "darwin" && raceEnabled {
		t.Skip("nats-server 在 macOS 上读取系统内存的代码不兼容 -race 的 checkptr 检查（Linux 上正常运行）")
	}
	mqttPort := freePort(t)
	rt, err := NewRuntime(writeMQTTRuntimeConfig(t, "embedded", mqttPort))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		rt.Bus.Close()
		if rt.NatsServer != nil {
			rt.NatsServer.Shutdown()
		}
	}()
	rt.started.Store(true)
	c := rt.HealthStatus().Components["mqtt"]
	if rt.NatsServer == nil {
		if c.Status != "degraded" || !strings.Contains(c.Message, "已有 NATS 服务器") {
			t.Fatalf("连接到已有服务器时应说明 MQTT 未启动: %+v", c)
		}
		t.Skip("4222 端口上已有 NATS 服务器，跳过内嵌 MQTT 的收发验证")
	}
	if c.Status != "healthy" {
		t.Fatalf("mqtt 组件: %+v", c)
	}
	cfg := mqttConfig{Host: "127.0.0.1", Port: mqttPort}
	cl, err := mqttClient(t, cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := rt.Bus.SubscribeSync("iot.rules.test")
	rt.Bus.Flush()
	if tok := cl.Publish("iot/rules/test", 0, false, "hi"); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatal(tok.Error())
	}
	if m, err := sub.NextMsg(5 * time.Second); err != nil || string(m.Data) != "hi" {
		t.Fatalf("内嵌 MQTT 消息未进入 NATS: %v %v", m, err)
	}
}
