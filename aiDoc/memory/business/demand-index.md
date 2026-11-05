# 业务需求索引

## Active

- [主播守护与直播榜单](active/guard-ranking.md)：代码、后台、APP、充值到期测试及远程15表/5菜单/7条回填完成；待实际发布、方案配置及Worker核验，守护开关保持关闭。

- [主播人工出款完整闭环](active/anchor-manual-payout.md)：公司人工账户、领取/尝试/私有凭证、独立复核、unknown调查、paid入账与取消解冻，礼物到付款记账的隔离全链测试；开发及隔离验收完成；远程结构已迁移，应用待发布。

- [主播提现提交打款中心与ready订单](active/anchor-payout-ready.md)：复用绑定选账/审核，新增独立一级打款中心、原子幂等订单、加密快照/历史/审计、列表详情、远程迁移和全链验证；无实际付款。

- [主播收款账户绑定与提现选账](active/anchor-payout-payee.md)：payout加密绑定、原子选账冻结、历史单隔离、后台去向筛选与H5管理，远程迁移及安全/竞态验证见docs/payout。

- [主播收益结算与提现审核](active/anchor-earning-phase1.md)：统一收益来源、结算兑换、独立可提现账户、冻结、自动对账、人工审核；后续ready订单见独立需求。提现不判断实名认证，H5已接入收益。2026-11-02补齐主播列表结算入口、规则分页选择、默认最近7天/当前秒和资金回归测试（生产限制见详情）。

- [礼物第一阶段](active/live-gift.md)：礼物配置、送礼退款、六项复核修复、客户端对接、prod草稿和菜单同步。2026-11-02优化场次礼物数量/价值，复验安全、并发、资金与性能（生产限制见详情）；历史规则清理保留审计。


- [社交关系模块](active/social-relationship.md)：统一公开资料；Redis延迟、后台任务、对外接口安全和生产性能加固复验（2026-10-30）。

- [仓库许可证迁移到 BSL 1.1](active/license-migrate-to-bsl.md)
- [管理后台品牌配置化](active/frontend-branding-customization.md)

## Done

- [后台固定超级 root 管理员](done/backend-root-admin.md)：ID 1 默认拥有全部功能权限，跨模块免角色授权并可独立复核；已按用户要求完成远程正式库四个审核触发器升级及校验（2026-11-01）。

- [钱包账户、余额流水与人工入账](done/wallet-accounting.md)：系统人工充值、管理后台三页面、客户端对接文档及真实MySQL并发/故障复验完成（2026-10-28）。

- [正式与测试环境配置分离](done/environment-config-separation.md)
- [客户端账号注册登录与独立鉴权](done/live-account-authentication.md)
- [LiveAnchor 第一版主播身份与管理模块](done/live-anchor-v1.md)
- [LiveCategory 第一版直播分类与后台管理](done/live-category-v1.md)
- [LiveRoom / LiveSession 第一版直播间与场次](done/live-room-session-v1.md)
- [客户端账号与主播调试页](done/live-client-debug-pages.md)
- [删除独立 API 管理功能](done/remove-api-management.md)
