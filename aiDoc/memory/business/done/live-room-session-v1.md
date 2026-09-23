# LiveRoom / LiveSession 第一版

## 目标

完成 `live_anchor 1:1 live_room 1:N live_session` 的直播间与逻辑直播场次闭环，包括 APP、SRS 回调、后台管理、断流重连和场次结算。

## 长期业务约束

- `live_room.id`、`live_session.id`、`anchor_id`、`room_id`、`current_session_id` 只允许后台和服务端使用；APP 只使用 `anchorNo`、`roomNo`、`sessionNo`。
- 主播申请审核通过时在同一事务创建唯一直播间，默认 `room_no = anchor_no`、`stream_name = anchor_no`；管理员可修改 `room_no`，但不联动稳定推流名称。
- `live_room` 保存长期房间配置和当前活动指针，不保存历史最近开播/下播字段。
- `live_session` 是直播历史的事实表；分类、标题、封面在创建场次时保存快照。
- `active_room_id` 是生成列，状态为准备中、直播中、结束中时等于 `room_id`，唯一索引保证一个房间最多一个活动场次。
- 场次状态：`0`准备中、`1`直播中、`2`结束中、`3`已结束、`4`已取消、`5`失败。
- SRS `on_unpublish` 不直接结束场次，而是开启可配置重连窗口；窗口内重连继续原场次，超时后才结束。
- `duration_ms = ended_at - started_at`，表示逻辑直播时长，包含成功重连前的短暂断流；断流超时时 `ended_at` 取 `last_unpublish_at`，不计算最终等待窗口。
- 房间状态：`0`后台/风控禁用、`1`正常、`2`主播主动关闭或运营停用。
- 主播禁用、封禁、注销或关闭开播权限时，准备中场次取消，直播中场次进入结束中。
- 礼物汇总字段为 `gift_count`、`gift_coin_amount`、`gift_user_count`；它们不是资金账本，账务以礼物订单和钱包流水为准。
- 主播累计时长统一使用 `total_live_duration_ms`，旧秒字段启动迁移时乘以 1000 后删除。

## 运行配置

- `live.reconnect-window-seconds`：断流重连窗口，未配置或小于等于 0 时服务使用 20 秒。
- `live.srs-hook-token`：SRS 和可信统计服务通过 `X-Live-Hook-Token` 使用的共享密钥；空值会拒绝全部 hook。
- `live.push-base-url`、`live.play-base-url`：准备接口拼接推流和播放 URL 的基础地址。
- 每 5 秒运行 `FinalizeLiveSessions`，先结算上一轮结束中场次，再标记本轮断流超时场次。

## 后台入口

- 直播管理 → 直播间管理：筛选、查看、修改房间编号和资料、禁用/恢复/关闭房间。
- 直播管理 → 直播场次：筛选、查看统计和断流信息、强制结束活动场次。
- 后台只保留角色菜单分配，不使用 Casbin API 路径鉴权；接口统一由管理后台 JWT 保护。
