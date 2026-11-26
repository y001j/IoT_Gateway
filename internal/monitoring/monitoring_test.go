package monitoring

import (
	"runtime"
	"testing"
	"time"
)

func TestSystemCollectorDefaultsAndMetrics(t *testing.T) {
	c := NewSystemCollector(SystemCollectorConfig{Enabled: true})
	cfg := c.GetConfig()
	if cfg.CollectInterval != 5*time.Second || cfg.Thresholds.CPUCritical != 95 || cfg.DiskPath != "/" {
		t.Fatalf("默认配置未生效: %+v", cfg)
	}
	m, err := c.GetMetricsForced()
	if err != nil {
		t.Fatalf("采集失败: %v", err)
	}
	if m.CPUCores != runtime.NumCPU() {
		t.Fatalf("CPU 核数 = %d, want %d", m.CPUCores, runtime.NumCPU())
	}
	if m.MemoryTotalBytes == 0 || m.MemoryUsage < 0 || m.MemoryUsage > 100 {
		t.Fatalf("内存指标异常: %+v", m)
	}
	if m.GoroutineCount <= 0 || m.HeapAllocBytes == 0 {
		t.Fatalf("协程数应大于 0")
	}
	cached, err := c.GetMetrics()
	if err != nil || cached == nil {
		t.Fatalf("获取缓存指标失败: %v", err)
	}
}

func TestSystemCollectorHealthThresholds(t *testing.T) {
	// 阈值设置得极低：内存使用率必然超过 critical
	c := NewSystemCollector(SystemCollectorConfig{Enabled: true, Thresholds: SystemThresholds{
		CPUWarning: 1000, CPUCritical: 1000, MemoryWarning: 0.0001, MemoryCritical: 0.0002, DiskWarning: 1000, DiskCritical: 1000,
	}})
	healthy, issues := c.IsHealthy()
	if healthy || len(issues) == 0 {
		t.Fatalf("超过 critical 阈值应不健康: %v %v", healthy, issues)
	}
	ok := NewSystemCollector(SystemCollectorConfig{Enabled: true, Thresholds: SystemThresholds{
		CPUWarning: 1000, CPUCritical: 1000, MemoryWarning: 1000, MemoryCritical: 1000, DiskWarning: 1000, DiskCritical: 1000,
	}})
	if healthy, issues := ok.IsHealthy(); !healthy || len(issues) != 0 {
		t.Fatalf("未超过阈值应健康: %v %v", healthy, issues)
	}
}

func TestSystemCollectorStartStopAndUpdateConfig(t *testing.T) {
	c := NewSystemCollector(SystemCollectorConfig{Enabled: true, CollectInterval: 20 * time.Millisecond})
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	if err := c.UpdateConfig(SystemCollectorConfig{Enabled: true, CollectInterval: time.Second, Thresholds: SystemThresholds{CPUWarning: 50}}); err != nil {
		t.Fatal(err)
	}
	if c.GetConfig().Thresholds.CPUWarning != 50 {
		t.Fatalf("配置更新失败")
	}
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestMonitoringService(t *testing.T) {
	s := NewService(DefaultServiceConfig())
	if s.IsStarted() {
		t.Fatalf("新建服务不应处于启动状态")
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if !s.IsStarted() {
		t.Fatalf("启动后状态错误")
	}
	if _, err := s.GetSystemMetricsForced(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSystemMetrics(); err != nil {
		t.Fatal(err)
	}
	s.IsSystemHealthy()
	if s.GetSystemCollector() == nil {
		t.Fatalf("应返回系统采集器")
	}
	st := s.GetStatus()
	if !st.Started {
		t.Fatalf("状态应为已启动: %+v", st)
	}
	cfg := DefaultServiceConfig()
	cfg.SystemCollector.Thresholds.CPUWarning = 70
	if err := s.UpdateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if s.GetConfig().SystemCollector.Thresholds.CPUWarning != 70 {
		t.Fatalf("配置更新失败")
	}
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if s.IsStarted() {
		t.Fatalf("停止后状态错误")
	}
}

func TestDisabledCollectorReturnsDefaults(t *testing.T) {
	c := NewSystemCollector(SystemCollectorConfig{Enabled: false})
	for _, get := range []func() (*SystemMetrics, error){c.GetMetrics, c.GetMetricsForced} {
		m, err := get()
		if err != nil || m == nil || m.CPUCores == 0 || m.CPUUsage != 0 {
			t.Fatalf("禁用时应返回默认指标: %+v %v", m, err)
		}
	}
}
