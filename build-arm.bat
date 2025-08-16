@echo off
echo ========================================
echo    IoT Gateway ARM Linux Build Script
echo    (Pure Go - No CGO Dependencies)
echo ========================================

REM 创建输出目录
if not exist "bin\arm" mkdir "bin\arm"

echo.
echo ℹ️ 注意: 使用纯Go编译 (CGO_ENABLED=0)
echo    这样可以避免需要ARM交叉编译工具链
echo    所有依赖库都使用纯Go实现
echo    网关程序内置Web服务器，无需单独编译server
echo.

echo [1/3] Building for ARM64 (64-bit Raspberry Pi 4/5)...
set GOOS=linux
set GOARCH=arm64
set CGO_ENABLED=0
go build -ldflags="-w -s" -o bin/arm/gateway-arm64 cmd/gateway/main.go
if %ERRORLEVEL% EQU 0 (
    echo ✓ ARM64 gateway build successful
) else (
    echo ✗ ARM64 gateway build failed
    goto :error
)

echo ℹ️ This project only has gateway binary (web server is embedded)

echo.
echo [2/3] Building for ARMv7 (32-bit Raspberry Pi 3/4)...
set GOOS=linux
set GOARCH=arm
set GOARM=7
set CGO_ENABLED=0
go build -ldflags="-w -s" -o bin/arm/gateway-armv7 cmd/gateway/main.go
if %ERRORLEVEL% EQU 0 (
    echo ✓ ARMv7 gateway build successful
) else (
    echo ✗ ARMv7 gateway build failed
    goto :error
)

echo ℹ️ ARMv7 gateway includes embedded web server

echo.
echo [3/3] Building for ARMv6 (Raspberry Pi Zero/1)...
set GOOS=linux
set GOARCH=arm
set GOARM=6
set CGO_ENABLED=0
go build -ldflags="-w -s" -o bin/arm/gateway-armv6 cmd/gateway/main.go
if %ERRORLEVEL% EQU 0 (
    echo ✓ ARMv6 gateway build successful
) else (
    echo ✗ ARMv6 gateway build failed
    goto :error
)

echo ℹ️ ARMv6 gateway includes embedded web server

echo.
echo ℹ️ Frontend is embedded in gateway binary (no separate build required)

echo.
echo ========================================
echo ✅ Build Summary:
echo ========================================
dir bin\arm\
echo.
echo 📋 Generated ARM binaries:
for %%f in (bin\arm\*) do (
    echo   %%f
)

echo.
echo 🚀 Deploy to Raspberry Pi:
echo   1. Upload files:
echo      scp bin/arm/gateway-arm* pi@your-pi-ip:/home/pi/
echo      scp config_rule_engine_test.yaml pi@your-pi-ip:/home/pi/config.yaml
echo.
echo   2. Run on Raspberry Pi:
echo      ssh pi@your-pi-ip
echo      chmod +x gateway-arm*
echo.
echo      # For 64-bit Pi (Pi 4/5):
echo      ./gateway-arm64 -config config.yaml
echo.
echo      # For 32-bit Pi (Pi 3/4):
echo      ./gateway-armv7 -config config.yaml
echo.
echo      # For Pi Zero:
echo      ./gateway-armv6 -config config.yaml
echo.
echo   3. Access Web UI: http://your-pi-ip:8081
echo.
echo 💡 Tips:
echo   - All binaries are statically linked (no external dependencies)
echo   - SQLite database uses pure Go implementation
echo   - NATS server is embedded in the binary
echo.
goto :end

:error
echo.
echo ❌ Build failed! Please check:
echo   1. Go is installed and in PATH
echo   2. Project dependencies are downloaded (go mod download)
echo   3. No syntax errors in source code
echo.
exit /b 1

:end
echo Build completed successfully! 🎉
pause