package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/y001j/iot-gateway/internal/core"

	// 导入所有内置连接器以触发注册
	_ "github.com/y001j/iot-gateway/internal/northbound/console"
	_ "github.com/y001j/iot-gateway/internal/northbound/influxdb"
	_ "github.com/y001j/iot-gateway/internal/northbound/jetstream"
	_ "github.com/y001j/iot-gateway/internal/northbound/mqtt"
	_ "github.com/y001j/iot-gateway/internal/northbound/redis"
	_ "github.com/y001j/iot-gateway/internal/northbound/websocket"

	// 导入所有内置适配器以触发注册
	_ "github.com/y001j/iot-gateway/internal/southbound/http"
	_ "github.com/y001j/iot-gateway/internal/southbound/mock"
	_ "github.com/y001j/iot-gateway/internal/southbound/modbus"
	_ "github.com/y001j/iot-gateway/internal/southbound/mqtt_sub"
)

func main() {
	cfgFile := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	rt, err := core.NewRuntime(*cfgFile)
	if err != nil {
		log.Fatal().Err(err).Msg("init runtime")
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := rt.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("start runtime")
	}

	// graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	rt.Stop(ctx)
}
