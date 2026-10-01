# LiveRoom / LiveSession 第一版

## 目标

完成 `live_anchor 1:1 live_room 1:N live_session` 的直播间与逻辑直播场次闭环，包括 APP、SRS 回调、后台管理、断流重连和场次结算。

## 长期业务约束

- `live_room.id`、`live_session.id`、`anchor_id`、`room_id`、`current_session_id`、`last_session_id` 只允许后台和服务端使用；APP 只使用 `anchorNo`、`roomNo`、`sessionNo`。
- 主播申请审核通过时在同一事务创建唯一直播间，默认 `room_no = anchor_no`、`stream_name = anchor_no`；管理员可修改 `room_no`，但不联动稳定推流名称。
- `live_room` 保存长期房间配置、当前活动指针和最近一次已完成直播指针；不保存历史最近开播/下播时间和场次统计。`last_session_id` 仅用于公开详情按主键快速读取上一场直播。
- `live_session` 是直播历史的事实表；分类、标题、封面在创建场次时保存快照。
- `active_room_id` 是生成列，状态为准备中、直播中、结束中时等于 `room_id`，唯一索引保证一个房间最多一个活动场次。
- 场次状态：`0`准备中、`1`直播中、`2`结束中、`3`已结束、`4`已取消、`5`失败。
- 主播端结束接口不接收 `sessionNo`，始终通过当前登录主播的 `live_room.current_session_id` 定位活动场次；后台强制结束仍使用内部 `sessionId`。
- SRS `on_unpublish` 不直接结束场次，而是开启可配置重连窗口；窗口内重连继续原场次，超时后才结束。
- 准备场次写入 `prepare_deadline_at`；超过 `live.prepare-timeout-seconds` 尚未首次发布时直接取消，不访问 SRS、不计直播统计。
- 发布和断流使用 `server_id + client_id + stream_id` 识别当前连接，旧连接迟到回调不能覆盖或断开新连接的业务状态。
- `duration_ms = ended_at - started_at`，表示逻辑直播时长，包含成功重连前的短暂断流；断流超时时 `ended_at` 取 `last_unpublish_at`，不计算最终等待窗口。
- 房间状态：`0`后台/风控禁用、`1`正常、`2`主播主动关闭或运营停用。
- 主播禁用、封禁、注销或关闭开播权限时，准备中场次取消，直播中场次进入结束中。
- 礼物汇总字段为 `gift_count`、`gift_coin_amount`、`gift_user_count`；它们不是资金账本，账务以礼物订单和钱包流水为准。
- 主播累计时长统一使用 `total_live_duration_ms`，旧秒字段启动迁移时乘以 1000 后删除。
- 公开详情无论是否开播都返回房间和完整 `anchorInfo`；`latestSession` 优先返回活动场次，否则返回 `last_session_id` 指向的最近已结束直播，从未完成直播时固定返回空对象 `{}`。
- 公开详情场次使用精简白名单，不返回内部主键、断流重连控制字段、失败原因、结算时间或礼物金额；公开列表响应保持不变。
- `live_session.publish_ip` 保存本场首次推流 IP，只允许管理后台场次列表/详情返回；`live_session.stream_info` 保存已清洗的公开流媒体信息 JSON，公开详情和通用场次响应未采集时返回 `{}`，公开发现列表不携带该 JSON。
- 当前只完成 `publish_ip/stream_info` 表结构、响应与展示接入，不实现从 SRS 获取或写入数据的时机与流程。
- `documents/主播开播流程说明.md` 记录准备中、开始推流、直播中、断流重连、结束中、最终结算的操作顺序、鉴权方式和接口示例。

## 运行配置

- `live.reconnect-window-seconds`：断流重连窗口，未配置或小于等于 0 时服务使用 20 秒。
- `live.prepare-timeout-seconds`：准备开播等待首次发布的时限，未配置或小于等于 0 时使用 120 秒。
- `live.srs.app`：SRS 流应用名，写入 pt 载荷并参与推流/播放 URL 拼接；`live.srs.http_apis` 用于查询发布流并强制断开推流客户端。
- `live.publish-token-key`：仅供 Go 服务加密、解密 pt；不配置到 SRS、代理或客户端，空值会导致 pt 无法生成或校验。
- `live.srs.push-base-url`、`live.srs.play-base-url`：准备接口拼接推流和播放 URL 的基础地址。
- `live.push-token-seconds`：pt 有效时长，单位秒。pt V1 载荷使用 `SHA-256(publish-token-key)` 派生密钥后做 AES-256-GCM 加密，再做 Base64URL 编码并添加 `v1_` 前缀。
- 内置 `LiveSessionLifecycle` 在表结构就绪后立即执行一次，之后默认每 5 秒调用 `ProcessPendingSessions`，处理准备超时、重连超时和结束中场次的 SRS 断流重试。

## 后台入口

- 直播管理 → 直播间管理：筛选、查看、修改房间编号和资料、禁用/恢复/关闭房间。
- 直播管理 → 直播场次：筛选、查看统计和断流信息、强制结束活动场次。
- 后台只保留角色菜单分配，不使用 Casbin API 路径鉴权；接口统一由管理后台 JWT 保护。
