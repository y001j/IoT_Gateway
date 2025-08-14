package monitoring

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// SystemMetrics 系统指标数据
type SystemMetrics struct {
	Timestamp           time.Time `json:"timestamp"`
	CPUUsage            float64   `json:"cpu_usage"`             // CPU使用率 (%)
	CPUCores            int       `json:"cpu_cores"`             // CPU核心数
	MemoryUsage         float64   `json:"memory_usage"`          // 内存使用率 (%)
	MemoryUsageBytes    uint64    `json:"memory_usage_bytes"`    // 内存使用量 (bytes)
	MemoryTotalBytes    uint64    `json:"memory_total_bytes"`    // 总内存 (bytes)
	DiskUsage           float64   `json:"disk_usage"`            // 磁盘使用率 (%)
	DiskUsageBytes      uint64    `json:"disk_usage_bytes"`      // 磁盘使用量 (bytes)
	DiskTotalBytes      uint64    `json:"disk_total_bytes"`      // 总磁盘容量 (bytes)
	// 网络累计流量
	NetworkInBytes      uint64    `json:"network_in_bytes"`      // 网络接收字节数（累计）
	NetworkOutBytes     uint64    `json:"network_out_bytes"`     // 网络发送字节数（累计）
	NetworkInPackets    uint64    `json:"network_in_packets"`    // 网络接收包数（累计）
	NetworkOutPackets   uint64    `json:"network_out_packets"`   // 网络发送包数（累计）
	// 网络实时速率
	NetworkInBytesPerSec  float64 `json:"network_in_bytes_per_sec"`  // 网络接收速率 (bytes/s)
	NetworkOutBytesPerSec float64 `json:"network_out_bytes_per_sec"` // 网络发送速率 (bytes/s)
	NetworkInPacketsPerSec  float64 `json:"network_in_packets_per_sec"`  // 网络接收包速率 (packets/s)
	NetworkOutPacketsPerSec float64 `json:"network_out_packets_per_sec"` // 网络发送包速率 (packets/s)
	GoroutineCount      int       `json:"goroutine_count"`       // Goroutine数量
	HeapAllocBytes      uint64    `json:"heap_alloc_bytes"`      // 堆内存分配
	HeapSizeBytes       uint64    `json:"heap_size_bytes"`       // 堆内存大小
	GCCount             uint32    `json:"gc_count"`              // GC次数
	SystemLoadAvg1      float64   `json:"system_load_avg1"`      // 系统1分钟负载平均值
}

// SystemThresholds 系统阈值配置
type SystemThresholds struct {
	CPUWarning    float64 `json:"cpu_warning" yaml:"cpu_warning"`       // CPU警告阈值
	CPUCritical   float64 `json:"cpu_critical" yaml:"cpu_critical"`     // CPU严重阈值
	MemoryWarning float64 `json:"memory_warning" yaml:"memory_warning"` // 内存警告阈值
	MemoryCritical float64 `json:"memory_critical" yaml:"memory_critical"` // 内存严重阈值
	DiskWarning   float64 `json:"disk_warning" yaml:"disk_warning"`     // 磁盘警告阈值
	DiskCritical  float64 `json:"disk_critical" yaml:"disk_critical"`   // 磁盘严重阈值
}

// SystemCollectorConfig 系统收集器配置
type SystemCollectorConfig struct {
	Enabled          bool              `json:"enabled" yaml:"enabled"`                     // 是否启用
	CollectInterval  time.Duration     `json:"collect_interval" yaml:"collect_interval"`   // 收集间隔
	CacheDuration    time.Duration     `json:"cache_duration" yaml:"cache_duration"`       // 缓存持续时间
	DiskPath         string            `json:"disk_path" yaml:"disk_path"`                 // 监控的磁盘路径
	NetworkInterface string            `json:"network_interface" yaml:"network_interface"` // 网络接口名称
	Thresholds       SystemThresholds  `json:"thresholds" yaml:"thresholds"`               // 系统阈值
}

// SystemCollector 系统指标收集器
type SystemCollector struct {
	config         SystemCollectorConfig
	lastMetrics    atomic.Value // *SystemMetrics
	lastUpdate     atomic.Value // time.Time
	collecting     atomic.Bool
	mutex          sync.RWMutex
	stopCh         chan struct{}
	
	// 网络统计缓存 (用于计算差值)
	lastNetStats   *net.IOCountersStat
	lastNetTime    time.Time
}

