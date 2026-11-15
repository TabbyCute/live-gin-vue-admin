# 收益历史核验扩展与后台审计（2026-11-12）

## 范围与信任边界

本轮将一次性加载全部历史、超过 20000 即拒绝的核验，改为完整历史 keyset 重放和有界批读，增加管理员后台审计队列。没有引入可被当作付款凭据的历史检查点，没有取消来源核验，没有更改钱款变动、root ID 1 自审例外、退款净额累计舍入、独立提现账户或付款终态。

**这不是 O(增量) 的在线核验。** 在线申请、审核和出款仍在原财务锁与事务内检查完整历史，总成本仍随历史增长，原 HTTP 8 秒预算保持不变。后台报告不能让超时账户跳过核验。真正的增量检查点问题仍未关闭：旧礼物原单在退款时会变化，其证明同时依赖钱包交易/流水/当前账户、退款、收入分成、事件；提现的审核和终态也会变化。仅签名余额/余数或缓存账户 version，无法发现全部旧来源损坏。缺少独立可信变更日志和完整依赖失效协议时，不能承诺既 O(增量)，又即时发现任意历史篡改。

## 核验实现

- 账本按 `(anchor_id,currency,version)` 已有唯一索引 keyset 读取，每页最多 128。首次读取不排除非正版本，防止损坏的 version=0/负值记录被遗漏。
- 页内批读结算、提现、审计日志、提现冻结/解冻/paid 流水、对账报告、出款事件、订单与成功 attempt。保留业务和审计一对一约束、终态证据匹配、四余额及版本链重放；用业务计数与重放计数相等检测缺少账本的业务事实。
- 多个结算组合为最多 1000 收入事实的批次，分别批读 item、allocation 和 income；逐结算独立校验 canonical hash、归属、币种、期间、金额。深核对这一整批调用可信来源适配器，避免每事实或每结算 N+1。
- 余数状态只保留最多 1000 个规则；跨页退款通过永久 allocation 和更早的已批准账户流水定位原结算 item，强制沿用原兑换率。没有从检查点跳过旧收入。
- 已通过的对账复用本次重放产生的收入/负向调整统计，省去第二次全历史读取。异常对账另做有界批量统计以保留完整账面金额，不把部分已核验金额标成总额。
- `gift.WithIncomeProofCache` 仅注入后台审计和只读 `CheckAccount` 的单个 repeatable-read 事务；资金写事务没有缓存。一次已完整核验的礼物证明可以覆盖同一快照中的原收入与多次退款，但每条 income 仍逐字段匹配可信事实。缓存最多 20000 个已验证事实，满后丢弃重读；这只是缓存容量，不是账户总历史限制。跨事务必定新建缓存。
- 保留每结算 1000 条、来源证明单批 20000 行及每币种 1000 规则保护。超大单一来源仍失败关闭，不通过增大上限掩盖异常。

## 后台作业与接口

新增 `anchor_earning_audit_job` 仅存操作报告，不进入财务凭据链。模型有请求幂等键、创建/取消管理员、排队/开始/结束时间、账户快照及摘要、进度、稳定失败代码和 `retryOf` 链接。

管理员界面：直播管理 → 收益深度核验，组件 `web/src/view/live/earning/audit/index.vue`。仅后台页面，无 H5 改动。

| 接口 | 请求 | 行为 |
|---|---|---|
| POST `/earning/audit/start` | `requestId,anchorNo,currency,retryOf` | 同键同参幂等；每主播最多一个活跃作业；失败/取消重试用新键并链接原作业 |
| GET `/earning/audit/list` | `page,pageSize,anchorNo,status,no` | `{page,pageSize,total,list}`，含历史快照和进度 |
| POST `/earning/audit/cancel` | `jobNo` | 原子取消排队/运行中作业；不改终态报告或资金 |

统一外层 `{code,data,msg}`，金额、版本、计数使用十进制字符串。新操作独立权限 `/earning/audit/start`、`/earning/audit/list`、`/earning/audit/cancel`，沿用活动管理员校验、root 例外和权限表。

`Service.RunAuditWorker(ctx)` 由服务主生命周期启动一个 goroutine；每 2 秒最多领取一个作业，以 CAS 防重复领取。单次执行最多 30 秒；财务查询在一个只读 RR 一致快照中完成，不持有主播财务写锁。进度经独立短事务写回，**连接池至少需要 2 个连接**。取消与完成使用条件更新，取消后的运行不能发布 passed。45 秒以上未结束的 running 被标为 `worker_interrupted`，进程重启不会遗留永久运行状态。失败重试从新快照重新开始；旧进度不作为续跑可信状态。

后台任务是时间有界执行，不保证任意规模账户都能在 30 秒完成。跨事务拼接“检查点”会丢失统一快照，因此没有为超时任务冒进实现无证明的断点续查。`passed` 仅表示该历史快照通过，任何后续交易仍要在自己的事务内重新核验。

Worker 已接入固定名 `earning_audit` 的进程内遥测。作业只存稳定错误代码，不存数据库错误、凭证、收款原文或源事件 payload。HTTP/Worker 启停与整体运维由主服务统一接入。

