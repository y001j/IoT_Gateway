# IoT Gateway 前端修复指南

## 🔧 已修复的配置问题

### 1. ESLint 配置 (✅ 已修复)
- 更新了 `.eslintrc.json` 配置
- 修复了 TypeScript ESLint 插件配置

### 2. 依赖更新 (✅ 已修复)  
- 更新了 `package.json` 中的所有依赖版本
- 修复了 Vite 配置兼容性

### 3. TypeScript 配置 (✅ 已修复)
- 添加了全局类型定义文件 `src/types/global.d.ts`
- 更新了 `tsconfig.json` 配置

## 🚨 需要手动修复的问题

### 1. 缺少的 Antd 组件导入

以下组件需要添加到相应文件的导入中：

```typescript
// 需要添加到各个页面文件顶部
import { 
  Space, 
  Badge, 
  Statistic 
} from 'antd';
```

**受影响的文件**：
- `src/pages/AlertsPage.tsx`
- `src/pages/Dashboard.tsx` 
- `src/pages/MonitoringPage.tsx`
- `src/pages/PluginsPage.tsx`
- `src/pages/RulesPage.tsx`
- `src/components/PluginDetailModal.tsx`
- `src/components/charts/DataFlowChart.tsx`

### 2. 缺少的图标导入

```typescript
// 需要添加到各个页面文件顶部
import {
  SettingOutlined,
  PlayCircleOutlined,
  EyeOutlined,
  WarningOutlined,
  PieChartOutlined
} from '@ant-design/icons';
```

### 3. TypeScript 类型问题

替换所有 `any` 类型为具体类型：

```typescript
// 替换前
const handleSubmit = (values: any) => { ... }

// 替换后  
interface FormValues {
  name: string;
  description: string;
  enabled: boolean;
}
const handleSubmit = (values: FormValues) => { ... }
```

### 4. React Hook 依赖问题

修复 useEffect 依赖数组：

```typescript
// 问题代码
useEffect(() => {
  fetchData();
}, []);

// 修复后
useEffect(() => {
  fetchData();
}, [fetchData]);

// 或使用 useCallback
const fetchData = useCallback(async () => {
  // 获取数据逻辑
}, []);
```

## 🚀 快速修复脚本

创建以下脚本来批量修复导入问题：

```bash
# 1. 修复 Antd 组件导入
npm run fix:imports

# 2. 修复类型定义
npm run fix:types  

# 3. 修复 Hook 依赖
npm run fix:hooks
```

## 📋 修复优先级

1. **高优先级** - 修复导入错误（阻止构建）
2. **中优先级** - 修复 TypeScript 类型错误
3. **低优先级** - 优化 React Hook 依赖

## 🔍 验证修复

```bash
# 检查语法错误
npm run lint

# 检查类型错误  
npm run build

# 启动开发服务器
npm run dev
```

## 💡 最佳实践建议

1. **使用 TypeScript 严格模式**
2. **避免使用 any 类型**
3. **正确处理 React Hook 依赖**
4. **使用 ESLint 自动修复功能**
5. **定期更新依赖版本**

## 📞 如需帮助

如果遇到具体的修复问题，请提供：
1. 具体的错误信息
2. 相关的文件路径
3. 期望的功能描述 