// NewSystemCollector 创建系统指标收集器
func NewSystemCollector(config SystemCollectorConfig) *SystemCollector {
	// 设置默认配置
	if config.CollectInterval == 0 {
		config.CollectInterval = 5 * time.Second
	}
	if config.CacheDuration == 0 {
		config.CacheDuration = 30 * time.Second
	}
	if config.DiskPath == "" {
		config.DiskPath = "/"
	}
	
	// 设置默认阈值
	if config.Thresholds.CPUWarning == 0 {
		config.Thresholds.CPUWarning = 80.0
	}
	if config.Thresholds.CPUCritical == 0 {
		config.Thresholds.CPUCritical = 95.0
	}
	if config.Thresholds.MemoryWarning == 0 {
		config.Thresholds.MemoryWarning = 85.0
	}
	if config.Thresholds.MemoryCritical == 0 {
		config.Thresholds.MemoryCritical = 95.0
	}
	if config.Thresholds.DiskWarning == 0 {
		config.Thresholds.DiskWarning = 90.0
	}
	if config.Thresholds.DiskCritical == 0 {
		config.Thresholds.DiskCritical = 95.0
	}

	collector := &SystemCollector{
		config: config,
		stopCh: make(chan struct{}),
	}

	// 初始化缓存
	collector.lastUpdate.Store(time.Time{})
	
	return collector
}

// Start 启动收集器
func (c *SystemCollector) Start() error {
	if !c.config.Enabled {
		return nil
	}

	if c.collecting.Load() {
		return fmt.Errorf("system collector is already running")
	}

	c.collecting.Store(true)
	
	// 立即收集一次
	if err := c.collectOnce(); err != nil {
		return fmt.Errorf("initial collection failed: %w", err)
	}

	// 启动定时收集
	go c.collectLoop()
	
	return nil
}

// Stop 停止收集器
func (c *SystemCollector) Stop() error {
	if !c.collecting.Load() {
		return nil
	}

	c.collecting.Store(false)
	close(c.stopCh)
	
	return nil
}

// GetMetrics 获取系统指标（带缓存）
func (c *SystemCollector) GetMetrics() (*SystemMetrics, error) {
	if !c.config.Enabled {
		return c.getDefaultMetrics(), nil
	}

	// 检查缓存是否有效
	lastUpdateTime, ok := c.lastUpdate.Load().(time.Time)
	if ok && time.Since(lastUpdateTime) < c.config.CacheDuration {
		if metrics, ok := c.lastMetrics.Load().(*SystemMetrics); ok && metrics != nil {
			return metrics, nil
		}
	}

	// 缓存过期或无效，重新收集
	return c.collectAndCache()
}

// GetMetricsForced 强制获取最新系统指标（绕过缓存）
func (c *SystemCollector) GetMetricsForced() (*SystemMetrics, error) {
	if !c.config.Enabled {
		return c.getDefaultMetrics(), nil
	}

	return c.collectOnceWithoutCache()
}

// IsHealthy 检查系统健康状态
func (c *SystemCollector) IsHealthy() (bool, []string) {
	metrics, err := c.GetMetrics()
	if err != nil {
		return false, []string{fmt.Sprintf("failed to get metrics: %v", err)}
	}

	var issues []string
	healthy := true

	// 检查CPU使用率
	if metrics.CPUUsage >= c.config.Thresholds.CPUCritical {
		issues = append(issues, fmt.Sprintf("CPU usage critical: %.1f%%", metrics.CPUUsage))
		healthy = false
	} else if metrics.CPUUsage >= c.config.Thresholds.CPUWarning {
		issues = append(issues, fmt.Sprintf("CPU usage warning: %.1f%%", metrics.CPUUsage))
	}

	// 检查内存使用率
	if metrics.MemoryUsage >= c.config.Thresholds.MemoryCritical {
		issues = append(issues, fmt.Sprintf("Memory usage critical: %.1f%%", metrics.MemoryUsage))
		healthy = false
	} else if metrics.MemoryUsage >= c.config.Thresholds.MemoryWarning {
		issues = append(issues, fmt.Sprintf("Memory usage warning: %.1f%%", metrics.MemoryUsage))
	}

	// 检查磁盘使用率
	if metrics.DiskUsage >= c.config.Thresholds.DiskCritical {
		issues = append(issues, fmt.Sprintf("Disk usage critical: %.1f%%", metrics.DiskUsage))
		healthy = false
	} else if metrics.DiskUsage >= c.config.Thresholds.DiskWarning {
		issues = append(issues, fmt.Sprintf("Disk usage warning: %.1f%%", metrics.DiskUsage))
	}

	return healthy, issues
}

