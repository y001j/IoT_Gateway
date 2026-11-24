package actions

import (
	"context"
	"math"
	"testing"

	"github.com/y001j/iot-gateway/internal/model"
)

func TestTransformFormatMapUnitsAndOutputType(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		typ  model.DataType
		cfg  map[string]interface{}
		want interface{}
	}{
		{"format", 21.5, model.TypeFloat, map[string]interface{}{"type": "format", "parameters": map[string]interface{}{"format": "%.0f°C"}}, "22°C"},
		{"map", 1, model.TypeInt, map[string]interface{}{"type": "map", "parameters": map[string]interface{}{"mapping": map[string]interface{}{"1": "ON", "0": "OFF"}}}, "ON"},
		{"km->m", 1.5, model.TypeFloat, map[string]interface{}{"type": "unit_convert", "parameters": map[string]interface{}{"from": "km", "to": "m"}}, 1500.0},
		{"kg->g", 2.0, model.TypeFloat, map[string]interface{}{"type": "unit_convert", "parameters": map[string]interface{}{"from": "kg", "to": "g"}}, 2000.0},
		{"F->C", 212.0, model.TypeFloat, map[string]interface{}{"type": "unit_convert", "parameters": map[string]interface{}{"from": "F", "to": "C"}}, 100.0},
		{"pass_through + output_type", 3.7, model.TypeFloat, map[string]interface{}{"type": "pass_through", "output_type": "string"}, "3.7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runTransform(t, point("d", "k", c.in, c.typ), c.cfg)
			if f, ok := c.want.(float64); ok {
				gf, ok := got.Value.(float64)
				if !ok || math.Abs(gf-f) > 1e-6 {
					t.Fatalf("got %v (%T), want %v", got.Value, got.Value, c.want)
				}
				return
			}
			if got.Value != c.want {
				t.Fatalf("got %v (%T), want %v", got.Value, got.Value, c.want)
			}
		})
	}
	h := NewTransformHandler(nil)
	r, _ := h.Execute(context.Background(), point("d", "k", 1.0, model.TypeFloat), testRule,
		map[string]interface{}{"type": "unit_convert", "parameters": map[string]interface{}{"from": "km", "to": "kg"}})
	if r.Success {
		t.Errorf("不兼容的单位换算应返回失败")
	}
}

func TestTransformCompositeTypes(t *testing.T) {
	loc := model.NewCompositePoint("gps", "car", &model.LocationData{Latitude: 39.9042, Longitude: 116.4074, Speed: 30})
	dist := runTransform(t, loc, map[string]interface{}{"type": "geo_distance",
		"parameters": map[string]interface{}{"target_latitude": 31.2304, "target_longitude": 121.4737, "unit": "km"}}) // 默认单位为米
	if d, ok := dist.Value.(float64); !ok || math.Abs(d-1067) > 20 {
		t.Fatalf("geo_distance = %v (%T)", dist.Value, dist.Value)
	}
	speed := runTransform(t, loc, map[string]interface{}{"type": "extract_field", "parameters": map[string]interface{}{"field": "speed"}})
	if speed.Value != 30.0 {
		t.Fatalf("extract_field speed = %v", speed.Value)
	}

	vec := model.NewCompositePoint("acc", "imu", &model.Vector3D{X: 3, Y: 4, Z: 0})
	mag := runTransform(t, vec, map[string]interface{}{"type": "vector_magnitude"})
	if mag.Value != 5.0 {
		t.Fatalf("vector_magnitude = %v", mag.Value)
	}

	arr := model.NewCompositePoint("vals", "d", &model.ArrayData{Values: []interface{}{3.0, 1.0, 2.0}})
	sum := runTransform(t, arr, map[string]interface{}{"type": "array_aggregate", "parameters": map[string]interface{}{"operation": "sum"}})
	if f, _ := sum.Value.(float64); f != 6 {
		t.Fatalf("array_aggregate sum = %v (%T)", sum.Value, sum.Value)
	}
	sorted := runTransform(t, arr, map[string]interface{}{"type": "array_sort", "parameters": map[string]interface{}{"sort_order": "asc"}})
	sd, ok := sorted.Value.(*model.ArrayData)
	if !ok || sd.Values[0] != 1.0 || sd.Values[2] != 3.0 {
		t.Fatalf("array_sort = %+v", sorted.Value)
	}
	sliced := runTransform(t, arr, map[string]interface{}{"type": "array_slice", "parameters": map[string]interface{}{"slice_start": 1.0, "slice_end": 3.0}})
	if sl, ok := sliced.Value.(*model.ArrayData); !ok || len(sl.Values) != 2 {
		t.Fatalf("array_slice = %+v", sliced.Value)
	}
	if len(arr.Value.(*model.ArrayData).Values) != 3 || arr.Value.(*model.ArrayData).Values[0] != 3.0 {
		t.Fatalf("排序/切片不应修改原数据点的数组")
	}
}
