# 部署与数据库迁移

## 已执行结果

2026-11-06（Asia/Kuala_Lumpur）已使用项目正式配置直接连接远程 `live_prod`，MySQL 8.0.46。迁移前活动场次为 0、历史场次 35；完成四张 presence 表、新场次列和相关索引的增量迁移。后续性能改进涉及 visit 点赞/事件检查点和 Worker 调度字段，已执行第二轮增量迁移。用户无需再手动迁移这个远程库。

两轮均先将受影响表的 DDL 和数据以一致性只读事务备份到私有目录，再执行增量 DDL；JSONL gzip 文件目录 0700、文件 0600。迁移前后 wallet/gift/anchor finance/payout 事实表逐表行数和 SHA256 一致。报告未保存数据库密码、网络地址或用户资料。

- 首轮证据：`deployment-result-20261106.json`
- 最终增量证据：`deployment-final-result-20261106.json`
- 最终独立只读复核：`deployment-verify-20261106.json`
- 最终备份：`/Users/tabby/.codex/backups/live-presence-20261106/presence-before-20261105T173156.180558000.jsonl.gz`
- 备份 SHA256：`89249d82d501b6cbd4a83b5a43dd2c1764ef34ff197ab569eb63d256282b3737`

备份文件名使用 UTC，因此日期前缀为 11 月 5 日。备份包含内部业务数据，应保持私有且不加入仓库。

**仅数据库已远程变更，Go 服务、后台静态资源、H5 静态资源尚未远程发布/重启。** 不会仅因表结构迁移而开始统计线上旧客户端。

## 发布顺序与配置

1. 先在目标环境完成本文结构校验；本次远程库已完成。保留旧二进制、前端构建产物和配置备份。
2. 所有后端实例使用相同 APP signing-key 与 presence 配置；密钥派生在启动时冻结，变更密钥须统一重启，旧凭证会失效。不要在运行中只更新部分实例。
3. Redis 建议使用专用实例或明确资源预算，启用合适的持久化/备份与内存告警；推荐 noeviction，容量失败应明确暴露而非静默丢在线数据。应用 Redis 客户端必须启用 context socket deadline（项目初始化已启用）。
4. 发布后端，检查日志、API/Worker 检查点；确认 SRS 只继续发送生命周期 Hook，停止调用退役 session/stats。
5. 发布当前仓库管理端及 H5Player。观察会员/游客进房、切号、关闭页、结束场次最终汇总；灰度阶段统计只覆盖新版接入客户端。
6. 在实际网络/代理/Redis 拓扑上执行生产验收，重点是可信代理、JWT、受限房间及媒体 URL、服务重启和热点容量。此任务没有替代 CDN/SRS 媒体防盗链验收。

三个 YAML 已增加带注释 presence 配置，默认 heartbeat-seconds=15、ttl-seconds=45、recheck-seconds=30、room-connections=10000、global-connections=100000、ip-connections=300、like-second=10、like-minute=120。代码对配置进行边界约束，不能将其改为无限。

## 工具复用

在 `server/` 执行，默认仅检查，不会写库：

```bash
go run ./cmd/presence-migrate -config config.prod.yaml
go run ./cmd/presence-migrate -config config.prod.yaml -verify
```

仅对其他明确目标环境需要迁移时使用（不要为验证随意写生产）：

```bash
go run ./cmd/presence-migrate -config /私有路径/目标配置.yaml -migrate -backup-dir /私有绝对路径/备份
```

迁移程序只管理四张 presence 表以及指定 live_session 列/索引，不执行整库 AutoMigrate，不重建资金表。DDL 不是事务性整体回滚；失败时保留私有备份并修复原因后幂等重跑，不能宣称失败已自动回滚。MySQL 大表加索引仍可能持元数据锁，其他环境需提前评估。

## 回退

应用问题优先回退对应应用版本并关闭新版前端上报；保留新增表列，不删历史访问或事件。旧应用不知道新表，不要求立即删列。若需恢复统计数据，应在停写、确认备份和核对新旧累计之后由操作人制定定向恢复步骤；禁止用整库备份覆盖期间产生的钱包、礼物或出款事实。不要启动会恢复旧外部统计写入的版本后继续接收旧 stats 推送。
