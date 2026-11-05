# 礼物账务与扩展契约

## 模块依赖与资金边界

`router/gift -> api/v1/gift -> service/gift -> model/gift` 经各层 enter.go 注册。

- live 提供主播/房间/场次状态与社交访问判定。gift 在同一数据库事务调用 live 的访问查询，不绕过私密、仅关注者或双向拉黑限制。
- wallet 是唯一用户 COIN 余额和扣款/退款内核，接口契约保持不变。
- gift 编排订单、退款、收入、权益补偿和事件，使用服务端快照。
- earning 当前提供整数分成计算、规则选择、待结算收益追加；规则配置/复核 HTTP 暂随礼物管理入口交付。2026-11-01已补齐独立[收益结算与提现审核](../earning/README.md)。

## 送礼原子事务

1. 用 JWT 用户ID锁定用户行；按 `(sender_user_id,request_id)` 查当前幂等订单。
2. 同参数重试核查既有礼物事实并返回原订单；参数改变返回48004。重试不依赖当前价格或当前场次仍开播。
3. 持目录共享锁，依次锁主播、房间、场次共享行，并复用 live/social 访问检查；主播用户ID与发送者相同则返回48012，禁止自送。
4. 检查分类/礼物启用、数量上限、`int64`乘法溢出；守护礼物还验证售卖开关、Provider 就绪、方案状态和有效期。
5. 读取当前有效、已双人复核的分成规则。
6. 服务端构造 `wallet.MovementInput`，调用 `DebitInTx(tx,...)`：AccountID=JWT用户，Amount=单价×件数，BusinessType固定gift，BusinessNo=服务端订单号，ActorType=user。
7. 同事务创建订单、正向收入、pending收益和pending广播事件。已启用的守护礼物还保存完整权益快照和 BenefitGrant，事务提交后由独立 Worker 履约。
8. 事务提交成功才返回成功。任意数据库写入或钱包错误均回滚。

统一锁顺序：用户排他锁→已有礼物订单→目录共享锁→主播共享锁→房间共享锁→场次共享锁→钱包内部用户/钱包/交易锁。配置与分成审核只持目录排他锁，不倒序获取用户/钱包。退款按用户→订单→钱包。同用户串行，不同用户无进程全局写锁；配置锁允许多个送礼事务并发读取。

外层READ COMMITTED。仅MySQL明确回滚的1213/1205有最多4次整事务尝试；网络/Commit未知结果不自动盲目重放。HTTP有8秒预算，客户端须保存原requestId，查询或原参数重试，不能生成新key来“确认”旧请求。

## 退款原子事务

仅启用管理员、当前数据库角色成员且具有 `/gift/refund POST` 授权可退款；APP没有退款路由。

用户锁和订单锁序列化该用户的退款/送礼；退款键全局唯一，并核验原操作者、原订单、数量、原因。退款金额始终取原单价，不读取当前价格。钱包通过 `RefundInTx` 关联原`wallet_transaction_no`，钱只退原用户COIN钱包。

钱包退款、礼物退款单、负向收入、负向收益、权益补偿、订单退款累计缓存和审计在同事务提交。原订单核心快照、原始收入/收益不被删除或重写。数据库CHECK和钱包内核同时限制超退。

分成使用整数万分比，`Share(x,bps) = (x/10000)*bps + ((x%10000)*bps)/10000`，向下取整并避免中间乘法溢出。每次收益冲减=`Share(退款前净额)-Share(退款后净额)`，所以多次部分退款与一次全退结果一致，不留下舍入收益。

2026-11-01起，结算后退款追加负向收益调整，不修改历史结算单；独立可提现账户不足部分记待抵扣欠额，具体实现见[收益账务契约](../earning/accounting.md)。

## 分成规则

范围优先级固定：anchor > gift > category > global；同优先级按priority DESC、version DESC、ID DESC稳定排序。同一scope_type+scope_no的active规则生效区间不能重叠，区间为`[effectiveFrom,effectiveTo)`。创建和审核在目录锁下进行；版本只增不改；审核人必须与创建人不同，包括超级管理员。

没有有效规则直接失败，不使用默认0、默认40%或字典值。创建及批准时验证范围对象存在；被规则引用的分类/礼物不能物理删除。提前终止通过anchor_rule_termination单独记录申请、原因与另一位管理员的复核；批准后停止匹配新订单，但不改写原规则或历史订单。新版本生效期不得与终止之前的旧有效期重叠。终止可能使新订单匹配较低优先级规则；没有其他规则则失败关闭。

## 事实统计

金额唯一来源是gift_order和gift_revenue_ledger。订单的refunded_coin_amount是由退款事务维护且可由退款事实重建的累计值。详情及退款前置检查共享逐笔核验：每笔订单/退款的wallet交易与流水编号、BusinessNo、原扣款、账户/币种/金额，收入业务编号/数量，收益规则/比例/主播/冲减编号及累计舍入结果。相同总额的关联错配也会失败关闭。使用固定数量的批量查询，不为每笔退款单独查库；完整钱包连续流水核查继续使用wallet账户核查接口。

SRS统计Hook即使接收旧giftCount/giftCoinAmount/giftUserCount也忽略这些字段。live_session旧列不再是金额来源，也不再序列化成HTTP财务数字。新的`giftSummary`来自订单/退款聚合；场次列表按页批量查询，避免N+1。

排行榜：按送礼时间选取订单集合，再扣除这些订单截至查询时的所有成功退款。日报、周报（周一开始）、月报统一UTC；自定义窗口最长93天。此为净消费榜，不是按退款发生日分桶的会计日报。全退订单保留订单数，净件数和有效送礼人数归零。六维为sender/anchor/room/session/gift/category，另有platform总览。聚合金额在MySQL以十进制字符串返回，可大于单笔int64；SQLite极大聚合溢出会报错，不能用于生产。

## 守护与广播

2026-11-04 已接入独立 [guard 模块](../guard/README.md)。守护接收者是 sender_user_id，关联 anchor_id。结构化 `GuardService` 接收完整不可变来源，按 grantNo/compensationNo 永久幂等履约；gift 不直接写 guard 表。`gift.guard-enabled` 默认 false，只有结构、方案、Provider、Worker 和事实核验就绪后才允许开启；普通礼物不依赖守护售卖开关。

真实权益退款生成唯一补偿记录并将原 grant 标记 compensating；Worker 按原来源只撤回未使用时长，支持先补偿后授予、部分退款、崩溃重试与租约回收。完整生命周期见守护文档。

生产统一入口 `ProcessBenefits` 采用 guard_task 永久任务与令牌隔离，60 秒租约、25 秒调用预算，失败退避，20 次后人工处理。历史 `ProcessBenefitCompensation` 仅保留兼容测试，不注册第二个生产 Worker。退款单的 benefitCompensationStatus 保留创建时快照，实时状态从补偿记录读取。

事件eventId固定等于订单号，永久唯一，发送者公开信息与礼物展示快照同事务写入，状态pending。当前没有IM调用或假delivered状态。将来Worker需在事务外投递、带eventId让接收端去重，支持至少一次传递、退避、死信告警和重放；在接入该Worker前不提供“标记已投递”按钮。

COIN 不直接代表法币；兑换、可提现账户、冻结和审核属于 [earning](../earning/README.md)，公司实际出款属于 [payout](../payout/README.md)。新 [ranking](../ranking/README.md) 按 Asia/Kuala_Lumpur 生成公开投影，不改旧后台 UTC 财务排行口径，不读取主播分成来算魅力榜。
