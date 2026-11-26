package rules

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func TestExpressionFunctionsDescribed(t *testing.T) {
	e := NewExpressionEngine()
	if len(e.functions) < 40 {
		t.Fatalf("注册的函数太少: %d", len(e.functions))
	}
	for name, fn := range e.functions {
		if fn.Name() != name {
			t.Errorf("函数注册名 %q 与 Name() %q 不一致", name, fn.Name())
		}
		if strings.TrimSpace(fn.Description()) == "" {
			t.Errorf("函数 %s 缺少说明", name)
		}
	}
}

func TestCustomExpressionFallback(t *testing.T) {
	e := NewExpressionEngine()
	e.SetVariable("key", "temp_01")
	cases := []struct {
		expr string
		want interface{}
		err  bool
	}{
		{"regex('temp_.*', key)", true, false},
		{"regex('hum_.*', key)", false, false},
		{"regex('x', missing)", false, false}, // 字段不存在
		{"regex('a')", nil, true},             // 参数个数错误
		{"regex('(', key)", nil, true},        // 非法正则
		{"time_range(0, 23)", nil, false},     // 结果取决于当前时间，只要求不报错
		{"time_range(22, 6)", nil, false},     // 跨午夜
		{"time_range(1)", nil, true},
		{"time_range(a, 2)", nil, true},
		{"time_range(1, b)", nil, true},
		{"time_range(25, 2)", nil, true},
		{"xx time_range(1, 2)", nil, true}, // 前缀不对
		{"in_array(value, [1])", nil, true},
		{"unknown()", nil, true},
	}
	for _, c := range cases {
		got, err := e.evaluateCustomExpression(c.expr)
		if (err != nil) != c.err {
			t.Errorf("%s: err = %v", c.expr, err)
			continue
		}
		if c.want != nil && got != c.want {
			t.Errorf("%s = %v, want %v", c.expr, got, c.want)
		}
	}
	// 跨午夜区间与当日区间互补
	h := float64(time.Now().Hour())
	if h >= 1 && h <= 21 {
		in, _ := e.evaluateCustomExpression("time_range(0, 23)")
		if in != true {
			t.Errorf("当前小时 %v 应在 0-23 内", h)
		}
	}
}

func TestExpressionTagAndSelectorVariables(t *testing.T) {
	e := NewExpressionEngine()
	p := pt("d", "k", 1.0, model.TypeFloat)
	p.AddTag("meta", `{"site":{"floor":3},"list":[1,{"x":2}]}`)
	p.AddTag("arr", `[{"y":5},7]`)
	p.AddTag("a.b", "dotted")
	p.AddTag("bad", "{not json")
	for expr, want := range map[string]interface{}{
		`tag_meta_site_floor == 3`:  true,
		`tag_meta_list_length == 2`: true,
		`tag_meta_list_0 == 1`:      true,
		`tag_arr_length == 2`:       true,
		`tag_arr_0_y == 5`:          true,
		`tag_a_b == "dotted"`:       true,
	} {
		got, err := e.Evaluate(expr, p)
		if err != nil || got != want {
			t.Errorf("%s = %v %v", expr, got, err)
		}
	}
	// 选择器表达式：复合数据字段以数据点 key 为前缀，如 gps.latitude
	loc := model.NewCompositePoint("gps", "d", &model.LocationData{Latitude: 31, Longitude: 121})
	if got, err := e.Evaluate("gps.latitude > 30", loc); err != nil || got != true {
		t.Fatalf("gps.latitude: %v %v", got, err)
	}
	if _, err := e.Evaluate("gps.nope > 1", loc); err == nil {
		t.Fatal("不存在的选择器字段应报错")
	}
	if _, err := e.Evaluate("a.b.c > 1", loc); err == nil {
		t.Fatal("多级选择器应报错")
	}
	if v, ok := e.parseJSONValue("[1]"); !ok || v == nil {
		t.Fatal("parseJSONValue")
	}
	if _, ok := e.parseJSONValue("{"); ok {
		t.Fatal("非法 JSON 应返回 false")
	}
}

