# 存储与管理 API 设计

## 数据边界

Model 层拥有配置 Option 的锁定/CAS 和事件 CRUD。Service 层提供 ConfigStore/EventStore 具体实现、原文加解密和 master-only 清理循环。Controller 只接受管理 DTO，不直接返回 GORM 模型。

事件正文使用自定义 GORM 长文本类型：MySQL 映射 LONGTEXT，PostgreSQL/SQLite 映射 TEXT。分类和证据以 TEXT JSON 保存，编解码使用 `common.*`。

## API 边界

路由前缀 `/api/prompt-audit`，统一 `middleware.RootAuth()`：

- GET/PUT `/config`
- GET `/runtime`
- POST `/endpoints/probe`
- GET `/events`
- GET/DELETE `/events/:id`
- POST `/events/batch-delete`

列表只返回脱敏元数据，详情单独解密。配置和删除写管理日志，节点探测响应不携带 Token 和 Guard 原始响应。

## 数据库约束

不使用外键、JSONB、BIGSERIAL、TIMESTAMPTZ、SKIP LOCKED 和方言专用 CHECK。索引围绕 created_at/id、request_id、user_id、token_id、group、decision、risk、endpoint 和 hash 建立。

