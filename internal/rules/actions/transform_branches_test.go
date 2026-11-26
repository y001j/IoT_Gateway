package actions

import (
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

type tcase struct {
	name string
	p    model.Point
	cfg  map[string]interface{}
}

func tp(typ string, params map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": typ, "parameters": params}
}

// 一条带趋势、周期和一个异常点的时间序列（每小时一个点，共 72 个）
func seriesPoint() model.Point {
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC) // 周一
	var ts []time.Time
	var vs []float64
	for i := 0; i < 72; i++ {
		ts = append(ts, base.Add(time.Duration(i)*time.Hour))
		v := 10 + 0.1*float64(i) + 3*math.Sin(2*math.Pi*float64(i)/12)
		if i == 40 {
			v += 50 // 异常
		}
		vs = append(vs, v)
	}
	return model.NewCompositePoint("ts", "d", &model.TimeSeriesData{Timestamps: ts, Values: vs})
}

// 结果必须是有限数值或非空的复合/结构化数据
func checkResult(t *testing.T, name string, p model.Point) {
	t.Helper()
	switch v := p.Value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s: 结果无效 %v", name, v)
		}
	case nil:
		t.Errorf("%s: 结果为空", name)
	case model.CompositeData:
		if err := v.Validate(); err != nil {
			t.Errorf("%s: 复合结果校验失败: %v (%#v)", name, err, v)
		}
	}
}

