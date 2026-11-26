package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/southbound"
	_ "github.com/y001j/iot-gateway/internal/southbound/composite_mock"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
	"github.com/y001j/iot-gateway/internal/testutil"
)

// collectRepoConfigPoints 用仓库 config.yaml 中的模拟适配器生成数据（缩短采集间隔）
func collectRepoConfigPoints(t *testing.T) []model.Point {
	t.Helper()
	v := viper.New()
	v.SetConfigFile("../../../config.yaml")
	if err := v.ReadInConfig(); err != nil {
		t.Skipf("无法读取 config.yaml: %v", err)
	}
	list, _ := v.Get("southbound.adapters").([]interface{})
	ch := make(chan model.Point, 10000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var adapters []southbound.Adapter
	for _, item := range list {
		cfg, _ := item.(map[string]interface{})
		typ, _ := cfg["type"].(string)
		if typ != "mock" && typ != "composite_mock" {
			continue
		}
		inner, _ := cfg["config"].(map[string]interface{})
		inner["interval"] = "20ms"
		data, _ := json.Marshal(inner)
		a, ok := southbound.Create(typ)
		if !ok {
			t.Fatalf("未注册的适配器类型 %s", typ)
		}
		if err := a.Init(data); err != nil {
			t.Fatalf("初始化 %v 失败: %v", cfg["name"], err)
		}
		if err := a.Start(ctx, ch); err != nil {
			t.Fatal(err)
		}
		adapters = append(adapters, a)
	}
	time.Sleep(500 * time.Millisecond)
	for _, a := range adapters {
		_ = a.Stop()
	}
	var points []model.Point
	for {
		select {
		case p := <-ch:
			points = append(points, p)
		default:
			return points
		}
	}
}

// TestRepoRulesRunWithoutActionErrors 仓库中的示例规则（包括禁用的）在仓库配置产生的数据上
// 执行时不应出现动作错误。以前很多示例规则缺少参数或使用了不存在的转换类型，
// 网关运行时每秒产生大量"动作执行失败"错误日志。
func TestRepoRulesRunWithoutActionErrors(t *testing.T) {
	if _, err := os.Stat("../../../rules"); err != nil {
		t.Skip("rules 目录不存在")
	}
	mgr := rules.NewManager("../../../rules")
	if err := mgr.LoadRules(); err != nil {
		t.Fatal(err)
	}
	points := collectRepoConfigPoints(t)
	if len(points) == 0 {
		t.Fatal("没有生成数据")
	}
	// 规则引擎实际收到的是经过 NATS（JSON）传输的数据点，复合值反序列化后是 map
	for _, p := range points[:len(points):len(points)] {
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var decoded model.Point
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		points = append(points, decoded)
	}

	_, nc := testutil.StartNATS(t)
	handlers := map[string]rules.ActionHandler{
		"filter":    NewFilterHandler(),
		"transform": NewTransformHandler(nc),
		"forward":   NewForwardHandler(nc),
		"aggregate": newAggregateActionHandler(),
	}
	evaluator := rules.NewEvaluator()
	var mu sync.Mutex
	failures := map[string]int{}
	matchedRules := 0

	for _, rule := range mgr.ListRules() {
		matched := false
		for _, p := range points {
			ok, err := evaluator.Evaluate(rule.Conditions, p)
			if err != nil {
				failures[fmt.Sprintf("%s 条件: %v", rule.ID, err)]++
				continue
			}
			if !ok {
				continue
			}
			matched = true
			current := p
			for i, action := range rule.Actions {
				h, ok := handlers[action.Type]
				if !ok {
					continue // alert 由规则引擎内置处理器提供
				}
				res, err := h.Execute(context.Background(), current, rule, action.Config)
				if err == nil && res != nil && !res.Success {
					err = fmt.Errorf("%s", res.Error)
				}
				if err != nil {
					mu.Lock()
					failures[fmt.Sprintf("%s 动作[%d] %s (%s/%s): %v", rule.ID, i, action.Type, p.DeviceID, p.Key, err)]++
					mu.Unlock()
					continue
				}
				out, _ := res.Output.(map[string]interface{})
				if f, _ := out["filtered"].(bool); f {
					break
				}
				switch tp := out["transformed_point"].(type) {
				case model.Point:
					current = tp
				case *model.Point:
					current = *tp
				}
			}
		}
		if matched {
			matchedRules++
		}
	}
	keys := make([]string, 0, len(failures))
	for k := range failures {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		t.Errorf("示例规则执行出错（%d 种）:\n%s", len(keys), strings.Join(keys, "\n"))
	}
	t.Logf("%d 个数据点，%d 条规则，其中 %d 条有匹配的数据", len(points), len(mgr.ListRules()), matchedRules)
}
