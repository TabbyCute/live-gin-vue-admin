# 待出款订单：迁移、运维和扩展

> 此文保留绑定/ready阶段记录；人工出款、paid、取消、权限与迁移以 [当前人工出款文档](manual-operations.md) 和 [前端契约](manual-frontend-integration.md) 为准。

## 迁移和发布

复用[绑定阶段密钥配置](deployment.md)，没有新增配置开关、第三方代付或自动成功路径。迁移不需要解密密钥，不初始化真实收款资料；运行新应用必须注入有效的三项PAYOUT密钥环境变量。启动同时检查绑定和订单引用的历史key，每类每key抽样认证；读详情、重放和创建仍验证目标密文。

```sh
cd server
go run ./cmd/payout-migrate -config config.prod.yaml
go run ./cmd/payout-migrate -config config.prod.yaml -migrate -sync-metadata -timeout 15m
go run ./cmd/payout-migrate -config config.prod.yaml -verify -timeout 15m
```

默认只读盘点；`-verify`只读校验表、索引、CHECK、触发器、菜单父子关系、API及角色888权限。`-sync-metadata`在财务控制行锁内幂等同步独立一级菜单和授权。root用户ID=1沿用服务端完整权限，普通角色不自动授权。元数据同步不调用真实出款，不写提现事实。维护预算默认5分钟，可用-timeout设置1—30分钟；不影响HTTP的8秒预算。已完整校验一致的结构跳过AutoMigrate，减少远程元数据往返及重复DDL锁。远程执行证据见[本阶段验证记录](order-verification-20261102.md)，无需再对该远程库手动迁移。

新增`payout_order`、`payout_order_request`、`payout_status_history`、`payout_audit_log`，保留现有三张收款账户表、提现数据和索引。以MySQL财务迁移互斥锁串行化结构更新，该锁内仍保留5分钟上限；总维护预算还覆盖前后数据盘点及只读核验。不兼容约束直接拒绝，不能删触发器绕过校验。快捷返回前同时验证列类型/长度/NULL/默认值、主键和CHECK精确表达式，不仅验证名称。MySQL DDL非整体事务，应用发布前应保存数据库与密钥可恢复备份；未来模型/CHECK扩展须同步定义校验和真实MySQL测试。

发布后端及后台静态资源后重新登录以刷新菜单；H5继续使用此前已完成的绑定选账代码。不能让旧后端接收缺少payeeAccountNo的新提现。数据库迁移不等于应用部署；本任务没有重启线上进程、注入真实密钥或提交真实订单。

## 资金和数据约束

创建事务：finance control共享锁 → anchor finance锁 → payout owner锁；读取最新提现，核验approved/passed/真实绑定/版本/币种/归属、冻结流水和余额、旧对账摘要、当前全链账务来源/退款/欠额/主播风险；锁后再次检查同来源订单。创建订单、ready历史、创建审计和幂等登记同事务完成。数据库错误、加密失败或取消全部回滚；没有余额、提现状态、审核结果写入。

永久唯一`(business_type,business_no)`保证一笔提现只有一笔主出款单。request_id也是唯一；额外request登记表永久保留所有曾接受的重试键，防止第二个键重放同单后又改投另一单。相同请求的恢复返回历史订单，不重新做停用后的付款授权；订单仍未派发。

订单资料先认证解密原绑定，再使用新随机nonce和`payout.order.v1`专属AAD加密。AAD绑定订单号、请求摘要、来源、主播/申请人、金额/币种、账户版本/脱敏值、创建核验、创建人/时间和载荷版本，不能直接复制绑定密文。订单保留独立加密快照和哈希，之后默认账户变化/停用不改变历史目的地。密钥轮换保留所有历史绑定与订单key，不能修改旧key内容、HMAC或原订单。

本期订单INSERT仅ready，UPDATE/DELETE全部拒绝；历史、审计和request登记禁止UPDATE/DELETE。CHECK预留六种将来状态，但没有状态迁移接口。初始历史目前按payout_no唯一，仅允许空状态→ready，审计仅order.create。未来状态机须以明确迁移改造这些限制、状态时间语义与校验器，不能只增加按钮或关闭触发器。后续业务类型也需要来源验证器、权限和CHECK迁移；本期只允许anchor_withdrawal。

触发器保护不代替数据库权限隔离：运行应用账号应限制DDL、TRUNCATE和触发器管理权限，结构迁移使用独立维护权限。具备DBA权限的人仍可改变保护机制，须由运维审计和备份恢复流程管理；本任务没有擅自变更现有数据库账号权限。

## 定位资金去向

关联链：APP requestId → withdrawalNo → payout_order.business_no → payoutNo → payeeAccountNo/version/不可变脱敏目标 → ready历史/创建审计traceId → 冻结流水及原对账。当前这条链证明“计划向哪个绑定支付”，没有“实际资金已到某账号”的支付凭证。

```sql
SELECT payout_no, business_no, anchor_no, amount, currency, payee_account_no,
       payee_version, payee_method, account_masked, payee_snapshot_hash,
       status, created_by, created_at
FROM payout_order WHERE business_type='anchor_withdrawal' AND business_no=?;
SELECT request_id, payout_no, created_by, created_at
FROM payout_order_request WHERE payout_no=?;
SELECT event_no, from_status, to_status, actor_id, created_at
FROM payout_status_history WHERE payout_no=? ORDER BY id;
SELECT event_no, action, actor_id, trace_id, snapshot, created_at
FROM payout_audit_log WHERE payout_no=? ORDER BY id;
```

仅授权运维使用；不查询/导出加密载荷、nonce、key ID、指纹或原文。应用成功日志关联traceId/payoutNo/withdrawalNo/payeeAccountNo/status；审计包含创建人和安全快照。失败日志仅固定类别business/storage/crypto_integrity和code，不输出原始SQL或请求body。后台错误提示固定，诊断使用响应X-Request-ID。网关/APM/WAF应同样禁记payout/earning请求内容及鉴权头。

监控49203、49103/crypto_integrity、49205、幂等冲突异常增长、429/503、SQL锁等待/死锁、ready长期积压、停用账户关联ready单、历史密钥缺失和当前核验失败。报警后按来源链核对；禁止改余额、删除/覆盖事实或换账户“修复”。正常创建不减少frozen、不增加paid、不把提现设为paid。

## 生产验收和后续边界

本机测试不是目标生产容量承诺。现有同步账本验证20000事实保护上限保留；高流水需检查点/分段核验，不能跳过验证。列表有组合索引、批量关联查询和分页上限；大历史数据COUNT/OFFSET、异常网络与多实例限流仍需生产基准。8秒预算、连接超时和有限重试保护单请求，不替代网关限流、HA、灾备及密钥恢复演练。

实际付款须后续单独实现：可信公司渠道/尝试记录、派发前实时核验、受控状态机、事务发件箱或可靠领取、外部幂等、回调验签/防重放、未知结果对账、取消失败语义及唯一资金结果流水。本阶段没有上述实现，也没有Mock成功、succeeded、paid、手工渠道流水号或frozen→paid。旧payout_center_pending单不补绑，approved取消解冻仍未实现。
