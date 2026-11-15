# 统一后端增量迁移与还原演练

日期：2026-11-12。命令：`server/cmd/backend-migrate`。开发与恢复演练只使用本地 Docker MySQL 临时库；生产增量迁移由主代理执行并已完成，未部署应用。

## 允许的变更

命令只调用 `MigrateEarningAudit`、`MigrateEventDelivery`、`MigrateBankReconcile`，以及明确选择时的三个新模块元数据同步。不会调用旧 wallet/gift/earning/payout 全量迁移或财务回填。

- 新增收益审计1表、事件投递5表、银行核对8表及对应索引/检查约束/不可变保护。
- 银行迁移另外在已有 `payout_attempt` 上新增 `idx_bank_attempt_account_status_time(channel_account_no,status,payment_time)`，使用 MySQL `ALGORITHM=INPLACE, LOCK=NONE`；不改业务行。
- 元数据仅新增新模块菜单/API；收益权限仅允许角色888新增 `/earning/audit/start`、`/earning/audit/cancel` 的 POST 和 `/earning/audit/list` 的 GET。所有原有权限行的 ID 和内容必须原样保留；银行普通角色财务权限不自动扩张。

## 操作顺序

在 `server/` 构建工具：

```sh
go build -o /tmp/backend-migrate ./cmd/backend-migrate
```

使用当前选中的服务 YAML；命令没有 DSN 环境变量覆盖，也不接受命令行密码。默认即为只读结构核验：

```sh
/tmp/backend-migrate -config /absolute/path/to/selected-config.yaml
```

新表尚未存在时，只读核验以非零退出，不会偷偷补表。执行写入必须提供绝对路径、权限恰为0700且自身不是符号链接的目录：

```sh
mkdir -p /absolute/private/backend-backups
chmod 700 /absolute/private/backend-backups
/tmp/backend-migrate -config /absolute/path/to/selected-config.yaml \
  -migrate -metadata -backup-dir /absolute/private/backend-backups \
  -timeout 15m -read-timeout 2m -progress
/tmp/backend-migrate -config /absolute/path/to/selected-config.yaml -verify
```

`-verify` 与写入动作互斥。`-migrate` 和 `-metadata` 可分开执行，每一次写入调用都必须重新生成所选范围的完整备份。`-timeout` 默认15分钟，只允许1至15分钟；`-read-timeout` 默认2分钟，只允许1秒至2分钟，单次网络读取仍受整体命令截止时间限制（含初始连接版本探测）。DDL 锁预算不随网络读取预算放宽。

工具使用同库专用 MySQL advisory lock（最多等待5秒）串行本工具；银行迁移还沿用既有财务迁移命名锁。会话 `lock_wait_timeout` 和 `innodb_lock_wait_timeout` 为5秒，银行在线索引 DDL 单独10秒；命令有整体截止时间。库连接上限2，避免已固定连接嵌套 `gorm.Connection` 导致错误。退出时释放命名锁，释放失败淘汰物理连接。

**不要同时运行其他 DDL 或财务写入。** Advisory lock 不会停业务服务；前后资金摘要变化会让工具非零退出，但已经提交的 DDL 无法回滚成事务原子操作。生产应选择无资金写入的窗口，或由授权运维控制写入。不要因为“业务并发可能导致摘要变化”而忽略失败。

## 写前备份与保真

`-backup-scope` 默认 `full`，保持以下全库备份行为不变。显式选择 `-backup-scope financial-and-metadata` 时，备份是**迁移相关范围备份，不是全库灾备**：

- 固定纳入所有 `wallet_`、`anchor_`、`payout_` 前缀表、`event_delivery` 及其前缀模块表，另含 `gift_order`、`gift_refund_order`、`gift_revenue_ledger` 三张可信来源表和 `sys_base_menus`、`sys_authority_menus`、`sys_apis` 三张元数据表。
- 在同一 RR 快照内递归纳入选中表的同库 FK 父表。CLI 将迁移前 proof 的全部实际财务表名作为强制覆盖检查；不能传自定义 allowlist/skip 表，也不会遇错跳过。
- 选中表或 FK 父表若非 InnoDB base table、包含空间类型、跨库 FK 或任何读取失败，均在写前失败关闭。只读取全库表目录的名称/类型/引擎；不会对范围外无关表读取 SHOW CREATE、COLUMNS、行或 trigger。
- 保留全部选中表的 trigger，保守支持现有金融保护 trigger 子集：按原 SQL_MODE 识别字面量/引号；FROM/JOIN 依赖必须在备份内。范围外/跨库表、CALL、动态 SQL、DML、未知函数、注释或无法明确分析的语法均阻断，不声称支持任意自定义 trigger 或整个应用依赖图。
- 最终 JSON 的 `backupScope`、`backupTables` 和 SQL 头记录范围。范围备份仍覆盖本次已存在的全部修改对象：`payout_attempt` 索引、旧金融及权限表、已有新模块表和菜单/API 元数据；不存在的新表没有旧行可备份，创建后仍检查为空。

