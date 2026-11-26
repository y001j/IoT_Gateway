package config

import (
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

// 每个默认配置只补齐必填字段即可通过校验，且解析后保留默认值
func TestSinkDefaultsParseAndValidate(t *testing.T) {
	base := `"name":"s","type":"t"`
	mqtt, err := NewParserWithDefaults(GetDefaultMQTTSinkConfig()).Parse([]byte(`{` + base + `,"broker":"tcp://localhost:1883","topic":"x"}`))
	if err != nil || mqtt.BatchSize != 1 || mqtt.BufferSize != 1000 || mqtt.FlushTimeout.Duration() != 5*time.Second || !mqtt.Enabled {
		t.Fatalf("mqtt: %v %+v", err, mqtt)
	}
	influx, err := NewParserWithDefaults(GetDefaultInfluxDBConfig()).Parse([]byte(`{` + base + `,"url":"http://localhost:8086","token":"t","org":"o","bucket":"b"}`))
	if err != nil || influx.BatchSize != 100 || influx.Precision != "ms" || influx.FlushInterval != 1000 {
		t.Fatalf("influxdb: %v %+v", err, influx)
	}
	redis, err := NewParserWithDefaults(GetDefaultRedisConfig()).Parse([]byte(`{` + base + `,"host":"localhost"}`))
	if err != nil || redis.Port != 6379 || redis.BatchSize != 10 {
		t.Fatalf("redis: %v %+v", err, redis)
	}
	console, err := NewParserWithDefaults(GetDefaultConsoleConfig()).Parse([]byte(`{` + base + `}`))
	if err != nil || console.Format != "json" || !console.Colors {
		t.Fatalf("console: %v %+v", err, console)
	}
	ws, err := NewParserWithDefaults(GetDefaultWebSocketConfig()).Parse([]byte(`{` + base + `}`))
	if err != nil || ws.Port != 8083 || ws.Path != "/ws" {
		t.Fatalf("websocket: %v %+v", err, ws)
	}
	js, err := NewParserWithDefaults(GetDefaultJetStreamConfig()).Parse([]byte(`{` + base + `,"stream":"S","subject":"x.>"}`))
	if err != nil || js.BatchSize != 100 || js.BufferSize != 10000 {
		t.Fatalf("jetstream: %v %+v", err, js)
	}
	// 显式配置覆盖默认值
	redis, err = NewParserWithDefaults(GetDefaultRedisConfig()).Parse([]byte(`{` + base + `,"host":"h","port":7000}`))
	if err != nil || redis.Port != 7000 {
		t.Fatalf("覆盖默认端口: %v %+v", err, redis)
	}
	// 缺少必填字段时报错
	if _, err := NewParserWithDefaults(GetDefaultJetStreamConfig()).Parse([]byte(`{` + base + `}`)); err == nil {
		t.Fatal("jetstream 缺少 stream/subject 应报错")
	}
}

func TestPluginDefaults(t *testing.T) {
	meta := GetDefaultPluginMetadata()
	if meta.Version != "1.0.0" || meta.Mode != "builtin" || meta.ISPTimeout != 30*time.Second || meta.Tags == nil {
		t.Fatalf("metadata: %+v", meta)
	}
	sidecar := GetDefaultSidecarPluginConfig()
	if !sidecar.Enabled || !sidecar.AutoRestart || sidecar.MaxRetries != 3 || sidecar.PluginConfig == nil || sidecar.Tags == nil {
		t.Fatalf("sidecar: %+v", sidecar)
	}
	ext := GetDefaultExternalPluginConfig()
	if !ext.Enabled || ext.Timeout != 60*time.Second || ext.Environment == nil || ext.MaxRetries != 3 {
		t.Fatalf("external: %+v", ext)
	}
	// 默认值配合必填字段可通过校验
	sc, err := NewParserWithDefaults(sidecar).Parse([]byte(`{"name":"p","type":"sidecar","isp_port":9000,"entry":"./p"}`))
	if err != nil || sc.MaxRetries != 3 {
		t.Fatalf("sidecar 解析: %v %+v", err, sc)
	}
	ec, err := NewParserWithDefaults(ext).Parse([]byte(`{"name":"p","type":"external","command":"./p"}`))
	if err != nil || ec.Timeout != 60*time.Second {
		t.Fatalf("external 解析: %v %+v", err, ec)
	}
}

func TestValidateRangeAndUnsignedBounds(t *testing.T) {
	check := func(v interface{}) error { return validateStruct(reflect.ValueOf(v), "") }
	type okRange struct {
		Port int `validate:"range=1-100"`
	}
	if err := check(okRange{Port: 50}); err != nil {
		t.Fatalf("range 内应通过: %v", err)
	}
	for _, v := range []okRange{{Port: 0}, {Port: 101}} {
		if err := check(v); err == nil {
			t.Errorf("%d 超出 range 应失败", v.Port)
		}
	}
	type negMin struct {
		Count uint `validate:"min=-5"`
	}
	if err := check(negMin{Count: 0}); err != nil {
		t.Fatalf("负下限对无符号字段应始终满足: %v", err)
	}
	type negMax struct {
		Limit uint `validate:"max=-1"`
	}
	if err := check(negMax{Limit: 0}); err == nil {
		t.Fatal("负上限对无符号字段应始终失败")
	}
	type badRange struct {
		Bad int `validate:"range=1"`
	}
	if err := check(badRange{}); err == nil {
		t.Fatal("非法 range 格式应报错")
	}
	type badRange2 struct {
		Bad int `validate:"range=a-b"`
	}
	if err := check(badRange2{}); err == nil {
		t.Fatal("非法 range 数值应报错")
	}
	type uport struct {
		P uint32 `validate:"port"`
	}
	if err := check(uport{P: 70000}); err == nil {
		t.Fatal("无符号端口超过 65535 应失败")
	}
	if err := check(uport{P: 8080}); err != nil {
		t.Fatalf("合法无符号端口: %v", err)
	}
}

func TestDurationYAML(t *testing.T) {
	type wrap struct {
		D Duration `yaml:"d"`
	}
	for in, want := range map[string]time.Duration{"d: 1500\n": 1500 * time.Millisecond, "d: 2.5\n": 2 * time.Millisecond, "d: 3s\n": 3 * time.Second} {
		var w wrap
		if err := yaml.Unmarshal([]byte(in), &w); err != nil || w.D.Duration() != want {
			t.Errorf("%q -> %v %v, want %v", in, w.D.Duration(), err, want)
		}
	}
	for _, in := range []string{"d: abc\n", "d: [1]\n"} {
		var w wrap
		if err := yaml.Unmarshal([]byte(in), &w); err == nil {
			t.Errorf("%q 应报错", in)
		}
	}
	out, err := yaml.Marshal(wrap{D: Duration(90 * time.Second)})
	if err != nil || string(out) != "d: 1m30s\n" {
		t.Fatalf("MarshalYAML: %q %v", out, err)
	}
}
