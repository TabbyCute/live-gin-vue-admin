# 单机可靠事件投递与未来 IM 边界

本模块仅负责已提交业务事实的通知投递，不执行资金、收益、提现、退款或出款命令。单业务进程 + MySQL/SQLite 持久队列；没有新增 MQ、WebSocket 服务或生产 mock HTTP。SRS 的媒体链路不经过本模块。

## 数据与投递契约

- `event_delivery_event`：白名单通知快照，`evt_` + SHA256(`source + NUL + sourceID`) 永久稳定 ID；`source/source_id` 唯一，schemaVersion 固定为 1。不可 UPDATE/DELETE；MySQL 允许完全相同的幂等 upsert。
- `event_delivery`：唯一 `(event_id, consumer)`，当前逻辑消费者为 `realtime.v1`；保存队列状态、总尝试数、本轮尝试数、租约、退避时间、有限错误码和确认时间。
- `event_delivery_attempt`：每次领取先持久化 STARTED；回执、未知结果、永久拒绝、旧租约过期均有记录。外部错误正文/URL/header/token 从不保存。
- `event_delivery_retry_audit`：root 的固定 requestId、代次、原因和时间；不可修改或删除。
- `event_delivery_cursor`：每来源最多 100 条主键 keyset 修复扫描，扫尾后回绕；延迟提交的较低 ID 不会永远漏掉。已投影记录批量查询去重，避免逐条重新写入历史记录。

原 `gift_event_outbox`、`payout_terminal_event`、`live_room_event` 保持原样。特别是旧 gift 表里的 `delivery_status` **不代表新消费者进度**；应通过 `event_delivery` 查询。不会因重试重新扣款、记收益或完成提现。

新生产者在原事务中经 `internal/eventcapture.Recorder` 记录通知：gift 送礼、earning 提现终态、presence worker/recovery。未安装 recorder 时保持原业务行为，不依赖新表。初始化先只读核验新 schema，再安装 recorder；启用后通知记录失败会回滚同事务，避免有事实却丢掉实时入队。历史和旧版本期间的记录由游标修复补齐。被排除的 `room.gift.sent` 投影不重复广播；关系事件与心跳检查点不广播。来源补扫各自推进，某一来源坏页不会饿死其他来源。仅旧生产者依赖补扫时，迟提交恢复延迟取决于完整环扫周期；持续旧来源积压无法承诺实时性。

### 状态与限制

`pending → leased → delivered / pending / dead`；未配置 Adapter 时初始状态为 `blocked`，最后代码 `CHANNEL_UNCONFIGURED`，不调用外部服务、不增加投递尝试、不伪造送达。

默认每轮最多 50 个领取、4 个并发；硬上限 200/8。每次临近发送才领取，不提前锁定整个批次。默认单调用 5 秒，租约 15 秒；配置强制租约至少比调用超时长 5 秒。MySQL 使用精确状态索引前缀 + `FOR UPDATE SKIP LOCKED` + READ COMMITTED；CAS fencing 要求原 token、leased 状态及租约未过期。仅本地数据库事务对死锁/锁等待作最多 3 次退避重试，外部调用不在该事务重试闭包内。

默认最多 8 次/轮，指数退避从 1 秒到 5 分钟，带确定性有界抖动。进程崩溃后旧 STARTED 记录变为 `LEASE_EXPIRED_UNKNOWN`，原 ID/幂等键重投；最终一次崩溃也受最大次数限制。无效/未持久确认 ACK 视为 `DELIVERY_UNKNOWN`，不标记 delivered。Adapter panic 隔离为未知结果。消费者可以重复收到同一个事件；不承诺网络 exactly-once 或跨事件顺序。

## Adapter 接入要求

实现 `service/eventdelivery.Adapter.Deliver(ctx, Envelope) (Ack,error)`，由启动组装注入，禁止通过用户请求设置目标 URL 或替换 Adapter。实现必须：

1. 尊重 context、并发安全，不自行无限重试。
2. 持久去重键为 `realtime.v1:<eventId>`；去重记录与实际下游副作用应原子提交，跨重启仍保留。服务端重试、管理员重放均不换键。
3. 返回 `Ack{EventID, IdempotencyKey, Durable:true}` 只表示下游已持久接受/去重。HTTP 200、代理接收、排队成功或用户已读不能混为一谈。
4. 严格执行 audience 路由和授权。`room` 的 destination 是 sessionNo，`user` 是内部可信收件人ID；私人 payout 事件绝不能广播到房间。
5. 配置通道前验收鉴权、幂等、超时后查询/重试、服务重启、私密收件人隔离和真实下游确认。当前没有真实通道，生产应保持 blocked。

安全负载仅：gift 的 orderNo/roomNo/sessionNo，payout 的 payoutNo/status，presence 的 roomNo/sessionNo。都是状态变化提示，客户端应通过现有有权限的业务查询取得详情；不复制原 JSON、金额、余额、账号、流水、证据、IP、游客身份、连接ID、昵称或凭证。发送前再次验证固定 schema、字段集合、payload hash 与 audience，损坏负载进入 dead。

## 管理端契约

