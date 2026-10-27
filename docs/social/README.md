# 社交关系模块交付与维护

本次按现有 gin-vue-admin 分层实现后端社交模块，保留既有直播生命周期；社交核心默认启用，九张社交表随服务启动由 GORM 自动迁移。CDN、系统推送、短视频和回放业务不属于当前需求范围；相关接口保留扩展边界，不作为本期未完成项。真实依赖边界见下文。

## 项目适配结果

- 用户事实仍为 `live_account`，主播仍通过 `live_anchor.user_id` 关联。客户端提交 `accountNo` 或 `anchorNo`，二选一，操作者仅来自 APP JWT。
- 公开直播间详情返回主播关联账号的 `userInfo`。前端以 `userInfo.accountNo` 作为社交操作的首选目标；`anchorInfo.anchorNo` 只是解析到同一账号的兼容入口。关系状态继续由受 APP JWT 保护的社交状态接口按当前观众查询，不写入公开资料对象。
- 用户关系展示统一复用 `model/common/response.UserPublicProfile`，含账号公开资料及当前观众可访问的 `isLive/roomNo/sessionNo`；资料与直播查询最多两条批量 SQL，关系缓存命中仍刷新公开资料。历史列表已改为用户卡片；`GET favorite/list` 已移除，其余收藏操作保留。
- 九张业务表为 `social_follow`、`social_user_counter`、`social_block`、`social_mute`、`social_favorite`、`social_watch_history`、`social_live_subscription`、`social_outbox_event`、`social_event_consumer`。单数表名、snake_case 列、camelCase DTO，不混用后台用户。
- 关系采用硬删除，未继承含 `DeletedAt` 的 GVA_MODEL；保留 ID、CreatedAt、UpdatedAt 习惯。计数表以 `user_id` 为主键，避免多余主键与唯一索引。
- 保留 `live_room.visibility`：0 私密，1 公开，2 仅关注者。不增加平行访问策略字段，不改房间/场次状态值。
- API/Router/Service 用 `enter.go` 注册；管理实现放 `_admin.go`。现有项目已移除 Casbin，社交管理接口在后台 JWT 之外强制 `RequireAuthority(888)`，菜单可见性不能代替授权。
- 统一 `{code,data,msg}`。新列表使用 `limit/cursor/nextCursor/hasMore`，不会伪装成 OFFSET 页码。原有接口保留原分页格式；原公开直播列表计数是候选总数，登录用户被过滤后可见条数可少于总数。
- 新表 datetime(3) 固定 UTC；`UTCTime` 使用字符串绑定与 UTC 墙钟扫描，避免现有 MySQL `loc=Local` 转换它，也不批量修改旧表时间。返回时间/进度均毫秒；客户端展示按 Asia/Shanghai。
- 启用社交后，各主播资料与房间详情的 `fansCount` 改读社交权威计数，旧 `live_anchor.fans_count` 仅保留为回退快照，不双写。旧数据若只有粉丝数字而无关注事实，不能凭数字生成关系。
- Swagger 对旧后台相对路径和新 `/v1/app` 路径做统一前缀处理，避免调试请求被错误拼接到 `/admin/v1/app`。

## 已实现业务与语义

关注、取消、互关、共同关注、本人关注/粉丝列表、批量状态、计数；双向拉黑与取消；分 scope 屏蔽；收藏；观看历史、进度与清除；主播提醒偏好；关注中的在线主播；计数核对/持锁修复、异常关系扫描、黑名单审计、事件查询与重放。

关注、取消、拉黑、计数与 outbox 在一个数据库事务内；`changed=false` 表示已处于目标状态。两端 counter 行按 user_id 升序加锁，再变更关系。拉黑解除两向关注，关闭两向提醒；取消拉黑不恢复。取消关注关闭提醒，重新关注不覆盖已有关闭偏好。首次关注自动提醒由 `auto-subscribe` 控制。

计数是全部现存关系；用户封禁、删除后其资料从列表隐藏，事实与计数仍保留。不能拿可见列表长度当总粉丝数。所有客户端敏感列表只允许本人读取。无账号隐私开关的既有模型不会被擅自新增字段；未来隐私规则必须接入 Directory/ResourceProvider。

