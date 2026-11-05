# 主播收款账户与人工出款闭环

当前阶段（2026-11-03）已实现公司人工出款账户、领取/释放、真实流水登记、私有凭证、独立复核、unknown调查、失败重试、取消和frozen→paid内部账务。管理后台和H5源码均已更新。

- [人工出款前端对接](manual-frontend-integration.md)
- [人工出款运维与资金追踪](manual-operations.md)
- [本次验证与远程迁移记录](manual-verification-20261103.md)

以下为已有绑定与ready阶段能力：

2026-11-02：新增 `payout` 模块，沿用 Router → API → Service → Model，APP 只能管理当前登录主播的收款账户。提现改为必须提交本人 `payeeAccountNo`，账户核验、不可变快照、冻结流水、提现单及财务审计在同一事务提交。

- [前端变更与兼容说明](frontend-integration.md)：H5 已实现入口、绑定、默认、停用、选账和结果恢复；其他客户端按此升级。
- [安全、迁移与运维](deployment.md)：密钥、远程迁移、未来出款边界、监控及去向排查。
- [验证记录](verification-20261102.md)：实际测试、负载、扫描及尚未验收的生产边界。
- [待出款订单前端对接](order-frontend-integration.md)：后台提交、幂等恢复、独立菜单、列表和只读详情。
- [订单迁移与运维](order-deployment.md)：资金保护、加密快照、追踪及后续状态机边界。
- [订单阶段验证](order-verification-20261102.md)：最新测试和远程结构/菜单执行证据；前面的绑定验证记录保留为阶段历史。

本次支持 `bank_card`、`alipay`、`wechat` 的资料绑定。**绑定不表示渠道认证、实名匹配或可实际出款**；微信只接收特定应用作用域的 OpenID，实际字段取决于未来接入的微信出款产品。提现申请和审核不判断用户实名认证、主播认证或收款实名匹配，保留主播审核状态、账号状态、运营提现权限、风险、有效兑换规则、余额、退款及账务检查。

新增三表：`payout_payee_account`、`payout_owner_lock`、`payout_payee_audit`；提现表只追加 `payee_ref` 查询索引，既有收款快照字段继续沿用。收款资料 AES-256-GCM 加密，密文带 key ID 和 payload version；防重用独立密钥的 HMAC-SHA256。密文、nonce、指纹、真实姓名、完整账号和内部 ID 均不进入 APP DTO、后台 DTO 或审计快照。

所有账户永久保留，敏感资料不允许原地更新，停用不可恢复。每主播最多100条历史绑定，分页最多100条，数量包括停用账户；相同主播/方式/币种/标识不能重复绑定。需要调整此上限或重新绑定同一已停用标识时，应新增经过审查的版本/生命周期方案，不能删历史记录绕过约束。

默认账户只是新申请时的选择提示。数据库唯一索引 `(owner_type, owner_no, currency, default_slot)` 以 NULL 表示非默认、1表示默认；结合永久 owner 行锁，阻止并发双默认。停用默认账户不会自动挑选其他账户。敏感版本当前为字符串 `"1"`，默认或状态变化不增加版本，也不修改提现快照。

当前`payout_order`支持七种人工流程状态。ready只表示待领取，processing及pending_confirmation继续冻结，人工独立核验成功才原子入账paid；failed继续冻结，unknown必须调查，cancelled仅在未付款时释放冻结并优先抵债。`payoutEnabled=false`保留自动外部支付能力语义，人工权限以capabilities/allowedActions为准。旧`payout_center_pending`单仍原样保留，不自动补选账户。没有接入银行API或自动银行对账，内部测试不等于真实银行卡到账。
