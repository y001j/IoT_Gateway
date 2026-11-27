package rules

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRuleFile(t *testing.T, path, id string, threshold float64) {
	t.Helper()
	rule := Rule{ID: id, Name: id, Enabled: true,
		Conditions: &Condition{Field: "value", Operator: "gt", Value: threshold},
		Actions:    []Action{{Type: "alert", Config: map[string]interface{}{"level": "info", "message": "x"}}}}
	data, _ := json.Marshal(rule)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func ruleThreshold(m *Manager, id string) (float64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rules[id]
	if !ok || r.Conditions == nil {
		return 0, false
	}
	v, _ := r.Conditions.Value.(float64)
	return v, true
}

func waitThreshold(t *testing.T, m *Manager, id string, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := ruleThreshold(m, id); ok && v == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	v, ok := ruleThreshold(m, id)
	t.Fatalf("规则 %s 的阈值应为 %v，实际 %v（存在=%v）", id, want, v, ok)
}

func startWatching(t *testing.T, dir string, cfg *HotReloadConfig) (*Manager, <-chan RuleChangeEvent) {
	t.Helper()
	m := NewManager(dir)
	m.SetHotReloadConfig(cfg)
	if err := m.LoadRules(); err != nil {
		t.Fatal(err)
	}
	ch, err := m.WatchChanges()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, ch
}

// 子目录中的规则也热加载（以前只监控顶层目录，rules/composite/ 等子目录的修改不生效）；
// 运行中新建的子目录同样加入监控
func TestHotReloadWatchesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "composite")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	writeRuleFile(t, filepath.Join(sub, "a.json"), "a", 1)
	m, _ := startWatching(t, dir, &HotReloadConfig{Enabled: true, DebounceDelay: "20ms", ResyncInterval: "0"})
	waitThreshold(t, m, "a", 1)

	writeRuleFile(t, filepath.Join(sub, "a.json"), "a", 2)
	waitThreshold(t, m, "a", 2)

	newSub := filepath.Join(dir, "later")
	if err := os.MkdirAll(newSub, 0o750); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // 等新目录加入监控
	writeRuleFile(t, filepath.Join(newSub, "b.json"), "b", 5)
	waitThreshold(t, m, "b", 5)
}

// 定期核对补上丢失的文件事件：修改和删除都能恢复，未变化的文件不重新加载
func TestResyncRecoversMissedChanges(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.json")
	changed := filepath.Join(dir, "changed.json")
	gone := filepath.Join(dir, "gone.json")
	writeRuleFile(t, keep, "keep", 1)
	writeRuleFile(t, changed, "changed", 1)
	writeRuleFile(t, gone, "gone", 1)
	m := NewManager(dir)
	if err := m.LoadRules(); err != nil {
		t.Fatal(err)
	}
	// 没有文件监控：模拟事件全部丢失
	writeRuleFile(t, changed, "changed", 2)
	_ = os.Chtimes(changed, time.Now().Add(time.Second), time.Now().Add(time.Second))
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	m.resyncFiles()
	if v, _ := ruleThreshold(m, "changed"); v != 2 {
		t.Fatalf("修改应被核对补上: %v", v)
	}
	if _, ok := ruleThreshold(m, "gone"); ok {
		t.Fatal("删除应被核对补上")
	}
	// 再次核对没有变化，不产生事件
	for len(m.changesChan) > 0 {
		<-m.changesChan
	}
	m.resyncFiles()
	if len(m.changesChan) != 0 {
		t.Fatal("没有变化时不应重新加载")
	}
	// 解析失败的文件也记录状态，不会每次核对都重复报错
	if err := os.WriteFile(keep, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.resyncFiles()
	m.mu.RLock()
	_, recorded := m.fileStamps[filepath.Clean(keep)]
	m.mu.RUnlock()
	if !recorded {
		t.Fatal("解析失败的文件也应记录状态")
	}
	if v, _ := ruleThreshold(m, "keep"); v != 1 {
		t.Fatal("解析失败时保留原规则")
	}
}

// 监控出错（例如事件队列溢出）时立即核对；定时核对补上监控之外的修改
func TestWatcherErrorAndTickerTriggerResync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r.json")
	writeRuleFile(t, path, "r", 1)
	m, _ := startWatching(t, dir, &HotReloadConfig{Enabled: true, DebounceDelay: "20ms", ResyncInterval: "0"})
	// 让监控看不到后续修改
	if err := m.watcher.Remove(dir); err != nil {
		t.Fatal(err)
	}
	writeRuleFile(t, path, "r", 7)
	_ = os.Chtimes(path, time.Now().Add(time.Second), time.Now().Add(time.Second))
	m.handleWatchError(errors.New("模拟事件队列溢出"))
	waitThreshold(t, m, "r", 7)

	dir2 := t.TempDir()
	path2 := filepath.Join(dir2, "r.json")
	writeRuleFile(t, path2, "r2", 1)
	m2, _ := startWatching(t, dir2, &HotReloadConfig{Enabled: true, DebounceDelay: "20ms", ResyncInterval: "50ms"})
	if err := m2.watcher.Remove(dir2); err != nil {
		t.Fatal(err)
	}
	writeRuleFile(t, path2, "r2", 9)
	_ = os.Chtimes(path2, time.Now().Add(time.Second), time.Now().Add(time.Second))
	waitThreshold(t, m2, "r2", 9)
}

// 连续多次写入同一文件只处理一次（真正的防抖），删除按文件当前状态处理
func TestHotReloadDebouncesBurst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "burst.json")
	m, ch := startWatching(t, dir, &HotReloadConfig{Enabled: true, DebounceDelay: "300ms", ResyncInterval: "0"})
	for i := 1; i <= 5; i++ {
		writeRuleFile(t, path, "burst", float64(i))
		time.Sleep(10 * time.Millisecond)
	}
	waitThreshold(t, m, "burst", 5)
	time.Sleep(400 * time.Millisecond)
	updates := 0
	for len(ch) > 0 {
		if ev := <-ch; ev.Type == RuleEventUpdate {
			updates++
		}
	}
	if updates == 0 || updates > 2 {
		t.Fatalf("连续写入应合并处理，实际处理了 %d 次", updates)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := ruleThreshold(m, "burst"); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("删除文件后规则应被移除")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
