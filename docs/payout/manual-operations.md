# 人工出款运维与资金追踪

本阶段实现人工登记及独立核验、内部资金闭环；不接银行API、第三方回调/查询/对账单，不执行自动转账，没有生产Mock成功接口。测试使用隔离库模拟银行结果，不能用内部测试证明真实银行卡到账。财务须在线下真实转账并核验银行/收款方记录；系统保存其不可变证据链。

## 启动与发布

保留现有 `PAYOUT_ACTIVE_KEY_ID`、`PAYOUT_ENCRYPTION_KEYS`、`PAYOUT_FINGERPRINT_KEY`。新增 `PAYOUT_EVIDENCE_DIR`，必须是显式绝对路径、应用工作目录和公开静态上传目录之外，目录权限0700、运行用户独占，文件0600。不得通过Nginx、CDN、对象存储公共桶或应用StaticFS发布该目录。进程通过os.Root限制路径边界，用随机内部对象编号、AES-GCM和包含业务/凭证元数据的AAD保存原始凭证，数据库仅保存元数据。未配置存储时禁止开始人工出款；已有凭证却缺存储/历史解密key时启动失败。生产多实例必须使用支持同一私有文件集合和正确持久化语义的共享部署，或将EvidenceStore按当前安全契约替换为受控对象存储，不能用各实例独立临时盘。

启动检查绑定、订单、公司账户和凭证使用的历史key，并抽样认证；业务读取仍逐一校验密文和SHA-256。密钥轮换只改变新记录的active key，保留所有历史key和稳定HMAC密钥；不能原地替换同key ID的内容。备份须同时覆盖DB、加密凭证文件、密钥（密钥与数据备份分离），定期在隔离环境做恢复和对账演练。

维护窗口先停止相关财务写入、保存可恢复备份，再从server目录运行：

```sh
go run ./cmd/payout-migrate -config config.prod.yaml -migrate -sync-metadata -backup-dir /secure/private/payout-backups -timeout 30m
go run ./cmd/payout-migrate -config config.prod.yaml -verify -timeout 15m
```

`-backup-dir`为必填绝对私有目录（0700）。CLI先以只读一致性快照写入0600 JSONL（受影响表DDL/行/触发器），文件完成并fsync后才开始DDL；备份失败不会启动迁移。快照保留在受控运维机器，不提交Git，不替代全库/PITR及密钥和证据文件备份。输出仅安全计数/历史财务摘要；新增字段按历史0/1/空值规范化摘要以便比较。全程1—30分钟显式预算，单条网络读写60秒，迁移命名锁串行；最终必须独立verify并再次核验历史摘要。

迁移增加paid前后值（历史默认为0，不猜测历史付款）、订单状态版本和连续历史、公司账户/attempt/私有凭证/一次性能力/幂等/调查/终态/对账等表。只初始化manual_bank能力，不生成虚假公司账户、流水或付款事实。识别并替换已知旧CHECK/触发器；未知保护拒绝迁移。MySQL DDL独立提交，可重复执行并必须完成最终结构核验。SQLite使用DDL事务暂时移除经过精确匹配的跨表保护、重建约束，再恢复全部保护后提交。

先完成数据库，再同时发布后端、管理后台和H5，重新登录刷新菜单。旧应用只支持paid=0，不能在新应用已经产生paid后回滚旧代码；如需紧急回滚先停止财务写入，保留所有事实与证据，只回滚到理解新状态/paid链的版本。不得回滚删列/删凭证、改余额或降级保护。运行账号不应拥有DDL/TRUNCATE/触发器管理权限；具备DBA权限的人仍可移除保护，须由权限隔离、维护审计和备份约束。

## 人工工作规程

1. 公司真实账户资料经组织流程核验后创建；平台不保存网银密码、验证码或U盾资料。公司账户启用只表示允许平台选用，不表示银行验证通过。
2. 领取ready订单，核对固定金额/币种/脱敏目标；单次高权限查看明文，在线下银行操作。渠道限额是UTC日累计领取风控额度，不是银行余额，释放/失败不会退还当日额度。
3. 银行已接受请求立即标记bank-submitted；未知真实状态时禁止释放、取消或再次支付。等待银行产生真实记录，不用UUID或虚构回单填空。
4. 上传原始PNG/JPEG凭证、填写真实流水号/支付时间，提交pending_confirmation；由不同管理员查看回单和银行结果后确认。ID1的同人例外有明确审计，正常财务职责分离不要依赖该例外。
5. unknown只能追加调查；调查人上传新的独立证据、创建本人调查记录，再由该独立人员显式解决成功/失败。失败继续冻结，重试生成新attempt。取消仅无付款的ready/failed，解冻优先还debt。
6. 成功/取消同事务写不可变终态事实和内部对账。`payout_terminal_event`是内部完成事实，未实现对外消息投递器，后续消费者应按eventNo永久去重并独立记录消费游标。

## 每笔钱的链路

`wallet_transaction / wallet_ledger → gift_order → gift_revenue_ledger / anchor_earning_ledger → anchor_income_fact → anchor_settlement_item/allocation → anchor_settlement → anchor_withdrawable_ledger(settlement_credit) → withdrawal_freeze → anchor_withdrawal_order → payout_order → payout_attempt → company account + immutable payee snapshot → bank reference + private evidence → reviewer/history/audit → withdrawal_paid → payout_terminal_event → payout_reconciliation`。

