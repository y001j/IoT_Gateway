module github.com/y001j/iot-gateway/plugins/modbus-sidecar

go 1.24.3

toolchain go1.24.5

require (
	github.com/goburrow/modbus v0.1.0
	github.com/rs/zerolog v1.32.0
	github.com/y001j/iot-gateway v0.0.0
)

require (
	github.com/goburrow/serial v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.31.0 // indirect
)

replace github.com/y001j/iot-gateway => ../../

replace github.com/y001j/iot-gateway/internal/plugin/proto => ../../internal/plugin/proto
