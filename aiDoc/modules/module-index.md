# 模块说明索引

## 推荐写法

每个重要模块或插件，尽量用一份文档回答下面几个问题：

1. 这个模块是做什么的
2. 它的后端入口在哪里
3. 它的前端入口在哪里
4. 它依赖哪些数据或其他模块
5. 它对外暴露什么契约
6. 它有哪些必须记住的限制

## 当前建议的模块分组

- `system-core`: 项目核心能力，主要分布在 `server/` 与 `web/src/`
- `plugins`: 插件化能力，分布在 `server/plugin/` 与 `web/src/plugin/`
- `deploy`: 运行与发布相关能力，位于 `deploy/`

## 已存在的约束文档

- `backend-layer-rules.md`: 后端分层、模型、Service、API、Router、初始化入口
- `plugin-development.md`: 前后端插件结构、插件入口与开发流程

## 命名建议

新增模块文档时，优先使用这类文件名：

- `system-core.md`
- `plugin-<name>.md`
- `deploy-runtime.md`

模块说明要聚焦职责和边界，不要堆砌实现细节。

## Social Relationship

- 公开用户资料唯一DTO：server/model/common/response/user_public_profile.go；live/social类型别名复用，详情和关系列表仅扩展各自业务字段。
- 模型/DTO：server/model/social；锁与计数：server/repository/social；业务/Worker：server/service/social。
- API/Router分APP与_admin；live模块通过SocialDirectory、SocialRoomResource及LiveAccessService适配，不在social模块直接操作live表。
- 九张社交表、索引和约束随服务启动由GORM自动迁移；外部资源/通知/媒体边界见docs/social/README.md。
