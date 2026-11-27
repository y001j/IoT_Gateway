package rules

import (
	"context"
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

type closingHandler struct{ closed int }

func (h *closingHandler) Name() string { return "closing" }
func (h *closingHandler) Execute(context.Context, model.Point, *Rule, map[string]interface{}) (*ActionResult, error) {
	return &ActionResult{Success: true}, nil
}
func (h *closingHandler) Close() { h.closed++ }

// 规则引擎停止时关闭动作处理器（聚合处理器在这里保存窗口状态）；以前从未调用
func TestStopClosesActionHandlers(t *testing.T) {
	s := NewRuleEngineService()
	h := &closingHandler{}
	s.RegisterActionHandler("closing", h)
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.closed != 1 {
		t.Fatalf("Stop 应调用处理器的 Close: %d", h.closed)
	}
}
