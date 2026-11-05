# 守护与榜单发布、备份和迁移

运行 MySQL 8 / InnoDB / utf8mb4_bin，CHECK 必须实际启用。SQLite 仅用于快测。迁移只创建/核验 guard/ranking 表并同步指定管理元数据；不修改历史钱包、收益、提现和出款记录。

## 顺序

1. 本地测试、构建、兼容代码准备完成。
2. 发布兼容版本。新模块表全部不存在且守护售卖关闭时，普通礼物继续工作，新接口 503；部分迁移或已开启守护但缺表则失败关闭。
3. 只读检查实际目标；确认脱敏主机指纹、库名和 MySQL 版本。先备份受影响表结构、触发器和数据。
4. 运行 guard、ranking 离线迁移，再同步菜单/API/角色权限。严格核验新表、列、索引、CHECK、触发器。
5. 启动榜单 Worker，分批回填历史并核验。CLI 可在未发布应用时完成有界回填，但不代表线上 Worker 已启动。
6. 发布后重启服务以装配守护授予、补偿和到期 Worker；核对任务进度、普通礼物和守护 Provider。
7. 运营创建方案并核验权益配置；最后开启 `gift.guard-enabled`，再启用具体贵重礼物的守护附赠。

没有完成第 6 步，不开启守护。关闭销售只禁止新的守护礼物订单；已有发放、退款补偿和到期 Worker 继续运行。

```yaml
gift:
  guard-enabled: false
ranking:
  timezone: Asia/Kuala_Lumpur
```

当前进程不支持热换数据库连接或守护开关；使用重启发布，避免关闭 Worker 正在使用的连接池。

## CLI

从 server 目录运行。`-config` 与 `GUARD_MIGRATION_DSN` / `RANKING_MIGRATION_DSN` 二选一；不把密码放到命令行。默认操作只读，输出目标主机指纹、库名、版本、现有表和财务行摘要，不输出记录原文。

```bash
go run ./cmd/guard-migrate -config config.prod.yaml
go run ./cmd/ranking-migrate -config config.prod.yaml
# BACKUP_DIR 是仓库外的私有绝对路径；目录 0700，SQL 文件 0600。
go run ./cmd/guard-migrate -config config.prod.yaml -migrate -backup-dir "$BACKUP_DIR"
go run ./cmd/ranking-migrate -config config.prod.yaml -migrate -metadata -backup-dir "$BACKUP_DIR"
go run ./cmd/guard-migrate -config config.prod.yaml -verify
go run ./cmd/ranking-migrate -config config.prod.yaml -verify
go run ./cmd/ranking-migrate -config config.prod.yaml -backfill -batch 20 -max-batches 1000 -backup-dir "$BACKUP_DIR"
go run ./cmd/ranking-migrate -config config.prod.yaml -reconcile
```

写操作必须提供备份目录。备份是 REPEATABLE READ 一致性 SQL 快照，包含已有 gift/guard/ranking 和 sys_base_menus/sys_authority_menus/sys_apis 的 DDL、数据与触发器；有 COMPLETE 标识、落盘 fsync 和 SHA256。文件没有 USE/DROP，不覆盖现有库。恢复应选择全新的隔离库，先验证再按变更方案恢复生产；不能直接将测试恢复命令指向正式库。

迁移使用数据库命名锁；MySQL DDL 无整体事务回滚，错误时停止、保留现场，禁止删表重建。重复迁移保留已有事实。正常运行账号不应拥有 DDL/DROP/TRIGGER 权限。DDL 与回填分别受 1～30 分钟可配置维护预算限制，HTTP 预算保持 8 秒；回填每批 1～200 条，任务中断后按代际收据恢复，不重新计分。

CLI 财务摘要覆盖 wallet、payout 及礼物、收益/可提现账户/提现事实。`financialUnchanged=true` 表示本次维护窗口前后行计数与 SHA256 相同；若有并行业务造成差异必须核查，不能直接认定是迁移改账或忽略差异。

## 菜单与权限

直播管理下新增守护方案、守护会员、守护权益流水、守护异常任务、直播榜单五个菜单。保留旧礼物消费排行。

元数据仅为角色 888 补充这五个菜单和新模块精确操作授权，不向其他角色放开接口。普通管理员仍验证当前账号启用、当前角色成员与完整方法/路径；ID 1 沿用固定超级管理员规则。后台从服务端 actions 展示实际可操作项，菜单隐藏不承担授权职责。

## 实际执行记录

本次记录保存在 [deployment-result-20261104.json](deployment-result-20261104.json)。远程 `live_prod`（主机指纹 `1a33ec8931e5`，MySQL 8.0.46）新增 15 张表、5 个菜单和 16 项精确授权；7 条真实历史收入已回填为 52 个桶，checkpoint=7，lagCount=0，逐桶核验一致。

维护窗口前后 44 张资金相关表的行数和 SHA256 全部相同。三份备份均位于 `/Users/tabby/.codex/backups/guard-ranking-20261104`（0700；SQL 为 0600），完整文件名和 SHA256 见 JSON。实际回填前生产备份已在仅监听本机的临时 MySQL 中恢复：29 张表、30 个触发器、7 条收入和 5 个菜单匹配，写入不可变收入被拒绝；临时恢复库已删除。

**本次未发布线上应用，未核验线上 Worker。** 配置文件中的 guard-enabled=false，方案数 0，启用守护附赠的礼物数 0。数据库核验通过不代表 Provider 已在线：先发布兼容后端和前端、启动并验证 Worker、配置方案，最后按就绪条件开售。本次没有更改生产配置、没有插入测试用户/订单/权益。
