# 前端控制台实施步骤

## 前置输入

存储与管理 API 子任务完成并冻结 DTO 后开始。读取 `web/AGENTS.md` 和项目 shadcn/ui 规范。

## 步骤

1. 定义 API 类型、Zod schema 和 React Query keys。
2. 实现配置、runtime、probe、events、detail、delete API 与 hooks。
3. 注册安全设置 section 和 Prompt Audit 页面。
4. 实现运行状态和策略表单，处理 CAS 冲突与 degraded。
5. 实现有序节点池和 write-only Token 交互。
6. 实现事件筛选、分页、详情按需加载、复制与删除确认。
7. 补齐相邻组件测试和可访问性断言。
8. 更新七个 locale，运行 i18n 同步。
9. 使用 Bun 执行 test、typecheck、lint、format:check 和 build。

## 交付证明

测试必须证明列表响应和缓存中没有 full_prompt，详情关闭后对应缓存被移除。

