# 社交关系模块

- 提出日期：2026-10-25
- 状态：active
- 2026-10-27 新增要求：粉丝、关注、历史与查看用户共用公开用户资料结构体，批量返回当前观众可见的直播状态；历史列表改为用户资料，删除 GET favorite/list；审查并优化安全、MySQL性能、并发一致性、可靠性与极限边界，提供可复现验证和容量限制。
- 需求：完整实现关注、拉黑、分范围屏蔽、收藏、观看历史、开播订阅、可靠事件及集中直播鉴权；命名严格遵守现有项目。
- 约束：GET/POST；APP JWT 与管理 JWT 分离；snake_case 单数表名、camelCase DTO、公开 roomNo/anchorNo/sessionNo；关系事实与计数同事务；GORM自动迁移；不把未接入的媒体/CDN/第三方渠道宣称完成。
- 调查：用户表 live_account；主播 live_anchor.user_id；live_room.visibility 已有 0私密/1公开/2关注者；无短视频/回放资源表和通知渠道。后台已移除 Casbin，敏感接口须额外 RequireAuthority(888)。
- 实现与验收记录：docs/social/README.md。

## 当前交付（2026-10-25）

- 九表、分层代码、39个APP/后台社交路由、集中直播鉴权及媒体代理适配、自动迁移、Swagger和维护工具已实现。
- 已运行真实MySQL/Redis集成、1000并发、局部race、故障注入和容量测量；详情见docs/social/verification.md。
- 已补充前端对接说明 `docs/social/frontend-integration.md`，明确当前可接的 APP/后台能力、真实 DTO、错误处理和暂不接入的推送、短视频、回放及媒体链路。
- 用户要求删除 `social.enabled` 总开关，社交核心路由与 Worker 改为随服务默认启动；表结构随服务启动自动迁移，媒体鉴权继续由独立开关控制。
- 用户已确认真实短视频/回放、通知渠道和生产 CDN 媒体链路不属于当前需求范围；文档中应标为范围外能力，不作为本期未完成项。
- 2026-10-27 直播间公开详情新增主播关联账号的 `userInfo`。社交关系仍以 `live_account.id` 为事实主体，前端优先使用 `userInfo.accountNo` 发起关注等操作；`anchorNo` 仅作为同一账号的兼容目标。当前观众关系状态通过 `/api/v1/app/social/follow/status` 查询，不混入公开 `userInfo`。

## 2026-10-27 公开资料统一与加固

- 新建唯一公开结构 `model/common/response.UserPublicProfile`，live/social类型别名复用；所有上述用户关系视图补齐公开资料及当前观众可见的直播状态，批量SQL避免N+1。
- 历史list改为用户资料+原资源键+时间；progress/batch保留资源进度；删除favorite/list路由/API/Service/Swagger，当前社交路由38个（APP31、后台7）。
- 单语句关系快照、当前场次主键查询、有界交集/异常候选扫描、减少已有counter重复写入与历史重复COUNT；Worker分工、分片历史租约、历史单调版本及Lua精度修复。
- 仍沿用九张表及GORM自动迁移，仅新增counter.history_version、扩展outbox.object_no到96字符。不部署、不修改生产数据库。
- 完整证据、前端兼容改动和容量边界见 `docs/social/production-review-20261027.md`；不承诺未经生产负载验证的容量或永久无需优化。
