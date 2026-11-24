package metrics

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestGetMetricsSnapshotConcurrent(t *testing.T) {
	m := NewLightweightMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				m.UpdateDataMetrics(int64(j), int64(j*10), 1)
				m.UpdateRuleMetrics(1, 1, int64(j), int64(j), int64(j), 0)
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				snap := m.GetMetrics()
				_ = snap.DataMetrics
			}
		}()
	}
	wg.Wait()
}

func TestSnapshotIsIndependent(t *testing.T) {
	m := NewLightweightMetrics()
	m.UpdateGatewayMetrics("running", "config.yaml", "./plugins", 8081, 8080)
	snap := m.GetMetrics()
	if snap.GatewayMetrics.Status != "running" {
		t.Fatalf("快照内容错误: %+v", snap.GatewayMetrics)
	}
	// 修改快照不影响原对象
	snap.GatewayMetrics.Status = "changed"
	if m.GatewayMetrics.Status != "running" {
		t.Fatalf("快照与原对象共享了数据")
	}
	// 缓存期内返回同一个快照
	if m.GetMetrics() != snap {
		t.Fatalf("缓存有效期内应返回缓存的快照")
	}
}

func TestSerialization(t *testing.T) {
	m := NewLightweightMetrics()
	m.UpdateRuleMetrics(3, 2, 10, 9, 8, 1)
	data, err := m.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("JSON 非法: %v", err)
	}
	if _, ok := out["rules"]; !ok {
		t.Fatalf("JSON 缺少 rules 字段")
	}
	if txt := m.ToPlainText(); !strings.Contains(txt, "\n") {
		t.Fatalf("纯文本输出异常")
	}
}
