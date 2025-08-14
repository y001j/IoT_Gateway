# Windows 下过滤器测试指南

本文档专门针对Windows环境下IoT Gateway过滤器功能的测试。

## 🚀 快速开始 (Windows)

### 方法一：使用批处理文件 (推荐)
```batch
# 运行交互式测试菜单
test_filters.bat
```

### 方法二：直接运行PowerShell
```powershell
# 快速验证 (2分钟)
.\quick_filter_test.ps1

# 完整测试 (10-15分钟) 
.\run_filter_tests.ps1

# 跳过编译和性能测试
.\run_filter_tests.ps1 -SkipBuild -SkipPerformance
```

### 方法三：手动执行Go命令
```batch
# 生成测试数据
go run cmd\test\filter_data_generator.go

# 编译并运行测试
go build -o bin\filter_test.exe cmd\test\filter_tests.go
.\bin\filter_test.exe
```

## 🔧 Windows 特殊配置

### 代理服务器配置 (重要!)
如果系统配置了代理服务器，需要先绕过代理以确保本地测试接口正常访问：

#### 方法一：使用批处理菜单
```batch
# 运行测试菜单并选择选项5
test_filters.bat
# 然后选择：5 - 配置代理绕过
```

#### 方法二：直接运行代理配置脚本
```powershell
# 配置代理绕过
.\scripts\setup_proxy_bypass.ps1

# 检查代理设置
go run cmd\test\proxy_test.go
```

#### 方法三：手动设置环境变量
```batch
# 在CMD中设置
set NO_PROXY=localhost,127.0.0.1,::1,*.local
set no_proxy=localhost,127.0.0.1,::1,*.local

# 或在PowerShell中设置
$env:NO_PROXY = "localhost,127.0.0.1,::1,*.local"
$env:no_proxy = "localhost,127.0.0.1,::1,*.local"
```

### PowerShell 执行策略
如果遇到执行策略错误：
```powershell
# 临时允许执行
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process

# 或者直接用 -ExecutionPolicy 参数
powershell -ExecutionPolicy Bypass -File "quick_filter_test.ps1"
```

### 路径配置
Windows下的配置文件已适配反斜杠路径：
```yaml
rule_engine:
  rules_dir: "configs\\test"  # Windows路径格式
```

### 文件权限
确保以下目录有写权限：
- `logs\filter_tests\` - 测试日志目录
- `test_data\` - 测试数据目录
- `bin\` - 编译输出目录

## 📁 Windows 文件结构

```
IoT Gateway\
├── test_filters.bat                    # 🎯 主要入口 - 交互式测试菜单
├── quick_filter_test.ps1              # ⚡ 快速验证脚本
├── run_filter_tests.ps1               # 🧪 完整测试脚本
├── cmd\test\
│   ├── filter_tests.go                # 主测试程序
│   └── filter_data_generator.go       # 数据生成器
├── configs\test\
│   ├── filter_test_rules.json         # 测试规则
│   └── filter_test_config.yaml        # 测试配置 (Windows路径)
├── bin\                               # 编译输出
│   ├── filter_test.exe                # 测试程序
│   └── filter_data_generator.exe      # 数据生成器
├── logs\filter_tests\                 # 测试日志
│   ├── filter_test_YYYYMMDD_HHMMSS.log
│   └── test_report_YYYYMMDD_HHMMSS.md
└── test_data\
    └── filter_test_scenarios.json     # 生成的测试数据
```

## 🛠️ 常见Windows问题

### 1. Go环境问题
```batch
# 检查Go安装
go version

# 如果未安装，从官网下载: https://golang.org/dl/
# 或使用包管理器: choco install golang
```

### 2. 路径问题
```batch
# Windows使用反斜杠，PowerShell中使用双引号包含路径
cd "C:\projects\IoT Gateway"

# 或使用正斜杠 (PowerShell支持)
cd "C:/projects/IoT Gateway" 
```

### 3. 编译问题
```batch
# 清理Go模块缓存
go clean -modcache
go mod download
go mod tidy

# 设置Go代理 (国内用户)
set GOPROXY=https://goproxy.cn,direct
```

### 4. 权限问题
```batch
# 以管理员身份运行PowerShell或CMD
# 或修改目录权限允许当前用户写入
```

### 5. 代理相关问题
```batch
# 检查代理设置
echo %HTTP_PROXY%
echo %NO_PROXY%

# 测试代理配置
go run cmd\test\proxy_test.go

# 临时禁用代理 (当前会话)
set HTTP_PROXY=
set HTTPS_PROXY=
set NO_PROXY=localhost,127.0.0.1,::1
```

### 6. 端口占用
```batch
# 检查端口占用
netstat -an | findstr :8081
netstat -an | findstr :4222

