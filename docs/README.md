# IoT Gateway 文档

本文档集包含 IoT Gateway 项目的核心设计文档和用户指南。

## 📚 文档结构

### 🚀 快速开始
- [快速入门指南](quick_start.md) - 项目安装、配置和基本使用指南

### 🏗️ 系统架构
- [核心运行时设计](core_runtime.md) - Core Runtime 模块设计文档
- [插件管理器设计](plugin_manager.md) - Plugin Manager 模块设计文档  
- [ISP协议架构](isp_architecture.md) - IoT Sidecar Protocol 架构设计
- [NATS消息总线架构](nats_architecture.md) - NATS 集成架构设计

### 📋 规则引擎
- [规则引擎概览](rule_engine.md) - 规则引擎完整文档
- [复合数据类型支持](complex_data_types.md) - GPS、3D向量、颜色等复合数据处理
- [聚合函数详解](aggregation_functions.md) - 28种聚合函数的完整说明
- [规则示例集合](rule_examples.md) - 丰富的规则配置示例和最佳实践
- [规则引擎详细文档](rule_engine/) - 分模块详细文档
  - [01_概览](rule_engine/01_overview.md) - 规则引擎概述
  - [02_配置](rule_engine/02_configuration.md) - 规则配置详解
  - [03_动作](rule_engine/03_actions.md) - 动作处理器详解
  - [04_API参考](rule_engine/04_api_reference.md) - API接口文档
  - [05_最佳实践](rule_engine/05_best_practices.md) - 使用最佳实践

## 🎯 文档指南

### 用户类型导航

**📱 新用户**
1. 阅读项目 [README.md](../README.md) 了解项目概况
2. 跟随 [快速入门指南](quick_start.md) 进行初始设置
3. 学习 [规则引擎概览](rule_engine.md) 了解核心功能

**🔧 开发者**
1. 学习 [系统架构](#🏗️-系统架构) 了解整体设计
2. 深入 [规则引擎详细文档](rule_engine/) 了解核心组件
3. 参考 [最佳实践](rule_engine/05_best_practices.md) 进行开发

**🚀 部署运维**
1. 查看 [NATS架构](nats_architecture.md) 了解消息总线配置
2. 学习 [插件管理](plugin_manager.md) 了解插件部署
3. 参考主项目 [README.md](../README.md) 的部署指南

### 技术主题导航

**🔌 插件开发**
- [插件管理器设计](plugin_manager.md) - 了解插件架构
- [ISP协议架构](isp_architecture.md) - Sidecar 插件通信协议

**📡 数据处理**  
- [规则引擎概览](rule_engine.md) - 数据处理规则系统
- [复合数据类型支持](complex_data_types.md) - 高级数据类型处理能力
- [聚合函数详解](aggregation_functions.md) - 28种统计分析函数
- [规则示例集合](rule_examples.md) - 实用的规则配置案例
- [NATS架构](nats_architecture.md) - 消息总线数据流

**⚙️ 系统集成**
- [核心运行时](core_runtime.md) - 系统启动和生命周期
- [快速入门](quick_start.md) - 完整集成示例

## 📝 文档维护

### 版本信息
- **文档版本**: v1.0.0
- **最后更新**: 2025-08-14
- **维护者**: IoT Gateway Team

### 贡献指南
欢迎改进文档！请遵循：
1. 保持文档结构清晰简洁
2. 使用清晰的标题和目录结构
3. 提供实用的代码示例
4. 及时更新过时信息

### 文档规范
- 使用 Markdown 格式编写
- 包含适当的代码示例和配置
- 提供清晰的图表和架构图
- 保持技术文档的准确性和时效性

---

💡 **提示**: 如果找不到所需信息，请查看主项目的 [README.md](../README.md) 或提交 [Issue](https://github.com/y001j/IoT_Gateway/issues) 获取帮助。
