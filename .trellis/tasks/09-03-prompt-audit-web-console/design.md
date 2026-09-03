# 前端控制台设计

## 结构

新增 `web/src/features/prompt-audit/`，按 api、types、schema、hooks、components 和 page 拆分。在 security section registry 注册 `prompt-audit`，不把复杂配置塞入通用 Option 表单。

## 组件

遵循项目 `base-nova`、Base UI、Tailwind 变量和 Hugeicons 配置，复用 Card、Tabs、Table、Form、Switch、Select、Badge、Alert、Dialog/Sheet、Pagination、Skeleton 和 Sonner。

React Query 使用 config/runtime/events/detail 分离的 query key。事件列表 DTO 没有 full_prompt；详情 query 设置短生命周期并在关闭 Sheet 后主动 removeQueries。

## 页面

- 运行状态：有效模式、配置版本、节点数量、degraded、未覆盖插件。
- 策略：总开关、审计范围、分组、分类、Pass 留存、保留期。
- 节点池：有序节点表单、Token 状态和探测。
- 事件：筛选、分页、风险 Badge、详情与删除。