MuteScope 固定为 `content`、`live_notification`、`recommendation`、`direct_message`。当前开播消费会执行 live_notification 过滤；其他三项提供存储及批量状态，内容流、推荐和私信模块接入时必须消费相应 scope。屏蔽不解除关注，也不默认禁止访问。

收藏/历史使用 `resourceType + resourceId`（公开编号），批量 ResourceProvider 校验与展示。**生产仅注册 live_room**；short_video 与 live_replay 尚无业务表，返回 46007。自动化测试使用独立测试适配器验证点播进度，不代表真实视频业务已接入。历史列表实时过滤不可见资源后返回所属用户公开资料；收藏列表接口已删除，批量收藏状态仍过滤不可见资源，取消收藏不要求资源仍存在。

## 观看历史可靠性

1. `history/open` 持久创建会话，返回服务端 sessionVersion、sessionToken 和 epoch。后打开的设备获得写权。`social_user_counter.history_version` 在用户锁下递增，删除记录后重开也不会复用旧版本；版本在 JS 安全整数范围内，不保证从1开始。
2. 同一会话按 `sequence` 单调递增；乱序/重复不覆盖，合法新序号可以回退播放位置。客户端时间不参与排序。直播 positionMs 固定 0、completed=false；点播独立保存进度及完成状态。
3. 普通心跳进入 Redis Lua 聚合：16 个 hash-tag 分片，各最多1000个资源记录；单记录 TTL 60秒，首次入队5秒后可落库。后续心跳不延后首次刷盘期限；不存在每次心跳都 UPDATE MySQL 的行为。
4. Worker 每分片一次最多读取32条；每次持久化重新检查账号、资源、服务端会话及 epoch。当前采用有界逐记录事务，尚未实现单条 SQL 批量合并写入。Redis Cluster 同分片 MGET/Lua 使用相同 hash tag；每分片有15秒租约，单轮最多10秒，减少多实例重复落库，数据库会话/epoch/sequence及缓冲CAS确认继续兜底。Lua保留原始JSON，避免cjson重编码损失大整数精度。
5. `checkpoint=true` 成功意味着同步 MySQL 提交；应用退出时应提交 checkpoint，但 App 被杀死/网络断开无法保证送达。
6. 普通心跳 `persisted=false` 只表示排队，数据库长时间不可用、Redis 丢失/过期或重启可丢失未持久化进度，最长未持久化缓冲保留60秒。该通道不是严格可靠观看事件流水。
7. 清空历史递增 epoch 并删除记录；单记录删除依靠撤销原 sessionToken；旧缓冲只能 UPDATE 已匹配记录，不能重新 INSERT。每用户最多1000条，只在新增记录时核对并清理超额，避免重开已有记录时重复COUNT；默认保留90天，可配置1—365天。
8. open 的 requestId 在当前会话内幂等；不要无限重放已被其他设备替换的旧 open。跨会话、长期幂等记录与严格观看事件账本尚未实现。

## Redis 与降级

`social:v1:relationship:<hash>`：展示状态 TTL 随机5—10秒；键含双方数据库版本，旧读只能回填旧版本。每次仍现查可见账号资料，封禁账号不会被缓存资料重新显示。所有授权路径直接读库，绝不靠缓存独立放行。

`social:v1:rate:{read|write}:<userID>`：60秒窗口，读600次/写60次；body 32KiB、批量100、列表100上限。Redis 不可用时敏感写入失败关闭；只读接口使用进程级100 QPS有界兜底后回源MySQL，最终授权仍只读库。后台修复不受此客户端限流。写入不使用本地限流冒充多实例限流；只读兜底仅保护单个实例。实际网关还应对匿名鉴权/媒体请求设置速率上限。

`social:v1:{history-NN}:item:<hash>` 和 `:pending`：有界心跳缓冲；无 KEYS，无完整粉丝集合。

