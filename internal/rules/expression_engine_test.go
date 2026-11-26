package rules

import (
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

// round(x[, n]) 与 value.xxx 字段访问（复合值可能是 map 或结构体）
func TestExpressionRoundAndValueFields(t *testing.T) {
	eval := func(expr string, p model.Point) (interface{}, error) {
		e := AcquireExpressionEngine()
		defer ReleaseExpressionEngine(e)
		return e.Evaluate(expr, p)
	}
	num := model.NewPoint("v", "d", 2.345, model.TypeFloat)
	for expr, want := range map[string]interface{}{"round(value)": 2.0, "round(value, 2)": 2.35, "round(value, 1) > 2.2": true} {
		if got, err := eval(expr, num); err != nil || got != want {
			t.Errorf("%s = %v %v, want %v", expr, got, err, want)
		}
	}
	for _, expr := range []string{"round()", "round('a')", "round(value, 'a')"} {
		if _, err := eval(expr, num); err == nil {
			t.Errorf("%s 应报错", expr)
		}
	}

	mapVec := model.Point{Key: "acc", Type: model.TypeVector3D, Value: map[string]interface{}{"x": 3.0, "y": 4.0, "z": 0.0, "hue": 1.0}}
	typedVec := model.NewCompositePoint("acc", "d", &model.Vector3D{X: 3, Y: 4})
	for name, p := range map[string]model.Point{"map": mapVec, "结构体": typedVec} {
		for expr, want := range map[string]interface{}{"value.magnitude > 4.9": true, "value.y": 4.0, "acc.magnitude": 5.0} {
			if got, err := eval(expr, p); err != nil || got != want {
				t.Errorf("%s: %s = %v %v, want %v", name, expr, got, err, want)
			}
		}
	}
	if got, err := eval("value.hue", mapVec); err != nil || got != 1.0 {
		t.Errorf("map 中的额外字段: %v %v", got, err)
	}
	if _, err := eval("value.nope > 1", mapVec); err == nil {
		t.Error("不存在的字段应报错")
	}
	if _, err := eval("value.x > 1", num); err == nil {
		t.Error("标量值没有字段")
	}
	if _, ok := selectField(map[string]interface{}{"x": 1.0}, "magnitude"); ok {
		t.Error("缺少 y/z 时不能计算模长")
	}
	if _, ok := selectField(&struct{ C chan int }{}, "C"); ok {
		t.Error("无法序列化的结构体")
	}
}
