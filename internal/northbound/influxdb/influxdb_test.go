package influxdb

import (
	"strings"
	"testing"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/y001j/iot-gateway/internal/model"
)

func lineOf(p *write.Point) string {
	return write.PointToLineProtocol(p, time.Nanosecond)
}

func TestBuildInfluxPointUsesAcquisitionTime(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p := model.NewPoint("temperature", "dev-1", 21.5, model.TypeFloat)
	p.Timestamp = ts
	ip, org, bucket, err := buildInfluxPoint(p, nil, "org", "bucket")
	if err != nil {
		t.Fatal(err)
	}
	if !ip.Time().Equal(ts) {
		t.Fatalf("应使用采集时间 %v，实际 %v（以前错误地使用 time.Now()）", ts, ip.Time())
	}
	if org != "org" || bucket != "bucket" {
		t.Fatalf("默认 org/bucket 错误")
	}
	line := lineOf(ip)
	if !strings.HasPrefix(line, "temperature,") || !strings.Contains(line, "device_id=dev-1") || !strings.Contains(line, "value=21.5") {
		t.Fatalf("line protocol = %s", line)
	}
}

func TestBuildInfluxPointIntegerTypes(t *testing.T) {
	// 回归：Modbus 产生的 int64/uint64 以前会导致整批写入失败
	for _, v := range []interface{}{int64(42), uint64(42), int32(42), uint16(42), 42} {
		ip, _, _, err := buildInfluxPoint(model.Point{Key: "k", DeviceID: "d", Type: model.TypeInt, Value: v}, nil, "o", "b")
		if err != nil {
			t.Fatalf("%T 应可写入: %v", v, err)
		}
		if line := lineOf(ip); !strings.Contains(line, "value=42i") {
			t.Fatalf("%T line = %s", v, line)
		}
	}
	if _, _, _, err := buildInfluxPoint(model.Point{Key: "k", Type: model.TypeInt, Value: "abc"}, nil, "o", "b"); err == nil {
		t.Fatalf("无法转换的值应返回错误")
	}
}

func TestBuildInfluxPointConfigAndTags(t *testing.T) {
	p := model.NewPoint("pressure", "d", 1.0, model.TypeFloat)
	p.AddTag("site", "A")
	p.AddTag("serial", "123")
	cfg := map[string]PointConfig{"pressure": {Measurement: "press", Bucket: "b2", Org: "o2", Tags: map[string]string{"unit": "kPa"}, Fields: []string{"serial"}}}
	ip, org, bucket, err := buildInfluxPoint(p, cfg, "o", "b")
	if err != nil {
		t.Fatal(err)
	}
	line := lineOf(ip)
	if org != "o2" || bucket != "b2" || !strings.HasPrefix(line, "press,") {
		t.Fatalf("点位配置未生效: %s %s %s", org, bucket, line)
	}
	if !strings.Contains(line, "site=A") || !strings.Contains(line, "unit=kPa") || !strings.Contains(line, `serial="123"`) {
		t.Fatalf("标签/字段处理错误: %s", line)
	}
}

func TestBuildInfluxPointBoolStringComposite(t *testing.T) {
	ip, _, _, _ := buildInfluxPoint(model.Point{Key: "on", Type: model.TypeBool, Value: true}, nil, "o", "b")
	if !strings.Contains(lineOf(ip), "value=true") {
		t.Fatalf("bool line = %s", lineOf(ip))
	}
	ip, _, _, _ = buildInfluxPoint(model.Point{Key: "s", Type: model.TypeString, Value: 5}, nil, "o", "b")
	if !strings.Contains(lineOf(ip), `value="5"`) {
		t.Fatalf("string line = %s", lineOf(ip))
	}
	vec := model.NewCompositePoint("acc", "d", &model.Vector3D{X: 3, Y: 4})
	ip, _, _, err := buildInfluxPoint(vec, nil, "o", "b")
	if err != nil {
		t.Fatal(err)
	}
	line := lineOf(ip)
	if !strings.Contains(line, "magnitude=5") || !strings.Contains(line, `"x\":3`) {
		t.Fatalf("复合类型应展开数值衍生字段并保留 JSON: %s", line)
	}
}
