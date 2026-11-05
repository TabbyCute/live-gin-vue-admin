# 收益事实、账户及扩展契约

## 数据与状态

| 表 | 作用 |
| --- | --- |
| anchor_income_fact | 不可变统一收益事实；永久唯一 `(source_type, source_entry_no)` |
| anchor_income_lock | 每主播串行化锁，礼物收益追加、退款、结算、冻结、审核共享 |
| anchor_finance_control | 兑换规则修改排他锁、结算读取共享锁、菜单同步锁 |
| anchor_settlement_rule | 显式整数兑换比例、币种、版本、生效期间及双人复核 |
| anchor_settlement | 结算申请、规则快照、总额、明细哈希及审批记录 |
| anchor_settlement_item | 固定收入事实哈希、原兑换规则、顺序、金额及兑换余数前后值 |
| anchor_settlement_allocation | 收益事实的永久结算归属，主键 income_no；仅批准时追加 |
| anchor_settlement_rounding | 每主播/兑换规则的累计分子余数、版本及最后结算单 |
| anchor_withdrawable_account | 按主播/币种独立的 available/frozen/debt/paid/version |
| anchor_withdrawable_ledger | 不可变余额前后值、版本、业务编号和变动种类 |
| anchor_withdrawal_order | 提现请求、申请时不可变脱敏收款快照、冻结及人工复核状态 |
| anchor_reconciliation | 每次系统对账的不可变明细快照与哈希 |
| anchor_finance_audit | 创建与审核行为审计，操作者、IP（管理操作）、快照 |
| anchor_finance_permission | 精确角色/HTTP 方法/路径授权，与菜单可见性分离 |

兑换规则 pending → active/rejected。结算 pending → approved/rejected。提现 pending_review → approved/rejected；approved 不等于已付款。审核通过不能再次改为驳回；没有 paid、dispatch 或手工调账通道。

## 金额与退款

金额持久化为有符号 int64，所有加减检查溢出；兑换乘法采用任意精度中间值。APP/后台 JSON 金额、版本、总条数使用十进制字符串，毫秒时间戳为数值。COIN 收益与目标币种最小单位分开，币种间禁止合并余额。

兑换规则定义 `coinUnits COIN = minorUnits 目标币种最小单位`。同主播同规则累计计算：

```
n = incomeAmount * minorUnits + remainderBefore
amount = floor(n / coinUnits)
remainderAfter = n - amount * coinUnits
0 <= remainderAfter < coinUnits
```

余数持久保留，分批结算不损失小额收益。退款使用原正向收益的兑换规则及对应余数，不能用后来更改的比例。规则切换时旧规则余数保留，不擅自按新汇率转换。每笔明细记录余数版本/顺序；另一笔结算抢先改变余数或收益归属后，旧预览批准失败 49004，需驳回后重新生成。待审核与驳回预览不占用收益。

结算入账、归属映射、余额流水、账户更新、余数更新和审计在同一数据库事务中提交。退款不改历史结算：增加负向收益，下一次结算扣减可用金额；不足部分记 debt。已有冻结不被退款窃取。后续收入或驳回释放先还 debt，剩余再成为 available。存在 debt 或任何未结算退款时禁止新提现与审核通过。

当前账户守恒式为 `结算正向累计 + 负向调整累计 = available + frozen - debt`；opening/manualAdjustments/paid 固定为 0。本期无人工余额调整和真实支付，不能假造付款流水。

## 对账和并发

系统在提现审核通过前自动重新对账，不采信客户端的 `consistent`、状态、金额或旧对账结论。核验账户版本链和每笔冻结/释放、结算唯一归属、收入及明细哈希、余数链、双人审批审计、礼物原订单/退款与钱包交易/流水逐笔关联、主播可用性、运营提现权限、风险与收款快照变化；不读取或判断实名认证与主播认证状态。异常保持 pending_review，保存 abnormal 快照并阻止通过；驳回须先验证账务后释放冻结。

快照是 JSON 字符串；`snapshotHash` 当前定义为 SHA-256(JSON 编码该字符串)，包括字符串转义与外层引号。它用于一致性验证，并非防数据库管理员篡改的外部签名。数据库 CHECK、唯一索引、不可变触发器和余额变动触发器共同保护事实；能修改数据库 schema 的管理员仍属于受信边界。

每主播锁、唯一业务键、余额版本 CAS 和同库事务防止重复入账/超额冻结/审核竞争。只重试 MySQL 死锁 1213 和锁超时 1205，最多 4 次，受请求 context 预算限制。请求重复使用原 requestId 和原参数；相同键不同参数必须报冲突。HTTP 处理中的收款服务调用在持锁前完成，最长 3 秒，取得后在锁内校验快照新鲜度。

## 新收益来源

可信业务服务在自己的支付/履约事务中调用 `service/earning.AccrueInTx(tx, IncomeInput)`。必须传主播内部 ID、稳定 sourceType/sourceNo/sourceEntryNo、单位、金额、分成规则及比例、创建时间、业务事实哈希。退款使用全新 sourceEntryNo，带 originalIncomeNo、负金额；校验同主播、来源、原订单、规则及累计退款上限。来源幂等键永久保留，禁止只依赖 Redis TTL。

同时注册 `SourceVerifier(tx, []Income) error`，批量核验原业务支付、退款、价格/收益快照、履约及唯一关联。不注册或核验不可用返回 49011/49009。不能将示例中仅返回 nil 的测试提供方注册到生产。当前仅注册 gift；没有任意来源入账 HTTP 接口。新来源必须沿用财务锁顺序，避免在持锁期间请求外部服务；持久发件箱仅作为跨库接入时需要另行实现的可靠交付协议，不在本期伪造“原子跨库”。

## 收款与未来统一出款中心

当前`DatabasePayeeProvider`在提现事务内锁定payout账户、认证解密并校验本人归属/状态/币种/资料版本，再将不可变脱敏快照与冻结流水、提现单和审计一起提交。不传RPC，不判断实名认证。旧payout_center_pending快照保持不变并阻止审核通过或实际出款；详见docs/payout。

未来出款中心接入真实收款提供方时，应从可信库/服务解析收款账户，保存稳定版本、脱敏账号、方式、资料变更时间和实时风险结果。是否要求收款身份匹配由出款中心的独立策略决定；领取审核单及实际转账前都不能直接沿用本阶段内部占位快照。

2026-11-02已以`(business_type=anchor_withdrawal,business_no=withdrawalNo)`永久幂等接收审核单，在同一事务重新核验冻结、审核、最新风险和资料版本，创建ready订单及历史/审计；不修改原财务事实。未来派发需另实现事务发件箱/领取协议、实际付款后唯一资金结果流水、失败/撤销/回调验签/重试与对账。历史approved和ready不能自动视为可付款，派发前仍须重新核验。具体见[订单运维与扩展](../payout/order-deployment.md)。
