# 安全、迁移与运维

> 此文保留绑定/ready阶段记录；人工出款、paid、取消、权限与迁移以 [当前人工出款文档](manual-operations.md) 和 [前端契约](manual-frontend-integration.md) 为准。

本页说明绑定阶段的密钥及收款保护。当前代码还包含待出款订单，新增表、菜单同步和远程核验见[订单迁移与运维](order-deployment.md)。

## 密钥和部署次序

服务启动必须注入三个环境变量，不允许使用示例、测试密钥或仓库常量：

| 名称 | 内容 |
| --- | --- |
| PAYOUT_ACTIVE_KEY_ID | 当前写入的key ID，1—32位字母/数字/下划线/连字符 |
| PAYOUT_ENCRYPTION_KEYS | JSON对象：key ID → 标准Base64编码的32字节AES密钥，最多16把 |
| PAYOUT_FINGERPRINT_KEY | 独立的随机32字节HMAC密钥，标准Base64，与AES密钥不可相同 |

由部署环境的secret manager生成、注入和备份真实密钥；不写入config.prod.yaml、数据库系统配置、SQL、审计、前端、命令行参数或本仓库。不在聊天或终端输出真实密钥。本次没有在远程DB生成任何绑定或测试资金，也没有创建代付能力。环境变量是进程启动时的不可变依赖，不支持后台热修改密钥。

AES-256-GCM每次写入生成随机nonce；`encrypted_payload`保存nonce+ciphertext+认证标签的Base64。AAD包含业务编号、owner、方式、币种、资料版本、key ID、指纹、脱敏值、创建时间、创建幂等键与摘要，防止密文跨行互换或不可变元数据被替换。HMAC防重比裸SHA256更能抵御低熵账号字典枚举，密钥必须与加密密钥分离。

启动校验密钥格式、表/索引/CHECK/触发器，以及每个历史key ID的一份认证密文与稳定HMAC指纹。缺失旧key、错误AES或HMAC配置会拒绝启动；每次新提现和审核仍验证目标完整密文，不把启动抽样当成全库健康保证。绑定接口随机数或加密失败直接回滚，绝不降级明文。

轮换：新增key ID并保留全部历史解密key，将active切到新key，重启后新账户使用新key。旧账户/提现事实不重写；不能直接替换已有key ID的内容或单独更换fingerprint密钥。批量重加密、历史资料失效和HMAC轮换需要后续专门的迁移、双版本索引与复核方案，本次未开放在线接口。丢失密钥无法恢复收款资料；备份恢复应同时演练DB和secret版本。

发布次序：执行本模块远程迁移 → 核验 → 在运行环境注入密钥 → 同步发布新后端与H5/后台 → 重启后确认健康。缺失密钥的新版本会失败关闭；数据库迁移完成不等于运行进程已更新。不要让旧版继续接收无payeeAccountNo的新提现写请求。已有approved占位单保持冻结，不自动转成可出款单。

## 可重入迁移

从server目录运行（本次远程执行记录见验证文档，用户无需再手工迁移当前远程库）：

```sh
go run ./cmd/payout-migrate -config config.prod.yaml
go run ./cmd/payout-migrate -config config.prod.yaml -migrate -sync-metadata
go run ./cmd/payout-migrate -config config.prod.yaml -verify
```

也可用PAYOUT_MIGRATION_DSN环境变量，和-config互斥。默认只读，-verify不能与-migrate或-sync-metadata组合。CLI不启动HTTP、不创建测试账户/密钥、不调用wallet/gift迁移、不删除或重写历史提现。绑定阶段三表及提现表idx_withdrawal_payee_ref索引保留，订单阶段新增四表；使用现有财务迁移互斥锁。MySQL8.0.16+、InnoDB、utf8mb4_bin、严格模式；启动只读验证保护约束，不兼容即失败。

