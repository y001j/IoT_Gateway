package model

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestEncodeDecodeSubjectToken(t *testing.T) {
	cases := map[string]string{
		"dev-001":       "dev-001",
		"":              "_",
		"plc.line1":     "plc~2Eline1",
		"a*b>c":         "a~2Ab~3Ec",
		"has space":     "has~20space",
		"tilde~x":       "tilde~7Ex",
		"tab\tnl\ncr\r": "tab~09nl~0Acr~0D",
		"温度传感器":         "温度传感器",
		"mixed.中文 *>~":  "mixed~2E中文~20~2A~3E~7E",
	}
	for in, want := range cases {
		got := EncodeSubjectToken(in)
		if got != want {
			t.Errorf("Encode(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, ".*> \t\r\n") {
			t.Errorf("Encode(%q) 结果仍含非法字符: %q", in, got)
		}
		if back := DecodeSubjectToken(got); back != in {
			t.Errorf("Decode(Encode(%q)) = %q", in, back)
		}
	}
	// 非法转义序列按原样保留
	if got := DecodeSubjectToken("a~ZZb~"); got != "a~ZZb~" {
		t.Errorf("非法转义应原样保留, got %q", got)
	}
	// 小写十六进制也能解码
	if got := DecodeSubjectToken("a~2eb"); got != "a.b" {
		t.Errorf("小写十六进制解码失败: %q", got)
	}
}

func TestDataAndRuleSubjects(t *testing.T) {
	if got := DataSubject("dev1", "temp"); got != "iot.data.dev1.temp" {
		t.Fatalf("DataSubject = %q", got)
	}
	// 设备 ID 含点号时主题层级数量必须保持不变
	got := DataSubject("plc.1", "a.b")
	if n := strings.Count(got, "."); n != 3 {
		t.Fatalf("DataSubject 层级错误: %q", got)
	}
	if got := RuleOutputSubject("forward", "d", "k"); got != "iot.rules.forward.d.k" {
		t.Fatalf("RuleOutputSubject = %q", got)
	}
	if strings.HasPrefix(RuleOutputSubject("forward", "d", "k"), "iot.data.") {
		t.Fatalf("规则输出主题不能落在 iot.data 下")
	}
}

func TestPointJSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 11, 24, 10, 0, 0, 123, time.UTC)
	p := NewPoint("temperature", "dev-1", 42.5, TypeFloat)
	p.Timestamp = ts
	p.Quality = 2
	p.AddTag("source", "modbus")

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tags":{"source":"modbus"}`) {
		t.Fatalf("序列化结果应包含标签: %s", data)
	}
	var back Point
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Key != p.Key || back.DeviceID != p.DeviceID || back.Type != p.Type ||
		back.Quality != 2 || !back.Timestamp.Equal(ts) || back.Value.(float64) != 42.5 {
		t.Fatalf("往返结果不一致: %+v", back)
	}
	if v, ok := back.GetTag("source"); !ok || v != "modbus" {
		t.Fatalf("标签丢失")
	}
}

func TestPointJSONWithoutTags(t *testing.T) {
	p := Point{Key: "k", DeviceID: "d", Type: TypeInt, Value: 1}
	data, _ := json.Marshal(p)
	if strings.Contains(string(data), "tags") {
		t.Fatalf("无标签时不应输出 tags 字段: %s", data)
	}
	// 复用同一个 Point 反序列化时，旧标签必须被清除
	old := NewPoint("x", "y", 1, TypeInt)
	old.AddTag("stale", "1")
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	if _, ok := old.GetTag("stale"); ok {
		t.Fatalf("反序列化后不应残留旧标签")
	}
	if err := json.Unmarshal([]byte("{bad"), &old); err == nil {
		t.Fatalf("非法 JSON 应返回错误")
	}
}