func TestEvaluatorSimpleExpressionFallback(t *testing.T) {
	e := NewEvaluator()
	p := pt("dev", "temp", 25.0, model.TypeFloat)
	cases := map[string]bool{
		"value > 20":                       true,
		"value >= 25 && key == temp":       true,
		"value < 10 || device_id == 'dev'": true,
		"value <= 10 || value != 25":       false,
		"value > 20 && value < 21":         false,
		"type == float":                    true,
		"1 < 2":                            true,
		"true == true":                     true,
		"abc == abc":                       true,
	}
	for expr, want := range cases {
		got, err := e.parseSimpleExpression(expr, p)
		if err != nil || got != want {
			t.Errorf("%s = %v %v", expr, got, err)
		}
	}
	for _, expr := range []string{"value", "value > 1 && nothing", "x.y > 1"} {
		if _, err := e.parseSimpleExpression(expr, p); err == nil {
			t.Errorf("%s 应报错", expr)
		}
	}
	if _, err := e.compareValues(1, 2, "~"); err == nil {
		t.Fatal("不支持的操作符应报错")
	}
	// 主表达式引擎报非语法错误时回退到简单解析
	ok, err := e.Evaluate(&Condition{Type: "expression", Expression: "unknownFunc(value) > 1 || value > 20"}, p)
	t.Logf("回退结果: %v %v", ok, err)
	// Lua 条件尚未实现，应返回错误而不是静默为 false
	if _, err := e.Evaluate(&Condition{Type: "lua", Script: "return true"}, p); err == nil {
		t.Fatal("lua 条件应返回未实现错误")
	}
}

type fakeChecker struct {
	name string
	err  error
}

func (f fakeChecker) Name() string                    { return f.name }
func (f fakeChecker) Check(ctx context.Context) error { return f.err }

func TestRuleMonitor(t *testing.T) {
	m := NewRuleMonitor(3)
	defer m.Close()
	for i := 0; i < 5; i++ {
		m.RecordError(ErrorTypeAction, ErrorLevelError, "失败", "details", map[string]string{"rule_id": "r"})
	}
	if errs := m.GetErrors(0); len(errs) != 3 {
		t.Fatalf("错误列表应限制为 3 条: %d", len(errs))
	}
	if errs := m.GetErrors(2); len(errs) != 2 {
		t.Fatalf("limit=2: %d", len(errs))
	}
	m.RecordRuleExecution("r", time.Millisecond, true, nil)
	m.RecordActionExecution("alert", time.Millisecond, false, errors.New("x"))
	m.RecordActionExecution("alert", time.Millisecond, true, nil)

	m.RegisterHealthChecker(fakeChecker{name: "ok"})
	m.performHealthChecks()
	if h := m.GetHealthStatus(); h.Status != "healthy" || h.CheckResults["ok"] != "PASS" {
		t.Fatalf("health = %+v", h)
	}
	m.RegisterHealthChecker(fakeChecker{name: "db", err: errors.New("down")})
	m.performHealthChecks()
	if h := m.GetHealthStatus(); h.Status != "unhealthy" || !strings.HasPrefix(h.CheckResults["db"], "FAIL") {
		t.Fatalf("health = %+v", h)
	}
	// 错误率、队列长度超过阈值时只记录日志
	m.performanceStats.QueueLength = m.alertThresholds.QueueLengthThreshold + 1
	m.alertThresholds.ErrorRateThreshold = 0.1
	m.checkAlerts()
	m.updatePerformanceStats()
	if m.performanceStats.LastUpdated.IsZero() {
		t.Fatal("性能统计时间未更新")
	}
	data, err := m.ToJSON()
	if err != nil || !json.Valid(data) || !strings.Contains(string(data), "recent_errors") {
		t.Fatalf("ToJSON: %v %s", err, data)
	}
}

// 以前 Close 关闭通知通道：run 循环读到 nil 后空指针 panic，之后的 RecordError 向已关闭通道发送也会 panic
func TestRuleMonitorCloseIsSafe(t *testing.T) {
	for i := 0; i < 50; i++ {
		m := NewRuleMonitor(10)
		m.RecordError(ErrorTypeSystem, ErrorLevelCritical, "boom", "", nil)
		m.Close()
		m.Close()
		m.RecordError(ErrorTypeSystem, ErrorLevelCritical, "after close", "", nil)
	}
	time.Sleep(50 * time.Millisecond) // 给 run 协程处理的时间，若读到 nil 会 panic
}