`social:v1:worker:pressure`：Worker 定期发布10秒有效健康/背压状态。Worker失联或待处理+失败事件达到50000时，新增关注、收藏、订阅和观看等写入拒绝。拉黑及取消/删除类隐私操作不依赖Worker健康，但仍要求Redis限流成功；这些操作仍产生事务内审计事件。多个实例允许同库同 Redis namespace，测试/生产使用各自 Redis DB。API返回46008/46009时客户端必须退避，不能无限重试。

当前未做独立热门计数、在线主播和订阅名单 Redis 缓存；优先保证权威计数与授权一致性。关系版本仍需数据库读取，缓存主要减少多张关系表的重复查询。

## Outbox 与通知边界

Worker 使用固定数量的独立循环处理审计/健康、历史刷盘、清理和外部事件；慢通知不阻塞心跳和历史刷盘，退出时等待循环结束。健康接口每状态最多读取1001条候选，输出最多1000及下界标记，避免频繁全量COUNT。

真正首次 Preparing→Living 的事务写入 `live.started:<session_no>`；同场重连不重复。不在 on_publish 遍历粉丝。关系审计批量消费最多500条；开播提醒按订阅 ID 每页20条处理，限速且不创建逐粉丝 goroutine。

MySQL SKIP LOCKED + 30秒租约 + 随机 fencing token；处理 context 15秒，下游单次1秒；进程崩溃后可重领。失败指数退避、抖动，最多8次进入 failed，后台重放保留去重记录。开播提醒超过10分钟跳过，每次投递前复核当前场次、主播/账号资格、关注、订阅、双向拉黑与免打扰。首次事件创建以后才建立的订阅不接收该场提醒。完成/跳过的事件与消费记录保留30天后分批清理，每状态每批最多250条，按保留期索引加锁并使用SKIP LOCKED避免实例互抢；失败记录保留给管理员归档或重放，并计入背压阈值。

**项目目前没有可复用的投递渠道，因此生产未注册 NotificationSink。** 没有 Sink 时不会创建开播通知事件，避免无意义失败和积压；关系审计事件与观看历史 Worker 仍正常工作。未来注册 Sink 后，Sink必须验证当前有效终端/推送token，接受 `eventId:consumer:userId` 幂等键，并遵守 context。测试 Sink 只验证协议。

外部发送成功后、消费记录提交前进程崩溃，可能重复调用下游；属于至少一次投递。校验与外部发送之间也存在不可消除的撤销竞态，最终渠道应支持到达前复核/撤回。不能承诺端到端 exactly-once 或注销瞬间撤销已送达消息。

## 媒体访问与上线门禁

集中 `LiveAccessService` 检查账号、主播审核/风险/开播权限、房间、场次关联、状态、关注及双向拉黑。公开详情接入可选 APP JWT；提供无效 JWT 不会降级匿名。现有公开列表保留格式并过滤当前登录观众无权访问的房间。授权不提供普通管理员绕过；后台审核资料仍走原有后台授权接口，审核播放需另行设计独立审计凭证。

仅关注者/私密房间的保存、开播、后台修改和 on_publish 均需可信网关就绪，否则拒绝。仅当受保护媒体网关就绪时，准备开播的旧 playUrl 返回空字符串，观众改用 `GET /api/v1/app/live/room/playback?roomNo=...`；这是为避免继续签发未鉴权地址所需的显式兼容调整。未启用受保护媒体时，公开房间继续使用现有播放链路。已有调试页的复制播放地址功能不能用于受保护播放。

当前适配只开放 HLS，30秒凭证绑定 APP 用户、场次和流名；每个 playlist/segment 请求同时要求 APP Bearer 与 X-Playback-Token，复制 URL 给其他用户不能播放。提供 `tests/social/media-gateway.nginx.conf` 模板。播放器须给子资源请求注入 Header 并在30秒到期前重新调用 playback 获取凭证；原生 HLS 无法注入 Header 时不支持此接入方式。

必须封闭源站所有 HTTP/RTMP/WebRTC 播放入口、禁止 CDN 缓存绕过鉴权、验证实际切片命名与跨域行为。仅设置 media-gateway-verified=true 不构成安全证明。FLV 长连接/WebRTC 会话强制断流未接入，因此受保护播放不开放这些协议。权限撤销后下一次 HLS 请求会拒绝；已下载或播放器缓存的切片不能回收，不能承诺即时断流。真实 SRS/CDN/浏览器媒体链路尚未验收。

