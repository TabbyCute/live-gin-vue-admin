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
- Redis必要操作/缓存分别1秒/500ms预算，初始化启用context socket deadline；历史先核验会话再聚合，消费轮转分片、批量CAS确认。APP鉴权前设请求预算，后台复核当前账号权限；2026-10-30无新增DDL，验证及Go1.26.10构建要求见docs/social/production-review-20261030.md。

## Wallet

- [钱包账务模块](wallet.md)：人工入账、双人复核、不可变流水、冲正、额度和分批对账。

- [gift礼物模块](gift.md)：分类配置、送礼退款、收入排行、权益/事件及钱包原子接入。
- [earning主播收益](earning.md)：统一收益事实、结算、可提现账户、冻结与审核；提现申请默认开放，人工出款成功paid版本链、取消优先抵欠款。

- [payout 主播收款账户与人工出款](payout.md)：本人绑定、加密资料、原子选账/冻结、独立打款中心、公司账户、私有凭证、独立复核、unknown调查、paid与资金去向追踪。

- [guard 主播守护](guard.md)：独立来源权益、续期、退款补偿、到期、永久幂等和租约任务；通过 gift 命令附赠，无直购 API。
- [ranking 直播榜单](ranking.md)：礼物原始净 COIN 投影、Asia/Kuala_Lumpur 日周月总榜、影子代际重建与核验；不修改财务事实。
