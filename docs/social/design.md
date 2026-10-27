# 社交关系设计与兼容性

## 已核对的约定

Router → API → Service → Repository → Model，enter.go 聚合。APP /api/v1/app/social，管理 /api/v1/admin/social；后台文件使用 _admin 后缀，登录之外强制高级管理员角色 888。统一 {code,data,msg}；新列表采用 limit/cursor/nextCursor/hasMore，避免把游标伪装成 OFFSET 页码，旧分页不变。

沿用 live_account 的 ID 作为内部 user_id，外部用户参数使用 accountNo；主播使用 anchorNo；资源编号使用 roomNo/sessionNo 等字符串，不暴露主键。social_* 为独立单数表，字段 snake_case；Go ID/UserID；DTO camelCase。关系硬删除，不能嵌入带 DeletedAt 的 GVA_MODEL；保留 ID/CreatedAt/UpdatedAt 命名并明确 bigint unsigned 主键、UTC datetime(3)，业务时间毫秒、展示 Asia/Shanghai。现有 live_room.visibility 已承担访问策略，禁止另加互相矛盾的 access_policy。

## 事务与状态

social_user_counter 按 user_id 升序 FOR UPDATE（仅缺行时 INSERT IF ABSENT 后重读），作为跨实例双用户串行化锁，然后读取关系、检查拉黑、插删关系、更新计数、写 outbox。关注、取消、双向拉黑均遵循相同顺序。计数必须条件更新，欠计数立即回滚报错，不静默夹到零。事务 READ COMMITTED，限定只重试 MySQL 1213/1205 并退避抖动；连接断开/提交结果未知不自动重试，客户端以最终状态幂等重试。锁定计数行后修复，在持锁事务内重新 COUNT，避免覆盖真实增量。

计数表示所有现存关系；列表隐藏不可用账号，故显示条数可能小于计数。不新增未定义的账号隐私字段。拉黑移除双方关注、关闭双方提醒；取消关注关闭提醒；首次关注是否创建提醒由配置控制，已有关闭记录永不隐式恢复。屏蔽不取消关注：content/content-feed，live_notification/开播提醒，recommendation/推荐，direct_message/后续消息适配，未接入消费者不宣称效果。

鉴权只读数据库事实；关系展示缓存使用数据库版本组成键，旧查询只能回填旧版本，不能污染新版本。Redis 故障读库，写限流故障拒绝敏感写入。缓存不用于授权。

资源使用 ResourceType + ResourceID（公开编号），各模块提供批量校验与展示适配器。现有只注册 live_room；short_video/live_replay 是受约束的扩展类型，未注册时明确失败，禁止凭空承诺资源存在。

## 历史和异步

播放先 open，服务端在用户counter锁下递增history_version、发放sessionToken（删除/清空不重置该版本），后开启的设备取得写权；同 session 只接受递增 seq，合法回退允许。心跳 Redis 有界聚合、周期落库；checkpoint 同步持久化。清空递增 history_epoch，删除移除会话令牌对应的事实行，旧缓冲不得复活。Redis/缓冲丢失只损失尚未持久化的心跳，严格 checkpoint 成功才保证落库。

开播事件在首次 Preparing→Living 的事务写入，以 session_no 为唯一业务键；重连不重复。outbox worker 用数据库行锁和持久游标分页扇出；消费者以 event_id+consumer+user_id 去重。外部发送通过 Sink 接口，要求下游接受 idempotency key；发送后崩溃允许重复调用，不宣称 exactly-once。没有真实投递渠道时不报告投递成功。

## 媒体门禁

API 权限与媒体权限分别校验。私密/仅关注者保存、开播需要显式启用可信媒体网关配置；默认拒绝上线。当前HLS网关每个playlist/segment请求都调用鉴权适配，校验 APP Bearer 身份、短期播放凭证及当前关系/场次。签名绑定用户和场次，不可单独复制 URL 跨用户播放。必须封闭源站所有直连播放入口，受保护FLV/WebRTC未接入，不对外开放；仅设置配置开关不能证明部署安全。详见上线文档与代理配置样例。

## 公开用户资料与列表（2026-10-27）

账号模块与社交模块使用同一 `common/response.UserPublicProfile`，禁止复制字段列表形成不同DTO。关系、历史、在线直播通过嵌入添加业务字段；普通用户资料只输出白名单字段。直播状态通过同一SQL中的账号/主播/房间/当前场次/双向拉黑/关注条件决定；`isLive`是当前观众可访问状态，不能用于绕过进入房间时的重新鉴权。

公开资料最多100人一批，基础资料与直播资格最多两条SQL；当前场次固定按主键关联，避免扫描历史场次。互关/共同关注先取有界本人关注候选再求交，空页仍通过候选游标继续。历史列表展示资源所属用户，保留原资源键删除记录；收藏列表已删除，其余收藏能力保留。完整兼容变化和验证见production-review-20261027.md。
