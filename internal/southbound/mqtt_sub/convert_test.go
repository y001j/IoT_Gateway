package mqtt_sub

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestMQTTSubCompositeTypesEndToEnd(t *testing.T) {
	nc, broker := testutil.StartNATSWithMQTT(t)
	a := NewAdapter().(*MQTTSubAdapter)
	cfg := fmt.Sprintf(`{"name":"sub","type":"mqtt_sub","broker":%q,"client_id":"sub-composite","topics":[
		{"topic":"gps/1","key":"location","type":"location","device_id":"truck1","tags":{"fleet":"a"},
		 "composite":{"location":{"latitude_path":"pos.lat","longitude_path":"pos.lng","altitude_path":"alt","speed_path":"spd","heading_path":"hdg","accuracy_path":"acc"}}},
		{"topic":"imu/1","key":"accel","type":"vector3d","composite":{"vector3d":{"x_path":"x","y_path":"y","z_path":"z"}}},
		{"topic":"led/1","key":"color","type":"color","composite":{"color":{"red_path":"r","green_path":"g","blue_path":"b","alpha_path":"a"}}},
		{"topic":"raw/1","key":"text","type":"string"},
		{"topic":"plain/1","key":"plain","type":"float"},
		{"topic":"quoted/1","key":"quoted","type":"string"},
		{"topic":"obj/1","key":"objtext","type":"string"},
		{"topic":"bad/1","key":"badloc","type":"location","composite":{"location":{"latitude_path":"a","longitude_path":"b"}}},
		{"topic":"num/1","key":"count","type":"int","path":"n"}]}`, broker)
	if err := a.Init(json.RawMessage(cfg)); err != nil {
		t.Fatal(err)
	}
	ch := make(chan model.Point, 20)
	if err := a.Start(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	time.Sleep(300 * time.Millisecond)

	pub := func(subject, payload string) { _ = nc.Publish(subject, []byte(payload)) }
	pub("gps.1", `{"pos":{"lat":31.23,"lng":121.47},"alt":12.5,"spd":"60","hdg":90,"acc":3}`)
	pub("imu.1", `{"x":0.1,"y":-0.2,"z":9.8}`)
	pub("led.1", `{"r":300,"g":"128","b":-5,"a":0.5}`)
	pub("raw.1", `not json at all`)
	pub("plain.1", ` 23.5 `)
	pub("quoted.1", `"hello"`)
	pub("obj.1", `{"a":1}`)
	pub("bad.1", `12`)          // 复合类型必须是 JSON 对象：丢弃
	pub("num.1", `{"other":1}`) // 路径不存在：丢弃
	pub("num.1", `{"n":"42"}`)
	pub("gps.1", `{"pos":{"lat":95,"lng":0}}`) // 纬度越界：验证失败，丢弃
	_ = nc.Flush()

	got := map[string]model.Point{}
	deadline := time.After(5 * time.Second)
	for len(got) < 8 {
		select {
		case p := <-ch:
			got[p.Key] = p
		case <-deadline:
			t.Fatalf("超时，已收到: %v", got)
		}
	}
	loc, ok := got["location"].Value.(*model.LocationData)
	if !ok || got["location"].Type != model.TypeLocation || loc.Latitude != 31.23 || loc.Altitude != 12.5 || loc.Speed != 60 || loc.Heading != 90 || loc.Accuracy != 3 {
		t.Fatalf("位置解析错误: %+v", got["location"])
	}
	locPoint := got["location"]
	if tags := locPoint.GetTagsCopy(); tags["fleet"] != "a" || tags["source"] != "mqtt_sub" || locPoint.DeviceID != "truck1" {
		t.Fatalf("标签或设备错误: %v %s", tags, got["location"].DeviceID)
	}
	if v, ok := got["accel"].Value.(*model.Vector3D); !ok || v.Z != 9.8 || got["accel"].DeviceID != "truck1" {
		t.Fatalf("向量解析错误（未配置 device_id 时使用第一个主题的设备）: %+v", got["accel"])
	}
	if c, ok := got["color"].Value.(*model.ColorData); !ok || c.R != 255 || c.G != 128 || c.B != 0 || c.A != 127 {
		t.Fatalf("颜色解析错误（越界分量应截断）: %+v", got["color"].Value)
	}
	if got["text"].Value != "not json at all" || got["text"].Type != model.TypeString {
		t.Fatalf("非 JSON 消息应作为字符串: %+v", got["text"])
	}
	// 回归：非 JSON 对象的消息以前全部被丢弃
	if got["plain"].Value != 23.5 || got["plain"].Type != model.TypeFloat {
		t.Fatalf("纯数值消息应按配置类型转换: %+v", got["plain"])
	}
	if got["quoted"].Value != "hello" {
		t.Fatalf("JSON 字符串应去掉引号: %+v", got["quoted"])
	}
	if got["objtext"].Value != `{"a":1}` {
		t.Fatalf("未指定路径的字符串类型应保留原文: %+v", got["objtext"])
	}
	if got["count"].Value != 42 || got["count"].Type != model.TypeInt {
		t.Fatalf("整数解析错误: %+v", got["count"])
	}
	select {
	case p := <-ch:
		t.Fatalf("无效消息不应产生数据点: %+v", p)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestConvertBasicType(t *testing.T) {
	a := &MQTTSubAdapter{}
	ok := []struct {
		typ  string
		in   interface{}
		want interface{}
		dt   model.DataType
	}{
		{"int", 3.9, 3, model.TypeInt}, {"int", "12", 12, model.TypeInt}, {"int", 7, 7, model.TypeInt},
		{"float", 1.5, 1.5, model.TypeFloat}, {"float", 2, 2.0, model.TypeFloat}, {"float", "2.25", 2.25, model.TypeFloat},
		{"bool", true, true, model.TypeBool}, {"bool", "on", true, model.TypeBool}, {"bool", "no", false, model.TypeBool},
		{"bool", 0.0, false, model.TypeBool}, {"bool", 2, true, model.TypeBool},
		{"string", "s", "s", model.TypeString}, {"string", 1.5, "1.5", model.TypeString},
		{"", "x", "x", model.TypeString}, {"other", true, "true", model.TypeString},
	}
	for _, c := range ok {
		dt, v, err := a.convertBasicType(c.typ, c.in)
		if err != nil || v != c.want || dt != c.dt {
			t.Errorf("convertBasicType(%s, %v) = %v %v %v, want %v %v", c.typ, c.in, dt, v, err, c.want, c.dt)
		}
	}
	bad := []struct {
		typ string
		in  interface{}
	}{{"int", "abc"}, {"int", true}, {"float", "x"}, {"float", []int{}}, {"bool", "maybe"}, {"bool", nil}}
	for _, c := range bad {
		if _, _, err := a.convertBasicType(c.typ, c.in); err == nil {
			t.Errorf("convertBasicType(%s, %v) 应报错", c.typ, c.in)
		}
	}
}

func TestConvertScalarPayload(t *testing.T) {
	a := &MQTTSubAdapter{}
	cases := []struct {
		typ, payload string
		want         interface{}
		dt           model.DataType
	}{
		{"float", "1.5", 1.5, model.TypeFloat},
		{"int", "7", 7, model.TypeInt},
		{"bool", "true", true, model.TypeBool},
		{"bool", "on", true, model.TypeBool},
		{"string", "plain text", "plain text", model.TypeString},
		{"", `"q"`, "q", model.TypeString},
		{"string", "[1,2]", "[1,2]", model.TypeString},
		{"string", "42", "42", model.TypeString},
		{"string", "null", "null", model.TypeString},
	}
	for _, c := range cases {
		v, dt, err := a.convertScalarPayload(c.typ, []byte(c.payload))
		if err != nil || v != c.want || dt != c.dt {
			t.Errorf("convertScalarPayload(%s, %q) = %v %v %v, want %v", c.typ, c.payload, v, dt, err, c.want)
		}
	}
	for _, typ := range []string{"location", "vector3d", "color"} {
		if _, _, err := a.convertScalarPayload(typ, []byte("1")); err == nil {
			t.Errorf("%s 类型的非对象消息应报错", typ)
		}
	}
	if _, _, err := a.convertScalarPayload("float", []byte("abc")); err == nil {
		t.Error("无法转换的纯文本应报错")
	}
}

func TestExtractHelpers(t *testing.T) {
	a := &MQTTSubAdapter{}
	data := map[string]interface{}{"a": map[string]interface{}{"b": 1.5, "s": "2.5", "bad": "x", "i": 3, "obj": true}, "leaf": 1.0}
	for path, want := range map[string]float64{"a.b": 1.5, "a.s": 2.5, "a.i": 3, "": 0} {
		if v, err := a.extractFloatValue(data, path); err != nil || v != want {
			t.Errorf("extractFloatValue(%q) = %v %v", path, v, err)
		}
	}
	for _, path := range []string{"a.bad", "a.obj", "a.missing", "x.y", "leaf.y"} {
		if _, err := a.extractFloatValue(data, path); err == nil {
			t.Errorf("extractFloatValue(%q) 应报错", path)
		}
	}
	if _, err := a.extractValue(data, ""); err == nil {
		t.Error("空路径应报错")
	}

	// 缺少复合配置
	empty := TopicConfig{}
	if _, _, err := a.extractLocationData(data, empty); err == nil {
		t.Error("location 缺少配置应报错")
	}
	if _, _, err := a.extractVector3DData(data, empty); err == nil {
		t.Error("vector3d 缺少配置应报错")
	}
	if _, _, err := a.extractColorData(data, empty); err == nil {
		t.Error("color 缺少配置应报错")
	}
	// 必需字段提取失败
	locCfg := TopicConfig{Composite: &CompositeExtractConfig{Location: &LocationExtractConfig{LatitudePath: "a.b", LongitudePath: "nope"}}}
	if _, _, err := a.extractLocationData(data, locCfg); err == nil || !strings.Contains(err.Error(), "经度") {
		t.Errorf("经度缺失应报错: %v", err)
	}
	locCfg.Composite.Location.LatitudePath = "nope"
	if _, _, err := a.extractLocationData(data, locCfg); err == nil || !strings.Contains(err.Error(), "纬度") {
		t.Errorf("纬度缺失应报错: %v", err)
	}
	for i, paths := range [][3]string{{"nope", "a.b", "a.b"}, {"a.b", "nope", "a.b"}, {"a.b", "a.b", "nope"}} {
		vc := TopicConfig{Composite: &CompositeExtractConfig{Vector3D: &Vector3DExtractConfig{XPath: paths[0], YPath: paths[1], ZPath: paths[2]}}}
		if _, _, err := a.extractVector3DData(data, vc); err == nil {
			t.Errorf("向量第 %d 个分量缺失应报错", i)
		}
		cc := TopicConfig{Composite: &CompositeExtractConfig{Color: &ColorExtractConfig{RedPath: paths[0], GreenPath: paths[1], BluePath: paths[2]}}}
		if _, _, err := a.extractColorData(data, cc); err == nil {
			t.Errorf("颜色第 %d 个分量缺失应报错", i)
		}
	}
	// 可选的 alpha 提取失败时默认不透明；不配置 alpha 也默认不透明
	cc := TopicConfig{Composite: &CompositeExtractConfig{Color: &ColorExtractConfig{RedPath: "a.b", GreenPath: "a.b", BluePath: "a.b", AlphaPath: "nope"}}}
	if v, _, err := a.extractColorData(data, cc); err != nil || v.(*model.ColorData).A != 255 {
		t.Errorf("alpha 缺失应默认 255: %v %v", v, err)
	}
	cc.Composite.Color.AlphaPath = ""
	if v, _, err := a.extractColorData(data, cc); err != nil || v.(*model.ColorData).A != 255 {
		t.Errorf("未配置 alpha 应默认 255: %v %v", v, err)
	}
}

// writeSelfSigned 生成自签名证书和私钥（PEM）
func writeSelfSigned(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	certPath, keyPath = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	return
}

func TestConfigureTLS(t *testing.T) {
	a := NewAdapter().(*MQTTSubAdapter)
	if err := a.Init(json.RawMessage(`{"name":"tls","type":"mqtt_sub","broker":"tcp://127.0.0.1:1","topics":[{"topic":"a","key":"k"}]}`)); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cert, key := writeSelfSigned(t, dir)
	garbage := filepath.Join(dir, "garbage.pem")
	os.WriteFile(garbage, []byte("not a cert"), 0o600)

	opts := mqtt.NewClientOptions()
	if err := a.configureTLS(opts, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.configureTLS(opts, &config.TLSConfig{CACert: cert, ClientCert: cert, ClientKey: key, SkipVerify: true}); err != nil {
		t.Fatalf("有效证书应配置成功: %v", err)
	}
	if tc := opts.TLSConfig; tc == nil || !tc.InsecureSkipVerify || tc.RootCAs == nil || len(tc.Certificates) != 1 {
		t.Fatalf("TLS 配置未生效: %+v", tc)
	}
	for name, tc := range map[string]*config.TLSConfig{
		"CA 文件不存在": {CACert: filepath.Join(dir, "missing.pem")},
		"CA 内容无效":  {CACert: garbage},
		"客户端证书无效":  {ClientCert: garbage, ClientKey: key},
	} {
		if err := a.configureTLS(mqtt.NewClientOptions(), tc); err == nil {
			t.Errorf("%s 应报错", name)
		}
	}
	// Init 时 TLS 配置错误应返回错误
	bad := fmt.Sprintf(`{"name":"tls","type":"mqtt_sub","broker":"tcp://127.0.0.1:1","topics":[{"topic":"a","key":"k"}],"tls":{"ca_cert":%q}}`, garbage)
	if err := NewAdapter().Init(json.RawMessage(bad)); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("TLS 配置错误时 Init 应失败: %v", err)
	}
}