# 杀死占用进程
taskkill /F /PID <进程ID>
```

## 🎯 测试场景详解

### 交互式菜单选项

#### 1. 快速验证
- **耗时**: 2分钟
- **内容**: 编译检查、配置验证、基础功能测试
- **适用**: 开发过程中快速检查

#### 2. 完整测试  
- **耗时**: 10-15分钟
- **内容**: 单元测试、性能测试、并发测试、内存检查
- **适用**: 发布前全面验证

#### 3. 生成测试数据
- **耗时**: 30秒
- **内容**: 创建6种过滤器的测试场景数据
- **输出**: `test_data\filter_test_scenarios.json`

#### 4. 编译测试程序
- **耗时**: 1分钟
- **内容**: 预编译测试程序，提高后续执行速度
- **输出**: `bin\filter_test.exe`, `bin\filter_data_generator.exe`

## 📊 测试结果解读

### PowerShell颜色输出
- 🟢 **绿色**: 测试通过
- 🟡 **黄色**: 警告或跳过的测试
- 🔴 **红色**: 测试失败或错误
- ⚪ **白色/灰色**: 信息输出

### 性能基准 (Windows)
```
预期性能指标:
├── 吞吐量: > 1000 ops/sec
├── P99延迟: < 10ms  
├── 内存使用: 稳定，无泄漏
└── 并发测试: 无竞态条件
```

### 日志位置
```
logs\filter_tests\
├── filter_test_20250808_120000.log      # 详细测试日志
├── test_report_20250808_120000.md       # Markdown格式报告
├── compile.log                          # 编译日志
└── gateway.log                          # Gateway运行日志
```

## 🔍 调试技巧 (Windows)

### 1. 查看详细日志
```powershell
# 实时查看日志
Get-Content -Path "logs\filter_tests\filter_test_*.log" -Wait -Tail 50

# 搜索错误
Select-String -Path "logs\filter_tests\*.log" -Pattern "error|failed|❌"
```

### 2. 手动调试
```batch
# 单独运行测试程序，查看详细输出
.\bin\filter_test.exe

# 运行带详细输出的完整测试
.\run_filter_tests.ps1 -Verbose
```

### 3. 检查Go编译问题
```batch
# 详细编译信息
go build -v -x -o bin\filter_test.exe cmd\test\filter_tests.go

# 检查依赖
go mod graph | findstr filter
go list -m all
```

### 4. 网络和端口调试
```batch
# 检查NATS端口
telnet localhost 4222

# 检查HTTP端口  
curl http://localhost:8081/health
```

## ⚙️ 自定义配置

### 修改测试参数
编辑 `configs\test\filter_test_config.yaml`:
```yaml
gateway:
  log_level: "debug"           # 日志级别
  http_port: 8081             # HTTP端口
  nats_embedded_port: 4222    # NATS端口

rule_engine:
  concurrent_workers: 4        # 并发工作者数量
  buffer_size: 1000           # 缓冲区大小
```

### 修改过滤器参数
编辑 `configs\test\filter_test_rules.json`:
```json
{
  "config": {
    "type": "quality",
    "parameters": {
      "allowed_quality": [0],   # 允许的质量码
      "cache_ttl": "300s"       # 缓存生存时间
    }
  }
}
```

## 🚀 持续集成 (CI)

### GitHub Actions示例
```yaml
name: Filter Tests (Windows)

on: [push, pull_request]

jobs:
  test-windows:
    runs-on: windows-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: 1.21
    
    - name: Run Filter Tests
      run: |
        .\run_filter_tests.ps1 -SkipPerformance
      shell: pwsh
    
    - name: Upload Test Results
      uses: actions/upload-artifact@v3
      with:
        name: filter-test-results
        path: logs/filter_tests/
```

### Azure DevOps示例
```yaml
trigger:
- main

pool:
  vmImage: 'windows-latest'

steps:
- task: GoTool@0
  displayName: 'Install Go'
  inputs:
    version: '1.21'

- powershell: |
    .\run_filter_tests.ps1 -SkipPerformance
  displayName: 'Run Filter Tests'

- task: PublishTestResults@2
  inputs:
    testResultsFormat: 'JUnit'
    testResultsFiles: 'logs/filter_tests/test_report_*.md'
```

## 📚 相关资源

### 官方文档
- [过滤器详细说明](enhanced_filters.md)
- [测试指南](FILTER_TESTING_GUIDE.md)
- [性能调优](../performance_tuning.md)

### Windows特定工具
- [PowerShell Documentation](https://docs.microsoft.com/en-us/powershell/)
- [Windows Terminal](https://github.com/microsoft/terminal)
- [Chocolatey Package Manager](https://chocolatey.org/)

### 社区资源
- [Go on Windows](https://golang.org/doc/install#windows)
- [Git for Windows](https://gitforwindows.org/)
- [Visual Studio Code](https://code.visualstudio.com/)

---

💡 **提示**: 首次运行建议使用 `test_filters.bat` 进行交互式测试，熟悉后可直接运行对应的PowerShell脚本。