// collectLoop 收集循环
func (c *SystemCollector) collectLoop() {
	ticker := time.NewTicker(c.config.CollectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.collectOnce(); err != nil {
				// 记录错误但继续运行
				continue
			}
		case <-c.stopCh:
			return
		}
	}
}

// collectAndCache 收集并缓存指标
func (c *SystemCollector) collectAndCache() (*SystemMetrics, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 双重检查锁定
	lastUpdateTime, ok := c.lastUpdate.Load().(time.Time)
	if ok && time.Since(lastUpdateTime) < c.config.CacheDuration {
		if metrics, ok := c.lastMetrics.Load().(*SystemMetrics); ok && metrics != nil {
			return metrics, nil
		}
	}

	return c.collectOnceWithoutCache()
}

// collectOnce 执行一次收集并更新缓存
func (c *SystemCollector) collectOnce() error {
	metrics, err := c.collectOnceWithoutCache()
	if err != nil {
		return err
	}

	c.lastMetrics.Store(metrics)
	c.lastUpdate.Store(time.Now())
	
	return nil
}

// collectOnceWithoutCache 执行一次收集（不使用缓存）
func (c *SystemCollector) collectOnceWithoutCache() (*SystemMetrics, error) {
	now := time.Now()
	metrics := &SystemMetrics{
		Timestamp: now,
	}

	// 并行收集各种指标以提高性能
	var wg sync.WaitGroup
	var cpuErr, memErr, diskErr, netErr error

	// CPU指标
	wg.Add(1)
	go func() {
		defer wg.Done()
		cpuErr = c.collectCPUMetrics(metrics)
	}()

	// 内存指标
	wg.Add(1)
	go func() {
		defer wg.Done()
		memErr = c.collectMemoryMetrics(metrics)
	}()

	// 磁盘指标
	wg.Add(1)
	go func() {
		defer wg.Done()
		diskErr = c.collectDiskMetrics(metrics)
	}()

	// 网络指标
	wg.Add(1)
	go func() {
		defer wg.Done()
		netErr = c.collectNetworkMetrics(metrics)
	}()

	// Go运行时指标 (无需goroutine，很快)
	c.collectRuntimeMetrics(metrics)

	wg.Wait()

	// 检查错误
	if cpuErr != nil {
		return nil, fmt.Errorf("failed to collect CPU metrics: %w", cpuErr)
	}
	if memErr != nil {
		return nil, fmt.Errorf("failed to collect memory metrics: %w", memErr)
	}
	if diskErr != nil {
		return nil, fmt.Errorf("failed to collect disk metrics: %w", diskErr)
	}
	if netErr != nil {
		return nil, fmt.Errorf("failed to collect network metrics: %w", netErr)
	}

	return metrics, nil
}

// collectCPUMetrics 收集CPU指标
func (c *SystemCollector) collectCPUMetrics(metrics *SystemMetrics) error {
	// CPU使用率 (1秒平均)
	cpuPercents, err := cpu.Percent(time.Second, false)
	if err != nil {
		return err
	}
	if len(cpuPercents) > 0 {
		metrics.CPUUsage = cpuPercents[0]
	}

	// CPU核心数
	cpuCounts, err := cpu.Counts(true)
	if err == nil {
		metrics.CPUCores = cpuCounts
	}

	return nil
}

// collectMemoryMetrics 收集内存指标
func (c *SystemCollector) collectMemoryMetrics(metrics *SystemMetrics) error {
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return err
	}

	metrics.MemoryUsage = memStats.UsedPercent
	metrics.MemoryUsageBytes = memStats.Used
	metrics.MemoryTotalBytes = memStats.Total

	return nil
}

// collectDiskMetrics 收集磁盘指标
func (c *SystemCollector) collectDiskMetrics(metrics *SystemMetrics) error {
	diskStats, err := disk.Usage(c.config.DiskPath)
	if err != nil {
		return err
	}

	metrics.DiskUsage = diskStats.UsedPercent
	metrics.DiskUsageBytes = diskStats.Used
	metrics.DiskTotalBytes = diskStats.Total

	return nil
}

