package actions

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

func tryTransform(t *testing.T, p model.Point, cfg map[string]interface{}) (model.Point, bool, string) {
	t.Helper()
	r, err := NewTransformHandler(nil).Execute(context.Background(), p, testRule, cfg)
	if err != nil {
		return model.Point{}, false, err.Error()
	}
	if !r.Success {
		return model.Point{}, false, r.Error
	}
	return output(t, r)["transformed_point"].(model.Point), true, ""
}

func num(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func TestCompositeTransformsNumericResults(t *testing.T) {
	loc := model.NewCompositePoint("gps", "car", &model.LocationData{Latitude: 39.9042, Longitude: 116.4074})
	vec := model.NewCompositePoint("v", "d", &model.VectorData{Values: []float64{1, 2, 3}, Dimension: 3})
	mat := model.NewCompositePoint("m", "d", &model.MatrixData{Values: [][]float64{{1, 2}, {3, 4}}, Rows: 2, Cols: 2})
	arr := model.NewCompositePoint("a", "d", &model.ArrayData{Values: []interface{}{1.0, 4.0, 9.0, 16.0}, Size: 4})
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := model.NewCompositePoint("ts", "d", &model.TimeSeriesData{
		Timestamps: []time.Time{base, base.Add(time.Second), base.Add(2 * time.Second), base.Add(3 * time.Second)},
		Values:     []float64{2, 4, 6, 8},
	})

	cases := []struct {
		name string
		p    model.Point
		cfg  map[string]interface{}
		want float64
	}{
		{"geo_bearing 正东", loc, map[string]interface{}{"type": "geo_bearing", "parameters": map[string]interface{}{"target_latitude": 39.9042, "target_longitude": 117.0}}, 90},
		{"vector_stats sum", vec, map[string]interface{}{"type": "vector_stats", "parameters": map[string]interface{}{"stat_type": "sum"}}, 6},
		{"vector_stats max", vec, map[string]interface{}{"type": "vector_stats", "parameters": map[string]interface{}{"stat_type": "max"}}, 3},
		{"vector_stats mean", vec, map[string]interface{}{"type": "vector_stats", "parameters": map[string]interface{}{"stat_type": "mean"}}, 2},
		{"vector_stats index", vec, map[string]interface{}{"type": "vector_stats", "parameters": map[string]interface{}{"stat_type": "index", "index": 1.0}}, 2},
		{"matrix trace", mat, map[string]interface{}{"type": "matrix_operation", "parameters": map[string]interface{}{"operation": "trace"}}, 5},
		{"matrix determinant", mat, map[string]interface{}{"type": "matrix_operation", "parameters": map[string]interface{}{"operation": "determinant"}}, -2},
		{"matrix element", mat, map[string]interface{}{"type": "matrix_operation", "parameters": map[string]interface{}{"operation": "element", "row": 1.0, "col": 0.0}}, 3},
		{"matrix sum", mat, map[string]interface{}{"type": "matrix_operation", "parameters": map[string]interface{}{"operation": "sum"}}, 10},
		{"timeseries mean", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "mean"}}, 5},
		{"timeseries latest", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "latest"}}, 8},
		{"timeseries range", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "range"}}, 6},
		{"timeseries length", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "length"}}, 4},
		{"vector_dot", vec, map[string]interface{}{"type": "vector_dot", "parameters": map[string]interface{}{"reference_x": 1.0, "reference_y": 1.0, "reference_z": 1.0}}, 6},
		{"array_aggregate max", arr, map[string]interface{}{"type": "array_aggregate", "parameters": map[string]interface{}{"operation": "max"}}, 16},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, msg := tryTransform(t, c.p, c.cfg)
			if !ok {
				t.Fatalf("执行失败: %s", msg)
			}
			f, isNum := num(got.Value)
			if !isNum || math.Abs(f-c.want) > 0.5 {
				t.Fatalf("got %v (%T), want %v", got.Value, got.Value, c.want)
			}
		})
	}
}

