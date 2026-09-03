# 常用命令（Darwin/zsh）

## 后端
- 启动：go run main.go
- 定向测试：go test -timeout 55s ./path/to/package
- 格式化：gofmt -w path/*.go
- 静态检查：go vet ./path/to/package
- 全模块入口：make test（注意按项目要求将后台测试命令控制在 60 秒内，必要时拆包）
- relaykit 独立构建：cd relaykit && GOWORK=off go build ./...

## 前端（web/）
- 安装：bun install
- 开发：bun run dev
- 测试：bun run test
- 类型：bun run typecheck
- Lint：bun run lint
- 格式：bun run format:check
- i18n：bun run i18n:sync
- 构建：bun run build

## Trellis
- 当前任务：python3 ./.trellis/scripts/task.py current
- 启动任务：python3 ./.trellis/scripts/task.py start <task-dir-or-name>
- 校验上下文：python3 ./.trellis/scripts/task.py validate <task-dir-or-name>

## 系统/版本控制
- 优先 FastCtx 做文件 glob/grep/读取；Shell 搜索优先 rg、rg --files。
- git status --short、git diff --check、git diff -- <path>。