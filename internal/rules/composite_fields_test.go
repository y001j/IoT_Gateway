package rules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// TestEvaluatorCompositeFields 条件中访问复合数据的子字段（<前缀>.<字段>）
func TestEvaluatorCompositeFields(t *testing.T) {
	now := time.Now()
	cases := []struct {
		point model.Point
		field string
		op    string
		value interface{}
	}{
		{pt("d", "gps", &model.LocationData{Latitude: 31.2, Longitude: 121.5, Altitude: 10, Accuracy: 3, Speed: 60, Heading: 90}, model.TypeLocation), "gps.latitude", "gt", 31.0},
		{pt("d", "gps", &model.LocationData{Latitude: 31.2, Longitude: 121.5, Speed: 60}, model.TypeLocation), "gps.speed", "eq", 60.0},
		{pt("d", "gps", &model.LocationData{Latitude: 31.2, Longitude: 121.5, Heading: 90}, model.TypeLocation), "gps.heading", "gte", 90.0},
		{pt("d", "gps", &model.LocationData{Latitude: 31.2, Longitude: 121.5, Altitude: 10, Accuracy: 3}, model.TypeLocation), "gps.altitude", "lt", 11.0},
		{pt("d", "acc", &model.Vector3D{X: 3, Y: 4, Z: 0}, model.TypeVector3D), "acc.x", "eq", 3.0},
		{pt("d", "acc", &model.Vector3D{X: 3, Y: 4, Z: 0}, model.TypeVector3D), "acc.magnitude", "eq", 5.0},
		{pt("d", "led", &model.ColorData{R: 255, G: 10, B: 0, A: 255}, model.TypeColor), "led.red", "eq", 255.0},
		{pt("d", "led", &model.ColorData{R: 255, G: 10, B: 0, A: 255}, model.TypeColor), "led.g", "lt", 20.0},
		{pt("d", "v", &model.VectorData{Values: []float64{1, 5, 2}, Dimension: 3, Labels: []string{"a", "b", "c"}}, model.TypeVector), "v.dimension", "eq", 3.0},
		{pt("d", "v", &model.VectorData{Values: []float64{1, 5, 2}, Dimension: 3, Labels: []string{"a", "b", "c"}}, model.TypeVector), "v.1", "eq", 5.0},
		{pt("d", "v", &model.VectorData{Values: []float64{1, 5, 2}, Dimension: 3, Labels: []string{"a", "b", "c"}}, model.TypeVector), "v.b", "eq", 5.0},
		{pt("d", "v", &model.VectorData{Values: []float64{1, 5, 2}, Dimension: 3}, model.TypeVector), "v.max", "eq", 5.0},
		{pt("d", "arr", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0}, DataType: "float", Size: 3}, model.TypeArray), "arr.size", "eq", 3.0},
		{pt("d", "arr", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0}, DataType: "float", Size: 3}, model.TypeArray), "arr.2", "eq", 3.0},
		{pt("d", "arr", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0}, DataType: "float", Size: 3}, model.TypeArray), "arr.data_type", "eq", "float"},
		{pt("d", "m", &model.MatrixData{Values: [][]float64{{1, 2}, {3, 4}}, Rows: 2, Cols: 2}, model.TypeMatrix), "m.rows", "eq", 2.0},
		{pt("d", "m", &model.MatrixData{Values: [][]float64{{1, 2}, {3, 4}}, Rows: 2, Cols: 2}, model.TypeMatrix), "m.cols", "eq", 2.0},
		{pt("d", "ts", &model.TimeSeriesData{Timestamps: []time.Time{now, now.Add(time.Second)}, Values: []float64{4, 6}, Unit: "C"}, model.TypeTimeSeries), "ts.length", "eq", 2.0},
		{pt("d", "ts", &model.TimeSeriesData{Timestamps: []time.Time{now, now.Add(time.Second)}, Values: []float64{4, 6}, Unit: "C"}, model.TypeTimeSeries), "ts.last_value", "eq", 6.0},
		{pt("d", "ts", &model.TimeSeriesData{Timestamps: []time.Time{now, now.Add(time.Second)}, Values: []float64{4, 6}, Unit: "C"}, model.TypeTimeSeries), "ts.first_value", "eq", 4.0},
		{pt("d", "ts", &model.TimeSeriesData{Timestamps: []time.Time{now, now.Add(time.Second)}, Values: []float64{4, 6}, Unit: "C"}, model.TypeTimeSeries), "ts.unit", "eq", "C"},
	}
	e := NewEvaluator()
	for _, c := range cases {
		ok, err := e.Evaluate(&Condition{Field: c.field, Operator: c.op, Value: c.value}, c.point)
		if err != nil || !ok {
			t.Errorf("%s %s %v: got %v err=%v", c.field, c.op, c.value, ok, err)
		}
	}

	// 不存在的子字段：返回错误，不 panic
	bad := []struct {
		point model.Point
		field string
	}{
		{pt("d", "gps", &model.LocationData{Latitude: 1, Longitude: 1}, model.TypeLocation), "gps.nope"},
		{pt("d", "acc", &model.Vector3D{}, model.TypeVector3D), "acc.w"},
		{pt("d", "led", &model.ColorData{}, model.TypeColor), "led.hue2"},
		{pt("d", "v", &model.VectorData{Values: []float64{1}, Dimension: 1}, model.TypeVector), "v.9"},
		{pt("d", "arr", &model.ArrayData{Values: []interface{}{1.0}, Size: 1}, model.TypeArray), "arr.9"},
		{pt("d", "m", &model.MatrixData{Values: [][]float64{{1}}, Rows: 1, Cols: 1}, model.TypeMatrix), "m.nope"},
		{pt("d", "ts", &model.TimeSeriesData{}, model.TypeTimeSeries), "ts.nope"},
	}
	for _, c := range bad {
		if ok, err := e.Evaluate(&Condition{Field: c.field, Operator: "eq", Value: 1.0}, c.point); err == nil && ok {
			t.Errorf("%s 不存在时不应匹配", c.field)
		}
	}
}

