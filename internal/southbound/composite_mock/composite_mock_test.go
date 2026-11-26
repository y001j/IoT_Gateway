package composite_mock

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
)

const fullConfig = `{"name":"cm","type":"composite_mock","interval":"10ms","tags":{"site":"s1","floor":2},
 "composite_data_types":[
  {"device_id":"d","key":"loc","data_type":"location","location_config":{"start_latitude":31,"start_longitude":121,"latitude_range":0.01,"longitude_range":0.01,"altitude_min":1,"altitude_max":5,"speed_min":10,"speed_max":20,"simulate_movement":true}},
  {"device_id":"d","key":"acc","data_type":"vector3d","vector3d_config":{"x_min":-1,"x_max":1,"y_min":-1,"y_max":1,"z_min":9,"z_max":10}},
  {"device_id":"d","key":"led","data_type":"color","color_config":{"color_mode":"rainbow"}},
  {"device_id":"d","key":"vec","data_type":"vector","vector_config":{"dimension":3,"min_values":[0,0],"max_values":[1,1],"labels":["a"],"unit":"u"}},
  {"device_id":"d","key":"arr","data_type":"array","array_config":{"size":3,"element_type":"int","max_value":10}},
  {"device_id":"d","key":"nocfg","data_type":"location"},
  {"device_id":"d","key":"bad","data_type":"unknown"}
 ]}`