// ToJSON 以前在持有读锁时再次加读锁，遇到等待中的写锁会死锁
func TestRuleMonitorToJSONConcurrentWithWrites(t *testing.T) {
	m := NewRuleMonitor(100)
	defer m.Close()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				m.RecordError(ErrorTypeRule, ErrorLevelWarning, "m", "", nil)
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			_, _ = m.ToJSON()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ToJSON 与写操作并发时死锁")
	}
	close(stop)
	wg.Wait()
}

func TestServiceAccessors(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	rule := &Rule{ID: "r1", Name: "r1", Enabled: true,
		Conditions: &Condition{Field: "key", Value: "temp"},
		Actions:    []Action{{Type: "alert", Config: map[string]interface{}{"message": "m"}}}}
	s := startService(t, nc, []*Rule{rule}, nil)

	if s.Name() != "rule-engine" || s.GetRuleManager() == nil {
		t.Fatal("Name/GetRuleManager")
	}
	if _, ok := s.GetActionHandler("alert"); !ok {
		t.Fatal("alert 处理器应已注册")
	}
	if _, ok := s.GetActionHandler("nope"); ok {
		t.Fatal("不存在的处理器")
	}
	if s.GetMonitor() == nil {
		t.Fatal("GetMonitor")
	}
	if m := s.GetMetrics(); m.RulesTotal != 1 || m.RulesEnabled != 1 {
		t.Fatalf("metrics = %+v", m)
	}
	s.RegisterHealthChecker(fakeChecker{name: "x"})
	s.GetMonitor().performHealthChecks()
	if h := s.GetHealthStatus(); h.CheckResults["x"] != "PASS" {
		t.Fatalf("health = %+v", h)
	}
	s.GetMonitor().RecordError(ErrorTypeSystem, ErrorLevelError, "boom", "", nil)
	if errs := s.GetErrors(5); len(errs) == 0 || errs[len(errs)-1].Message != "boom" {
		t.Fatalf("errors = %+v", errs)
	}
	if data, err := s.GetMonitoringJSON(); err != nil || !json.Valid(data) {
		t.Fatalf("GetMonitoringJSON: %v", err)
	}
	s.SetMetricsEnabled(false)
	if s.enableMetrics {
		t.Fatal("SetMetricsEnabled")
	}
	if st := s.GetRuleIndexStats(); st["enabled"] != true {
		t.Fatalf("index stats = %v", st)
	}
	r2 := &Rule{ID: "r2", Enabled: true, Conditions: &Condition{Field: "key", Value: "hum"}, Actions: rule.Actions}
	s.updateRuleIndex(r2, "add")
	if len(s.ruleIndex.Match(pt("d", "hum", 1.0, model.TypeFloat))) != 1 {
		t.Fatal("updateRuleIndex add")
	}
	s.updateRuleIndex(r2, "remove")
	if len(s.ruleIndex.Match(pt("d", "hum", 1.0, model.TypeFloat))) != 0 {
		t.Fatal("updateRuleIndex remove")
	}
	if s.manager.(*Manager).GetIndex() == nil {
		t.Fatal("Manager.GetIndex")
	}

	// 未初始化监控器、未启用索引时的返回值
	empty := &RuleEngineService{}
	if empty.GetMetrics() == nil || empty.GetHealthStatus().Status != "unknown" || len(empty.GetErrors(1)) != 0 {
		t.Fatal("未初始化监控器")
	}
	if data, _ := empty.GetMonitoringJSON(); string(data) != "{}" {
		t.Fatal("未初始化监控器的 JSON")
	}
	empty.RegisterHealthChecker(fakeChecker{name: "x"})
	if st := empty.GetRuleIndexStats(); st["enabled"] != false {
		t.Fatal("未启用索引")
	}
	empty.updateRuleIndex(r2, "add")
}

// 未注册处理器时，transform/alert 不再走重复的简化实现，而是报错
func TestUnregisteredActionHandlers(t *testing.T) {
	s := newTestService()
	for _, typ := range []string{"transform", "alert", "filter", "aggregate", "weird"} {
		res, err := s.executeAction(&Action{Type: typ}, pt("d", "k", 1.0, model.TypeFloat), &Rule{ID: "r"})
		if err == nil && (res == nil || res.Success) {
			t.Errorf("%s: 未注册处理器应失败: %+v", typ, res)
		}
	}
}