路由 `/event-delivery/list`、`/event-delivery/detail`（GET）与 `/event-delivery/retry`（POST）挂在 admin URL group，路由按 Budget → JWTAuth 顺序核验，保证鉴权失败也 no-store。服务层每次独立核验**启用且未删除的 root 用户 ID=1**，角色 888 不能替代用户身份。后台页面 `web/src/view/operations/event-delivery/index.vue`，无生产 mock 路由。

列表采用 `{page,pageSize,total,list}`，每页 1..100、页码最多 1000，可按状态/稳定事件ID筛选。详情只提供安全通知内容、有限投递历史及重试审计，不公开收件人路由字段。重试输入 `{requestId,deliveryId:string,generation,reason}`，原因 8..200 字节。只允许 dead/blocked，必须配置真实 Adapter；代次 CAS + 同 requestId 参数校验保护并发/未知HTTP结果，每条最多 10 次人工重试代次。已确认事件不可重发。页面保留失败请求的 requestId，避免网络重试被解释为新命令。

统一响应 `{code,data,msg}`，错误只返回固定码。请求体上限 2KiB，要求 application/json 和 UTF-8，拒绝未知/大小写别名/重复字段、尾随JSON和非法编码或重复查询参数。四个请求槽与5秒预算覆盖读取慢正文和JWT拒绝。菜单/API 元数据的可见性不是授权依据。

## 启动、运维与迁移

- `initialize.InitializeEventDelivery(db, nil)`：只读 schema 校验 + 安装 recorder；不做 DDL。若功能禁用，应不调用该入口。
- `initialize.StartEventDeliveryWorker(ctx)`：返回 done channel；单进程持有一个生命周期 context，停止前先停止接收业务请求，等待 worker 和请求完成后关闭数据库。
- `service/eventdelivery.Default.Snapshot(ctx)`：状态计数各最多 10000，超过时 `countsCapped=true`；含最老未完成年龄、游标进度、最后运行/成功时间与固定故障代码。没有凭证、收件人或业务原文。
- worker 向 `internal/telemetry` 报告固定名字 `event_delivery`。queue dead/blocked 数上升、最老年龄增长、worker stale 或连续失败应被运营关注；未配置通道的 blocked 是明确当前能力边界。
- 核心事实和幂等映射默认不自动清理。容量增长需做离线归档设计；删除消费者幂等记录可能导致旧事件重复通知。

CLI 不读取命令行 DSN、不打印配置凭证。默认只检查；写操作前必须使用非符号链接、0700 的绝对目录生成私有 JSONL 一致性备份。备份包含模块表及菜单/API 元数据的 DDL、触发器定义、原始类型值，`[]byte` 为 JSON base64；恢复只能先还原到隔离库。迁移前后调用既有 financial Proof，对资金表事实作比对。迁移只增加模块拥有的五张表/索引/不可变保护，metadata 单独可选；不运行全站 AutoMigrate。

```sh
cd server
go run ./cmd/event-delivery-migrate -config /absolute/config.yaml
go run ./cmd/event-delivery-migrate -config /absolute/config.yaml -migrate -metadata -backup-dir /absolute/private-backups
go run ./cmd/event-delivery-migrate -config /absolute/config.yaml -verify
```

先迁移、核验，再发布启用新 recorder 的应用。旧应用继续写原有 outbox，不受新增表影响；回退应用时保留新表与幂等记录。远程迁移、部署和容量验收不属于本子任务已执行证据。

## 本地验证证据（2026-11-12）

SQLite 与隔离 MySQL 8 在同一套行为用例下验证：未配置通道/零尝试、root与停用用户拒绝、未知结果稳定键重试且只有一个下游效果、旧租约过期/重启与旧 worker fencing、错误 ACK 死信、人工重放幂等和原因冲突、两个 worker 并发领取/严格批次上限、原业务事务回滚、不可变投影/审计、敏感字段白名单、私人 payout audience、回绕修复迟提交且 source 状态不变。

首次 MySQL 并发测试发现真实锁竞争，最终实现改为精确状态索引扫描、READ COMMITTED、仅数据库闭包有界重试；修正后通过。API 单测覆盖匿名、角色888非root、停用root、未知/重复/大小写别名/null字段、超大请求、非法编码/分号/重复查询参数及no-store。最终跨SQLite/MySQL 11组行为测试、API/router均通过 `-race`（15.511s/4.751s/4.687s）；请求体fuzz 3秒运行6314次通过。CLI私有备份的完整JSONL解析/表记录计数/SHA256/0600权限/0755及符号链接目录拒绝在本机MySQL通过（2.810s）。新后台页Vue SFC script/template编译通过。

```sh
go test ./service/eventdelivery ./internal/eventcapture ./api/v1/eventdelivery ./router/eventdelivery ./cmd/event-delivery-migrate
EVENT_DELIVERY_MYSQL_DSN='<isolated local DSN>' go test ./service/eventdelivery -run TestMySQLDelivery -count=1
```

测试 fake 仅在 `_test.go`，没有生产通知成功模拟器。测试在建立连接前强制校验 tcp `127.0.0.1:13329` 和专属 `event_test_delivery` 库（其他主机、端口、库名、Unix socket全部拒绝），使用本机 Docker 的专属数据库，无 SSH、无远程行为测试；未读取或修改 H5。