func TestCompositeMockPublishesAllTypes(t *testing.T) {
	a := NewCompositeMockAdapter()
	if err := a.Init(json.RawMessage(fullConfig)); err != nil {
		t.Fatal(err)
	}
	if a.Name() != "composite_mock" || a.interval != 10*time.Millisecond {
		t.Fatalf("name=%s interval=%v", a.Name(), a.interval)
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
	got := map[string]model.Point{}
	deadline := time.After(3 * time.Second)
	for len(got) < 5 {
		select {
		case p := <-ch:
			got[p.Key] = p
		case <-deadline:
			t.Fatalf("超时: %v", got)
		}
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(); err != nil { // 可重复调用
		t.Fatal(err)
	}
	if _, ok := got["nocfg"]; ok {
		t.Fatal("缺少配置的点位不应发送")
	}
	if _, ok := got["bad"]; ok {
		t.Fatal("不支持的类型不应发送")
	}
	for key, typ := range map[string]string{"loc": "location", "acc": "vector3d", "led": "color", "vec": "vector", "arr": "array"} {
		p := got[key]
		if string(p.Type) != typ || p.DeviceID != "d" {
			t.Fatalf("%s: %+v", key, p)
		}
		tags := p.GetTagsCopy()
		if tags["source"] != "composite_mock" || tags["site"] != "s1" || tags["floor"] != "2" {
			t.Fatalf("%s 标签 = %v", key, tags)
		}
	}
	vec := got["vec"].Value.(map[string]interface{})
	if labels := vec["labels"].([]string); len(labels) != 3 || labels[0] != "a" {
		t.Fatalf("向量标签应补齐: %v", labels)
	}
	arr := got["arr"].Value.(map[string]interface{})["elements"].([]interface{})
	for _, e := range arr {
		if v, ok := e.(int); !ok || v < 0 || v > 10 {
			t.Fatalf("int 数组元素超出范围: %v", arr)
		}
	}
}

func TestCompositeMockInitAndRestart(t *testing.T) {
	if err := NewCompositeMockAdapter().Init(json.RawMessage(`{`)); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
	// 间隔非法、为 0 或负数时使用默认值（以前 0 会让 NewTicker panic）
	for _, iv := range []string{"bad", "0s", "-1s"} {
		a := NewCompositeMockAdapter()
		if err := a.Init(json.RawMessage(`{"interval":"` + iv + `"}`)); err != nil || a.interval != 3*time.Second {
			t.Fatalf("interval %s: %v %v", iv, err, a.interval)
		}
	}
	// int 数组 max_value < 1 时以前 rand.Intn(0) 会 panic
	a := NewCompositeMockAdapter()
	if err := a.Init(json.RawMessage(`{"interval":"5ms","composite_data_types":[{"device_id":"d","key":"a","data_type":"array","array_config":{"size":2,"element_type":"int","max_value":0}}]}`)); err != nil {
		t.Fatal(err)
	}
	ch := make(chan model.Point, 10)
	ctx, cancel := context.WithCancel(context.Background())
	if err := a.Start(ctx, ch); err != nil {
		t.Fatal(err)
	}
	p := <-ch
	_ = a.Stop()
	// 停止后可以重新启动；取消上下文同样停止
	if err := a.Start(ctx, ch); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = a.Stop()
	if el := p.Value.(map[string]interface{})["elements"].([]interface{}); el[0] != 0 {
		t.Fatalf("elements = %v", el)
	}
}

func TestCompositeMockGenerators(t *testing.T) {
	a := NewCompositeMockAdapter()
	// location：静态与移动；移动时保持在范围内
	for _, move := range []bool{true, false} {
		dp := &CompositeDataPoint{DataType: "location", LocationConfig: &config.MockLocationConfig{StartLatitude: 31, StartLongitude: 121,
			LatitudeRange: 0.001, LongitudeRange: 0.001, SpeedMin: 1000, SpeedMax: 2000, SimulateMovement: move}}
		_ = a.initCompositeDataState(dp)
		dp.locationState.LastUpdate = time.Now().Add(-time.Hour) // 让一次更新移动很远，触发边界处理
		for i := 0; i < 20; i++ {
			v, err := a.generateLocationData(dp)
			if err != nil {
				t.Fatal(err)
			}
			m := v.(map[string]interface{})
			if lat := m["latitude"].(float64); lat < 31-0.0005-1e-9 || lat > 31+0.0005+1e-9 {
				t.Fatalf("move=%v 纬度超出范围: %v", move, lat)
			}
		}
	}
	// vector3d：振荡与随机，均在范围内
	for _, osc := range []bool{true, false} {
		dp := &CompositeDataPoint{DataType: "vector3d", Vector3DConfig: &config.MockVector3DConfig{XMin: -1, XMax: 1, YMin: -1, YMax: 1, ZMin: -1, ZMax: 1, Oscillation: osc, Frequency: 1}}
		_ = a.initCompositeDataState(dp)
		for i := 0; i < 20; i++ {
			v, _ := a.generateVector3DData(dp)
			m := v.(map[string]interface{})
			if x := m["x"].(float64); x < -1 || x > 1 {
				t.Fatalf("x 超出范围: %v", x)
			}
		}
	}
	// color：色相循环覆盖所有区间
	dp := &CompositeDataPoint{DataType: "color", ColorConfig: &config.MockColorConfig{}}
	_ = a.initCompositeDataState(dp)
	for i := 0; i < 200; i++ {
		v, _ := a.generateColorData(dp)
		m := v.(map[string]interface{})
		for _, c := range []string{"r", "g", "b"} {
			if x := m[c].(int); x < 0 || x > 255 {
				t.Fatalf("%s 超出范围: %v", c, x)
			}
		}
	}
	// vector：三种模式
	for _, pattern := range []string{"smooth", "oscillate", "random"} {
		dp := &CompositeDataPoint{DataType: "vector", VectorConfig: &config.MockVectorConfig{Dimension: 2, MinValues: []float64{0}, MaxValues: []float64{1}}}
		_ = a.initCompositeDataState(dp)
		dp.vectorState.Pattern = pattern
		for i := 0; i < 10; i++ {
			v, _ := a.generateVectorData(dp)
			vals := v.(map[string]interface{})["values"].([]float64)
			if vals[0] < 0 || vals[0] > 1 || vals[1] < 0 || vals[1] > 100 {
				t.Fatalf("%s 超出范围: %v", pattern, vals)
			}
		}
	}
	// array：各元素类型
	for _, et := range []string{"float", "int", "bool", "mixed"} {
		dp := &CompositeDataPoint{DataType: "array", ArrayConfig: &config.MockArrayConfig{Size: 3, ElementType: et, MaxValue: 50}}
		_ = a.initCompositeDataState(dp)
		dp.arrayState.LastValues = dp.arrayState.LastValues[:1] // 覆盖"没有上次值"的分支
		for i := 0; i < 5; i++ {
			v, _ := a.generateArrayData(dp)
			if el := v.(map[string]interface{})["elements"].([]interface{}); len(el) != 3 {
				t.Fatalf("%s: %v", et, el)
			}
		}
		dp.arrayState.LastValues = []interface{}{"x", "x", "x"} // 上次值类型不符
		if _, err := a.generateArrayData(dp); err != nil {
			t.Fatal(err)
		}
	}
	// 缺少状态或配置
	empty := &CompositeDataPoint{}
	for name, fn := range map[string]func(*CompositeDataPoint) (interface{}, error){
		"location": a.generateLocationData, "vector3d": a.generateVector3DData, "color": a.generateColorData,
		"vector": a.generateVectorData, "array": a.generateArrayData,
	} {
		if _, err := fn(empty); err == nil {
			t.Errorf("%s 缺少配置应返回错误", name)
		}
	}
}

func TestCompositeMockSendTimeout(t *testing.T) {
	a := NewCompositeMockAdapter()
	if err := a.Init(json.RawMessage(`{"composite_data_types":[{"device_id":"d","key":"c","data_type":"color","color_config":{}}]}`)); err != nil {
		t.Fatal(err)
	}
	a.ch = make(chan model.Point) // 无缓冲且无人接收
	start := time.Now()
	a.generateAndPublishData()
	if time.Since(start) < 90*time.Millisecond {
		t.Fatal("通道阻塞时应等待超时后放弃")
	}
}

func TestSimHelpers(t *testing.T) {
	if simIntUpTo(0) != 0 || simIntUpTo(-5) != 0 {
		t.Fatal("max < 1 应返回 0")
	}
	if v := simIntUpTo(3); v < 0 || v >= 3 {
		t.Fatal("超出范围")
	}
	if f := simFloat32(); f < 0 || f >= 1 {
		t.Fatal("simFloat32")
	}
}