func TestTimeSeriesAnalysisBranches(t *testing.T) {
	ts := seriesPoint()
	ta := func(params map[string]interface{}) map[string]interface{} {
		params2 := map[string]interface{}{}
		for k, v := range params {
			params2[k] = v
		}
		return tp("timeseries_analysis", params2)
	}
	var cases []tcase
	for _, m := range []string{"linear", "polynomial", "exponential", "seasonal"} {
		cases = append(cases, tcase{"trend " + m, ts, ta(map[string]interface{}{"analysis": "trend_analysis", "trend_method": m, "window_size": 12.0})})
	}
	for _, m := range []string{"additive", "multiplicative"} {
		cases = append(cases, tcase{"decompose " + m, ts, ta(map[string]interface{}{"analysis": "seasonal_decompose", "decompose_model": m, "seasonal_period": 12.0})})
	}
	for _, w := range []string{"sliding", "expanding", "fixed"} {
		cases = append(cases, tcase{"moving_average " + w, ts, ta(map[string]interface{}{"analysis": "moving_average", "window_type": w, "window_size": 6.0})})
	}
	cases = append(cases,
		tcase{"diff", ts, ta(map[string]interface{}{"analysis": "diff", "diff_order": 2.0})},
		tcase{"seasonal diff", ts, ta(map[string]interface{}{"analysis": "diff", "diff_seasonal": true, "seasonal_period": 12.0})},
	)
	for _, f := range []string{"minute", "hour", "day", "week", "month"} {
		for _, m := range []string{"mean", "sum", "max", "min", "first", "last"} {
			cases = append(cases, tcase{"resample " + f + " " + m, ts, ta(map[string]interface{}{"analysis": "resample", "resample_frequency": f, "resample_method": m})})
		}
	}
	for _, m := range []string{"zscore", "iqr", "isolation_forest", "local_outlier"} {
		cases = append(cases, tcase{"anomaly " + m, ts, ta(map[string]interface{}{"analysis": "anomaly_detection", "anomaly_method": m, "anomaly_threshold": 2.0})})
	}
	for _, m := range []string{"linear", "exponential_smoothing", "seasonal_naive", "arima"} {
		cases = append(cases, tcase{"forecast " + m, ts, ta(map[string]interface{}{"analysis": "forecast", "forecast_method": m, "forecast_steps": 5.0, "seasonal_period": 12.0})})
	}
	cases = append(cases, tcase{"correlation", ts, ta(map[string]interface{}{"analysis": "correlation", "correlation_lag": 12.0})})

	for _, c := range cases {
		got, ok, msg := tryTransform(t, c.p, c.cfg)
		if !ok {
			t.Errorf("%s: 失败: %s", c.name, msg)
			continue
		}
		checkResult(t, c.name, got)
	}

	// 重采样按真实时间分桶：72 小时的数据按天是 3 个桶，按小时是 72 个，按周是 1 个（周一开始）
	for f, want := range map[string]int{"minute": 72, "hour": 72, "day": 3, "week": 1, "month": 1} {
		got, ok, msg := tryTransform(t, ts, ta(map[string]interface{}{"analysis": "resample", "resample_frequency": f, "resample_method": "max"}))
		if !ok {
			t.Fatalf("resample %s: %s", f, msg)
		}
		r := got.Value.(*model.TimeSeriesData)
		if len(r.Values) != want || len(r.Timestamps) != want {
			t.Errorf("resample %s: %d 个桶, want %d", f, len(r.Values), want)
		}
	}
	got, _, _ := tryTransform(t, ts, ta(map[string]interface{}{"analysis": "resample", "resample_frequency": "day", "resample_method": "mean"}))
	if r := got.Value.(*model.TimeSeriesData); !r.Timestamps[1].Equal(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("第二天的桶起始时间错误: %v", r.Timestamps)
	}
	// 滑动平均的时间戳与输入尾部对齐，固定窗口取窗口末尾
	got, _, _ = tryTransform(t, ts, ta(map[string]interface{}{"analysis": "moving_average", "window_type": "sliding", "window_size": 6.0}))
	in := ts.Value.(*model.TimeSeriesData)
	if r := got.Value.(*model.TimeSeriesData); !r.Timestamps[0].Equal(in.Timestamps[5]) || !r.Timestamps[len(r.Timestamps)-1].Equal(in.Timestamps[71]) {
		t.Fatalf("滑动平均时间戳未对齐")
	}
	got, _, _ = tryTransform(t, ts, ta(map[string]interface{}{"analysis": "moving_average", "window_type": "fixed", "window_size": 6.0}))
	if r := got.Value.(*model.TimeSeriesData); !r.Timestamps[0].Equal(in.Timestamps[5]) || len(r.Values) != 12 {
		t.Fatalf("固定窗口时间戳错误")
	}
	if _, ok, _ := tryTransform(t, ts, ta(map[string]interface{}{"analysis": "diff", "diff_order": 100.0})); ok {
		t.Fatal("差分阶数过大应失败")
	}
	if _, ok, _ := tryTransform(t, ts, ta(map[string]interface{}{"analysis": "resample", "resample_method": "nope"})); ok {
		t.Fatal("未知重采样方法应失败")
	}
	noTS := model.NewCompositePoint("ts", "d", &model.TimeSeriesData{Timestamps: in.Timestamps[:3], Values: in.Values})
	if _, ok, _ := tryTransform(t, noTS, ta(map[string]interface{}{"analysis": "resample"})); ok {
		t.Fatal("时间戳与数值数量不一致时重采样应失败")
	}

	// 异常检测应找出注入的异常点
	got, ok, _ := tryTransform(t, ts, ta(map[string]interface{}{"analysis": "anomaly_detection", "anomaly_method": "zscore", "anomaly_threshold": 3.0}))
	if !ok || got.Value.(map[string]interface{})["anomaly_count"] != 1 {
		t.Fatalf("zscore 应找出 1 个异常点: %#v", got.Value)
	}

	// 参数错误
	for name, cfg := range map[string]map[string]interface{}{
		"未知分析":    ta(map[string]interface{}{"analysis": "nope"}),
		"未知趋势方法":  ta(map[string]interface{}{"analysis": "trend_analysis", "trend_method": "nope"}),
		"未知窗口类型":  ta(map[string]interface{}{"analysis": "moving_average", "window_type": "nope", "window_size": 3.0}),
		"窗口过大":    ta(map[string]interface{}{"analysis": "moving_average", "window_size": 1000.0}),
		"未知重采样频率": ta(map[string]interface{}{"analysis": "resample", "resample_frequency": "nope"}),
		"未知异常方法":  ta(map[string]interface{}{"analysis": "anomaly_detection", "anomaly_method": "nope"}),
		"未知预测方法":  ta(map[string]interface{}{"analysis": "forecast", "forecast_method": "nope"}),
		"未知分解模型":  ta(map[string]interface{}{"analysis": "seasonal_decompose", "decompose_model": "nope", "seasonal_period": 12.0}),
	} {
		if _, ok, _ := tryTransform(t, ts, cfg); ok {
			t.Errorf("%s: 应失败", name)
		}
	}
	short := model.NewCompositePoint("ts", "d", &model.TimeSeriesData{Timestamps: []time.Time{time.Now()}, Values: []float64{1}})
	for _, a := range []string{"trend_analysis", "correlation", "forecast", "anomaly_detection"} {
		if _, ok, _ := tryTransform(t, short, ta(map[string]interface{}{"analysis": a})); ok {
			t.Errorf("%s: 数据点不足应失败", a)
		}
	}
}

