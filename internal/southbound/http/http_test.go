package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

const sampleBody = `{
  "device": {"temp": 21.5, "count": 7, "on": true, "label": "A1", "temp_str": "3.5", "count_str": "12", "flag_str": "on"},
  "gps": {"lat": 31.2, "lon": 121.5, "alt": 10, "acc": 2, "speed": "1.5", "heading": 90},
  "acc": {"x": 0.1, "y": -0.2, "z": 9.8},
  "led": {"r": 300, "g": 128, "b": -5, "a": 0.5}
}`

func newTestServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/data":
			if r.Method == http.MethodPost {
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"q":1}` || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Token") != "t" {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
			}
			_, _ = io.WriteString(w, sampleBody)
		case "/fail":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "/garbage":
			_, _ = io.WriteString(w, "not json")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func initAdapter(t *testing.T, cfg map[string]interface{}) *HTTPAdapter {
	t.Helper()
	raw, _ := json.Marshal(cfg)
	a := NewAdapter().(*HTTPAdapter)
	if err := a.Init(raw); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return a
}

func allDataPoints() []map[string]interface{} {
	return []map[string]interface{}{
		{"key": "temp", "path": "device.temp", "type": "float", "device_id": "d1", "tags": map[string]string{"unit": "C"}},
		{"key": "count", "path": "device.count", "type": "int"},
		{"key": "on", "path": "device.on", "type": "bool"},
		{"key": "label", "path": "device.label", "type": "string"},
		{"key": "missing", "path": "device.nope", "type": "float"}, // 提取失败，跳过
		{"key": "gps", "path": "gps", "type": "location", "composite": map[string]interface{}{"location": map[string]string{
			"latitude_path": "gps.lat", "longitude_path": "gps.lon", "altitude_path": "gps.alt",
			"accuracy_path": "gps.acc", "speed_path": "gps.speed", "heading_path": "gps.heading"}}},
		{"key": "acc", "path": "acc", "type": "vector3d", "composite": map[string]interface{}{"vector3d": map[string]string{
			"x_path": "acc.x", "y_path": "acc.y", "z_path": "acc.z"}}},
		{"key": "led", "path": "led", "type": "color", "composite": map[string]interface{}{"color": map[string]string{
			"red_path": "led.r", "green_path": "led.g", "blue_path": "led.b", "alpha_path": "led.a"}}},
	}
}

func collect(ch <-chan model.Point, n int, timeout time.Duration) map[string]model.Point {
	got := map[string]model.Point{}
	deadline := time.After(timeout)
	for len(got) < n {
		select {
		case p := <-ch:
			got[p.Key] = p
		case <-deadline:
			return got
		}
	}
	return got
}

func TestHTTPAdapterPollsAndExtractsAllTypes(t *testing.T) {
	var hits atomic.Int32
	srv := newTestServer(t, &hits)
	a := initAdapter(t, map[string]interface{}{
		"name": "h1", "type": "http", "enabled": true,
		"url": srv.URL + "/data", "method": "POST", "body": `{"q":1}`,
		"headers":  map[string]string{"X-Token": "t"},
		"interval": "30ms", "timeout": "2s",
		"data_points": allDataPoints(),
	})
	if a.Name() != "h1" {
		t.Fatalf("Name = %q", a.Name())
	}

	ch := make(chan model.Point, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(ctx, ch); err != nil { // 重复启动无副作用
		t.Fatal(err)
	}
	got := collect(ch, 7, 3*time.Second)
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil { // 可重复调用
		t.Fatal(err)
	}

	if len(got) != 7 {
		t.Fatalf("应收到 7 个数据点（missing 被跳过），got %d: %v", len(got), got)
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("路径不存在的数据点不应发送")
	}
	temp := got["temp"]
	if temp.Value != 21.5 || temp.Type != model.TypeFloat || temp.DeviceID != "d1" {
		t.Fatalf("temp = %+v", temp)
	}
	if tags := temp.GetTagsCopy(); tags["unit"] != "C" || tags["source"] != "http" || tags["url"] != srv.URL+"/data" {
		t.Fatalf("temp 标签 = %v", tags)
	}
	if got["count"].Value != 7 || got["count"].DeviceID != "http" {
		t.Fatalf("count = %+v（未配置设备 ID 时默认为 http）", got["count"])
	}
	if got["on"].Value != true || got["label"].Value != "A1" {
		t.Fatalf("on/label = %+v %+v", got["on"], got["label"])
	}
	loc, ok := got["gps"].Value.(*model.LocationData)
	if !ok || loc.Latitude != 31.2 || loc.Longitude != 121.5 || loc.Altitude != 10 || loc.Accuracy != 2 || loc.Speed != 1.5 || loc.Heading != 90 {
		t.Fatalf("gps = %#v", got["gps"].Value)
	}
	vec, ok := got["acc"].Value.(*model.Vector3D)
	if !ok || vec.X != 0.1 || vec.Y != -0.2 || vec.Z != 9.8 {
		t.Fatalf("acc = %#v", got["acc"].Value)
	}
	col, ok := got["led"].Value.(*model.ColorData)
	// 超出 0-255 的分量被截断，alpha 0-1 映射到 0-255
	if !ok || col.R != 255 || col.G != 128 || col.B != 0 || col.A != 127 {
		t.Fatalf("led = %#v", got["led"].Value)
	}
}

func TestHTTPAdapterErrorResponsesProduceNoData(t *testing.T) {
	var hits atomic.Int32
	srv := newTestServer(t, &hits)
	for _, path := range []string{"/fail", "/garbage"} {
		a := initAdapter(t, map[string]interface{}{
			"name": "h", "type": "http", "url": srv.URL + path, "interval": "20ms",
			"data_points": []map[string]interface{}{{"key": "temp", "path": "device.temp", "type": "float"}},
		})
		ch := make(chan model.Point, 10)
		before := hits.Load()
		if err := a.Start(context.Background(), ch); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for hits.Load()-before < 2 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		_ = a.Stop()
		if hits.Load()-before < 2 {
			t.Fatalf("%s: 应持续轮询", path)
		}
		if len(ch) != 0 {
			t.Fatalf("%s: 错误响应不应产生数据", path)
		}
	}

	// 连接失败记录最后错误
	a := initAdapter(t, map[string]interface{}{"name": "h", "type": "http", "url": "http://127.0.0.1:1/x", "timeout": "200ms"})
	a.pollEndpoint(context.Background(), a.endpoints[0], make(chan model.Point, 1))
	if a.GetLastError() == nil {
		t.Fatal("连接失败应记录错误")
	}
}

func TestHTTPAdapterContextCancelStopsPolling(t *testing.T) {
	var hits atomic.Int32
	srv := newTestServer(t, &hits)
	a := initAdapter(t, map[string]interface{}{"name": "h", "type": "http", "url": srv.URL + "/data", "interval": "10ms"})
	ctx, cancel := context.WithCancel(context.Background())
	if err := a.Start(ctx, make(chan model.Point, 100)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
	n := hits.Load()
	time.Sleep(80 * time.Millisecond)
	if hits.Load() != n {
		t.Fatal("上下文取消后应停止轮询")
	}
	_ = a.Stop()
}

func TestHTTPAdapterInitRejectsInvalidConfig(t *testing.T) {
	for name, cfg := range map[string]string{
		"非法JSON": `{`,
		"缺少URL":  `{"name":"h","type":"http"}`,
		"非法方法":   `{"name":"h","type":"http","url":"http://x","method":"TRACE"}`,
	} {
		if err := NewAdapter().Init(json.RawMessage(cfg)); err == nil {
			t.Errorf("%s: 应返回错误", name)
		}
	}
}

func TestCreateRequest(t *testing.T) {
	a := &HTTPAdapter{}
	req, err := a.createRequest(Endpoint{URL: "http://x/y", Body: "ignored"})
	if err != nil || req.Method != "GET" || req.Body != nil {
		t.Fatalf("默认 GET 且不带请求体: %v %v", req, err)
	}
	req, _ = a.createRequest(Endpoint{URL: "http://x", Method: "put", Body: "a", Headers: map[string]string{"Content-Type": "text/plain"}})
	if req.Method != "PUT" || req.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("显式 Content-Type 应保留: %v", req.Header)
	}
	if _, err := a.createRequest(Endpoint{URL: "://bad"}); err == nil {
		t.Fatal("非法 URL 应返回错误")
	}
}

func TestExtractValue(t *testing.T) {
	a := &HTTPAdapter{}
	data := map[string]interface{}{"a": map[string]interface{}{"b": 1.0}, "s": "x"}
	if v, err := a.extractValue(data, "a.b"); err != nil || v != 1.0 {
		t.Fatalf("a.b = %v %v", v, err)
	}
	for _, p := range []string{"a.c", "z.b", "s.b", "nope"} {
		if _, err := a.extractValue(data, p); err == nil {
			t.Errorf("路径 %s 应返回错误", p)
		}
	}
}

func TestConvertBasicType(t *testing.T) {
	a := &HTTPAdapter{}
	ok := []struct {
		typ  string
		in   interface{}
		want interface{}
		dt   model.DataType
	}{
		{"int", 3.9, 3, model.TypeInt}, {"int", "12", 12, model.TypeInt}, {"int", 5, 5, model.TypeInt},
		{"float", 1.5, 1.5, model.TypeFloat}, {"float", 2, 2.0, model.TypeFloat}, {"float", "3.5", 3.5, model.TypeFloat},
		{"bool", true, true, model.TypeBool}, {"bool", "yes", true, model.TypeBool}, {"bool", "off", false, model.TypeBool},
		{"bool", 0.0, false, model.TypeBool}, {"bool", 2, true, model.TypeBool},
		{"string", "s", "s", model.TypeString}, {"string", 1.5, "1.5", model.TypeString},
		{"", "d", "d", model.TypeString}, {"other", true, "true", model.TypeString},
	}
	for _, c := range ok {
		dt, v, err := a.convertBasicType(c.typ, c.in, "u")
		if err != nil || v != c.want || dt != c.dt {
			t.Errorf("%s(%v) = %v %v %v, want %v %v", c.typ, c.in, dt, v, err, c.dt, c.want)
		}
	}
	bad := []struct {
		typ string
		in  interface{}
	}{{"int", "x"}, {"int", true}, {"float", "x"}, {"float", true}, {"bool", "maybe"}, {"bool", []int{}}}
	for _, c := range bad {
		if _, _, err := a.convertBasicType(c.typ, c.in, "u"); err == nil {
			t.Errorf("%s(%v) 应返回错误", c.typ, c.in)
		}
	}
}

func TestExtractFloatValue(t *testing.T) {
	a := &HTTPAdapter{}
	data := map[string]interface{}{"f": 1.5, "i": 2, "s": "2.5", "bad": "x", "b": true}
	cases := map[string]float64{"": 0, "f": 1.5, "i": 2, "s": 2.5}
	for p, want := range cases {
		if v, err := a.extractFloatValue(data, p); err != nil || v != want {
			t.Errorf("%q = %v %v", p, v, err)
		}
	}
	for _, p := range []string{"bad", "b", "missing"} {
		if _, err := a.extractFloatValue(data, p); err == nil {
			t.Errorf("%q 应返回错误", p)
		}
	}
}

func TestCompositeExtractionErrors(t *testing.T) {
	a := &HTTPAdapter{}
	data := map[string]interface{}{"lat": 100.0, "lon": 1.0, "x": 1.0, "s": "bad", "c": 10.0}
	// 缺少复合配置
	if _, _, err := a.extractLocationData(data, DataPoint{}); err == nil {
		t.Error("location 缺少配置应报错")
	}
	if _, _, err := a.extractVector3DData(data, DataPoint{}); err == nil {
		t.Error("vector3d 缺少配置应报错")
	}
	if _, _, err := a.extractColorData(data, DataPoint{}); err == nil {
		t.Error("color 缺少配置应报错")
	}
	loc := func(lat, lon string) DataPoint {
		return DataPoint{Composite: &CompositeExtractConfig{Location: &LocationExtractConfig{LatitudePath: lat, LongitudePath: lon}}}
	}
	for _, dp := range []DataPoint{loc("missing", "lon"), loc("lon", "missing"), loc("lat", "lon")} { // 最后一个纬度越界
		if _, _, err := a.extractLocationData(data, dp); err == nil {
			t.Errorf("location %+v 应报错", dp.Composite.Location)
		}
	}
	vec := func(x, y, z string) DataPoint {
		return DataPoint{Composite: &CompositeExtractConfig{Vector3D: &Vector3DExtractConfig{XPath: x, YPath: y, ZPath: z}}}
	}
	for _, dp := range []DataPoint{vec("s", "x", "x"), vec("x", "s", "x"), vec("x", "x", "s")} {
		if _, _, err := a.extractVector3DData(data, dp); err == nil {
			t.Errorf("vector3d %+v 应报错", dp.Composite.Vector3D)
		}
	}
	col := func(r, g, b, al string) DataPoint {
		return DataPoint{Composite: &CompositeExtractConfig{Color: &ColorExtractConfig{RedPath: r, GreenPath: g, BluePath: b, AlphaPath: al}}}
	}
	for _, dp := range []DataPoint{col("s", "c", "c", ""), col("c", "s", "c", ""), col("c", "c", "s", "")} {
		if _, _, err := a.extractColorData(data, dp); err == nil {
			t.Errorf("color %+v 应报错", dp.Composite.Color)
		}
	}
	// alpha 提取失败或未配置时默认不透明
	for _, al := range []string{"", "s"} {
		v, _, err := a.extractColorData(data, col("c", "c", "c", al))
		if err != nil || v.(*model.ColorData).A != 255 {
			t.Errorf("alpha=%q: %v %v", al, v, err)
		}
	}
}
