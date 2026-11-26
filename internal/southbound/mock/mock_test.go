package mock

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
)

func initMock(t *testing.T, cfg string) *MockAdapter {
	t.Helper()
	a := NewAdapter().(*MockAdapter)
	if err := a.Init(json.RawMessage(cfg)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return a
}

// collectRounds 启动适配器，收集每个 key 的若干条数据
func collectRounds(t *testing.T, a *MockAdapter, keys, rounds int) map[string][]model.Point {
	t.Helper()
	ch := make(chan model.Point, keys*rounds+10)
	if err := a.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Stop() }()
	got := map[string][]model.Point{}
	deadline := time.After(5 * time.Second)
	for n := 0; n < keys*rounds; n++ {
		select {
		case p := <-ch:
			got[p.Key] = append(got[p.Key], p)
		case <-deadline:
			t.Fatalf("超时，只收到 %d 条: %v", n, got)
		}
	}
	return got
}

// 仓库 config.yaml 中的复合模拟点位以前全部被转换成 float 0
func TestMockCompositeTypesKeepTheirType(t *testing.T) {
	a := initMock(t, `{"name":"m","type":"mock","interval":"10ms","tags":{"site":"s1"},"data_points":[
	  {"device_id":"d","key":"temp","min_value":10,"max_value":20},
	  {"device_id":"d","key":"loc","data_type":"location","location_config":{"start_latitude":31,"start_longitude":121,"latitude_range":0.01,"longitude_range":0.01,"altitude_min":1,"altitude_max":5,"speed_min":1,"speed_max":3,"simulate_movement":true,"movement_pattern":"random_walk"}},
	  {"device_id":"d","key":"acc","data_type":"vector3d","vector3d_config":{"x_min":-1,"x_max":1,"y_min":-1,"y_max":1,"z_min":9,"z_max":10,"correlation":0.5,"oscillation":true,"frequency":1}},
	  {"device_id":"d","key":"led","data_type":"color","color_config":{"color_mode":"rainbow","hue_change_speed":10}},
	  {"device_id":"d","key":"vec","data_type":"vector","vector_config":{"dimension":3,"global_min":0,"global_max":1,"labels":["a","b","c"],"change_pattern":"walk"}},
	  {"device_id":"d","key":"arr","data_type":"array","array_config":{"size":4,"element_type":"float","min_value":0,"max_value":10,"change_elements":2}},
	  {"device_id":"d","key":"mat","data_type":"matrix","matrix_config":{"rows":2,"cols":3,"min_value":0,"max_value":1}},
	  {"device_id":"d","key":"ts","data_type":"timeseries","timeseries_config":{"length":5,"interval":"1s","base_value":10,"trend":0.1,"noise":0.1}}
	]}`)
	got := collectRounds(t, a, 8, 3)
	want := map[string]model.DataType{
		"temp": model.TypeFloat, "loc": model.TypeLocation, "acc": model.TypeVector3D, "led": model.TypeColor,
		"vec": model.TypeVector, "arr": model.TypeArray, "mat": model.TypeMatrix, "ts": model.TypeTimeSeries,
	}
	for key, typ := range want {
		ps := got[key]
		if len(ps) == 0 {
			t.Fatalf("%s 没有数据", key)
		}
		for _, p := range ps {
			if p.Type != typ {
				t.Fatalf("%s 类型 = %s, want %s (value %#v)", key, p.Type, typ, p.Value)
			}
			if p.DeviceID != "d" {
				t.Fatalf("%s 设备 = %s", key, p.DeviceID)
			}
			tags := p.GetTagsCopy()
			if tags["source"] != "mock" || tags["site"] != "s1" {
				t.Fatalf("%s 标签 = %v", key, tags)
			}
			if typ == model.TypeFloat {
				if v := p.Value.(float64); v < 10 || v > 20 {
					t.Fatalf("temp 超出范围: %v", v)
				}
				continue
			}
			cd, ok := p.Value.(model.CompositeData)
			if !ok {
				t.Fatalf("%s 值不是复合数据: %#v", key, p.Value)
			}
			if err := cd.Validate(); err != nil {
				t.Fatalf("%s 校验失败: %v (%#v)", key, err, cd)
			}
		}
	}
	vec := got["vec"][0].Value.(*model.VectorData)
	if len(vec.Values) != 3 {
		t.Fatalf("vector 维度 = %d", len(vec.Values))
	}
	mat := got["mat"][0].Value.(*model.MatrixData)
	if mat.Rows != 2 || mat.Cols != 3 {
		t.Fatalf("matrix = %dx%d", mat.Rows, mat.Cols)
	}
}

