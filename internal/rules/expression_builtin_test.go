package rules

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestExpressionBuiltinFunctions 通过表达式引擎调用每个内置函数（包括参数解析和结果比较）
func TestExpressionBuiltinFunctions(t *testing.T) {
	now := time.Now()
	vec3 := &model.Vector3D{X: 3, Y: 4, Z: 0}
	vec := &model.VectorData{Values: []float64{1, 2, 2}, Dimension: 3}
	arr := &model.ArrayData{Values: []interface{}{1.0, 5.0, "x", 3.0, nil}, DataType: "mixed", Size: 5}
	mat := &model.MatrixData{Values: [][]float64{{1, 2}, {3, 4}}, Rows: 2, Cols: 2}
	ts := &model.TimeSeriesData{Timestamps: []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}, Values: []float64{1, 2, 6}}
	color := &model.ColorData{R: 255, G: 0, B: 0, A: 255}
	loc := &model.LocationData{Latitude: 31.23, Longitude: 121.47}

	mk := func(v interface{}, typ model.DataType) model.Point {
		p := model.NewPoint("k", "dev-1", v, typ)
		p.AddTag("zone", "north")
		return p
	}
	num := mk(12.5, model.TypeFloat)
	str := mk("Hello World", model.TypeString)

	cases := []struct {
		expr  string
		point model.Point
	}{
		// 数学
		{"abs(-3) == 3", num}, {"max(1, 7, 3) == 7", num}, {"min(4, 2, 9) == 2", num},
		{"sqrt(16) == 4", num}, {"pow(2, 10) == 1024", num}, {"floor(2.7) == 2", num}, {"ceil(2.1) == 3", num},
		{"abs(value - 12.5) < 0.001", num},
		// 字符串
		{"len(value) == 11", str}, {"upper(value) == 'HELLO WORLD'", str}, {"lower(value) == 'hello world'", str},
		{"contains(value, 'World')", str}, {"startsWith(value, 'Hello')", str}, {"endsWith(value, 'World')", str},
		{"regex('^Hel+o', value)", str},
		// 字段和时间
		{"exists('value')", num}, {"now() > 0", num},
		{"timeDiff(now(), now() - 60) >= 59", num},
		{"len(timeFormat(now(), '2006')) == 4", num},
		{"time_range(0, 24) || !time_range(0, 24)", num},
		// 类型转换
		{"toString(12) == '12'", num}, {"toNumber('3.5') == 3.5", num}, {"toBool('true')", num}, {"!toBool(0)", num},
		// 统计和数据质量
		{"avg(2, 4, 6) == 4", num}, {"stddev(2, 4, 4, 4, 5, 5, 7, 9) > 1.9", num},
		{"!isNaN(value)", num}, {"!isInf(value)", num}, {"isFinite(value)", num},
		// 三维向量
		{"vectorMagnitude(value) == 5", mk(vec3, model.TypeVector3D)},
		{"vectorMagnitude(3, 4, 0) == 5", num},
		{"vectorDot(1, 0, 0, 0, 1, 0) == 0", num},
		{"abs(vectorAngle(1, 0, 0, 0, 1, 0) - 1.5707963) < 0.0001", num}, // 返回弧度
		{"vectorDistance(0, 0, 0, 3, 4, 0) == 5", num},
		{"vectorMagnitude3D(value) == 5", mk(vec3, model.TypeVector3D)},
		{"vectorDotProduct3D(value, value) == 25", mk(vec3, model.TypeVector3D)},
		// 通用向量
		{"genericVectorMagnitude(value) == 3", mk(vec, model.TypeVector)},
		{"vectorSum(value) == 5", mk(vec, model.TypeVector)},
		{"abs(vectorMean(value) - 1.6666667) < 0.001", mk(vec, model.TypeVector)},
		{"vectorMin(value) == 1", mk(vec, model.TypeVector)},
		{"vectorMax(value) == 2", mk(vec, model.TypeVector)},
		{"vectorDotProduct(value, value) == 9", mk(vec, model.TypeVector)},
		// 数组（非数值元素被忽略）
		{"arrayLength(value) == 5", mk(arr, model.TypeArray)},
		{"arraySum(value) == 9", mk(arr, model.TypeArray)},
		{"arrayMean(value) == 3", mk(arr, model.TypeArray)},
		{"arrayMin(value) == 1", mk(arr, model.TypeArray)},
		{"arrayMax(value) == 5", mk(arr, model.TypeArray)},
		{"arrayGet(value, 1) == 5", mk(arr, model.TypeArray)},
		// 矩阵
		{"matrixTrace(value) == 5", mk(mat, model.TypeMatrix)},
		{"matrixDeterminant(value) == -2", mk(mat, model.TypeMatrix)},
		{"matrixSum(value) == 10", mk(mat, model.TypeMatrix)},
		{"matrixMean(value) == 2.5", mk(mat, model.TypeMatrix)},
		{"matrixGet(value, 1, 0) == 3", mk(mat, model.TypeMatrix)},
		// 时间序列
		{"timeSeriesLength(value) == 3", mk(ts, model.TypeTimeSeries)},
		{"timeSeriesMean(value) == 3", mk(ts, model.TypeTimeSeries)},
		{"timeSeriesMin(value) == 1", mk(ts, model.TypeTimeSeries)},
		{"timeSeriesMax(value) == 6", mk(ts, model.TypeTimeSeries)},
		{"timeSeriesVariance(value) > 4", mk(ts, model.TypeTimeSeries)},
		{"timeSeriesStdDev(value) > 2", mk(ts, model.TypeTimeSeries)},
		// 复合数据通用函数
		{"compositeType(value) == 'vector3d'", mk(vec3, model.TypeVector3D)},
		{"compositeSize(value) == 4", mk(mat, model.TypeMatrix)},
		{"compositeValidate(value)", mk(color, model.TypeColor)},
		// 地理函数
		{"valid_coordinate(31.2, 121.5)", mk(loc, model.TypeLocation)},
		{"distance(31.23, 121.47, 39.9, 116.4) > 1000", num},
		// 标签与逻辑
		{"tags.zone == 'north' && value >= 12.5 && value <= 12.5", num},
		{"value > 100 || device_id == 'dev-1'", num},
	}
	e := NewExpressionEngine()
	for _, c := range cases {
		got, err := e.Evaluate(c.expr, c.point)
		if err != nil {
			t.Errorf("%s: 求值出错: %v", c.expr, err)
			continue
		}
		if got != true {
			t.Errorf("%s: 期望 true, 得到 %v", c.expr, got)
		}
	}
}

