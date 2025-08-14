# Contributing to IoT Gateway

感谢您对 IoT Gateway 项目的关注！我们欢迎各种形式的贡献，无论是代码、文档、bug报告还是功能建议。

## 📋 如何贡献

### 🐛 报告 Bug

如果您发现了 bug，请通过以下步骤报告：

1. 检查 [Issues](https://github.com/y001j/IoT_Gateway/issues) 中是否已经有相同的问题
2. 如果没有，请创建新的 issue 并包含以下信息：
   - 清晰的问题描述
   - 重现步骤
   - 期望的结果
   - 实际的结果
   - 系统环境信息（操作系统、Go版本、Node.js版本等）
   - 相关的日志或截图

### 💡 建议新功能

我们欢迎新功能建议！请：

1. 检查是否已有类似建议
2. 创建 Feature Request issue
3. 详细描述功能需求和使用场景
4. 如果可能，请提供设计方案或代码示例

### 🔧 代码贡献

#### 开发环境设置

1. **Fork 项目**
```bash
git clone https://github.com/YOUR_USERNAME/IoT_Gateway.git
cd IoT_Gateway
```

2. **设置开发环境**
```bash
# 安装 Go 依赖
go mod download

# 安装前端依赖
cd web/frontend
npm install
cd ../..
```

3. **创建分支**
```bash
git checkout -b feature/your-feature-name
# 或
git checkout -b fix/issue-number
```

#### 代码规范

##### Go 代码规范
- 遵循 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- 使用 `gofmt` 格式化代码
- 使用 `go vet` 进行静态检查
- 添加必要的单元测试
- 保持函数简洁，单个函数不超过 50 行
- 添加适当的注释，特别是导出的函数和类型

```bash
# 格式化代码
go fmt ./...

# 静态检查
go vet ./...

# 运行测试
go test ./...
```

##### TypeScript/React 代码规范
- 遵循项目的 ESLint 配置
- 使用 TypeScript 类型定义
- 组件使用函数式组件和 Hooks
- 遵循 React 最佳实践

```bash
cd web/frontend

# 代码检查
npm run lint

# 格式化
npm run lint:fix

# 测试
npm test
```

#### 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 格式：

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

**类型 (Type):**
- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `style`: 代码格式修改（不影响代码逻辑）
- `refactor`: 代码重构
- `perf`: 性能优化
- `test`: 测试相关
- `chore`: 其他修改

**示例:**
```bash
feat(rules): add support for complex data type aggregation
fix(frontend): resolve styled-jsx compilation error
docs: update API documentation for new endpoints
```

#### Pull Request 流程

1. **确保代码质量**
   - 所有测试通过
   - 代码已格式化
   - 没有 linting 错误
   - 添加了必要的测试

2. **更新文档**
   - API 变更需要更新文档
   - 新功能需要更新 README
   - 配置变更需要更新配置说明

3. **创建 Pull Request**
   - 提供清晰的标题和描述
   - 链接相关的 issue
   - 添加截图（如果是 UI 变更）
   - 填写 PR 模板

4. **代码审查**
   - 耐心等待代码审查
   - 根据反馈及时修改
   - 保持讨论的专业性和建设性

### 📝 文档贡献

文档改进包括：
- 修复错别字
- 改进现有文档
- 添加示例
- 翻译文档

### 🧪 测试

#### 测试类型

1. **单元测试**
```bash
# 运行所有单元测试
go test ./...

# 运行特定包的测试
go test ./internal/rules/...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

2. **集成测试**
```bash
# 运行集成测试
go test -tags=integration ./...
```

3. **前端测试**
```bash
cd web/frontend
npm test
```

#### 测试要求

- 新功能必须包含单元测试
- 测试覆盖率应保持在 80% 以上
- 集成测试应该测试端到端的功能
- 性能敏感的代码应包含基准测试

## 📦 发布流程

### 版本号规范

使用 [Semantic Versioning](https://semver.org/):
- `MAJOR.MINOR.PATCH`
- `MAJOR`: 不兼容的 API 变更
- `MINOR`: 向后兼容的功能新增
- `PATCH`: 向后兼容的 bug 修复

### 发布检查清单

- [ ] 所有测试通过
- [ ] 文档已更新
- [ ] CHANGELOG 已更新
- [ ] 版本号已更新
- [ ] 性能测试通过
- [ ] 安全扫描通过

## 🏷️ Issue 和 PR 标签

### Issue 标签
- `bug`: Bug 报告
- `enhancement`: 功能增强
- `feature`: 新功能
- `documentation`: 文档相关
- `question`: 问题咨询
- `help wanted`: 需要帮助
- `good first issue`: 适合新贡献者

### PR 标签
- `ready for review`: 准备审查
- `work in progress`: 开发中
- `needs tests`: 需要测试
- `needs documentation`: 需要文档

## 💬 沟通渠道

- **GitHub Issues**: Bug 报告和功能请求
- **GitHub Discussions**: 一般讨论和问答
- **Pull Requests**: 代码审查和讨论

## 🎯 贡献者指南

### 首次贡献者

如果您是首次贡献者，建议从以下开始：
- 标记为 `good first issue` 的问题
- 文档改进
- 单元测试补充
- 代码注释完善

### 核心贡献者

对于活跃的贡献者，我们可能邀请您成为项目维护者，参与：
- 代码审查
- 发布管理
- 架构决策
- 社区管理

## 📜 行为准则

请遵循我们的行为准则：
- 尊重所有参与者
- 保持专业和建设性的讨论
- 欢迎新贡献者
- 专注于技术问题
- 避免人身攻击或歧视

## ❓ 常见问题

### Q: 我应该从哪里开始？
A: 查看标记为 `good first issue` 的问题，或者帮助改进文档。

### Q: 我的 PR 多久会被审查？
A: 通常在 3-5 个工作日内。复杂的 PR 可能需要更长时间。

### Q: 如何运行完整的测试套件？
A: 运行 `make test` 或查看 `.github/workflows/` 中的 CI 配置。

### Q: 我可以修改项目架构吗？
A: 大的架构变更请先创建 RFC（Request for Comments）issue 进行讨论。

---

感谢您的贡献！🎉