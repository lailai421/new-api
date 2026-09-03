# new-api 项目概览

- 用途：统一代理 40+ AI 上游供应商的 API 网关，包含用户、鉴权、计费、限流、管理后台与多模型渠道适配。
- 后端：Go（当前 go.mod 为 1.25.1）、Gin、GORM v2；主链路按 Router → Controller → Service → Model 分层。
- 前端：React 19、TypeScript、Rsbuild、Base UI、Tailwind CSS；位于 web/，优先使用 Bun。
- 数据：SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6 必须同时兼容；Redis 与内存缓存。
- 主要目录：router/、controller/、service/、model/、relay/、middleware/、setting/、common/、dto/、constant/、types/、i18n/、oauth/、pkg/、web/。
- relaykit/ 是独立 Go 模块，不得依赖根 new-api 模块；触及时需用 GOWORK=off 独立验证。
- 项目身份 new-api 与 QuantumNous 相关引用、品牌、元数据和归属信息受保护，禁止修改或删除。