func TestArrayTransformBranches(t *testing.T) {
	arr := model.NewCompositePoint("a", "d", &model.ArrayData{Values: []interface{}{1.0, 2.0, 3.0, 100.0, 2.0, -1.0, 2.5, 3.0, 1.5, 2.0}, Size: 10})
	cases := []tcase{
		{"filter range", arr, tp("array_filter", map[string]interface{}{"filter_type": "value_range", "min_value": 0.0, "max_value": 10.0})},
		{"filter zscore", arr, tp("array_filter", map[string]interface{}{"filter_type": "outliers", "outlier_method": "zscore", "outlier_threshold": 2.0})},
		{"filter iqr", arr, tp("array_filter", map[string]interface{}{"filter_type": "outliers", "outlier_method": "iqr", "outlier_threshold": 1.5})},
		{"filter percentile", arr, tp("array_filter", map[string]interface{}{"filter_type": "outliers", "outlier_method": "percentile", "outlier_threshold": 10.0})},
		{"filter expr >=", arr, tp("array_filter", map[string]interface{}{"filter_type": "expression", "filter_condition": "value >= 2"})},
		{"filter expr <=", arr, tp("array_filter", map[string]interface{}{"filter_type": "expression", "filter_condition": "value <= 2"})},
		{"filter expr >", arr, tp("array_filter", map[string]interface{}{"filter_type": "expression", "filter_condition": "value > 2"})},
		{"filter expr <", arr, tp("array_filter", map[string]interface{}{"filter_type": "expression", "filter_condition": "value < 2"})},
		{"filter expr ==", arr, tp("array_filter", map[string]interface{}{"filter_type": "expression", "filter_condition": "value == 2"})},
		{"filter expr !=", arr, tp("array_filter", map[string]interface{}{"filter_type": "expression", "filter_condition": "value != 2"})},
		{"sort abs desc", arr, tp("array_sort", map[string]interface{}{"sort_by": "abs_value", "sort_order": "desc"})},
		{"sort index", arr, tp("array_sort", map[string]interface{}{"sort_by": "index"})},
		{"smooth moving", arr, tp("array_smooth", map[string]interface{}{"smooth_method": "moving_average", "smooth_window": 3.0})},
		{"smooth gaussian", arr, tp("array_smooth", map[string]interface{}{"smooth_method": "gaussian", "smooth_window": 3.0})},
		{"smooth savgol", arr, tp("array_smooth", map[string]interface{}{"smooth_method": "savgol", "smooth_window": 5.0})},
		{"normalize minmax", arr, tp("array_normalize", map[string]interface{}{"normalize_method": "minmax"})},
		{"normalize zscore", arr, tp("array_normalize", map[string]interface{}{"normalize_method": "zscore"})},
		{"normalize robust", arr, tp("array_normalize", map[string]interface{}{"normalize_method": "robust"})},
		{"transform sqrt", arr, tp("array_transform", map[string]interface{}{"transform_type": "sqrt"})},
		{"transform log", arr, tp("array_transform", map[string]interface{}{"transform_type": "log"})},
		{"transform square", arr, tp("array_transform", map[string]interface{}{"transform_type": "square"})},
		{"transform exp", arr, tp("array_transform", map[string]interface{}{"transform_type": "exp"})},
		{"transform abs", arr, tp("array_transform", map[string]interface{}{"transform_type": "abs"})},
	}
	for _, c := range cases {
		got, ok, msg := tryTransform(t, c.p, c.cfg)
		if !ok {
			t.Errorf("%s: 失败: %s", c.name, msg)
			continue
		}
		checkResult(t, c.name, got)
	}
	// 结果检查
	got, _, _ := tryTransform(t, arr, tp("array_filter", map[string]interface{}{"filter_type": "outliers", "outlier_method": "iqr", "outlier_threshold": 1.5}))
	for _, v := range got.Value.(*model.ArrayData).Values {
		if v == 100.0 {
			t.Fatal("IQR 应过滤掉 100")
		}
	}
	got, _, _ = tryTransform(t, arr, tp("array_normalize", map[string]interface{}{"normalize_method": "robust"}))
	if len(got.Value.(*model.ArrayData).Values) != 10 {
		t.Fatal("robust 归一化应保持长度")
	}
	for name, cfg := range map[string]map[string]interface{}{
		"未知过滤类型":  tp("array_filter", map[string]interface{}{"filter_type": "nope"}),
		"缺少过滤表达式": tp("array_filter", map[string]interface{}{"filter_type": "expression"}),
		"未知离群方法":  tp("array_filter", map[string]interface{}{"filter_type": "outliers", "outlier_method": "nope"}),
		"未知平滑方法":  tp("array_smooth", map[string]interface{}{"smooth_method": "nope"}),
		"未知归一化":   tp("array_normalize", map[string]interface{}{"normalize_method": "nope"}),
		"未知变换":    tp("array_transform", map[string]interface{}{"transform_type": "nope"}),
	} {
		if _, ok, _ := tryTransform(t, arr, cfg); ok {
			t.Errorf("%s: 应失败", name)
		}
	}
}

