package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 设置日志
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 获取ISP端口
	ispPort := "50052" // 默认ISP端口
	if port := os.Getenv("ISP_PORT"); port != "" {
		ispPort = port
	}

	address := ":" + ispPort
	log.Printf("启动ISP Modbus Sidecar服务器，监听地址: %s", address)

	// 创建ISP服务器
	server := NewISPServer(address)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动服务器
	if err := server.Start(ctx); err != nil {
		log.Fatalf("启动ISP服务器失败: %v", err)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("ISP Modbus Sidecar服务器启动成功，按Ctrl+C停止")

	// 等待信号
	<-sigChan
	log.Println("收到停止信号，正在关闭服务器...")

	// 停止服务器
	if err := server.Stop(); err != nil {
		log.Printf("停止ISP服务器时出错: %v", err)
	}

	log.Println("ISP Modbus Sidecar服务器已停止")
}