## 迁移与回滚

1. 用新二进制的 `earning-migrate -config <选中的后端配置> -audit-schema` 创建/校验作业表。此专用动作与其他迁移动作互斥，只做新增表/索引/检查约束，不访问旧财务数据、不重建旧财务触发器。
2. `earning-migrate -config <配置> -metadata-only` 增补新菜单和角色 888 权限。既有 `-migrate` 也包含作业表；已有系统仅启用本功能时优先专用 `-audit-schema`。
3. `earning-migrate -config <配置> -verify` 校验全收益结构及既有收入覆盖；无数据回填要求。新服务启动验证会要求新表存在。
4. 发布应用并确认主生命周期启动 worker。启动一笔已知账户作业，核对状态、进度、版本和取消功能。

旧应用可在线忽略新增表；DDL 不变更旧应用访问的财务表。回滚应用后保留作业表和历史报告，不删除事实；停旧 worker 后尚未完成作业可由以后新版本恢复为失败。UI/元数据菜单可保留到重新上线，也可由管理员收回权限。禁止把报告 passed 作为旧版 20000 限制的人工放行理由。未在本代理执行远程库操作或 SSH。

## 验证证据

本地 MySQL 8 Docker，专属 `127.0.0.1:13329/earning_test_scale`；测试 fixture 严格拒绝其他地址/库名。SQLite 用于完整现有服务回归与可控来源破坏测试。没有连接 prod。测试凭据不写在本文。

| 用例 | 结果 |
|---|---|
| `TestAuditScaleMySQL` | 26001 收入、131 账本版本，跨第128页退款；完整核验 92 个 ORM 查询、872.7ms；8 并发 worker 仅1个领取；损坏、context 取消、事务回滚均验证 |
| `TestAuditScaleHistory` | 同规模 SQLite 完整核验约485ms，逐条来源计数恰为26001 |
| `TestRealGiftAuditSecondPageTampering` | 通过真实 wallet/gift/earning 服务生成130礼物、130结算，再对第1礼物退款并结算；131版本完整可信来源核验2批、约16.7ms（SQLite）；篡改第130礼物 revenue 后在第2页拒绝 |
| `TestGiftAuditMemoizationIsSnapshotScoped` | 每个独立快照首次来源证明10查询；同礼物两个退款来源重复证明0额外查询；下一事务仍重新10查询；命中缓存时修改事实也拒绝 |
| `TestAuditJobCancelRetryAndStaleReport` | 权限、幂等、排队取消、失败/取消重试、旧 passed 报告不能掩盖新损坏 |
| `TestAuditRunningCancellationCannotPublishPassed` | 执行中取消战胜 worker 完成 CAS，未发布 passed |
| `TestAuditRejectsNonpositiveLedgerVersion` | version=0 的孤立流水不可绕过第一页检查 |
| `TestMySQLEarningAuditAdditiveMigration` | 迁移连续执行两遍，结构验证通过，旧账户流水数量不变 |

**26001 大历史 fixture 使用自定义可信来源适配器逐字段比对保存的期望事实，不是实际礼物全链吞吐。** 真实礼物链由单独的130礼物/退款用例覆盖。以下对照同样只比较账户核验核心，不能作为生产 QPS 承诺。

对当前 HEAD 旧 `verifyAccount` 做临时测试函数提取，与新算法在同一个20000事实 SQLite fixture 比较：

| 算法 | ORM查询 | 耗时 | 本次累计分配字节 |
|---|---:|---:|---:|
| 原全量装载 | 8 | 779.5ms | 194822720 |
| 新分批完整重放 | 67 | 446.6ms | 171535632 |

旧20001事实返回 ErrLimit，新实现通过。测量为一次本机样本；累计分配不是峰值常驻内存。新算法以更多小查询换取有界工作集和短 SQL 参数列表，不主张所有环境都更快。临时 baseline 测试文件已删除，生产没有保留旧核验代码。

运行方式：

```sh
cd server
go test ./service/earning ./service/gift ./router/earning ./initialize -count=1
# DSN 仅从本地安全环境提供；fixture 拒绝远端或其他库名。
go test ./service/earning -run 'TestAuditScaleMySQL|TestMySQLEarningAuditAdditiveMigration' -v -count=1
```

本轮有意义的安全边界是完整核验与有界资源消耗，而不是提前信任历史。后续若实现真正增量放款核验，需先定义所有可变来源依赖、不可伪造的变更世代、检查点签名/轮换/回滚检测、历史审计责任及停用流程，并单独评审。

最终补充：`go test -race ./service/earning -run 'TestAuditRunningCancellationCannotPublishPassed|TestAuditJobCancelRetryAndStaleReport|TestGiftAuditMemoizationIsSnapshotScoped'` 通过；既有9项admin earning测试通过（Node `--experimental-vm-modules`）。整组 earning/gift/router/initialize/earning-migrate 回归通过。统一生产增量工具与全库备份还原规程见 [migration.md](migration.md)。
