# 数据口径与架构

## 职责与事实归属

Router → API → Service → Model，注册沿用 enter.go。`LiveAccessService` 继续判定账号、主播、房间、场次、可见性及拉黑/关注权限；默认 30 秒重新校验有效连接，APP JWT 每请求验证签名，避免心跳每次查询账号表。账号撤权存在这个有界传播窗口；场次关闭另由 Worker 收敛。tokenVersion 变化也会使会话失效。

Redis 是当前在线状态真源；MySQL 是访问事实、累计检查点与最终场次统计的持久存储。SRS 管媒体生命周期；social 管关系与最近观看；gift、wallet、earning、payout 保留资金唯一事实。presence 不自动收费或派奖。

观看时长使用 Redis 服务端时间，前后两次状态都 playing=true 且 visible=true 才累计；一次最多两倍心跳间隔，默认 30 秒。首次心跳建立播放起点。单连接分别计时，同一身份多连接按保守最大有效时间线并集计时，迟到且已覆盖的区间不再计入，可能少计，不会把同时双设备翻倍。超时退出仅补受上限约束的尾段。浏览器声明无法证明实际观看。

## Redis 键与上限

每场前缀 `live:presence:v1:{sessionNo}:`，依次为 stats、connections、expiry、members、guests、actors、seen、pending、events、active、likes（以 `presenceKeys()` 为准）。同场 Lua 所有键共用 hash tag；清理按 ZSET 时间索引，不用 KEYS/全库扫描。active 为同分值的字典序索引，游标无需深 offset。

- stats 保存 epoch、revision、累计值、最后成功落库时间与关闭标记；在线值由截止 Redis TIME 的有效 ZSET 元素计算，即使清理滞后也不把已过期项算在线。
- connections/actors 保存受控会话元数据；IP 仅保存用途隔离 HMAC，JWT/presence bearer 不落库。内部 social history token 仅在 Redis 适配器使用，不公开、不打印。
- seen 保证场次身份去重；likes 保存原键原参数回执。每场最多 100000 条连接历史、100000 条点赞回执；达到容量返回背压，需要新场次或完成汇总，不静默删回执重新计数。
- 默认有效连接上限：每身份每房 4、跨房 12、每 IP 300、每房 10000、全局 100000；配置有代码硬界限。游客身份限额不能作为自然人防刷，需结合边缘网络限流。
- 新进房在 pending 达 10000 或事件接近 20000 时停止；已有连接仍可能变脏，绝对元数据数量受 100000 场次容量约束。清理每批最多 100；队列满先排空已提交待办，避免清理/落库互相阻塞。
- 活跃场次键 7 天滚动 TTL，最终汇总后 24 小时；连接凭证最长 7 天，超长直播必须安排换场。固定窗口请求限流键 60 秒，跨房点赞预留回执 120 秒，全局入场预留随连接 TTL 到期。
- 全局入场限额采用独立 `{admission}` slot，不能和场次原子提交，先预留后写场次；失败保留短暂预留，避免并发同键补偿误删有效限额。跨房点赞配额同样保守预留，失败可能消耗短暂配额，但不会超额点赞。

同场热点及全局 admission 是实际单 slot 热点；本期并非无限横向扩展架构。增加单房容量前必须实测热点 CPU、命令延迟、内存、Worker 消费能力。

## 持久化、竞态与故障

四张新表：live_viewer_visit（每连接永久唯一）、live_viewer_session（场次+身份时长）、live_presence_checkpoint（epoch/revision/运行状态）、live_room_event（永久事件 ID）。live_session 增加 total_watch_duration_ms、stats_checkpoint_at；使用原 stats_finalized_at 并增加查找索引。

Worker 默认 2 秒调度，每轮最多 32 个最久未尝试场次、并行 4；每场 15 秒 Redis 租约、8 秒工作预算，每批 100 visits + 100 events。MySQL 锁检查点、epoch fencing、条件 UPSERT 防止旧版本覆盖新事实；revision 最后赋值以符合 MySQL 顺序求值语义。SQL 提交后才按元数据 revision CAS 确认待办，事件确认后删除流项。崩溃或重试不会重复计数；旧代际不能确认新代际工作。

结束后先关闭 Redis 写入，分批清理并排空待办，再设置最终时间、更新主播累计观众与峰值。Ending 不是最终状态，不会提前封存；只有 Ended/Cancelled/Failed 最终汇总。生命周期仍负责主播开播次数/直播时长；gift 仍负责礼物统计。原外部场次统计写入口已关闭。

Redis 失败时 open/heartbeat/like 明确失败。MySQL 暂时失败保留 Redis 待办；权限复核失败或最后成功检查点超过 60 秒时停止接受相关写入，避免无限未落库增长。MySQL 检查点不是每次心跳一条永久记录。正常数据丢失窗口受调度、负载、故障及背压影响，不能声称固定 2 秒或绝对零丢失。

Redis 整场丢失不自动伪造集合；保留数据库事实并标记 dataLost，受控恢复命令先换 epoch，再从已落库累计值和身份时长重建基线。部分键损坏不自动删除其他键。详情见运维文档。

## 事件与后续扩展

事件信封有 eventId/eventType/schemaVersion/roomNo/sessionNo/actorId/sourceType/sourceNo/occurredAt/payload，字段目前是内部持久模型，不向 APP 直接暴露。

entered、left、timeout、session.ended 来自 Redis 事实；heartbeat_checkpoint 最多每连接每分钟一条；liked 按检查点合并已接受差量。followed 引用已提交 social outbox，需匹配主播及访问时间区间；gift.sent 必须 JOIN 实际 gift_order 匹配房间、场次。两者不消费或改写原 outbox 的状态，永久事件 ID 防重。关注最多回看 7 天，和支持的场次生命周期一致。

业务事件投影每 10 秒独立调度、5 秒预算、每类最多 100 个；按已有永久事件反连接查缺，避免按递增 ID 单游标漏掉迟提交事务。数据增长后需专门评估反连接扫描及吞吐，不能把这条通用投影直接当成高频财务流水总线。后续增加推送或分析消费者时以事件 ID 幂等，使用独立投影与授权，不把原业务事务绑在网络广播上。