所有绑定禁止DELETE；只允许active状态下修改默认/停用及时间字段，密文、owner、编号、指纹和资料版本不可原地修改；所有payout审计禁止UPDATE/DELETE。单条永久owner锁串行化新增/默认/停用与提现选账。提现和订单创建按finance control → anchor finance lock → payout owner lock顺序加锁；绑定操作不反向获取财务锁。死锁/锁等待有限重试，继承8秒请求预算。

MySQL DDL不能整体事务回滚；迁移发现不兼容时不能删表重建。生产仍须完成真实备份、恢复与迁移失败恢复演练。业务表数据不会被迁移修复或补写，CLI输出财务表前后计数和SHA256摘要，仅用于核对，不输出账务/收款原文；有并行业务写入时摘要变化需核对业务事实。

## 去向排查与审计

本阶段资金只有available↔frozen/debt状态变化，没有实际资金转账目的地执行记录。`payeeRef`表示申请选定的绑定，不能把它解释成钱已经转到该账户。

排查链：APP requestId → withdrawalNo → payeeRef/payeeVersion/申请脱敏快照 → payout_payee_account当前状态 → payout_payee_audit → anchor_withdrawable_ledger冻结/释放 → anchor_finance_audit → anchor_reconciliation。后台提现列表支持payeeAccountNo精确筛选，详情同时展示历史快照与当前状态；普通管理员继续沿用earning精确权限校验。

授权运维可只读以下安全字段，严禁导出encrypted_payload、密钥、指纹或明文材料：

```sql
SELECT withdrawal_no, request_id, anchor_no, currency, amount, payee_ref,
       payee_version, payee_method, account_masked, status, reconciliation_status
FROM anchor_withdrawal_order WHERE payee_ref = ? ORDER BY id DESC LIMIT 100;
SELECT payee_account_no, owner_no, method, currency, masked_account,
       status, version, default_slot, created_at, updated_at, disabled_at
FROM payout_payee_account WHERE payee_account_no = ?;
SELECT event_no, action, trace_id, actor_id, snapshot, created_at
FROM payout_payee_audit WHERE payee_account_no = ? ORDER BY id DESC LIMIT 100;
SELECT ledger_no, entry_type, change_amount, available_before, available_after,
       frozen_before, frozen_after, debt_before, debt_after, created_at
FROM anchor_withdrawable_ledger WHERE business_no = ? ORDER BY id;
```

收款API提供服务端X-Request-ID，并在账户审计保存traceId；新增/默认/停用的成功日志只打印操作路径、traceId、公开业务编号、状态。提现申请成功日志关联requestId/withdrawalNo/payeeAccountNo/version；错误仅打印固定错误类别与traceId，不输出底层SQL错误或请求资料。异常恢复和开发access日志对payout/earning过滤headers、query、body和panic值，通用操作日志也增补收款字段脱敏。网关、APM、WAF、浏览器埋点仍必须在目标环境配置相同排除规则；应用代码无法约束外部代理的日志。

关注：49103/crypto_integrity、49003/49009/49012、49104异常增长、429/503、DB死锁与锁等待、长时间pending_review/approved冻结、停用账户关联未出款单、缺失历史key。不要通过改status、换payeeRef、删流水或改余额“修复”。当前pending_review可经账务验证后驳回释放，approved没有取消状态机；维持阻止直至后续功能实现。

## 扩展边界

- 新业务owner_type、新渠道或敏感资料版本需要独立授权规则、验证器与数据库迁移；不得复用主播接口传ownerNo。
- payout_order已独立建模，以withdrawalNo为来源引用，保存专属加密的不可变账户版本；公司出款账户和实际支付产品仍属后续范围。
- 实际派发前再次锁定/核验收款账户状态及版本，阻止在审核后被停用的绑定；当前approved仅代表历史审核通过，不能当作永久支付授权。
- 原有20000事实同步财务核验上限仍在。当前有限数据负载测试不能替代历史大账本检查点、多实例限流、HA/网络分区、RPO/RTO或备份恢复验收。