付款前后账户版本链记录available/frozen/debt/paid四项；成功frozen减少与paid增加恰好等于提现金额。每笔提现最多一个主出款单，每单最多一个活动尝试、一个成功尝试，同公司账户流水指纹永久唯一。失败尝试、取消单、旧收款快照均不删除、不覆盖。

常用安全查询（仅授权运维，以绑定参数执行）：

```sql
SELECT payout_no,business_no,anchor_no,amount,currency,status,state_version,
       payee_account_no,account_masked,payee_snapshot_hash,last_transition_no
FROM payout_order WHERE business_no=?;
SELECT attempt_no,sequence,channel_account_no,company_masked,amount,currency,
       status,operator_id,reviewer_id,payment_reference,payment_time,
       evidence_reference,evidence_hash,failure_code,created_at,completed_at
FROM payout_attempt WHERE payout_no=? ORDER BY sequence;
SELECT ledger_no,entry_type,version,change_amount,available_before,available_after,
       frozen_before,frozen_after,paid_before,paid_after,debt_before,debt_after
FROM anchor_withdrawable_ledger WHERE business_no=? ORDER BY version;
SELECT event_no,sequence,attempt_no,from_status,to_status,actor_id,request_id,
       reason,created_at FROM payout_status_history WHERE payout_no=? ORDER BY sequence;
SELECT event_no,action,object_no,transition_no,actor_id,trace_id,created_at
FROM payout_audit_log WHERE payout_no=? ORDER BY id;
SELECT * FROM payout_terminal_event WHERE payout_no=?;
SELECT reconciliation_no,status,snapshot_hash,created_by,created_at
FROM payout_reconciliation WHERE payout_no=? ORDER BY created_at DESC;
```

支付流水只在授权后台/运维查询使用，不打印到一般访问日志。查实物凭证走独立evidence/read接口，每次业务查看留审计，不将私有目录映射成公开下载。一般日志仅打印服务端traceId、内部业务编号、状态/错误类别；SQL日志使用参数化输出。网关/APM/WAF/错误收集同样禁止记录鉴权头、请求正文、敏感响应和原始查询串。自由文本说明提示不得填敏感资料，常见号码/邮箱/URL脱敏不能取代操作者培训和审计访问控制。

## 性能、容量及故障处理

账户锁顺序：finance control共享锁 → 主播财务锁 → 出款订单 → 尝试 → 公司账户/日额度或查看凭证。成功与取消都复用earning唯一post写账，事务任何一步失败回滚。网络结果不明先按原请求恢复，不能换流水/换请求制造新付款。MySQL死锁/锁等待按原事务有限重试，数据库唯一键和CAS是最终防线；进程内限流不是资金互斥锁。

上限：每主播历史绑定100、公司账户1000、每订单尝试100、每尝试凭证10、每订单调查1000；普通列表pageSize≤100，深度核验沿用earning同步事实数量保护。长期高流水需要经过验证的检查点、分段对账和归档索引，不能删除历史事实或关闭核验。当前同步核验不是无限规模账本方案。列表Count/OFFSET和文件读取在真实数据量、网络与存储下仍需容量基线。

告警建议：unknown或processing超时、pending_confirmation积压、49203/49205/49209增长、非预期幂等冲突、429/503、数据库死锁/锁等待、磁盘空间/IO错误、备份失败、历史key缺失、文件哈希或AAD失败、额度耗尽、root同人复核。失败日志不附原始SQL/敏感错误，按traceId与审计再定位。定期经授权批次运行内部对账；它不是银行自动对账。

文件内容与目录项均fsync成功后才提交元数据；正常失败且确认对象未被DB引用时清理，提交结果不明确/数据库不可用时保留文件以免删掉已提交凭证。崩溃可能留下加密孤儿文件，维护时在停止上传后按DB object_reference核对，先备份并隔离；不得自动删除任何已引用对象。原证据被篡改/丢失会阻止提交、复核或使对账异常；从受信备份恢复相同字节，不“重传覆盖”历史事实。

远程结构执行情况和测试实测数据另见本次验证记录；代码实现、结构迁移、应用发布和真实银行付款是不同验收项。

## 权限落地与发布验收

迁移注册全部26项出款接口（此前3项 + 本次23项）和3个菜单，只保留角色888原来3项ready权限，不给普通角色自动增加敏感查看或付款复核权限。ID1沿用项目超级root规则。普通财务须经组织批准，以受控数据库变更维护`anchor_finance_permission(authority_id,path,method)`精确记录（路径前缀`/payout/`）；菜单/Casbin授权不能替代此表。目前没有新增金融权限管理页面，不可将页面可见误作授权已完成。领取/提交角色与复核/调查角色应分别配置，并同时具备必要的order/detail、channel-account/list、evidence/read查询能力。每次授权变更记录工单、操作者和变更前后值。

应用发布验收：配置并持久保存加密密钥、显式私有目录和独占运行身份；部署新版后端/admin/H5；确认网关不记录财务请求正文/敏感响应且私有目录没有公开路由；用获批真实公司账户和真实银行记录完成人工操作验收。最后一步由财务执行，自动化脚本没有真实转账功能。真实转账已发生时先保留证据，记录不明确结果，禁止通过重试来猜测到账。
