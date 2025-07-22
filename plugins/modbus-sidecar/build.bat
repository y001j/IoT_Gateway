@echo off
echo 编译Modbus ISP Sidecar插件...

REM 设置构建参数
set GOOS=windows
set GOARCH=amd64

REM 编译为可执行文件
go build -o modbus-sidecar.exe main.go isp_server.go

if %ERRORLEVEL% NEQ 0 (
    echo 编译失败！
    exit /b 1
)

echo 编译成功！Modbus ISP Sidecar插件已生成: modbus-sidecar.exe