func TestBuiltinAlertChannels(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	sub, _ := nc.SubscribeSync("custom.alerts")
	nc.Flush()
	h := &BuiltinAlertHandler{natsConn: nc, throttleMap: map[string]time.Time{}}
	alert := &Alert{ID: "a1", RuleID: "r"}
	if err := h.sendNATSAlert(alert, map[string]interface{}{"subject": "custom.alerts"}); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.NextMsg(time.Second); err != nil {
		t.Fatal("应收到 NATS 告警")
	}
	if err := (&BuiltinAlertHandler{}).sendNATSAlert(alert, nil); err == nil {
		t.Fatal("NATS 未连接应报错")
	}
	// 邮件、短信尚未实现：以前返回 nil，投递结果显示成功
	if err := h.sendEmailAlert(alert, map[string]interface{}{"password": "p"}); !errors.Is(err, ErrChannelNotImplemented) {
		t.Fatalf("email: %v", err)
	}
	if err := h.sendSMSAlert(alert, nil); !errors.Is(err, ErrChannelNotImplemented) {
		t.Fatalf("sms: %v", err)
	}
	if h.Name() != "BuiltinAlertHandler" {
		t.Fatal("Name")
	}
}

func TestAlertMessageFieldExtraction(t *testing.T) {
	h := &BuiltinAlertHandler{}
	type gps struct {
		Speed float64
		inner int
	}
	cases := []struct {
		value interface{}
		path  string
		want  interface{}
	}{
		{map[string]interface{}{"speed": 1.5}, "speed", 1.5},
		{map[string]interface{}{"Speed": 2.0}, "speed", 2.0},
		{`{"speed": 3}`, "speed", 3.0},
		{`{"SPEED": 4}`, "speed", 4.0},
		{&model.LocationData{Speed: 5}, "speed", 5.0},
		{gps{Speed: 6}, "Speed", 6.0},
		{struct{ X chan int }{}, "x", nil}, // 无法序列化，反射找到但字段值为 nil 通道
		{nil, "speed", nil},
		{42, "speed", nil},
	}
	for _, c := range cases {
		got := h.extractFieldFromValue(c.value, c.path)
		if c.want == nil {
			continue
		}
		if got != c.want {
			t.Errorf("%#v.%s = %#v, want %#v", c.value, c.path, got, c.want)
		}
	}
	if h.extractFieldUsingReflection((*gps)(nil), "Speed") != nil || h.extractFieldUsingReflection(1, "x") != nil {
		t.Fatal("nil 指针和非结构体应返回 nil")
	}
	if h.extractFieldUsingReflection(&gps{Speed: 7}, "speed") != 7.0 {
		t.Fatal("指针结构体字段（不区分大小写）")
	}
	if h.extractFieldUsingReflection(gps{inner: 1}, "inner") != nil {
		t.Fatal("未导出字段应返回 nil")
	}
	if h.extractFieldUsingReflection(gps{}, "nope") != nil {
		t.Fatal("不存在的字段应返回 nil")
	}
}

func TestMiscRuleHelpers(t *testing.T) {
	red := RedactConfig(map[string]interface{}{"password": "x", "api_key": "k", "host": "h",
		"nested": map[string]interface{}{"token": "t", "port": 1}})
	if red["password"] != "***" || red["api_key"] != "***" || red["host"] != "h" {
		t.Fatalf("redact = %v", red)
	}
	if n := red["nested"].(map[string]interface{}); n["token"] != "***" || n["port"] != 1 {
		t.Fatalf("nested redact = %v", n)
	}
	for _, c := range []struct {
		dt   interface{}
		want string
	}{{nil, ""}, {"location", "location"}, {map[string]interface{}{"type": "vector3d"}, "vector3d"}, {&DataTypeDefinition{Type: "color"}, "color"}, {42, ""}} {
		r := &Rule{DataType: c.dt}
		if got := r.GetDataTypeName(); got != c.want {
			t.Errorf("GetDataTypeName(%v) = %q", c.dt, got)
		}
	}
	if (panicError{v: 1}).Error() == "" {
		t.Fatal("panicError")
	}
}

// exists 以前对 temperature 等"常见字段名"一律返回 true
func TestExistsFunctionChecksActualFields(t *testing.T) {
	e := NewExpressionEngine()
	p := pt("d", "pressure", 1.0, model.TypeFloat)
	for expr, want := range map[string]bool{"exists('value')": true, "exists('pressure')": true, "exists('temperature')": false} {
		got, err := e.Evaluate(expr, p)
		if err != nil || got != want {
			t.Errorf("%s = %v %v, want %v", expr, got, err, want)
		}
	}
}
