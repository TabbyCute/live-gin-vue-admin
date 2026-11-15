# 业务需求索引

## Active

- [观众访问的主播公开主页一期](active/anchor-personal-center.md)：代码、H5/后台及本地隔离回归完成；远程prod仅只读预检，MySQL握手超时，尚未迁移/发版，待连接恢复后备份迁移复验（2026-11-14）。


- [客户端注册至主播出款交付复核](active/client-payout-delivery-audit.md)：全链路隔离测试、安全与可观测性加固、远程结构核验及13模块评估；已获服务器访问方式，尚未SSH连接或发布。

- [直播观众与在线状态](active/live-presence.md)：签名游客与会员、后台按主播查看人数、H5心跳/点赞、隔离验证及远程结构迁移完成；资金摘要不变，应用待发布。

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

- [前台客户端生产级 UI/UX 需求提取](done/client-ui-ux-requirements.md)：仅审计用户/主播前台 H5Player；十份功能、页面、状态、流程、设计系统、多平台提示与覆盖报告已完成，未修改业务代码；原生 Android/iOS 实现待确认（2026-11-14）。

- [H5对接本轮后端优化](done/h5-backend-hardening-integration.md)：结算记录、状态刷新、请求定位与IM入口；67项测试、手机UI模拟与生产资源打包完成，用户已确认只做本地隔离测试、账号仅作身份参考，未部署（2026-11-14确认）。

- [单机后端P1/P2优化及本地资金链模拟](done/backend-single-node-hardening.md)：后端/必要admin、本地Docker测试完成；prod范围备份47表、新增14表、财务不变及独立核验通过；无H5/SSH/应用发布，遗留边界见交付报告（2026-11-12）。

- [提现加密密钥直接读取YAML](done/payout-yaml-keys.md)：三项加密配置与中文注释、随机密钥、配置API隐藏及保存保护，取消旧环境变量依赖（2026-11-05）。

- [后台固定超级 root 管理员](done/backend-root-admin.md)：ID 1 默认拥有全部功能权限，跨模块免角色授权并可独立复核；已按用户要求完成远程正式库四个审核触发器升级及校验（2026-11-01）。

- [钱包账户、余额流水与人工入账](done/wallet-accounting.md)：系统人工充值、管理后台三页面、客户端对接文档及真实MySQL并发/故障复验完成（2026-10-28）。

- [正式与测试环境配置分离](done/environment-config-separation.md)
- [客户端账号注册登录与独立鉴权](done/live-account-authentication.md)
- [LiveAnchor 第一版主播身份与管理模块](done/live-anchor-v1.md)
- [LiveCategory 第一版直播分类与后台管理](done/live-category-v1.md)
- [LiveRoom / LiveSession 第一版直播间与场次](done/live-room-session-v1.md)
- [客户端账号与主播调试页](done/live-client-debug-pages.md)
- [删除独立 API 管理功能](done/remove-api-management.md)
