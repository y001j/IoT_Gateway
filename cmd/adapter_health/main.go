package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// AdapterStatus 适配器状态
type AdapterStatus struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Mode    string `json:"mode"`
	Running bool   `json:"running"`
	Status  string `json:"status"`
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	LastCheck time.Time `json:"last_check"`
}

// AdapterMetrics 适配器指标
type AdapterMetrics struct {
	DataPointsCollected int64         `json:"data_points_collected"`
	ErrorsCount         int64         `json:"errors_count"`
	LastDataPointTime   time.Time     `json:"last_data_point_time"`
	ConnectionUptime    time.Duration `json:"connection_uptime"`
	LastError           string        `json:"last_error,omitempty"`
}

func main() {
	// 检查网关API端点是否可访问
	baseURL := "http://localhost:8081"
	
	fmt.Println("IoT Gateway 适配器健康检查工具")
	fmt.Println("================================")
	
	// 检查适配器状态
	checkAdapters(baseURL)
	
	// 持续监控
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			fmt.Printf("\n[%s] 定期健康检查\n", time.Now().Format("15:04:05"))
			fmt.Println("================================")
			checkAdapters(baseURL)
		}
	}
}

func checkAdapters(baseURL string) {
	// 获取所有适配器状态
	adapters := getAdapters(baseURL)
	if adapters == nil {
		fmt.Println("❌ 无法获取适配器状态")
		return
	}
	
	fmt.Printf("发现 %d 个适配器:\n", len(adapters))
	
	for _, adapter := range adapters {
		fmt.Printf("\n📋 适配器: %s (%s)\n", adapter.Name, adapter.Type)
		
		// 检查运行状态
		if adapter.Running {
			fmt.Printf("   状态: ✅ 运行中\n")
		} else {
			fmt.Printf("   状态: ❌ 停止\n")
			continue
		}
		
		// 获取健康状态
		health := getAdapterHealth(baseURL, adapter.Name)
		if health != nil {
			switch health.Status {
			case "healthy":
				fmt.Printf("   健康: ✅ %s\n", health.Message)
			case "degraded":
				fmt.Printf("   健康: ⚠️  %s\n", health.Message)
			case "unhealthy":
				fmt.Printf("   健康: ❌ %s\n", health.Message)
			default:
				fmt.Printf("   健康: ❓ %s\n", health.Message)
			}
		}
		
		// 获取指标
		metrics := getAdapterMetrics(baseURL, adapter.Name)
		if metrics != nil {
			fmt.Printf("   数据点: %d 个\n", metrics.DataPointsCollected)
			fmt.Printf("   错误数: %d 个\n", metrics.ErrorsCount)
			if !metrics.LastDataPointTime.IsZero() {
				fmt.Printf("   最后数据: %s\n", metrics.LastDataPointTime.Format("15:04:05"))
			}
			if metrics.ConnectionUptime > 0 {
				fmt.Printf("   运行时间: %v\n", metrics.ConnectionUptime.Truncate(time.Second))
			}
			if metrics.LastError != "" {
				fmt.Printf("   最后错误: ⚠️ %s\n", metrics.LastError)
			}
		}
	}
}

func getAdapters(baseURL string) []AdapterStatus {
	resp, err := http.Get(baseURL + "/api/adapters")
	if err != nil {
		log.Printf("获取适配器列表失败: %v", err)
		return nil
	}
	defer resp.Body.Close()
	
	var adapters []AdapterStatus
	if err := json.NewDecoder(resp.Body).Decode(&adapters); err != nil {
		log.Printf("解析适配器列表失败: %v", err)
		return nil
	}
	
	return adapters
}

func getAdapterHealth(baseURL, name string) *HealthStatus {
	resp, err := http.Get(fmt.Sprintf("%s/api/adapters/%s/health", baseURL, name))
	if err != nil {
		log.Printf("获取适配器 %s 健康状态失败: %v", name, err)
		return nil
	}
	defer resp.Body.Close()
	
	var health HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		log.Printf("解析适配器 %s 健康状态失败: %v", name, err)
		return nil
	}
	
	return &health
}

func getAdapterMetrics(baseURL, name string) *AdapterMetrics {
	resp, err := http.Get(fmt.Sprintf("%s/api/adapters/%s/metrics", baseURL, name))
	if err != nil {
		log.Printf("获取适配器 %s 指标失败: %v", name, err)
		return nil
	}
	defer resp.Body.Close()
	
	var metrics AdapterMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		log.Printf("解析适配器 %s 指标失败: %v", name, err)
		return nil
	}
	
	return &metrics
}