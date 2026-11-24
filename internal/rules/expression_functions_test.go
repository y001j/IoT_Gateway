package rules

import (
	"math"
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

func evalExpr(t *testing.T, expr string, p model.Point) interface{} {
	t.Helper()
	eng := AcquireExpressionEngine()
	defer ReleaseExpressionEngine(eng)
	v, err := eng.Evaluate(expr, p)
	if err != nil {
		t.Fatalf("%q: %v", expr, err)
	}
	return v
}

func asFloat(t *testing.T, v interface{}) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	t.Fatalf("不是数字: %v (%T)", v, v)
	return 0
}

func TestExpressionMathAndStringFunctions(t *testing.T) {
	p := pt("dev-1", "temperature", -12.5, model.TypeFloat)
	numeric := map[string]float64{
		"abs(value)":        12.5,
		"max(1, 7, 3)":      7,
		"min(4, 2, 9)":      2,
		"sqrt(16)":          4,
		"pow(2, 10)":        1024,
		"floor(2.7)":        2,
		"ceil(2.1)":         3,
		"len('abcd')":       4,
		"toNumber('42.5')":  42.5,
		"avg(2, 4, 6)":      4,
		"abs(value) * 2":    25,
		"(1 + 2) * 3 - 4/2": 7,
		"10 % 4":            2,
	}
	for expr, want := range numeric {
		if got := asFloat(t, evalExpr(t, expr, p)); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s = %v, want %v", expr, got, want)
		}
	}
	truthy := []string{
		"upper('abc') == 'ABC'",
		"lower('ABC') == 'abc'",
		"contains(device_id, 'dev')",
		"startsWith(key, 'temp')",
		"endsWith(key, 'ture')",
		"toString(5) == '5'",
		"toBool('true')",
		"isFinite(value)",
		"!isNaN(value)",
		"regex('^dev-[0-9]+$', device_id)", // 文档约定：regex(pattern, text)
		"exists('value')",
		"stddev(2, 4, 4, 4, 5, 5, 7, 9) > 1.9",
	}
	for _, expr := range truthy {
		if v := evalExpr(t, expr, p); v != true {
			t.Errorf("%s = %v, want true", expr, v)
		}
	}
}

func TestExpressionCompositeFunctions(t *testing.T) {
	vec := model.NewCompositePoint("acc", "d", &model.Vector3D{X: 3, Y: 4, Z: 12})
	if got := asFloat(t, evalExpr(t, "vectorMagnitude(value)", vec)); got != 13 {
		t.Errorf("vectorMagnitude = %v", got)
	}
	arr := model.NewCompositePoint("arr", "d", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0, 6.0}})
	if got := asFloat(t, evalExpr(t, "arraySum(value)", arr)); got != 12 {
		t.Errorf("arraySum = %v", got)
	}
	if got := asFloat(t, evalExpr(t, "arrayMax(value)", arr)); got != 6 {
		t.Errorf("arrayMax = %v", got)
	}
	loc := model.NewCompositePoint("gps", "d", &model.LocationData{Latitude: 39.9042, Longitude: 116.4074})
	d := asFloat(t, evalExpr(t, "distance(39.9042, 116.4074, 31.2304, 121.4737)", loc))
	if math.Abs(d-1067) > 15 {
		t.Errorf("distance = %v km, 约应为 1067", d)
	}
}

func TestExpressionUnknownFunction(t *testing.T) {
	eng := AcquireExpressionEngine()
	defer ReleaseExpressionEngine(eng)
	if _, err := eng.Evaluate("noSuchFunction(1)", pt("d", "k", 1.0, model.TypeFloat)); err == nil {
		t.Fatalf("未知函数应报错")
	}
}