## 启用、自动迁移与回退

1. 审核并备份现有库。先确认 MySQL >=8.0.16/InnoDB、`disable-auto-migrate: false`、测试/生产 Redis 隔离，并保留原有 app/admin JWT 分离。
2. 服务启动时由 GORM 创建或更新九张社交表、唯一约束、复合索引与 CHECK 约束；MySQL 新表使用 InnoDB、utf8mb4 和 utf8mb4_bin。启动后还会分批为已有关注事实补齐缺失的计数行，但不会覆盖已经维护的权威计数。
3. AutoMigrate 失败会阻止服务继续启动。应根据启动日志修复数据库权限或不兼容结构后重新启动，不要绕过结构校验。
4. 如需导入旧关系，先停止全部新旧写入口，再导入事实；服务启动会补齐缺失计数，上线后的计数修复只能使用持锁工具。不能用旧 fans_count 反推事实。
5. 社交核心和 Worker 默认启动，不再提供 `social.enabled`、`worker-enabled` 开关；Redis必须可用，auto-subscribe默认false，保留期默认90天。没有媒体改造时保持 protected-playback-enabled=false。
6. 私密/关注者开播需先验收媒体网关与源站封闭，再设置 HTTPS playback-base-url、独立32字节以上 playback-signing-key、protected-playback-enabled 和 media-gateway-verified。密钥从私有配置提供，禁止复用 JWT/推流密钥。
7. 开启时先小范围灰度，观察 health 的积压、失败、计数差异和事务延迟。当前没有实际推送渠道，应先选择/接入真实 Sink 再把“提醒已投递”作为产品承诺。
8. 回退先停止应用实例以终止社交写入与 Worker，并保留社交事实表。回退到仍读取 `live_anchor.fans_count` 的旧版本前，必须通过经过审查的维护程序在停写状态同步一次粉丝快照；即使回退也不能开放旧版未鉴权的受保护媒体入口。
9. 社交模块启用时数据库配置变更要求进程重启，禁止旧 Reload 直接关闭正在被Worker使用的连接池。

计数维护示例（凭证使用环境变量，不写入命令参数或日志）：

```sh
cd server
# SOCIAL_MYSQL_DSN由运维密钥环境注入，parseTime=true&loc=UTC及连接超时。
go run ./cmd/social-check --account-no 10000001
go run ./cmd/social-check --account-no 10000001 --repair --confirm REPAIR --operator oncall --reason '历史导入后核对'
```

CLI输出 JSON 审计记录，需由运维留存；HTTP修复有后台 OperationRecord 和事务内变更事件。单次核对一人，避免后台接口触发全库长事务。

## 验证与交付索引

- `design.md`：设计和事务边界。
- `api.md`：接口、错误码、权限矩阵。
- `frontend-integration.md`：前端当前可对接功能、接口字段、页面范围与暂缓能力。
- `production-review-20261027.md`：统一公开资料、破坏性接口变更、SQL与竞态修复、最新验证证据和容量边界。
- `verification.md`：实测结果、复现命令、40项验收矩阵、性能与未验证项。
- `server/model/social/`：模型、DTO与 UTC 类型。
- `server/repository/social/`：锁、重试、计数不变量。
- `server/service/social/`：业务、缓存、历史、Worker、审计和测试。
- `server/service/live/social_adapter.go`、`live_access.go`：模块适配与媒体授权。
- `server/api/v1/social/`、`server/router/social/`：APP与后台接口。
- `server/initialize/social.go`、`server/core/server.go`：启动与可停止Worker。
- `server/tests/social/run.sh`：隔离 MySQL/Redis、集成、race、Benchmark 一键运行。

当前需求范围外：真实短视频/回放资源适配；真实站内信/WebSocket/系统推送渠道；受保护FLV/WebRTC；生产CDN/源站验收。已知后续扩展项：严格观看事件账本与长期open幂等、大规模生产容量证明、后台管理页面。现有后端接口、持久化与安全门禁均为可执行实现，不以空适配器返回成功。
