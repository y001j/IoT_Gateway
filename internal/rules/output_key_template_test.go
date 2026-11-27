package rules

import (
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// output_key 中的模板会被替换（以前只有 output.key_template 支持，"{{.Key}}_stats" 被原样当作键名）
func TestAggregateOutputKeyTemplate(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	s := startService(t, nc, []*Rule{}, nil)
	sub, _ := nc.SubscribeSync("iot.rules.aggregate.>")
	nc.Flush()
	p := model.NewPoint("humidity", "temp_sensor_01", 50.0, model.TypeFloat)
	res := &AggregateResult{DeviceID: p.DeviceID, Key: p.Key, Functions: map[string]interface{}{"avg": 50.0}, Count: 1}
	action := &Action{Type: "aggregate", Config: map[string]interface{}{"output_key": "{{.Key}}_stats", "forward": true, "functions": []interface{}{"avg"}}}
	if err := s.handleAggregateResult(res, p, &Rule{ID: "r"}, action); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Subject != model.RuleOutputSubject("aggregate", "temp_sensor_01", "humidity_stats") {
		t.Fatalf("输出主题 = %s", msg.Subject)
	}

	pt := model.NewPoint("t", "d1", 1.0, model.TypeFloat)
	for tpl, want := range map[string]string{
		"{{.Key}}_{{.DeviceID}}": "t_d1", "{{key}}-{{device_id}}": "t-d1", "{{.key}}/{{.device_id}}": "t/d1",
		"%s_avg": "t_avg", "plain": "plain", "": "t_processed",
	} {
		if got := s.formatOutputKey(tpl, pt); got != want {
			t.Errorf("formatOutputKey(%q) = %q, want %q", tpl, got, want)
		}
	}
}
