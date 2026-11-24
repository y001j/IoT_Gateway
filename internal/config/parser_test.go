package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestModbusConfigDefaultsAndValidation(t *testing.T) {
	p := NewParserWithDefaults(GetDefaultModbusConfig())
	cfg, err := p.Parse(json.RawMessage(`{
		"name": "plc1", "type": "modbus",
		"host": "10.0.0.1",
		"registers": [
			{"address": 0, "type": "holding_register", "device_id": "d", "key": "k"}
		]
	}`))
	if err != nil {
		t.Fatalf("合法配置解析失败: %v", err)
	}
	if cfg.Port != 502 || cfg.SlaveID != 1 || cfg.Protocol != "tcp" {
		t.Fatalf("默认值未生效: %+v", cfg)
	}
	if cfg.Interval.Duration() != 5*time.Second {
		t.Fatalf("默认采集间隔 = %v", cfg.Interval.Duration())
	}
}

func TestOneOfAllowsEmpty(t *testing.T) {
	// parity/byte_order/data_type 未配置时不应因 oneof 校验失败
	p := NewParserWithDefaults(GetDefaultModbusConfig())
	_, err := p.Parse(json.RawMessage(`{"name":"x","type":"modbus","host":"h","registers":[{"type":"coil","device_id":"d","key":"k"}]}`))
	if err != nil {
		t.Fatalf("空的可选枚举字段不应报错: %v", err)
	}
}

func TestSliceElementsAreValidated(t *testing.T) {
	p := NewParserWithDefaults(GetDefaultModbusConfig())
	cases := map[string]string{
		"寄存器类型非法":     `{"name":"x","type":"modbus","host":"h","registers":[{"type":"bogus","device_id":"d","key":"k"}]}`,
		"数据类型非法":      `{"name":"x","type":"modbus","host":"h","registers":[{"type":"coil","data_type":"int128","device_id":"d","key":"k"}]}`,
		"缺少device_id": `{"name":"x","type":"modbus","host":"h","registers":[{"type":"coil","key":"k"}]}`,
		"字节序非法":       `{"name":"x","type":"modbus","host":"h","byte_order":"XYZW","registers":[{"type":"coil","device_id":"d","key":"k"}]}`,
		"协议非法":        `{"name":"x","type":"modbus","host":"h","protocol":"udp","registers":[{"type":"coil","device_id":"d","key":"k"}]}`,
		"缺少寄存器":       `{"name":"x","type":"modbus","host":"h","registers":[]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := p.Parse(json.RawMessage(raw)); err == nil {
				t.Fatalf("应校验失败")
			}
		})
	}
}

func TestNewDataTypesAccepted(t *testing.T) {
	p := NewParserWithDefaults(GetDefaultModbusConfig())
	for _, dt := range []string{"uint16", "int16", "uint32", "int32", "float32", "uint64", "int64", "float64"} {
		raw := `{"name":"x","type":"modbus","host":"h","registers":[{"type":"holding_register","data_type":"` + dt + `","device_id":"d","key":"k"}]}`
		if _, err := p.Parse(json.RawMessage(raw)); err != nil {
			t.Errorf("数据类型 %s 应被接受: %v", dt, err)
		}
	}
}

func TestValidateFieldRules(t *testing.T) {
	type sample struct {
		Name  string `validate:"required"`
		Count int    `validate:"min=1,max=10"`
		Port  int    `validate:"port"`
		URL   string `validate:"url"`
		Mode  string `validate:"oneof=a b"`
	}
	p := NewParser[sample]()
	ok := &sample{Name: "n", Count: 5, Port: 80, URL: "http://x", Mode: "a"}
	if err := p.Validate(ok); err != nil {
		t.Fatalf("合法结构体校验失败: %v", err)
	}
	bad := []sample{
		{Count: 5, Port: 80},                          // 缺少 Name
		{Name: "n", Count: 0, Port: 80},               // Count < 1
		{Name: "n", Count: 11, Port: 80},              // Count > 10
		{Name: "n", Count: 1, Port: 70000},            // 端口越界
		{Name: "n", Count: 1, Port: 80, URL: "ftp:x"}, // URL 非法
		{Name: "n", Count: 1, Port: 80, Mode: "c"},    // 枚举非法
	}
	for i := range bad {
		if err := p.Validate(&bad[i]); err == nil {
			t.Errorf("用例 %d 应校验失败: %+v", i, bad[i])
		}
	}
}

func TestParseFromMap(t *testing.T) {
	p := NewParserWithDefaults(GetDefaultModbusConfig())
	cfg, err := p.ParseFromMap(map[string]interface{}{
		"name": "m", "type": "modbus", "host": "h",
		"registers": []interface{}{map[string]interface{}{"type": "coil", "device_id": "d", "key": "k"}},
	})
	if err != nil || cfg.Host != "h" {
		t.Fatalf("ParseFromMap 失败: %v", err)
	}
	if _, err := p.Parse(json.RawMessage(`{bad`)); err == nil || !strings.Contains(err.Error(), "") {
		t.Fatalf("非法 JSON 应返回错误")
	}
}