本次远端全库尝试在现有无关公告表 `gva_announcements_info` 的结构读取阶段超时，均停在写前。**该读取障碍没有被修复**。显式范围选项避免读取不受本次迁移影响的该表；若它成为选中表的 FK 父依赖，仍必须纳入并成功读取，不会特殊跳过它。

```sh
/tmp/backend-migrate -config /absolute/path/to/selected-config.yaml \
  -migrate -metadata -backup-dir /absolute/private/backend-backups \
  -backup-scope financial-and-metadata -timeout 15m -read-timeout 2m -progress
```

`BackupAll` 在任何业务写入/DDL之前，以只读 RR 快照导出全业务库的 base tables、DDL、全部行与触发器；文件0600，位于0700目录。文件和目录均 fsync，计算 SHA-256 后，才允许迁移。失败的半成品文件删除。

当前备份只支持 InnoDB base tables 和触发器。遇到非 InnoDB 表、视图、routine、event、跨库外键或空间类型会在 `backup` 阶段失败关闭；不会声称已备份自己不支持的对象。此 SQL 备份不包含数据库用户、授权或服务 YAML/密钥，需要另行安全保管配置与访问权限。源 trigger 的 definer 不写入还原脚本，恢复时由恢复账户创建；body、时机、动作和 SQL_MODE 保留。

导出按列类型处理：binary/varbinary/bit/BLOB 使用裸十六进制保留任意字节；DECIMAL、文本和 JSON 显式 UTF-8 字符转换。**不能将所有 `[]byte` 都导出成裸 `X'…'`：MySQL 在 DECIMAL 上可能按十六进制数解释而改变金额。** 还原演练已经捕获并修正该问题。

生成列不重复 INSERT，DEFAULT_GENERATED 的可写列仍被完整备份；UTC 保留 TIMESTAMP 时刻；SQL_MODE 保留0自增ID和零日期；表DDL还保留源 ANSI_QUOTES/NO_BACKSLASH_ESCAPES 语义；每个触发器用其原 SQL_MODE 创建。SQL 不包含 `DROP`、`USE`、密码或账户授权，只允许还原到新建隔离库。

## 资金和权限验证

`Proof` 对 wallet_/payout_/anchor_ 及礼物原单、退款、收入流水读取有序全行摘要，按真实 PRIMARY KEY（含复合键）排序，不依赖第一列或偶然执行计划。原有金融表逐表比较行数和 SHA-256；任何遗漏/删除/变动都失败。

迁移前、迁移后分别通过 `ProofInSnapshot` 在**同一 RR 只读事务内**同时采集财务摘要和权限明细，再进行判断。避免“摘要与权限明细来自不同快照，整表豁免后漏掉并发改权”的间隙。权限原行逐条保持，新行严格只允许上述三项。

新收益审计和 payout_bank 表第一次出现时必须为空；既有表即使是这些模块，也必须保持原数据摘要相同。事件投递新表也检查为空。不会对整个 before/after map 简单 DeepEqual，从而不会把预期的新空表误判成金融数据变化，也不会允许借“新增表”放入测试记录。

输出仅有结构化 JSON：成功/失败阶段、迁移/核验/元数据状态、私有备份路径和SHA、保护表数量、新增表名及资金未变标志；不输出 DSN、主机地址、配置值、SQL错误原文或行内容。任一失败均非零退出。可选 `-progress` 仅在 stderr 输出 JSONL 阶段名、备份子阶段和经 ASCII 标识符校验的表名/序号；stdout 始终只有最终 JSON。备份进度区分 `table_schema`、`table_rows`、`table_complete`，便于识别长网络等待的位置。错误摘要的 `failureCode` 仅为 `timeout`、`canceled`、`mysql_数字` 或省略；不会输出底层错误文本。若解锁清理同时失败，保留最初的 `stage` 和主错误，额外设置 `cleanupFailed` 及安全 `cleanupFailureCode`，并淘汰连接；只有没有主错误时才使用 `migration_lock_release` 阶段。最后备份步骤和表标识保留在失败摘要中，半备份仍删除。驱动原始诊断关闭，`BackupError`/阶段错误仅通过 Unwrap 保留可分类原因，Error() 本身始终安全。`applicationDeployed` 恒为false：数据库迁移不是应用发布或真实银行验收。

## 本地验收证据

