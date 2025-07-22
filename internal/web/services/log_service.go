package services

import "time"

// LogService 日志服务接口
type LogService interface {
	GetLogs() ([]interface{}, error)
	SearchLogs(query string) ([]interface{}, error)
	GetLogStats() (interface{}, error)
}

// logService 日志服务实现
type logService struct {
}

// NewLogService 创建日志服务
func NewLogService() LogService {
	return &logService{}
}

// GetLogs 获取日志列表
func (l *logService) GetLogs() ([]interface{}, error) {
	logs := []interface{}{
		map[string]interface{}{
			"id":        1,
			"level":     "info",
			"message":   "系统启动成功",
			"timestamp": time.Now().Add(-time.Hour).Unix(),
			"source":    "system",
		},
		map[string]interface{}{
			"id":        2,
			"level":     "debug",
			"message":   "处理数据点: 温度=25.5°C",
			"timestamp": time.Now().Add(-time.Minute * 30).Unix(),
			"source":    "plugin",
		},
		map[string]interface{}{
			"id":        3,
			"level":     "error",
			"message":   "连接设备失败",
			"timestamp": time.Now().Add(-time.Minute * 15).Unix(),
			"source":    "adapter",
		},
	}
	return logs, nil
}

// SearchLogs 搜索日志
func (l *logService) SearchLogs(query string) ([]interface{}, error) {
	return []interface{}{
		map[string]interface{}{
			"id":        1,
			"level":     "info",
			"message":   "搜索结果: " + query,
			"timestamp": time.Now().Unix(),
			"source":    "system",
		},
	}, nil
}

// GetLogStats 获取日志统计
func (l *logService) GetLogStats() (interface{}, error) {
	return map[string]interface{}{
		"total_logs": 1250,
		"level_stats": map[string]int{
			"info":  800,
			"debug": 300,
			"warn":  100,
			"error": 50,
		},
		"source_stats": map[string]int{
			"system":  400,
			"plugin":  500,
			"adapter": 350,
		},
	}, nil
}
