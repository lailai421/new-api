# 完成任务检查清单

1. 对照 PRD/设计逐项核对可观察验收标准与范围边界。
2. 对受影响 Go 文件执行 gofmt，按包运行小于 60 秒的定向测试和 go vet。
3. 前端改动在 web/ 使用 Bun 执行 test、typecheck、lint、format:check、i18n:sync、build。
4. 涉及数据库行为时必须用真实 SQLite、MySQL、PostgreSQL 完成规定矩阵；未执行不得声称三库兼容。
5. 触及 relaykit 时执行 cd relaykit && GOWORK=off go build ./...。
6. 检查 JSON 包装、日志敏感信息、错误边界、无 TODO/占位、无无关改动。
7. 检查 git diff、git diff --check、git status；不主动 commit/push。
8. 使用 Trellis check/finish-work 流程，并把关键约束、决策、验证结果与经验写入 Serena memory。