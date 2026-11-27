package actions

import (
	"context"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func execTransform(t *testing.T, point model.Point, cfg map[string]interface{}) (model.Point, *rules.ActionResult) {
	t.Helper()
	h := NewTransformHandler(nil)
	res, err := h.Execute(context.Background(), point, &rules.Rule{ID: "r"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		return model.Point{}, res
	}
	out := res.Output.(map[string]interface{})
	return out["transformed_point"].(model.Point), res
}

// 参数直接写在动作配置顶层（多数示例规则的写法）也能生效，parameters 中的键优先
func TestTransformFlatParameters(t *testing.T) {
	p := model.NewPoint("temperature", "d", 100.0, model.TypeFloat)
	got, res := execTransform(t, p, map[string]interface{}{"type": "unit_convert", "from": "C", "to": "F"})
	if res.Success != true || got.Value != 212.0 {
		t.Fatalf("顶层参数未生效: %+v %v", res, got.Value)
	}
	got, _ = execTransform(t, p, map[string]interface{}{"type": "scale", "factor": 3.0,
		"parameters": map[string]interface{}{"factor": 2.0}})
	if got.Value != 200.0 {
		t.Fatalf("parameters 应优先: %v", got.Value)
	}
	cfg := map[string]interface{}{"type": "scale", "factor": 2.0, "parameters": map[string]interface{}{}}
	execTransform(t, p, cfg)
	if len(cfg["parameters"].(map[string]interface{})) != 0 {
		t.Fatal("不应修改规则配置")
	}
	if m := mergeFlatParameters(map[string]interface{}{"type": "x"}, nil, transformReservedKeys); m == nil || len(m) != 0 {
		t.Fatalf("没有参数时应返回空 map: %v", m)
	}
}

// identity 是透传的别名；复合数据透传保留原值；不存在的类型不受 error_action 影响
func TestTransformIdentityAndPassThroughComposite(t *testing.T) {
	p := model.NewPoint("v", "d", 1.5, model.TypeFloat)
	got, res := execTransform(t, p, map[string]interface{}{"type": "identity", "add_tags": map[string]interface{}{"a": "1"}})
	if !res.Success || got.Value != 1.5 || got.GetTagsCopy()["a"] != "1" {
		t.Fatalf("identity: %+v", res)
	}
	raw := map[string]interface{}{"x": 1.0, "y": 2.0, "z": 2.0, "magnitude": 3.0}
	cp := model.Point{DeviceID: "d", Key: "acc", Type: model.TypeVector3D, Value: raw}
	got, res = execTransform(t, cp, map[string]interface{}{"add_tags": map[string]interface{}{"k": "v"}})
	if !res.Success || got.Value.(map[string]interface{})["magnitude"] != 3.0 || got.Type != model.TypeVector3D {
		t.Fatalf("复合数据透传应保留原值: %+v %+v", res, got)
	}
	_, res = execTransform(t, p, map[string]interface{}{"type": "nope", "error_action": "ignore"})
	if res.Success {
		t.Fatal("不存在的转换类型应报错")
	}
}

// field 选择要转换的字段：value.xxx、value.arr[0]、timestamp；数组字段逐个元素转换
func TestTransformFieldSelection(t *testing.T) {
	color := model.Point{DeviceID: "d", Key: "color", Type: model.TypeColor,
		Value: map[string]interface{}{"r": 1, "g": 2, "b": 3, "hue": 123.456}}
	got, res := execTransform(t, color, map[string]interface{}{"type": "round", "field": "value.hue", "precision": 1.0, "output_key": "hue"})
	if !res.Success || got.Value != 123.5 || got.Type != model.TypeFloat || got.Key != "hue" {
		t.Fatalf("value.hue: %+v %+v", res, got)
	}

	vec := model.NewCompositePoint("acc", "d", &model.Vector3D{X: 3, Y: 4})
	got, res = execTransform(t, vec, map[string]interface{}{"type": "scale", "field": "value.y", "factor": 2.0})
	if !res.Success || got.Value != 8.0 {
		t.Fatalf("类型化复合值的字段: %+v %+v", res, got)
	}

	arr := model.Point{DeviceID: "d", Key: "arr", Type: model.TypeArray,
		Value: map[string]interface{}{"elements": []float64{1.234, 5.678}}}
	got, res = execTransform(t, arr, map[string]interface{}{"type": "scale", "field": "value.elements", "factor": 1.0, "precision": 1.0})
	ad, ok := got.Value.(*model.ArrayData)
	if !res.Success || !ok || ad.Size != 2 || ad.Values[1] != 5.7 || got.Type != model.TypeArray {
		t.Fatalf("数组逐元素转换: %+v %+v", res, got)
	}
	got, _ = execTransform(t, arr, map[string]interface{}{"type": "scale", "field": "value.elements[1]", "factor": 2.0})
	if got.Value != 11.356 {
		t.Fatalf("数组下标: %v", got.Value)
	}

	ts := time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC)
	tp := model.Point{DeviceID: "d", Key: "loc", Type: model.TypeLocation, Value: map[string]interface{}{"latitude": 1.0}, Timestamp: ts}
	got, res = execTransform(t, tp, map[string]interface{}{"type": "format", "field": "timestamp", "format": "2006-01-02 15:04"})
	if !res.Success || got.Value != "2026-11-27 08:30" || got.Type != model.TypeString {
		t.Fatalf("格式化时间戳: %+v %+v", res, got)
	}

	for name, cfg := range map[string]map[string]interface{}{
		"字段不存在":   {"type": "round", "field": "value.nope"},
		"下标越界":    {"type": "round", "field": "value.elements[9]"},
		"下标非数字":   {"type": "round", "field": "value.elements[a]"},
		"元素转换失败":  {"type": "lookup", "field": "value.elements", "table": map[string]interface{}{}},
		"元素结果非数值": {"type": "format", "field": "value.elements", "format": "%v 伏"},
	} {
		if _, res := execTransform(t, arr, cfg); res.Success {
			t.Errorf("%s 应失败", name)
		}
	}
	if _, ok := compositeFieldValue(make(chan int), "value.x"); ok {
		t.Error("无法序列化的值")
	}
	if _, ok := compositeFieldValue(1.5, "value.x"); ok {
		t.Error("非对象的值")
	}
	if _, ok := compositeFieldValue(map[string]interface{}{"a": 1}, "value.a.b"); ok {
		t.Error("中间值不是对象")
	}
	if inferValueType(true) != model.TypeBool || inferValueType(int64(1)) != model.TypeInt || inferValueType(1.5) != model.TypeFloat {
		t.Error("inferValueType")
	}
	if _, ok := numericSlice([]interface{}{1, "x"}); ok {
		t.Error("含非数值元素")
	}
	if _, ok := numericSlice(1.5); ok {
		t.Error("非切片")
	}
}

// 表达式使用规则引擎的表达式引擎：支持 value、pow、round(x, n)、复合值字段；变量 x 等同于 value；
// 引擎不支持的函数回退到内置计算器
func TestTransformExpressionEngine(t *testing.T) {
	p := model.NewPoint("v", "d", 3.0, model.TypeFloat)
	for expr, want := range map[string]float64{
		"pow(value, 2) * 0.5":    4.5,
		"round(value * 1.1, 2)":  3.3,
		"max(0, 100 - (x / 2))":  98.5,
		"exp(0) + ln(1) + x":     4,
		"value * 2 + sin(0) * 0": 6,
	} {
		got, res := execTransform(t, p, map[string]interface{}{"type": "expression", "expression": expr})
		if !res.Success || got.Value != want {
			t.Errorf("%s = %v（%s），want %v", expr, got.Value, res.Error, want)
		}
	}
	vec := model.Point{DeviceID: "d", Key: "acc", Type: model.TypeVector3D, Value: map[string]interface{}{"x": 3.0, "y": 4.0, "z": 0.0}}
	got, res := execTransform(t, vec, map[string]interface{}{"type": "expression", "expression": "value.magnitude", "output_key": "m"})
	if !res.Success || got.Value != 5.0 || got.Type != model.TypeFloat {
		t.Fatalf("复合值表达式: %+v %+v", res, got)
	}
	for _, expr := range []string{"", "   ", "nosuchfunc(value)"} {
		if _, res := execTransform(t, p, map[string]interface{}{"type": "expression", "expression": expr}); res.Success {
			t.Errorf("%q 应失败", expr)
		}
	}
}

// filter 的顶层参数和 field
func TestFilterFlatParametersAndField(t *testing.T) {
	h := NewFilterHandler()
	exec := func(p model.Point, cfg map[string]interface{}) *rules.ActionResult {
		res, err := h.Execute(context.Background(), p, &rules.Rule{ID: "f"}, cfg)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	p := model.NewPoint("p", "d", 50.0, model.TypeFloat)
	res := exec(p, map[string]interface{}{"type": "range", "min": 0.0, "max": 100.0})
	if !res.Success {
		t.Fatalf("顶层参数未生效: %s", res.Error)
	}
	arr := model.Point{DeviceID: "d", Key: "a", Type: model.TypeArray, Value: map[string]interface{}{"elements": []interface{}{150.0}}}
	res = exec(arr, map[string]interface{}{"type": "range", "field": "value.elements[0]", "min": 0.0, "max": 100.0, "drop_on_match": false})
	out := res.Output.(map[string]interface{})
	if !res.Success || out["point"].(model.Point).Type != model.TypeArray {
		t.Fatalf("按字段过滤: %+v", res)
	}
	if res := exec(arr, map[string]interface{}{"type": "range", "field": "value.nope", "min": 0.0}); res.Success {
		t.Fatal("字段不存在应失败")
	}
}

// 聚合从复合数据中提取数值：类型化数据、[]float64、颜色亮度
func TestExtractNumericFromCompositeValues(t *testing.T) {
	for name, c := range map[string]struct {
		v    interface{}
		want float64
	}{
		"位置速度":      {&model.LocationData{Speed: 12}, 12},
		"三维向量模长":    {&model.Vector3D{X: 3, Y: 4}, 5},
		"颜色亮度":      {&model.ColorData{R: 255, G: 255, B: 255}, 100},
		"向量平均值":     {&model.VectorData{Values: []float64{1, 3}}, 2},
		"数组首元素":     {&model.ArrayData{Values: []interface{}{7.0}}, 7},
		"map x/y/z": {map[string]interface{}{"x": 3.0, "y": 4.0, "z": 0.0}, 5},
		"map 亮度":    {map[string]interface{}{"lightness": 50.0}, 50},
		"map 数值切片":  {map[string]interface{}{"values": []float64{2, 4}}, 3},
		"map 元素切片":  {map[string]interface{}{"elements": []float64{9, 1}}, 9},
	} {
		got, err := extractNumericValue(c.v)
		if err != nil || got != c.want {
			t.Errorf("%s = %v %v, want %v", name, got, err, c.want)
		}
	}
	for _, v := range []interface{}{&model.VectorData{}, &model.ArrayData{}, &model.MatrixData{}} {
		if _, err := extractNumericValue(v); err == nil {
			t.Errorf("%T 没有可用数值时应报错", v)
		}
	}
	if sliceElements(1.5) != nil {
		t.Error("非切片应返回 nil")
	}
}

// transform 的发布主题转义设备 ID 和键（以前直接拼接，含 "." 的设备 ID 多出主题层级）
func TestTransformPublishSubjectEscapesTokens(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, _ := nc.SubscribeSync("transformed.*.*")
	nc.Flush()
	h := NewTransformHandler(nc)
	p := model.NewPoint("temp.c", "line.1 a", 1.5, model.TypeFloat)
	if res, err := h.Execute(context.Background(), p, &rules.Rule{ID: "r"}, map[string]interface{}{"type": "scale", "factor": 2.0}); err != nil || !res.Success {
		t.Fatalf("%v %+v", err, res)
	}
	msg, err := sub.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatalf("应发布到两级转义后的主题: %v", err)
	}
	want := "transformed." + model.EncodeSubjectToken("line.1 a") + "." + model.EncodeSubjectToken("temp.c")
	if msg.Subject != want {
		t.Fatalf("主题 = %s, want %s", msg.Subject, want)
	}
}
