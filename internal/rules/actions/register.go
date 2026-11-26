package actions

import (
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nats-io/nats.go"
	"github.com/y001j/iot-gateway/internal/rules"
)

// RegisterBuiltinHandlers 把 actions 包中的完整动作处理器注册到规则引擎。
//
// 以前运行时只注册了 transform，filter 实际执行的是一个只打日志的空实现，
// forward、aggregate 走的是 service.go 里的简化回退实现，
// actions 包中完整实现的 FilterHandler / ForwardHandler / AggregateHandler 从未被使用。
//
// alert 使用规则引擎内置的 BuiltinAlertHandler（与 Web 告警服务对接），这里不覆盖。
// 必须在规则引擎 Start 之前调用。
func RegisterBuiltinHandlers(svc *rules.RuleEngineService, nc *nats.Conn) {
	RegisterBuiltinHandlersWithOptions(svc, nc, HandlerOptions{})
}

// HandlerOptions 动作处理器的可选配置
type HandlerOptions struct {
	// AggregateStateKV 不为空时，默认聚合实现把窗口状态保存到 JetStream KV（IOT_AGG_STATE），
	// 网关重启后恢复，避免长时间窗口的统计周期被打断
	AggregateStateKV nats.JetStreamContext
	// AggregateSaveInterval 保存间隔（默认 30 秒）
	AggregateSaveInterval time.Duration
}

// RegisterBuiltinHandlersWithOptions 同 RegisterBuiltinHandlers，可开启聚合状态持久化
func RegisterBuiltinHandlersWithOptions(svc *rules.RuleEngineService, nc *nats.Conn, opts HandlerOptions) {
	svc.RegisterActionHandler("filter", NewFilterHandler())
	svc.RegisterActionHandler("transform", NewTransformHandler(nc))
	svc.RegisterActionHandler("forward", NewForwardHandler(nc))
	agg := newAggregateActionHandler()
	if h, ok := agg.(*AggregateHandler); ok && opts.AggregateStateKV != nil {
		p, err := NewKVAggregateStatePersister(opts.AggregateStateKV)
		if err == nil {
			err = h.manager.EnablePersistence(p, opts.AggregateSaveInterval)
		}
		if err != nil {
			log.Warn().Err(err).Msg("聚合状态持久化未开启（重启后聚合窗口从零开始）")
		}
	} else if opts.AggregateStateKV != nil {
		log.Warn().Msg("分片聚合实现（IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE）不支持状态持久化")
	}
	svc.RegisterActionHandler("aggregate", agg)
}

// newAggregateActionHandler 选择聚合实现：
//   - 默认：AggregateHandler（增量统计，按规则+分组维护滑动/时间窗口）；
//   - 环境变量 IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE=true：OptimizedAggregateHandler（分片管理 + 高性能统计）。
//
// 以前第二种实现只能经由 service.go 中的回退路径使用，而 service.go 自己还有第三套聚合实现；
// 现在统一由动作处理器提供，service.go 中的重复实现已删除。
func newAggregateActionHandler() rules.ActionHandler {
	if os.Getenv("IOT_GATEWAY_ENABLE_OPTIMIZED_AGGREGATE") == "true" {
		return NewOptimizedAggregateHandler()
	}
	return NewAggregateHandler()
}