// 缺少复合配置的点位被跳过，不会发送空值或导致 panic
func TestMockCompositeWithoutConfigIsSkipped(t *testing.T) {
	a := initMock(t, `{"name":"m","type":"mock","interval":"10ms","data_points":[
	  {"device_id":"d","key":"ok","min_value":0,"max_value":1},
	  {"device_id":"d","key":"loc","data_type":"location"},
	  {"device_id":"d","key":"weird","data_type":"unknown"}
	]}`)
	got := collectRounds(t, a, 1, 3)
	if len(got["ok"]) != 3 || len(got["loc"]) != 0 {
		t.Fatalf("got %v", got)
	}
	// 未知复合类型生成 nil，同样被跳过
	if _, ok := got["weird"]; ok {
		t.Fatal("未知复合类型不应发送")
	}
}

func TestMockBasicTypeConversions(t *testing.T) {
	a := NewMockAdapter()
	a.interval = 10 * time.Millisecond
	a.deviceID = "fallback"
	a.points = []mockPoint{
		{Key: "c_int", Type: "int", Constant: 3.7},
		{Key: "c_int64", Type: "int", Constant: int64(4)},
		{Key: "c_f32int", Type: "int", Constant: float32(5.5)},
		{Key: "c_badint", Type: "int", Constant: "x"},
		{Key: "c_bool", Type: "bool", Constant: 1},
		{Key: "c_boolf", Type: "bool", Constant: 0.0},
		{Key: "c_boolb", Type: "bool", Constant: true},
		{Key: "c_boolx", Type: "bool", Constant: "x"},
		{Key: "c_str", Type: "string", Constant: 12},
		{Key: "c_strs", Type: "string", Constant: "s"},
		{Key: "c_fint", Type: "float", Constant: 2},
		{Key: "c_f32", Type: "float", Constant: float32(1.5)},
		{Key: "c_fbad", Type: "float", Constant: "x"},
		{Key: "vals", Type: "int", Values: []interface{}{7}},
		{Key: "walk", Type: "float", Min: 0, Max: 1, lastVal: 0.5},
	}
	got := collectRounds(t, a, len(a.points), 1)
	want := map[string]interface{}{
		"c_int": 3, "c_int64": 4, "c_f32int": 5, "c_badint": 0,
		"c_bool": true, "c_boolf": false, "c_boolb": true, "c_boolx": false,
		"c_str": "12", "c_strs": "s", "c_fint": 2.0, "c_f32": 1.5, "c_fbad": 0.0, "vals": 7,
	}
	for k, v := range want {
		if got[k][0].Value != v {
			t.Errorf("%s = %#v, want %#v", k, got[k][0].Value, v)
		}
	}
	if got["walk"][0].DeviceID != "fallback" {
		t.Fatal("未配置设备 ID 时应使用适配器默认值")
	}
	if v := got["walk"][0].Value.(float64); v < 0 || v > 1 {
		t.Fatalf("随机波动超出范围: %v", v)
	}
}