func TestGeoVectorColorDispatchBranches(t *testing.T) {
	loc := model.NewCompositePoint("gps", "car", &model.LocationData{Latitude: 39.9042, Longitude: 116.4074})
	vec3 := model.NewCompositePoint("acc", "d", &model.Vector3D{X: 3, Y: 0, Z: 4})
	vec := model.NewCompositePoint("v", "d", &model.VectorData{Values: []float64{1, 2, 3}, Dimension: 3})
	color := model.NewCompositePoint("c", "d", &model.ColorData{R: 200, G: 100, B: 50, A: 255})
	ref := map[string]interface{}{"latitude": 31.2304, "longitude": 121.4737}
	cases := []tcase{}
	for _, unit := range []string{"kilometers", "meters", "miles"} {
		cases = append(cases, tcase{"geo distance " + unit, loc, tp("geo_transform", map[string]interface{}{"sub_type": "distance", "reference_point": ref, "unit": unit})})
	}
	cases = append(cases,
		tcase{"geo bearing", loc, tp("geo_transform", map[string]interface{}{"sub_type": "bearing", "reference_point": ref})},
		tcase{"geo geofence", loc, tp("geo_transform", map[string]interface{}{"sub_type": "geofence", "reference_point": ref, "radius": 1000.0})},
		tcase{"geo convert", loc, tp("geo_transform", map[string]interface{}{"sub_type": "coordinate_convert", "source_coordinate_system": "WGS84", "target_coordinate_system": "BD09"})},
	)
	for _, conv := range [][2]string{{"WGS84", "GCJ02"}, {"GCJ02", "WGS84"}, {"WGS84", "BD09"}, {"BD09", "WGS84"}, {"GCJ02", "BD09"}, {"BD09", "GCJ02"}, {"WGS84", "WGS84"}} {
		cases = append(cases, tcase{"convert " + conv[0] + "->" + conv[1], loc, tp("geo_coordinate_convert", map[string]interface{}{"source_coordinate_system": conv[0], "target_coordinate_system": conv[1]})})
	}
	for _, p := range []model.Point{vec3, vec} {
		for _, sub := range []string{"magnitude", "normalize", "projection", "cross_product", "dot_product"} {
			cases = append(cases, tcase{"vector " + sub, p, tp("vector_transform", map[string]interface{}{"sub_type": sub, "reference_vector": map[string]interface{}{"x": 1.0, "y": 1.0, "z": 0.0}})})
		}
		for _, axis := range []string{"x", "y", "z"} {
			cases = append(cases, tcase{"rotate " + axis, p, tp("vector_transform", map[string]interface{}{"sub_type": "rotation", "rotation_axis": axis, "rotation_angle": 90.0})})
		}
		cases = append(cases, tcase{"rotate custom", p, tp("vector_transform", map[string]interface{}{"sub_type": "rotation", "rotation_axis": "custom", "rotation_angle": 45.0, "custom_axis": map[string]interface{}{"x": 1.0, "y": 1.0, "z": 1.0}})})
	}
	cases = append(cases,
		tcase{"color similarity", color, tp("color_similarity", map[string]interface{}{"reference_r": 255.0, "reference_g": 0.0, "reference_b": 0.0})},
		tcase{"color dominant", color, tp("color_extract_dominant", nil)},
	)
	for _, c := range cases {
		got, ok, msg := tryTransform(t, c.p, c.cfg)
		if !ok {
			t.Errorf("%s: 失败: %s", c.name, msg)
			continue
		}
		checkResult(t, c.name, got)
	}
	// 旋转 90° 后 (3,0,4) 绕 z 轴应得到 (0,3,4)
	got, ok, _ := tryTransform(t, vec3, tp("vector_transform", map[string]interface{}{"sub_type": "rotation", "rotation_axis": "z", "rotation_angle": 90.0}))
	if v := got.Value.(*model.VectorData).Values; !ok || math.Abs(v[0]) > 1e-9 || math.Abs(v[1]-3) > 1e-9 || v[2] != 4 {
		t.Fatalf("绕 z 轴旋转 90°: %v", got.Value)
	}
	// 坐标往返误差应很小
	g, _, _ := tryTransform(t, loc, tp("geo_coordinate_convert", map[string]interface{}{"source_coordinate_system": "WGS84", "target_coordinate_system": "GCJ02"}))
	back, _, _ := tryTransform(t, g, tp("geo_coordinate_convert", map[string]interface{}{"source_coordinate_system": "GCJ02", "target_coordinate_system": "WGS84"}))
	if l := back.Value.(*model.LocationData); math.Abs(l.Latitude-39.9042) > 1e-4 || math.Abs(l.Longitude-116.4074) > 1e-4 {
		t.Fatalf("GCJ02 往返误差过大: %+v", l)
	}
	for name, c := range map[string]tcase{
		"geo 缺少 sub_type":    {p: loc, cfg: tp("geo_transform", map[string]interface{}{})},
		"geo 未知 sub_type":    {p: loc, cfg: tp("geo_transform", map[string]interface{}{"sub_type": "nope"})},
		"vector 缺少 sub_type": {p: vec3, cfg: tp("vector_transform", map[string]interface{}{})},
		"vector 未知 sub_type": {p: vec3, cfg: tp("vector_transform", map[string]interface{}{"sub_type": "nope"})},
		"vector 非向量":         {p: loc, cfg: tp("vector_transform", map[string]interface{}{"sub_type": "magnitude"})},
		"未知旋转轴":              {p: vec3, cfg: tp("vector_transform", map[string]interface{}{"sub_type": "rotation", "rotation_axis": "w"})},
		"未知坐标转换":             {p: loc, cfg: tp("geo_coordinate_convert", map[string]interface{}{"source_coordinate_system": "X", "target_coordinate_system": "Y"})},
		"颜色转换非颜色":            {p: vec3, cfg: tp("color_similarity", nil)},
	} {
		if _, ok, _ := tryTransform(t, c.p, c.cfg); ok {
			t.Errorf("%s: 应失败", name)
		}
	}
}