// TestExpressionFunctionResults 检查返回复合值的函数和错误分支
func TestExpressionFunctionResults(t *testing.T) {
	e := NewExpressionEngine()
	call := func(name string, args ...interface{}) (interface{}, error) {
		t.Helper()
		fn, ok := e.functions[name]
		if !ok {
			t.Fatalf("函数 %s 未注册", name)
		}
		if fn.Description() == "" {
			t.Errorf("函数 %s 缺少描述", name)
		}
		return fn.Call(args...)
	}

	// 回归：vectorNormalize 与通用向量版本重名，三维用法以前必然报错
	v, err := call("vectorNormalize", 3.0, 4.0, 0.0)
	if err != nil || !approx(v.(*model.Vector3D).X, 0.6) {
		t.Fatalf("vectorNormalize(x,y,z) 错误: %v %v", v, err)
	}
	v, err = call("vectorNormalize", &model.Vector3D{X: 0, Y: 0, Z: 2})
	if err != nil || v.(*model.Vector3D).Z != 1 {
		t.Fatalf("vectorNormalize(Vector3D) 错误: %v %v", v, err)
	}
	v, err = call("vectorNormalize", &model.VectorData{Values: []float64{0, 3, 4}, Dimension: 3})
	if err != nil || !approx(v.(*model.VectorData).Values[2], 0.8) {
		t.Fatalf("vectorNormalize(VectorData) 错误: %v %v", v, err)
	}
	if v, err := call("vectorNormalize", &model.VectorData{Values: []float64{0, 0}, Dimension: 2}); err != nil || v.(*model.VectorData).Values[0] != 0 {
		t.Fatalf("零向量归一化应保持为零: %v %v", v, err)
	}
	v, err = call("vectorCross", 1.0, 0.0, 0.0, 0.0, 1.0, 0.0)
	if err != nil || v.(*model.Vector3D).Z != 1 {
		t.Fatalf("vectorCross 错误: %v %v", v, err)
	}
	v, err = call("vectorCross3D", &model.Vector3D{X: 1}, &model.Vector3D{Y: 1})
	if err != nil || v.(*model.Vector3D).Z != 1 {
		t.Fatalf("vectorCross3D 错误: %v %v", v, err)
	}
	now := time.Now()
	trend, err := call("timeSeriesTrend", &model.TimeSeriesData{Timestamps: []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}, Values: []float64{1, 2, 3}})
	if err != nil || trend == nil {
		t.Fatalf("timeSeriesTrend 错误: %v %v", trend, err)
	}
	// 零向量夹角定义为 0；空时间序列均值为 0（不返回 NaN）
	if v, err := call("vectorAngle", 0.0, 0.0, 0.0, 1.0, 0.0, 0.0); err != nil || v != 0.0 {
		t.Fatalf("零向量夹角应为 0: %v %v", v, err)
	}
	if v, err := call("timeSeriesMean", &model.TimeSeriesData{}); err != nil || v != 0.0 {
		t.Fatalf("空时间序列均值应为 0: %v %v", v, err)
	}
	if n, err := call("arrayCount", &model.ArrayData{Values: []interface{}{1.0, nil, 2.0}, Size: 3}); err != nil || n == nil {
		t.Fatalf("arrayCount 错误: %v %v", n, err)
	}

	// 错误分支：参数个数或类型不对时返回错误而不是 panic
	bad := map[string][]interface{}{
		"abs": {"x"}, "sqrt": {-1.0}, "pow": {1.0}, "len": {}, "contains": {"a"},
		"toNumber": {"abc"}, "avg": {}, "regex": {"(", "x"},
		"vectorMagnitude": {"x"}, "vectorDot": {1.0}, "vectorNormalize": {}, "vectorAngle": {1.0, 2.0},
		"genericVectorMagnitude": {1.0}, "vectorSum": {"x"}, "vectorMin": {&model.VectorData{}},
		"vectorDotProduct": {&model.VectorData{Values: []float64{1}}, &model.VectorData{Values: []float64{1, 2}}},
		"arrayLength":      {1.0}, "arrayGet": {&model.ArrayData{Values: []interface{}{1.0}}, 5.0},
		"matrixTrace":    {&model.MatrixData{Values: [][]float64{{1, 2, 3}}, Rows: 1, Cols: 3}},
		"matrixGet":      {&model.MatrixData{Values: [][]float64{{1}}, Rows: 1, Cols: 1}, 3.0, 3.0},
		"timeSeriesMean": {"x"}, "compositeType": {}, "vectorCross3D": {1.0},
	}
	for name, args := range bad {
		if _, err := call(name, args...); err == nil {
			t.Errorf("%s(%v) 应返回错误", name, args)
		}
	}
	// 每个注册的函数都有名称和描述
	for name, fn := range e.functions {
		if fn.Name() != name && !strings.EqualFold(fn.Name(), name) {
			t.Errorf("函数注册名 %s 与 Name() %s 不一致", name, fn.Name())
		}
	}
}
