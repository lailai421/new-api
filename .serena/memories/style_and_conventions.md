# 编码风格与关键约束

- 中文沟通、文档和必要代码注释；实现要求完整可运行，不留 TODO/MVP 占位。
- 代码直接可读，优先早返回和清晰分支；遵守 SOLID、DRY、关注点分离、YAGNI。
- 不为仅一个调用者且无稳定业务语义的机械步骤新增包级 helper。
- 所有 JSON Marshal/Unmarshal 走 common/json.go 包装；业务代码不得直接调用 encoding/json 编解码函数。
- 数据库代码必须兼容 SQLite/MySQL/PostgreSQL；优先 GORM，行锁统一用 model.lockForUpdate。
- 前端所有用户文案使用 i18next 的 t('English key')，同步 en/zh/zh-TW/fr/ru/ja/vi。
- 新或大改 Go 后端测试使用 testify/require 做致命断言、testify/assert 做非致命断言；使用确定性行为测试，禁止随机、sleep、纯覆盖率测试。
- 保留用户工作区已有改动；小改动使用 apply_patch，机械替换可用 FastCtx replace；危险删除、数据库结构变更、提交推送等先获确认。