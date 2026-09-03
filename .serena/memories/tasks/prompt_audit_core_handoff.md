# prompt-audit-core 交接提示词要点（2026-09-03）

- 父任务 09-03-prompt-audit 与 core 子任务均保持 planning；未获得用户对最新规划总结的后续明确批准前，不运行 task.py start、不改业务代码。
- core 只负责 service/promptaudit 领域契约、两态配置、AES-256-GCM 用途隔离、不可变运行时/degraded、Unicode 分片、并发隔离、有序 Guard failover、Qwen3Guard 严格解析和确定性测试。
- 不负责数据库/Option、Controller/Router/Relay、协议提取、Task Plugin、Realtime、前端，也不得改 relaykit。
- 交接提示词已写入 .trellis/tasks/09-03-prompt-audit-core/handoff-prompt.md，包含恢复上下文、审批门禁、完整范围、安全约束、测试矩阵和完成标准。
- 当前文档检查：新文件存在，git diff --check 通过；仅创建 handoff-prompt.md，未改业务代码。