# 提示词审计存储与管理 API

## 目标

实现配置和审计事件的三数据库兼容持久化、完整提示词加密留存、保留清理及 Root 专用管理 API。

## 依赖

- 依赖父任务 `09-03-prompt-audit`。
- 强依赖 `09-03-prompt-audit-core` 已冻结 ConfigStore、EventStore、Encryptor、PublicConfig、Decision 和错误码接口。
- 为 HTTP、Realtime、Task Plugin 和前端子任务提供可用的事件记录与管理 API。

## 范围

- `PromptAuditConfigSecret` Option 的显式隐藏、通用更新拒绝和 config_version CAS。
- `PromptAuditEvent` GORM 模型、跨库 LongText、索引和 AutoMigrate。
- Block/Error 必存、Pass/Flag 按开关存储；必要写入失败时返回 503。
- 完整原文加密落库，列表排除原文，Root 详情按需解密。
- 默认永久保留、可配置自动清理、单条和最多 500 条批量删除。
- Root 配置、运行状态、节点探测和事件管理 API。

## 不包含

- 具体业务协议提取及 Relay 接线。
- 前端页面。

## 验收标准

- [ ] SQLite、MySQL、PostgreSQL 的迁移、写入、查询和删除语义一致。
- [ ] 超过 64 KiB 的完整提示词不会被 MySQL TEXT 截断，可逐字符解密还原。
- [ ] 通用 Option API、事件列表和日志不返回 Token、密文或完整提示词。
- [ ] 只有 Root 详情 API 返回完整提示词，并设置 `Cache-Control: no-store`。
- [ ] CAS 冲突返回 409；配置损坏或密钥失效在 runtime 中显示 degraded。
- [ ] 必要事件写入失败时返回 `prompt_audit_record_failed`。
- [ ] 保留期 0 不自动删除；有限天数只删除截止时间之前的数据。
- [ ] 手动删除有二次调用契约和管理日志，日志不含原文。

