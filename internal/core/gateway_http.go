package core

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/utils"
)

// ComponentHealth 单个组件的健康状态
type ComponentHealth struct {
	Status  string `json:"status"` // healthy | degraded | unhealthy
	Message string `json:"message,omitempty"`
}

// HealthReport 健康检查结果
type HealthReport struct {
	Status     string                     `json:"status"`
	Timestamp  string                     `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

// HealthStatus 汇总各组件的真实健康状态。
// 以前 /health 固定返回 healthy，无法用于监控探针。
//
//   - NATS 未连接：unhealthy（HTTP 503）
//   - 运行时尚未启动完成：unhealthy
//   - 有适配器/连接器未运行、出现 slow consumer 丢消息：degraded（HTTP 200）
func (rt *Runtime) HealthStatus() HealthReport {
	report := HealthReport{
		Status:     "healthy",
		Timestamp:  time.Now().Format(time.RFC3339),
		Components: map[string]ComponentHealth{},
	}
	worst := 0 // 0 healthy, 1 degraded, 2 unhealthy
	set := func(name string, level int, msg string) {
		status := []string{"healthy", "degraded", "unhealthy"}[level]
		report.Components[name] = ComponentHealth{Status: status, Message: msg}
		if level > worst {
			worst = level
		}
	}

	if rt.Bus == nil || !rt.Bus.IsConnected() {
		set("nats", 2, "NATS 未连接")
	} else if drops := rt.slowConsumerDrops.Load(); drops > 0 {
		set("nats", 1, fmt.Sprintf("发生过 %d 次 slow consumer（消息被丢弃）", drops))
	} else {
		set("nats", 0, "")
	}

	if !rt.started.Load() {
		set("runtime", 2, "服务尚未启动完成")
	} else {
		set("runtime", 0, "")
	}

	if rt.PluginMgr != nil {
		var total, running int
		var stopped []string
		for _, p := range rt.PluginMgr.GetPlugins() {
			if p.Type != "adapter" && p.Type != "sink" {
				continue
			}
			total++
			if p.Status == "running" {
				running++
			} else {
				stopped = append(stopped, p.Name)
			}
		}
		if total > 0 && running < total {
			set("plugins", 1, fmt.Sprintf("%d/%d 个插件在运行，未运行: %v", running, total, stopped))
		} else {
			set("plugins", 0, fmt.Sprintf("%d 个插件在运行", running))
		}
		var dropped uint64
		for _, st := range rt.PluginMgr.GetSinkDispatchStats() {
			dropped += st.DroppedPoints
		}
		if dropped > 0 {
			set("sinks", 1, fmt.Sprintf("连接器队列已丢弃 %d 个数据点", dropped))
		}
	}

	// 本地暂存：积压逼近保留上限、有数据在写入目标前被淘汰、数据目录所在磁盘快满时降级。
	// 注意不按流的占用率判断：流是环形缓冲区，写满 max_bytes 是正常状态。
	if rt.PluginMgr != nil && rt.PluginMgr.GetStore() != nil {
		alerts, err := rt.PluginMgr.GetStore().Assess(rt.PluginMgr.GetBufferStats())
		switch {
		case err != nil:
			set("storage", 1, "无法读取本地暂存状态: "+err.Error())
		case len(alerts) > 0:
			msgs := make([]string, 0, len(alerts))
			for _, a := range alerts {
				msgs = append(msgs, a.Message)
			}
			set("storage", 1, strings.Join(msgs, "；"))
		default:
			set("storage", 0, "")
		}
	}

	for _, svc := range rt.Svcs {
		if rs, ok := svc.(*rules.RuleEngineService); ok {
			if d := rs.GetDroppedPoints(); d > 0 {
				set("rule_engine", 1, fmt.Sprintf("规则引擎过载，已丢弃 %d 个任务", d))
			} else {
				set("rule_engine", 0, "")
			}
		}
	}

	report.Status = []string{"healthy", "degraded", "unhealthy"}[worst]
	return report
}

// gatewayCORS 为 8080 端口的接口设置跨域头。
// 允许的来源由 web_ui.cors.allowed_origins 配置；未配置时不允许跨域（同源访问不受影响）。
func (rt *Runtime) gatewayCORS(w http.ResponseWriter, r *http.Request) bool {
	var allowed []string
	if rt.V != nil {
		allowed = rt.V.GetStringSlice("web_ui.cors.allowed_origins")
	}
	origin := r.Header.Get("Origin")
	if ok, wildcard := utils.OriginAllowed(origin, allowed); ok {
		if wildcard {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// GatewayHTTPHandler 返回 8080 端口的 HTTP 处理器（/metrics、/health、/info）
func (rt *Runtime) GatewayHTTPHandler(gatewayPort string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if rt.gatewayCORS(w, r) {
			return
		}
		if rt.metrics == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}
		rt.UpdateMetrics()
		switch format {
		case "json":
			w.Header().Set("Content-Type", "application/json")
			data, err := rt.metrics.ToJSON()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_, _ = w.Write(data)
		case "text", "plain":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(rt.metrics.ToPlainText()))
			if rt.PluginMgr != nil && rt.PluginMgr.GetStore() != nil {
				_, _ = w.Write([]byte(rt.PluginMgr.GetStore().PrometheusText(rt.PluginMgr.GetBufferStats())))
			}
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "不支持的格式，支持的格式: json, text, plain"})
		}
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if rt.gatewayCORS(w, r) {
			return
		}
		report := rt.HealthStatus()
		w.Header().Set("Content-Type", "application/json")
		if report.Status == "unhealthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	})

	// 本地暂存状态：持久化流占用和各缓冲连接器的积压/丢失情况
	mux.HandleFunc("/storage", func(w http.ResponseWriter, r *http.Request) {
		if rt.gatewayCORS(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if rt.PluginMgr == nil || rt.PluginMgr.GetStore() == nil {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"enabled": false})
			return
		}
		st, err := rt.PluginMgr.GetStore().Status()
		resp := map[string]interface{}{"enabled": true, "sinks": rt.PluginMgr.GetBufferStats()}
		if err != nil {
			resp["error"] = err.Error()
		} else {
			resp["stream"] = st
		}
		if ru := rt.PluginMgr.GetStore().Rollup(); ru != nil {
			resp["rollup"] = ru.Stats()
		}
		if alerts, err := rt.PluginMgr.GetStore().Assess(rt.PluginMgr.GetBufferStats()); err == nil {
			resp["alerts"] = alerts
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		if rt.gatewayCORS(w, r) {
			return
		}
		natsPort := "N/A"
		if rt.NatsServer != nil {
			if addr, ok := rt.NatsServer.Addr().(*net.TCPAddr); ok {
				natsPort = fmt.Sprintf("%d", addr.Port)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"name":         "IoT Gateway",
			"version":      "1.0.0",
			"nats_port":    natsPort,
			"gateway_port": gatewayPort,
		})
	})

	return mux
}
