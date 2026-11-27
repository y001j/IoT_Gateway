package actions

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
	"github.com/y001j/iot-gateway/internal/testutil"
)

func aggExec(t *testing.T, h *AggregateHandler, ruleID string, v float64, cfg map[string]interface{}) map[string]interface{} {
	t.Helper()
	res, err := h.Execute(context.Background(), model.NewPoint("temp", "d1", v, model.TypeFloat), &rules.Rule{ID: ruleID}, cfg)
	if err != nil || !res.Success {
		t.Fatalf("聚合失败: %v %+v", err, res)
	}
	return res.Output.(map[string]interface{})
}

func countWindow(size int) map[string]interface{} {
	return map[string]interface{}{"window_type": "count", "window_size": float64(size), "functions": []interface{}{"avg", "count"}}
}

// 重启后从 JetStream KV 恢复聚合状态，窗口继续累积（以前重启后从零开始）
func TestAggregateStatePersistsAcrossRestart(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	p1, err := NewKVAggregateStatePersister(js)
	if err != nil {
		t.Fatal(err)
	}
	h1 := NewAggregateHandler()
	if err := h1.manager.EnablePersistence(p1, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{1, 2, 3} {
		aggExec(t, h1, "r1", v, countWindow(10))
	}
	h1.Close() // 停止时保存

	// 第二次打开：存储桶已存在
	p2, err := NewKVAggregateStatePersister(js)
	if err != nil {
		t.Fatal(err)
	}
	h2 := NewAggregateHandler()
	defer h2.Close()
	if err := h2.manager.EnablePersistence(p2, time.Hour); err != nil {
		t.Fatal(err)
	}
	out := aggExec(t, h2, "r1", 4, countWindow(10))
	if out["count"] != int64(4) {
		t.Fatalf("恢复后应继续累积: count=%v", out["count"])
	}

	// 状态键含有 KV 不允许的字符（设备 ID 中的空格、冒号）也能保存
	cfg := countWindow(10)
	cfg["group_by"] = []interface{}{"device_id"}
	res, err := h2.Execute(context.Background(), model.NewPoint("k", "dev 1:a", 1.0, model.TypeFloat), &rules.Rule{ID: "r:2"}, cfg)
	if err != nil || !res.Success {
		t.Fatal(err)
	}
	if err := h2.manager.saveChanged(); err != nil {
		t.Fatalf("特殊字符的状态键应能保存: %v", err)
	}
	snaps, err := p2.Load()
	if err != nil || len(snaps) != 2 {
		t.Fatalf("Load = %d %v", len(snaps), err)
	}
}

// 规则的窗口配置变化（热加载修改规则）时重建状态，不混合两种配置的数据
func TestAggregateStateResetsWhenWindowConfigChanges(t *testing.T) {
	h := NewAggregateHandler()
	defer h.Close()
	for _, v := range []float64{1, 2, 3} {
		aggExec(t, h, "r1", v, countWindow(5))
	}
	if out := aggExec(t, h, "r1", 4, countWindow(5)); out["count"] != int64(4) {
		t.Fatalf("配置不变时应继续累积: %v", out["count"])
	}
	if out := aggExec(t, h, "r1", 9, countWindow(7)); out["count"] != int64(1) {
		t.Fatalf("窗口配置变化后应重建: %v", out["count"])
	}
	if configFingerprint(5, nil) == configFingerprint(5, &AggregateConfig{WindowType: "count"}) {
		t.Fatal("有无配置的指纹应不同")
	}
}

// 闲置过期时间至少覆盖两个时间窗口：1 小时窗口的低频数据不会在 10 分钟后被清掉
func TestAggregateStateTTLCoversTimeWindow(t *testing.T) {
	h := NewAggregateHandler()
	defer h.Close()
	m := h.manager
	m.persister = &memPersister{}
	cfg := map[string]interface{}{"window_type": "time", "window": "1h", "functions": []interface{}{"avg"}}
	out := aggExec(t, h, "slow", 1, cfg)
	key := out["state_key"].(string)

	m.mu.RLock()
	st := m.states[key]
	m.mu.RUnlock()
	if st.ttl != 2*time.Hour {
		t.Fatalf("ttl = %v", st.ttl)
	}
	st.LastAccess = time.Now().Add(-15 * time.Minute)
	m.cleanupExpiredStates()
	if _, ok := m.states[key]; !ok {
		t.Fatal("窗口未结束的状态不应被清理")
	}
	st.LastAccess = time.Now().Add(-3 * time.Hour)
	m.cleanupExpiredStates()
	if _, ok := m.states[key]; ok {
		t.Fatal("超过两个窗口未更新的状态应被清理")
	}
	if len(m.removed) != 1 || m.removed[0] != key {
		t.Fatalf("清理的状态应等待从持久化存储删除: %v", m.removed)
	}
	if stateTTL(10*time.Minute, &AggregateConfig{TTL: time.Hour}) != time.Hour || stateTTL(10*time.Minute, nil) != 10*time.Minute {
		t.Fatal("stateTTL")
	}
	// 强制清理同样按状态自己的过期时间（的一半）
	aggExec(t, h, "slow", 1, cfg)
	m.mu.Lock()
	for _, s := range m.states {
		s.LastAccess = time.Now().Add(-30 * time.Minute)
	}
	m.forceCleanup()
	n := len(m.states)
	m.mu.Unlock()
	if n != 1 {
		t.Fatal("强制清理不应清掉未过半个过期时间的状态")
	}
}

type memPersister struct {
	snaps   []aggStateSnapshot
	saved   []aggStateSnapshot
	deleted []string
	failErr error
}

func (p *memPersister) Load() ([]aggStateSnapshot, error) { return p.snaps, p.failErr }
func (p *memPersister) Save(s []aggStateSnapshot, removed []string) error {
	if p.failErr != nil {
		return p.failErr
	}
	p.saved = append(p.saved, s...)
	p.deleted = append(p.deleted, removed...)
	return nil
}

// 过期的快照不恢复并被删除；保存失败时下次重试
func TestAggregatePersistenceExpiryAndRetry(t *testing.T) {
	old := aggStateSnapshot{Key: "old", TTL: time.Minute, LastAccess: time.Now().Add(-time.Hour), WindowSize: 3,
		Stats: NewIncrementalStats(3).snapshot()}
	fresh := aggStateSnapshot{Key: "fresh", LastAccess: time.Now(), WindowSize: 3, Stats: NewIncrementalStats(3).snapshot()}
	p := &memPersister{snaps: []aggStateSnapshot{old, fresh}}
	m := NewAggregateManager()
	defer m.Close()
	if err := m.EnablePersistence(p, 0); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.states["old"]; ok {
		t.Fatal("过期快照不应恢复")
	}
	if m.states["fresh"].ttl != m.defaultTTL {
		t.Fatal("没有 TTL 的快照使用默认值")
	}
	if err := m.saveChanged(); err != nil || len(p.deleted) != 1 || p.deleted[0] != "old" {
		t.Fatalf("过期快照应被删除: %v %v", p.deleted, err)
	}

	// 保存失败：保留待删除列表和变化，下次重试
	m.states["fresh"].LastAccess = time.Now()
	m.removed = []string{"x"}
	p.failErr = errors.New("kv down")
	if err := m.saveChanged(); err == nil {
		t.Fatal("应返回保存错误")
	}
	p.failErr = nil
	if err := m.saveChanged(); err != nil || len(p.saved) != 1 || p.deleted[len(p.deleted)-1] != "x" {
		t.Fatalf("重试应保存变化并删除: saved=%d deleted=%v %v", len(p.saved), p.deleted, err)
	}
	if err := m.saveChanged(); err != nil || len(p.saved) != 1 {
		t.Fatal("没有变化时不应重复保存")
	}

	if err := m.EnablePersistence(nil, 0); err == nil {
		t.Fatal("persister 为空应报错")
	}
	bad := NewAggregateManager()
	defer bad.Close()
	if err := bad.EnablePersistence(&memPersister{failErr: errors.New("boom")}, 0); err == nil {
		t.Fatal("加载失败应报错")
	}
	// 未开启持久化时 saveChanged 什么都不做
	if err := NewAggregateManager().saveChanged(); err != nil {
		t.Fatal(err)
	}
}

// 快照完整还原统计状态，包括没有数据时的正负无穷最值
func TestStatsSnapshotRoundTrip(t *testing.T) {
	empty := statsFromSnapshot(NewIncrementalStats(4).snapshot())
	if !math.IsInf(empty.min, 1) || !math.IsInf(empty.max, -1) {
		t.Fatal("空状态的最值应保持无穷")
	}
	upper := 10.0
	s := NewIncrementalStatsWithWindow(4, "count", 0, "none")
	s.upperLimit = &upper
	for _, v := range []float64{5, 1, 9, 3, 7} {
		s.AddValue(v)
	}
	r := statsFromSnapshot(s.snapshot())
	if r.GetCount() != s.GetCount() || r.min != s.min || r.max != s.max || r.valueIndex != s.valueIndex ||
		r.windowFull != s.windowFull || *r.upperLimit != 10 || r.GetMean() != s.GetMean() {
		t.Fatalf("恢复的状态不一致: %+v vs %+v", r.snapshot(), s.snapshot())
	}
	ts := NewIncrementalStatsWithWindow(3, "time", time.Minute, "none")
	ts.AddValue(1)
	rt := statsFromSnapshot(ts.snapshot())
	if len(rt.timeValues) != 3 || rt.timeValues[0].Value != ts.timeValues[0].Value {
		t.Fatal("时间窗口数据应恢复")
	}
	// 缓冲区长度与窗口大小不一致的快照只恢复汇总值
	sn := s.snapshot()
	sn.Values = []float64{1}
	if statsFromSnapshot(sn).valueIndex != 0 {
		t.Fatal("长度不一致时不应恢复缓冲区")
	}
	if finiteOrNil(math.NaN()) != nil {
		t.Fatal("NaN 应为 nil")
	}
}