// collectNetworkMetrics 收集网络指标
func (c *SystemCollector) collectNetworkMetrics(metrics *SystemMetrics) error {
	netStats, err := net.IOCounters(false)
	if err != nil {
		return err
	}

	if len(netStats) > 0 {
		totalStats := netStats[0]
		
		// 如果指定了网络接口，尝试获取该接口的统计
		if c.config.NetworkInterface != "" {
			interfaceStats, err := net.IOCounters(true)
			if err == nil {
				for _, stat := range interfaceStats {
					if stat.Name == c.config.NetworkInterface {
						totalStats = stat
						break
					}
				}
			}
		}

		currentTime := time.Now()
		
		// 设置累计值
		metrics.NetworkInBytes = totalStats.BytesRecv
		metrics.NetworkOutBytes = totalStats.BytesSent
		metrics.NetworkInPackets = totalStats.PacketsRecv
		metrics.NetworkOutPackets = totalStats.PacketsSent
		
		// 计算实时速率
		if c.lastNetStats != nil && !c.lastNetTime.IsZero() {
			timeDiff := currentTime.Sub(c.lastNetTime).Seconds()
			
			if timeDiff > 0 {
				// 计算字节速率
				inBytesDiff := float64(totalStats.BytesRecv) - float64(c.lastNetStats.BytesRecv)
				outBytesDiff := float64(totalStats.BytesSent) - float64(c.lastNetStats.BytesSent)
				inPacketsDiff := float64(totalStats.PacketsRecv) - float64(c.lastNetStats.PacketsRecv)
				outPacketsDiff := float64(totalStats.PacketsSent) - float64(c.lastNetStats.PacketsSent)
				
				metrics.NetworkInBytesPerSec = inBytesDiff / timeDiff
				metrics.NetworkOutBytesPerSec = outBytesDiff / timeDiff
				metrics.NetworkInPacketsPerSec = inPacketsDiff / timeDiff
				metrics.NetworkOutPacketsPerSec = outPacketsDiff / timeDiff
			}
		} else {
			// 首次收集，速率为0
			metrics.NetworkInBytesPerSec = 0
			metrics.NetworkOutBytesPerSec = 0
			metrics.NetworkInPacketsPerSec = 0
			metrics.NetworkOutPacketsPerSec = 0
		}
		
		// 更新缓存的网络统计和时间
		c.lastNetStats = &totalStats
		c.lastNetTime = currentTime
	}

	return nil
}

// collectRuntimeMetrics 收集Go运行时指标
func (c *SystemCollector) collectRuntimeMetrics(metrics *SystemMetrics) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	metrics.GoroutineCount = runtime.NumGoroutine()
	metrics.HeapAllocBytes = memStats.Alloc
	metrics.HeapSizeBytes = memStats.Sys
	metrics.GCCount = memStats.NumGC
}

// getDefaultMetrics 获取默认指标（当收集器禁用时）
func (c *SystemCollector) getDefaultMetrics() *SystemMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &SystemMetrics{
		Timestamp:        time.Now(),
		CPUUsage:         0.0,
		CPUCores:         runtime.NumCPU(),
		MemoryUsage:      0.0,
		MemoryUsageBytes: 0,
		MemoryTotalBytes: 0,
		DiskUsage:        0.0,
		DiskUsageBytes:   0,
		DiskTotalBytes:   0,
		NetworkInBytes:   0,
		NetworkOutBytes:  0,
		GoroutineCount:   runtime.NumGoroutine(),
		HeapAllocBytes:   memStats.Alloc,
		HeapSizeBytes:    memStats.Sys,
		GCCount:          memStats.NumGC,
	}
}

// GetConfig 获取收集器配置
func (c *SystemCollector) GetConfig() SystemCollectorConfig {
	return c.config
}

// UpdateConfig 更新收集器配置
func (c *SystemCollector) UpdateConfig(config SystemCollectorConfig) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.config = config
	
	// 如果收集器正在运行且配置更改，需要重启
	if c.collecting.Load() {
		if err := c.Stop(); err != nil {
			return err
		}
		return c.Start()
	}

	return nil
}