func TestMockLifecycle(t *testing.T) {
	// 空配置使用默认点位
	a := NewAdapter().(*MockAdapter)
	if err := a.Init(nil); err != nil || len(a.points) != 2 {
		t.Fatalf("默认配置: %v %d", err, len(a.points))
	}
	// 非法配置
	for _, cfg := range []string{`{`, `{"name":"m","type":"mock","data_points":[]}`, `{"name":"m","type":"mock","pattern":"x","data_points":[{"device_id":"d","key":"k"}]}`} {
		if err := NewAdapter().Init(json.RawMessage(cfg)); err == nil {
			t.Errorf("%s 应返回错误", cfg)
		}
	}
	// 无点位时启动不产生数据；Stop 可重复调用；停止后不再产生数据
	empty := NewMockAdapter()
	ch := make(chan model.Point, 1)
	if err := empty.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	_ = empty.Stop()
	_ = empty.Stop()

	b := initMock(t, `{"name":"m","type":"mock","interval":"5ms","data_points":[{"device_id":"d","key":"k","min_value":0,"max_value":1}]}`)
	ch = make(chan model.Point, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	if err := b.Start(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(ctx, ch); err != nil { // 重复启动无副作用
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	_ = b.Stop()
	time.Sleep(20 * time.Millisecond)
	n := len(ch)
	time.Sleep(30 * time.Millisecond)
	if len(ch) != n || n == 0 {
		t.Fatalf("停止后应不再产生数据: %d -> %d", n, len(ch))
	}
	// 可以重新启动，取消上下文同样会停止
	if err := b.Start(ctx, ch); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = b.Stop()
}

func TestMockLocationPatterns(t *testing.T) {
	a := NewMockAdapter()
	for _, pattern := range []string{"random_walk", "circular", "linear", ""} {
		for _, move := range []bool{true, false} {
			cfg := &config.MockLocationConfig{StartLatitude: 89.9999, StartLongitude: 179.9999, LatitudeRange: 0.001, LongitudeRange: 0.001,
				AltitudeMin: 1, AltitudeMax: 2, SpeedMin: 1, SpeedMax: 2, SimulateMovement: move, MovementPattern: pattern}
			p := &mockPoint{LocationConfig: cfg, locationState: &locationState{currentLat: cfg.StartLatitude, currentLng: cfg.StartLongitude, speed: 1, lastUpdate: time.Now().Add(-time.Hour)}}
			for i := 0; i < 20; i++ {
				v, typ := a.generateLocationData(p)
				if typ != model.TypeLocation || v.Validate() != nil {
					t.Fatalf("%s/%v: %#v %v", pattern, move, v, v.Validate())
				}
			}
		}
	}
	if v, _ := a.generateLocationData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}
}

func TestMockVector3DAndColorModes(t *testing.T) {
	a := NewMockAdapter()
	for _, osc := range []bool{true, false} {
		p := &mockPoint{Vector3DConfig: &config.MockVector3DConfig{XMin: -1, XMax: 1, YMin: -1, YMax: 1, ZMin: -1, ZMax: 1, Correlation: 0.8, Oscillation: osc, Frequency: 2}, vector3dState: &vector3dState{}}
		for i := 0; i < 20; i++ {
			v, _ := a.generateVector3DData(p)
			if v == nil || v.X < -1 || v.X > 1 || v.Z < -1 || v.Z > 1 {
				t.Fatalf("vector3d 超出范围: %#v", v)
			}
		}
	}
	if v, _ := a.generateVector3DData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}

	modes := []*config.MockColorConfig{
		{ColorMode: "random"},
		{ColorMode: "rainbow", HueChangeSpeed: 30, BrightnessRange: [2]uint8{50, 100}, SaturationRange: [2]uint8{50, 100}},
		{ColorMode: "gradient"},
		{ColorMode: "fixed", FixedColors: []string{"#FF0000", "00ff00", "bad"}},
		{ColorMode: "fixed"},
		{ColorMode: ""},
	}
	for _, cfg := range modes {
		p := &mockPoint{ColorConfig: cfg, colorState: &colorState{currentHue: 350}}
		for i := 0; i < 10; i++ {
			v, typ := a.generateColorData(p)
			if v == nil || typ != model.TypeColor || v.Validate() != nil {
				t.Fatalf("color %+v: %#v", cfg, v)
			}
		}
	}
	p := &mockPoint{ColorConfig: &config.MockColorConfig{ColorMode: "fixed", FixedColors: []string{"#FF0000"}}, colorState: &colorState{}}
	if v, _ := a.generateColorData(p); v.R != 255 || v.G != 0 || v.B != 0 {
		t.Fatalf("固定颜色解析错误: %#v", v)
	}
	if v, _ := a.generateColorData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}
}

func TestMockVectorModes(t *testing.T) {
	a := NewMockAdapter()
	for _, dist := range []string{"uniform", "normal", "exponential"} {
		for _, pattern := range []string{"random", "walk", "oscillate"} {
			cfg := &config.MockVectorConfig{Dimension: 4, MinValues: []float64{0, 0}, MaxValues: []float64{1, 1}, GlobalMin: -5, GlobalMax: 5,
				Labels: []string{"a"}, Correlation: 0.5, Distribution: dist, ChangePattern: pattern, Unit: "u"}
			p := &mockPoint{VectorConfig: cfg, vectorState: a.initVectorState(cfg)}
			for i := 0; i < 10; i++ {
				v, typ := a.generateVectorData(p)
				if v == nil || typ != model.TypeVector || len(v.Values) != 4 || v.Validate() != nil {
					t.Fatalf("%s/%s: %#v", dist, pattern, v)
				}
				for d, x := range v.Values {
					lo, hi := -5.0, 5.0
					if d < 2 {
						lo, hi = 0, 1
					}
					if x < lo || x > hi || math.IsNaN(x) {
						t.Fatalf("%s/%s 第 %d 维超出范围: %v", dist, pattern, d, x)
					}
				}
			}
		}
	}
	if v, _ := a.generateVectorData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}
}