func TestRuleErrors(t *testing.T) {
	cause := errors.New("root")
	cases := []struct {
		err       *RuleError
		typ       ErrorType
		level     ErrorLevel
		retryable bool
	}{
		{NewConditionError("C1", "条件", cause), ErrorTypeCondition, ErrorLevelError, false},
		{NewActionError("A1", "动作", cause), ErrorTypeAction, ErrorLevelError, true},
		{NewValidationError("V1", "校验"), ErrorTypeValidation, ErrorLevelError, false},
		{NewSystemError("S1", "系统", cause), ErrorTypeSystem, ErrorLevelCritical, true},
		{NewNetworkError("N1", "网络", cause), ErrorTypeNetwork, ErrorLevelWarning, true},
		{NewTimeoutError("T1", "超时"), ErrorTypeTimeout, ErrorLevelWarning, true},
		{NewConfigError("F1", "配置"), ErrorTypeConfig, ErrorLevelError, false},
	}
	for _, c := range cases {
		if c.err.Type != c.typ || c.err.Level != c.level || IsRetryableError(c.err) != c.retryable {
			t.Errorf("%s: type=%s level=%s retryable=%v", c.err.Code, c.err.Type, c.err.Level, c.err.Retryable)
		}
		if GetErrorType(c.err) != c.typ || GetErrorLevel(c.err) != c.level {
			t.Errorf("%s: GetErrorType/GetErrorLevel 错误", c.err.Code)
		}
		if !strings.Contains(c.err.Error(), string(c.err.Type)) || !strings.Contains(c.err.Error(), c.err.Message) {
			t.Errorf("错误信息应包含类型和消息: %s", c.err.Error())
		}
	}
	wrapped := fmt.Errorf("外层: %w", NewActionError("A2", "x", cause).WithDetails("细节").WithContext("rule", "r1"))
	if !errors.Is(wrapped, cause) {
		t.Fatal("应能通过 Unwrap 找到根因")
	}
	if !IsRetryableError(wrapped) || GetErrorType(wrapped) != ErrorTypeAction {
		t.Fatal("包装后的规则错误应能识别类型和可重试")
	}
	plain := errors.New("plain")
	if IsRetryableError(plain) || GetErrorType(plain) != ErrorTypeSystem || GetErrorLevel(plain) != ErrorLevelError {
		t.Fatal("普通错误的默认分类错误")
	}
}

func TestRegexCache(t *testing.T) {
	rc := &RegexCache{cache: make(map[string]*RegexCacheEntry), maxSize: 2}
	for i := 0; i < 3; i++ {
		if _, err := rc.GetCompiledRegex(fmt.Sprintf("^p%d$", i)); err != nil {
			t.Fatal(err)
		}
	}
	rc.GetCompiledRegex("^p2$") // 命中
	rc.GetCompiledRegex("^p2$")
	if rc.CacheSize() > 2 {
		t.Fatalf("超过上限应淘汰最少使用的条目, size=%d", rc.CacheSize())
	}
	size, hitRate, requests, hits := rc.GetCacheStats()
	if size == 0 || requests != 5 || hits != 2 || hitRate <= 0 {
		t.Fatalf("统计错误: size=%d rate=%v req=%d hits=%d", size, hitRate, requests, hits)
	}
	if top := rc.GetTopPatterns(1); len(top) != 1 || top[0] != "^p2$" {
		t.Fatalf("最常用模式错误: %v", top)
	}
	if _, err := rc.GetCompiledRegex("("); err == nil {
		t.Fatal("无效正则应报错")
	}
	// 回归：以前淘汰的是得分最低（最近、最常用）的条目
	rc.SetMaxSize(1)
	if rc.CacheSize() != 1 || rc.GetTopPatterns(1)[0] != "^p2$" {
		t.Fatalf("缩小上限后应保留最常用的模式: %v", rc.GetTopPatterns(5))
	}
	rc.ClearCache()
	if rc.CacheSize() != 0 {
		t.Fatal("清空失败")
	}
	if ok, err := MatchString("^a+$", "aaa"); err != nil || !ok {
		t.Fatal("MatchString 错误")
	}
	if _, err := GetCompiledRegex("^x$"); err != nil {
		t.Fatal(err)
	}
	if size, _, _, _ := GetGlobalCacheStats(); size == 0 {
		t.Fatal("全局缓存应有条目")
	}
}
