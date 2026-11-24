package model

import (
	"math"
	"testing"
	"time"
)

func TestColorDerivedValues(t *testing.T) {
	red := (&ColorData{R: 255}).GetDerivedValues()
	if red["hue"].(float64) != 0 || red["saturation"].(float64) != 1 || red["lightness"].(float64) != 0.5 {
		t.Fatalf("纯红色 HSL 错误: %v", red)
	}
	green := (&ColorData{G: 255}).GetDerivedValues()
	if green["hue"].(float64) != 120 {
		t.Fatalf("纯绿色色相 = %v", green["hue"])
	}
	gray := (&ColorData{R: 128, G: 128, B: 128}).GetDerivedValues()
	if gray["saturation"].(float64) != 0 {
		t.Fatalf("灰色饱和度应为 0")
	}
	c := &ColorData{}
	if c.Type() != TypeColor || c.Validate() != nil {
		t.Fatalf("颜色类型/校验错误")
	}
}

func TestMatrixDerivedValues(t *testing.T) {
	m := &MatrixData{Values: [][]float64{{1, 0}, {0, 1}}}
	if err := m.Validate(); err != nil || m.Rows != 2 || m.Cols != 2 {
		t.Fatalf("矩阵校验失败: %v", err)
	}
	d := m.GetDerivedValues()
	if d["trace"].(float64) != 2 || d["is_identity"] != true || d["is_diagonal"] != true || d["sum"].(float64) != 2 {
		t.Fatalf("单位矩阵衍生值错误: %v", d)
	}
	if err := (&MatrixData{Values: [][]float64{{1, 2}, {3}}}).Validate(); err == nil {
		t.Fatalf("列数不一致应报错")
	}
	if err := (&MatrixData{Values: [][]float64{{math.NaN()}}}).Validate(); err == nil {
		t.Fatalf("NaN 应报错")
	}
	if err := (&MatrixData{}).Validate(); err == nil {
		t.Fatalf("空矩阵应报错")
	}
	// 未经校验的空行不应 panic
	_ = (&MatrixData{Values: [][]float64{{}}}).GetDerivedValues()
	nonSquare := (&MatrixData{Values: [][]float64{{1, 2, 3}}, Rows: 1, Cols: 3}).GetDerivedValues()
	if nonSquare["is_square"] != false {
		t.Fatalf("非方阵判断错误")
	}
}

func TestTimeSeriesDerivedValues(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := &TimeSeriesData{
		Timestamps: []time.Time{base, base.Add(time.Second), base.Add(2 * time.Second)},
		Values:     []float64{1, 2, 3},
	}
	if err := ts.Validate(); err != nil {
		t.Fatal(err)
	}
	d := ts.GetDerivedValues()
	if d["trend"] != "increasing" || d["mean"].(float64) != 2 || d["avg_interval"] != "1s" {
		t.Fatalf("时间序列衍生值错误: %v", d)
	}
	bad := []*TimeSeriesData{
		{},
		{Timestamps: []time.Time{base}, Values: []float64{1, 2}},
		{Timestamps: []time.Time{base.Add(time.Second), base}, Values: []float64{1, 2}},
		{Timestamps: []time.Time{base}, Values: []float64{math.Inf(1)}},
	}
	for i, b := range bad {
		if b.Validate() == nil {
			t.Errorf("用例 %d 应校验失败", i)
		}
	}
	// 以前空数据会因下标越界 panic（NewCompositePoint 在校验前就计算衍生值）
	p := NewCompositePoint("ts", "d", &TimeSeriesData{})
	if p.Type != TypeTimeSeries {
		t.Fatalf("类型错误")
	}
}

func TestVectorAndArrayDerivedValues(t *testing.T) {
	v := &VectorData{Values: []float64{3, 4}, Labels: []string{"x", "y"}}
	if v.Validate() != nil || v.Type() != TypeVector {
		t.Fatalf("向量校验失败")
	}
	if d := v.GetDerivedValues(); d["dimension"] != 2 {
		t.Fatalf("向量维度 = %v", d["dimension"])
	}
	if (&VectorData{Values: []float64{1}, Labels: []string{"a", "b"}}).Validate() == nil {
		t.Fatalf("标签数量不匹配应报错")
	}
	a := &ArrayData{Values: []interface{}{1.0, nil, 3.0}, DataType: "float"}
	a.Validate()
	d := a.GetDerivedValues()
	if d["size"] != 3 || d["data_type"] != "float" {
		t.Fatalf("数组衍生值错误: %v", d)
	}
	if (&ArrayData{Values: []interface{}{1}, Size: 3}).Validate() == nil {
		t.Fatalf("数组大小不匹配应报错")
	}
	loc := (&LocationData{Altitude: 3000, Speed: 100}).GetDerivedValues()
	if loc["elevation_category"] == nil || loc["speed_category"] != "fast" {
		t.Fatalf("位置衍生值错误: %v", loc)
	}
	vec := (&Vector3D{X: 0, Y: 0, Z: -9.8}).GetDerivedValues()
	if vec["dominant_axis"] != "z" {
		t.Fatalf("主导轴 = %v", vec["dominant_axis"])
	}
}
