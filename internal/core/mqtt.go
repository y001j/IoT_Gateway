package core

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// mqttConfig 内嵌 NATS 服务器的 MQTT 监听配置（gateway.mqtt）。
//
// NATS 服务器内置 MQTT 3.1.1 服务端（QoS 0/1/2、保留消息、持久会话），MQTT 主题与 NATS 主题互通：
// MQTT 的 "/" 对应 NATS 的 "."，"+" 对应 "*"，"#" 对应 ">"。例如 MQTT 客户端订阅 iot/rules/#
// 就能收到规则引擎发布到 iot.rules.> 的结果；mqtt 连接器也可以把 broker 指向这个端口。
type mqttConfig struct {
	Enabled  bool
	Host     string
	Port     int
	Username string
	Password string
}

func (c mqttConfig) addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

// mqttConfigFromViper 读取并校验 gateway.mqtt 配置。默认关闭，只监听本机回环地址
func mqttConfigFromViper(v *viper.Viper) (mqttConfig, error) {
	cfg := mqttConfig{
		Enabled:  v.GetBool("gateway.mqtt.enabled"),
		Host:     v.GetString("gateway.mqtt.host"),
		Port:     1883,
		Username: v.GetString("gateway.mqtt.username"),
		Password: v.GetString("gateway.mqtt.password"),
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if v.IsSet("gateway.mqtt.port") {
		cfg.Port = v.GetInt("gateway.mqtt.port")
	}
	if !cfg.Enabled {
		return cfg, nil
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("gateway.mqtt.port 必须在 1-65535 之间: %d", cfg.Port)
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return cfg, fmt.Errorf("gateway.mqtt.username 和 password 需要同时配置")
	}
	return cfg, nil
}

// isLoopbackHost 判断监听地址是否只对本机开放
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// applyMQTTOptions 检查端口并把 MQTT 监听配置写入内嵌 NATS 服务器的选项。
// 端口被占用时返回错误，调用方应跳过 MQTT：nats-server 在 MQTT 端口监听失败时会调用 Fatalf 结束整个进程。
func applyMQTTOptions(opts *server.Options, cfg mqttConfig) error {
	ln, err := net.Listen("tcp", cfg.addr())
	if err != nil {
		return fmt.Errorf("MQTT 端口 %s 不可用: %w", cfg.addr(), err)
	}
	_ = ln.Close()

	if opts.ServerName == "" {
		opts.ServerName = "embedded-nats" // NATS 的 MQTT 功能要求设置 server_name
	}
	opts.MQTT.Host = cfg.Host
	opts.MQTT.Port = cfg.Port
	opts.MQTT.Username = cfg.Username
	opts.MQTT.Password = cfg.Password
	if !isLoopbackHost(cfg.Host) && cfg.Username == "" {
		log.Warn().Str("addr", cfg.addr()).Msg("内嵌 MQTT 对外开放且没有配置用户名密码，任何能访问该端口的设备都能读写网关数据")
	}
	return nil
}

// waitMQTTListening 等待 MQTT 端口开始监听（nats-server 在客户端端口就绪后才启动 MQTT 监听）
func waitMQTTListening(cfg mqttConfig, timeout time.Duration) error {
	host := cfg.Host
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("MQTT 端口 %s 没有开始监听: %w", cfg.addr(), err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// mqttHealth 返回 MQTT 组件的健康状态；未启用时不报告
func (rt *Runtime) mqttHealth() (level int, msg string, report bool) {
	if !rt.mqttEnabled {
		return 0, "", false
	}
	if rt.mqttAddr != "" {
		return 0, "监听 " + rt.mqttAddr, true
	}
	return 1, rt.mqttProblem, true
}