func TestPointCloneIsolatesTags(t *testing.T) {
	p := NewPoint("k", "d", 1, TypeInt)
	p.AddTag("a", "1")
	c := p.Clone()
	c.AddTag("b", "2")
	c.AddTag("a", "changed")
	if v, _ := p.GetTag("a"); v != "1" {
		t.Fatalf("修改副本标签影响了原数据点")
	}
	if _, ok := p.GetTag("b"); ok {
		t.Fatalf("副本新增的标签出现在原数据点上")
	}
	// 无标签的点也能克隆
	q := Point{Key: "k"}
	if q.Clone().SafeTags != nil {
		t.Fatalf("无标签点克隆后不应凭空创建标签容器")
	}
}

func TestPointTagHelpers(t *testing.T) {
	var p Point
	if _, ok := p.GetTag("x"); ok {
		t.Fatalf("空点不应有标签")
	}
	if len(p.GetTagsCopy()) != 0 || len(p.GetTagsSafe()) != 0 {
		t.Fatalf("空点标签副本应为空")
	}
	p.SetTagsSafe(map[string]string{"a": "1", "b": "2"})
	if len(p.GetTagsCopy()) != 2 {
		t.Fatalf("SetTagsSafe 失败")
	}
	p.SetQuality(5)
	if p.Quality != 5 {
		t.Fatalf("SetQuality 失败")
	}
}

func TestCompositePoints(t *testing.T) {
	loc := &LocationData{Latitude: 39.9, Longitude: 116.4, Altitude: 50, Speed: 40}
	p := NewCompositePoint("gps", "car-1", loc)
	if p.Type != TypeLocation || !p.IsComposite() {
		t.Fatalf("复合点类型错误")
	}
	if v, ok := p.GetTag("coordinate_system"); !ok || v != "WGS84" {
		t.Fatalf("衍生值应写入标签")
	}
	got, err := p.GetLocationData()
	if err != nil || got.Latitude != 39.9 {
		t.Fatalf("GetLocationData 失败: %v", err)
	}
	if _, err := p.GetVector3DData(); err == nil {
		t.Fatalf("非向量点获取向量应报错")
	}
	cd, err := p.GetCompositeData()
	if err != nil || cd.Type() != TypeLocation {
		t.Fatalf("GetCompositeData 失败")
	}

	simple := NewPoint("t", "d", 1.0, TypeFloat)
	if simple.IsComposite() {
		t.Fatalf("简单点不应是复合类型")
	}
	if _, err := simple.GetCompositeData(); err == nil {
		t.Fatalf("简单点获取复合数据应报错")
	}

	vec := NewCompositePoint("acc", "d", &Vector3D{X: 3, Y: 4, Z: 0})
	v, err := vec.GetVector3DData()
	if err != nil || v.X != 3 {
		t.Fatalf("GetVector3DData 失败")
	}
	if m, _ := vec.GetTag("magnitude"); m != "5" {
		t.Fatalf("向量模长衍生值 = %q, want 5", m)
	}
}

func TestCompositeValidation(t *testing.T) {
	if err := (&LocationData{Latitude: 91}).Validate(); err == nil {
		t.Errorf("纬度越界应报错")
	}
	if err := (&LocationData{Longitude: -181}).Validate(); err == nil {
		t.Errorf("经度越界应报错")
	}
	if err := (&LocationData{Latitude: 10, Longitude: 10}).Validate(); err != nil {
		t.Errorf("合法坐标不应报错: %v", err)
	}
	if err := (&Vector3D{X: math.NaN()}).Validate(); err == nil {
		t.Errorf("NaN 向量应报错")
	}
	if err := (&VectorData{}).Validate(); err == nil {
		t.Errorf("空向量应报错")
	}
	vd := &VectorData{Values: []float64{1, 2}}
	if err := vd.Validate(); err != nil || vd.Dimension != 2 {
		t.Errorf("向量维度应自动设置: %v", err)
	}
	if err := (&VectorData{Values: []float64{1}, Dimension: 3}).Validate(); err == nil {
		t.Errorf("维度不匹配应报错")
	}
	if err := (&ArrayData{}).Validate(); err == nil {
		t.Errorf("空数组应报错")
	}
	ad := &ArrayData{Values: []interface{}{1, 2, 3}}
	if err := ad.Validate(); err != nil || ad.Size != 3 {
		t.Errorf("数组大小应自动设置")
	}
}