func TestMockArrayElementTypes(t *testing.T) {
	a := NewMockAdapter()
	for _, et := range []string{"int", "float", "string", "bool", "mixed", ""} {
		cfg := &config.MockArrayConfig{Size: 6, ElementType: et, MinValue: 1, MaxValue: 9, StringOptions: []string{"x", "y"},
			BoolProbability: 0.5, NullProbability: 0.2, Labels: []string{"l"}, ChangeElements: 2, Unit: "u"}
		p := &mockPoint{ArrayConfig: cfg, arrayState: a.initArrayState(cfg)}
		for i := 0; i < 10; i++ {
			v, typ := a.generateArrayData(p)
			if v == nil || typ != model.TypeArray || len(v.Values) != 6 || v.Validate() != nil {
				t.Fatalf("%s: %#v", et, v)
			}
		}
		// 没有字符串选项时也能生成
		if a.generateArrayElement(&config.MockArrayConfig{ElementType: "string"}) == nil {
			t.Fatal("string 元素不应为 nil")
		}
	}
	if v, _ := a.generateArrayData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}
}

func TestMockMatrixTypes(t *testing.T) {
	a := NewMockAdapter()
	for _, mt := range []string{"general", "diagonal", "identity", "symmetric"} {
		for _, dist := range []string{"uniform", "normal"} {
			cfg := &config.MockMatrixConfig{Rows: 3, Cols: 3, MinValue: 0, MaxValue: 1, MatrixType: mt, Distribution: dist, Sparsity: 0.3, Unit: "u"}
			p := &mockPoint{MatrixConfig: cfg, matrixState: a.initMatrixState(cfg)}
			// 多次生成：中间的微调不能破坏矩阵结构
			for gen := 0; gen < 12; gen++ {
				v, typ := a.generateMatrixData(p)
				if v == nil || typ != model.TypeMatrix || v.Validate() != nil {
					t.Fatalf("%s/%s: %#v", mt, dist, v)
				}
				for r := 0; r < 3; r++ {
					for c := 0; c < 3; c++ {
						x := v.Values[r][c]
						switch mt {
						case "identity":
							if (r == c && x != 1) || (r != c && x != 0) {
								t.Fatalf("单位矩阵错误: %v", v.Values)
							}
						case "diagonal":
							if r != c && x != 0 {
								t.Fatalf("对角矩阵非对角元素应为 0: %v", v.Values)
							}
						case "symmetric":
							if x != v.Values[c][r] {
								t.Fatalf("对称矩阵错误: %v", v.Values)
							}
						}
					}
				}
			}
		}
	}
	if v, _ := a.generateMatrixData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}
}

func TestMockTimeSeries(t *testing.T) {
	a := NewMockAdapter()
	cfgs := []*config.MockTimeSeriesConfig{
		{Length: 10, Interval: "1s", StartTime: "2026-01-01T00:00:00Z", BaseValue: 10, Trend: 0.5, Noise: 0.2, Unit: "u",
			Seasonality: &config.SeasonalityConfig{Period: "5s", Amplitude: 2, Phase: 0.1},
			Anomalies:   &config.AnomaliesConfig{Probability: 1, Magnitude: 3, Duration: 2}},
		{Length: 3, Interval: "bad", StartTime: "bad", Seasonality: &config.SeasonalityConfig{Period: "bad"}},
	}
	for _, cfg := range cfgs {
		p := &mockPoint{TimeSeriesConfig: cfg, timeseriesState: a.initTimeSeriesState(cfg)}
		for i := 0; i < 3; i++ {
			v, typ := a.generateTimeSeriesData(p)
			if v == nil || typ != model.TypeTimeSeries || len(v.Values) != cfg.Length || len(v.Timestamps) != cfg.Length || v.Validate() != nil {
				t.Fatalf("%+v: %#v", cfg, v)
			}
		}
	}
	if v, _ := a.generateTimeSeriesData(&mockPoint{}); v != nil {
		t.Fatal("缺少配置应返回 nil")
	}
}

func TestFitLabels(t *testing.T) {
	if got := fitLabels([]string{"a"}, 3); len(got) != 3 || got[0] != "a" || got[2] != "2" {
		t.Fatalf("补齐: %v", got)
	}
	if got := fitLabels([]string{"a", "b", "c"}, 2); len(got) != 2 || got[1] != "b" {
		t.Fatalf("截断: %v", got)
	}
	if fitLabels(nil, 3) != nil {
		t.Fatal("无标签时保持为空")
	}
}

func TestMockHelpers(t *testing.T) {
	if max(1, 2) != 2 || max(3, 2) != 3 {
		t.Fatal("max")
	}
	if !isNilValue(nil) || !isNilValue((*model.LocationData)(nil)) || isNilValue(1.0) || isNilValue(&model.LocationData{}) {
		t.Fatal("isNilValue")
	}
	if b := simByte(); int(b) > 255 {
		t.Fatal("simByte")
	}
	if n := simIntn(3); n < 0 || n >= 3 {
		t.Fatal("simIntn")
	}
}
