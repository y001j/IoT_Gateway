package model

import (
	"encoding/json"
	"testing"
)

// 经过 JSON 传输的复合值是 map，GetCompositeData 应能还原（以前直接报错，规则中的复合数据转换全部失败）
func TestGetCompositeDataFromMap(t *testing.T) {
	cases := []struct {
		typ   DataType
		value map[string]interface{}
		check func(CompositeData) bool
	}{
		{TypeLocation, map[string]interface{}{"latitude": 39.9, "longitude": 116.4, "speed": 30.0, "extra": "x"},
			func(c CompositeData) bool { l := c.(*LocationData); return l.Latitude == 39.9 && l.Speed == 30 }},
		{TypeVector3D, map[string]interface{}{"x": 1.0, "y": 2.0, "z": 2.0, "magnitude": 3.0},
			func(c CompositeData) bool { v := c.(*Vector3D); return v.X == 1 && v.Z == 2 }},
		{TypeColor, map[string]interface{}{"r": 255.0, "g": 127.6, "b": -5, "hue": 30.0},
			func(c CompositeData) bool {
				cd := c.(*ColorData)
				return cd.R == 255 && cd.G == 128 && cd.B == 0 && cd.A == 255
			}},
		{TypeVector, map[string]interface{}{"values": []interface{}{1.0, 2.0}, "labels": []interface{}{"a", "b"}},
			func(c CompositeData) bool { v := c.(*VectorData); return v.Dimension == 2 && v.Labels[1] == "b" }},
		{TypeArray, map[string]interface{}{"elements": []float64{5, 6, 7}, "unit": "V"},
			func(c CompositeData) bool {
				a := c.(*ArrayData)
				return a.Size == 3 && a.Values[0] == 5.0 && a.Unit == "V"
			}},
		{TypeMatrix, map[string]interface{}{"values": [][]float64{{1, 2}, {3, 4}}},
			func(c CompositeData) bool { m := c.(*MatrixData); return m.Rows == 2 && m.Cols == 2 }},
		{TypeTimeSeries, map[string]interface{}{"values": []interface{}{1.0}, "timestamps": []interface{}{"2026-01-01T00:00:00Z"}},
			func(c CompositeData) bool {
				ts := c.(*TimeSeriesData)
				return len(ts.Values) == 1 && ts.Timestamps[0].Year() == 2026
			}},
	}
	for _, c := range cases {
		p := Point{Type: c.typ, Value: c.value}
		got, err := p.GetCompositeData()
		if err != nil {
			t.Fatalf("%s: %v", c.typ, err)
		}
		if !c.check(got) {
			t.Errorf("%s: 转换结果错误 %+v", c.typ, got)
		}
	}

	// 原始 map 中带有 elements 时不应被修改
	m := map[string]interface{}{"elements": []interface{}{1.0}}
	if _, err := CompositeFromMap(TypeArray, m); err != nil || m["values"] != nil {
		t.Fatalf("不应修改原 map: %v %v", m, err)
	}

	// JSON 往返后能还原
	orig := NewCompositePoint("acc", "d", &Vector3D{X: 3, Y: 4})
	data, _ := json.Marshal(orig)
	var decoded Point
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if cd, err := decoded.GetCompositeData(); err != nil || cd.(*Vector3D).Y != 4 {
		t.Fatalf("JSON 往返后无法还原: %v %v", cd, err)
	}

	// 值类型（非指针）的结构体
	for _, v := range []interface{}{LocationData{}, Vector3D{}, ColorData{}, VectorData{}, ArrayData{}, MatrixData{}, TimeSeriesData{}} {
		p := Point{Type: TypeLocation, Value: v}
		if _, err := p.GetCompositeData(); err != nil {
			t.Errorf("%T: %v", v, err)
		}
	}

	// 错误情况
	for name, p := range map[string]Point{
		"非复合类型":   {Type: TypeFloat, Value: map[string]interface{}{}},
		"值类型错误":   {Type: TypeVector3D, Value: 1.5},
		"字段类型错误":  {Type: TypeVector3D, Value: map[string]interface{}{"x": "abc"}},
		"颜色分量非数值": {Type: TypeColor, Value: map[string]interface{}{"r": "red"}},
		"无法序列化":   {Type: TypeVector3D, Value: map[string]interface{}{"x": make(chan int)}},
	} {
		if _, err := p.GetCompositeData(); err == nil {
			t.Errorf("%s 应返回错误", name)
		}
	}
	if _, err := CompositeFromMap(TypeInt, map[string]interface{}{}); err == nil {
		t.Error("非复合类型应返回错误")
	}
	for _, v := range []interface{}{float32(1), 1, int64(1), int32(1), uint8(1), json.Number("1")} {
		if f, ok := toFloat(v); !ok || f != 1 {
			t.Errorf("toFloat(%T) = %v %v", v, f, ok)
		}
	}
	if _, ok := toFloat("1"); ok {
		t.Error("字符串不应转换")
	}
}