func TestExpressionParserFunctions(t *testing.T) {
	cases := map[string]float64{
		"abs(-3)": 3, "sqrt(16)": 4, "sin(0)": 0, "cos(0)": 1, "tan(0)": 0, "ln(1)": 0, "log(100)": 2,
		"exp(0)": 1, "exp(x-4)": 1, "2-x": -2, "floor(2.7)": 2, "ceil(2.1)": 3, "round(2.5)": 3, "(1+2)*3": 9, "x*2": 8,
	}
	for expr, want := range cases {
		got, ok, msg := tryTransform(t, model.NewPoint("k", "d", 4.0, model.TypeFloat), tp("expression", map[string]interface{}{"expression": expr}))
		if !ok {
			t.Errorf("%s: %s", expr, msg)
			continue
		}
		if v, _ := num(got.Value); math.Abs(v-want) > 1e-9 {
			t.Errorf("%s = %v, want %v", expr, got.Value, want)
		}
	}
	for _, expr := range []string{"sqrt(-1)", "ln(0)", "log(-1)", "nope(1)", "abs", "(1+2", "1/0"} {
		if _, ok, _ := tryTransform(t, model.NewPoint("k", "d", 4.0, model.TypeFloat), tp("expression", map[string]interface{}{"expression": expr})); ok {
			t.Errorf("%s: 应失败", expr)
		}
	}
}
