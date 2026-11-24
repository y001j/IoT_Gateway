package actions

import (
	"os"

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
	svc.RegisterActionHandler("filter", NewFilterHandler())
	svc.RegisterActionHandler("transform", NewTransformHandler(nc))
	svc.RegisterActionHandler("forward", NewForwardHandler(nc))
	svc.RegisterActionHandler("aggregate", newAggregateActionHandler())
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