`TestMySQLUnifiedMigrationAndFullRestore` 严格限制 `tcp / 127.0.0.1:13329 / earning_test_scale` 为控制连接，再创建唯一 `backend_test_*` 源库和新还原库；测试退出删除自己创建的两库，不清空共享库。

已验证：

1. 原始基础表上默认只读核验失败且没有任何新增表。
2. 不支持的 VIEW 导致备份失败，未执行后续DDL；命名锁竞争最多5秒后失败且没有备份/变更。
3. 两次增量迁移与元数据同步成功：首次新增14个空表，第二次无新表；随后默认 `-config` 入口只读核验成功，输出无地址/密码/行内容。
4. 在摘要完成、权限明细读取前，从另一连接修改旧权限：同一快照仍一致看到原权限；下一快照正确检测修改并拒绝。没有对权限表笼统放行。
5. 在备份开始读取第一张表时，控制连接主动断开**本次新建临时库**的连接：备份与清理均失败，报告仍保留 `stage=backup`、`cleanupFailed=true`、最后备份表和步骤；半文件删除，后续DDL没有执行。随后数据库连接恢复，迁移可以继续。
6. `-progress` 实际入口输出逐表 schema/rows 进度，stdout 仍为单一 JSON；双流均不含地址、密码、SQL或样本行。超时/取消/MySQL数字错误分类及主错误保留有独立单测。
7. 范围备份演练保留 full 还原用例：全库模式遇模拟不可读公告表失败；范围模式未访问该表。选中财务表不可读、漏覆盖 proof 表、trigger 引用范围外表均阻断。26 张范围表还原成功，包括16张 proof 表、gift可信来源、三张元数据表、event模块和两层 FK 父依赖；FK及不可变trigger继续生效。
8. 只读本地 `tcp / 127.0.0.1:13328 / earning_test_delivery` 完整金融库，生成范围备份并还原到新 `backend_scope_real_*` 临时库：45个财务表完整 proof、83个真实金融保护 trigger 的 body/SQL_MODE 完全一致；来源库前后摘要不变。不在来源库做插入、更新、删除或结构修改。
9. 将写前全库 SQL 用 Docker mysql 客户端还原到全新库，三个测试财务表的完整摘要一致；二进制/BLOB、DECIMAL(30,8)、JSON、latin1/中文/emoji、显式ID=0、生成列、微秒TIMESTAMP、默认时间与zero DATETIME均保真；不可变trigger继续拒绝修改，trigger body与SQL_MODE一致。

测试命令（DSN只从本地受控环境注入，勿将测试指向生产）：

最终 CLI/backup `-race` 结果：`internal/backendcli` 16.488s（full+scope演练10.34s、完整真实trigger范围还原0.92s），`internal/engagementcli` 6.154s，`cmd/backend-migrate` 编译通过。日志位于本地 `/tmp/backend-scope-race-20261112.log`。完整金融来源测试必须与会重建 `earning_test_delivery` 的业务测试串行，避免把并行fixture变化误判成备份问题。

```sh
cd server
go test ./internal/backendcli ./internal/engagementcli ./cmd/backend-migrate -count=1
# BACKEND_MIGRATION_TEST_DSN 设置为上述专用本地连接后：
go test -race ./internal/backendcli -run TestMySQLUnifiedMigrationAndFullRestore -v -count=1
# BACKEND_SCOPE_SOURCE_TEST_DSN 为上述13328只读来源连接时：
go test -race ./internal/backendcli -run TestMySQLScopedBackupRealFinancialTriggers -v -count=1
```

## 失败与回滚

保留 JSON 摘要、备份文件及 SHA。非零退出后先确认失败阶段和已有部分 DDL，不要盲目反复执行。修正已知结构/权限条件后允许幂等重跑；新写入动作仍重新备份。

应用回滚时保留新增表与审计历史，旧版本可忽略新表。不得用写前备份覆盖仍有业务写入的生产库，也不得删改旧财务流水。恢复只在独立新库完成并通过摘要/结构/触发器验证，再由授权运维决定切换。工具不自动部署、切库、删除生产数据或恢复生产资金。

## 本次生产结果

最终显式范围迁移成功，47张表备份、14张新表及必要索引/元数据完成；迁移前44张受保护财务表的旧资金与既有权限检查通过，清理锁成功。新增权限仅限既定三项root收益审计权限。另一次独立`-verify`通过，53张受保护表摘要前后一致，无写入。报告分别位于[evidence/prod-migration.json](evidence/prod-migration.json)与[evidence/prod-verification.json](evidence/prod-verification.json)。备份0600/目录0700、独立SHA与迁移报告一致，具体私有位置见[evidence/prod-backup-integrity.json](evidence/prod-backup-integrity.json)。范围备份不是全库灾备，公告表读取障碍仍未解决。
