# 集成检查实施步骤

## 前置输入

六个实现子任务全部完成并合并后开始。

## 步骤

1. 建立父 PRD 验收项到代码与测试的映射清单。
2. 执行各入口 Pass/Block/Error 测试并记录业务上游和预扣费计数。
3. 检查配置版本、degraded、密钥、事件写入和权限边界。
4. 检查日志、API、前端列表与缓存的敏感数据泄漏。
5. 分别运行 SQLite、MySQL、PostgreSQL 的新建和升级矩阵。
6. 分包执行 Go 测试，单条后台命令不超过 60 秒。
7. 使用 Bun 执行前端 test、typecheck、lint、format:check、i18n:sync、build。
8. 检查 relaykit 未修改；若修改则独立 GOWORK=off 构建。
9. 对发现的问题在责任子任务边界内修复并重新验证，不削弱安全契约。
10. 输出精确版本、命令、结果和剩余风险。

## 完成标准与交付证据
 
 只有全部依赖完成、所有验收证据齐全且三数据库矩阵通过后，才能建议父任务完成。
 
- **交付状态**: 已完成 (COMPLETED)
- **父 PRD 验收覆盖**: 20/20 全部覆盖并通过（见 `research/integration-report.md`）。
- **三数据库真实矩阵验证**:
  - SQLite 3: 内存与本地文件引擎，AutoMigrate 幂等性、TEXT 类型映射、>64 KiB 加密读写、CAS 与清理全部通过。
  - MySQL 8.0: Docker 真实实例 (`127.0.0.1:13306`)，AutoMigrate 幂等性、`longtext` 映射验证、>64 KiB 逐字符比对一致、行锁 CAS、升级测试全部通过。
  - PostgreSQL 18: Docker 真实实例 (`127.0.0.1:15432`)，AutoMigrate 幂等性、`text` 映射验证、>64 KiB 逐字符比对一致、升级测试全部通过。
- **全套质量命令**:
  - 后端分包单测全部通过 (包含 `go test -race`，单条超时 <=55s)；
  - `go vet` 零警告零错误；
  - `relaykit` 独立构建 `cd relaykit && GOWORK=off go build ./...` 通过；
  - 前端 Bun 检查通过 (`test`, `typecheck`, `oxlint`, `format:check`, `copyright:check`, `i18n:sync`, `build`)；
  - 敏感数据隔离与零日志泄漏验证通过。