func TestCompositeTransformsStructuredResults(t *testing.T) {
	loc := model.NewCompositePoint("gps", "car", &model.LocationData{Latitude: 39.9042, Longitude: 116.4074})
	vec3 := model.NewCompositePoint("acc", "d", &model.Vector3D{X: 3, Y: 0, Z: 4})
	vec := model.NewCompositePoint("v", "d", &model.VectorData{Values: []float64{1, 0, 0}, Dimension: 3})
	color := model.NewCompositePoint("c", "d", &model.ColorData{R: 255})
	arr := model.NewCompositePoint("a", "d", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0, 100.0, 2.0, 1.0}, Size: 6})
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var tsT []time.Time
	var tsV []float64
	for i := 0; i < 24; i++ {
		tsT = append(tsT, base.Add(time.Duration(i)*time.Minute))
		tsV = append(tsV, float64(i%6))
	}
	ts := model.NewCompositePoint("ts", "d", &model.TimeSeriesData{Timestamps: tsT, Values: tsV})

	cases := []struct {
		name string
		p    model.Point
		cfg  map[string]interface{}
	}{
		{"geo_geofence", loc, map[string]interface{}{"type": "geo_geofence", "parameters": map[string]interface{}{"center_lat": 39.9, "center_lng": 116.4, "radius": 5000.0}}},
		{"geo_coordinate_convert", loc, map[string]interface{}{"type": "geo_coordinate_convert", "parameters": map[string]interface{}{"source_coordinate_system": "WGS84", "target_coordinate_system": "GCJ02"}}},
		{"vector_normalize", vec3, map[string]interface{}{"type": "vector_normalize"}},
		{"vector_projection", vec, map[string]interface{}{"type": "vector_projection", "parameters": map[string]interface{}{"reference_x": 1.0, "reference_y": 1.0, "reference_z": 0.0}}},
		{"vector_cross", vec, map[string]interface{}{"type": "vector_cross", "parameters": map[string]interface{}{"reference_x": 0.0, "reference_y": 1.0, "reference_z": 0.0}}},
		{"color_convert", color, map[string]interface{}{"type": "color_convert", "parameters": map[string]interface{}{"target_format": "hsv"}}},
		{"composite_to_array location", loc, map[string]interface{}{"type": "composite_to_array"}},
		{"composite_to_array vector3d", vec3, map[string]interface{}{"type": "composite_to_array"}},
		{"array_filter value_range", arr, map[string]interface{}{"type": "array_filter", "parameters": map[string]interface{}{"filter_type": "value_range", "min_value": 0.0, "max_value": 10.0}}},
		{"array_filter outliers", arr, map[string]interface{}{"type": "array_filter", "parameters": map[string]interface{}{"filter_type": "outliers", "outlier_method": "iqr"}}},
		{"array_smooth", arr, map[string]interface{}{"type": "array_smooth", "parameters": map[string]interface{}{"smooth_method": "moving_average", "smooth_window": 3.0}}},
		{"array_normalize minmax", arr, map[string]interface{}{"type": "array_normalize", "parameters": map[string]interface{}{"normalize_method": "minmax"}}},
		{"array_normalize zscore", arr, map[string]interface{}{"type": "array_normalize", "parameters": map[string]interface{}{"normalize_method": "zscore"}}},
		{"array_transform sqrt", arr, map[string]interface{}{"type": "array_transform", "parameters": map[string]interface{}{"transform_type": "sqrt"}}},
		{"timeseries moving_average", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "moving_average"}}},
		{"timeseries trend_analysis", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "trend_analysis"}}},
		{"timeseries anomaly_detection", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "anomaly_detection"}}},
		{"timeseries forecast", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "forecast"}}},
		{"timeseries seasonal_decompose", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "seasonal_decompose"}}},
		{"timeseries diff", ts, map[string]interface{}{"type": "timeseries_analysis", "parameters": map[string]interface{}{"analysis": "diff"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, msg := tryTransform(t, c.p, c.cfg)
			if !ok {
				t.Fatalf("执行失败: %s", msg)
			}
			if got.Value == nil {
				t.Fatalf("结果为空")
			}
			if f, isNum := num(got.Value); isNum && (math.IsNaN(f) || math.IsInf(f, 0)) {
				t.Fatalf("结果为非法数值: %v", f)
			}
		})
	}

	// 范围过滤后不应包含 100
	got, _, _ := tryTransform(t, arr, map[string]interface{}{"type": "array_filter", "parameters": map[string]interface{}{"filter_type": "value_range", "min_value": 0.0, "max_value": 10.0}})
	if ad, ok := got.Value.(*model.ArrayData); ok {
		for _, v := range ad.Values {
			if v == 100.0 {
				t.Fatalf("范围过滤未生效: %v", ad.Values)
			}
		}
	}
	// 归一化后在 [0,1]
	got, _, _ = tryTransform(t, arr, map[string]interface{}{"type": "array_normalize", "parameters": map[string]interface{}{"normalize_method": "minmax"}})
	if ad, ok := got.Value.(*model.ArrayData); ok {
		for _, v := range ad.Values {
			if f, _ := num(v); f < -1e-9 || f > 1+1e-9 {
				t.Fatalf("minmax 归一化越界: %v", ad.Values)
			}
		}
	}
}

func TestCompositeTransformErrors(t *testing.T) {
	loc := model.NewCompositePoint("gps", "car", &model.LocationData{Latitude: 1, Longitude: 1})
	arr := model.NewCompositePoint("a", "d", &model.ArrayData{Values: []interface{}{1.0}, Size: 1})
	for name, c := range map[string]struct {
		p   model.Point
		cfg map[string]interface{}
	}{
		"缺少目标坐标":       {loc, map[string]interface{}{"type": "geo_distance", "parameters": map[string]interface{}{}}},
		"类型不匹配":        {loc, map[string]interface{}{"type": "array_sort"}},
		"未知复合转换":       {arr, map[string]interface{}{"type": "no_such_transform"}},
		"未知矩阵操作":       {model.NewCompositePoint("m", "d", &model.MatrixData{Values: [][]float64{{1}}, Rows: 1, Cols: 1}), map[string]interface{}{"type": "matrix_operation", "parameters": map[string]interface{}{"operation": "explode"}}},
		"extract 未知字段": {loc, map[string]interface{}{"type": "extract_field", "parameters": map[string]interface{}{"field": "nonsense"}}},
	} {
		if _, ok, _ := tryTransform(t, c.p, c.cfg); ok {
			t.Errorf("%s 应返回失败", name)
		}
	}
}
