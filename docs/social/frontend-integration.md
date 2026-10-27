# 社交关系模块前端对接说明

更新日期：2026-10-27

本文面向 APP 前端和管理后台前端，说明当前后端已经提供、可以开始联调的社交能力。接口真实行为以生成的 Swagger 和后端实现为准；本文用于前端排期、页面设计和接口封装。

## 1. 当前可对接范围

### 1.1 APP 前端可直接对接

| 功能 | 可用于的页面或交互 | 状态 |
|---|---|---|
| 关注、取消关注 | 用户主页、主播主页、直播间主播卡片 | 可直接对接 |
| 关注状态与批量状态 | 列表关注按钮、用户卡片、详情页初始化 | 可直接对接 |
| 关注数、粉丝数 | 个人主页、主播主页、直播间资料 | 可直接对接 |
| 关注列表、粉丝列表 | 个人中心关系页 | 可直接对接 |
| 互关列表、共同关注 | 用户关系页、社交证明展示 | 可直接对接 |
| 拉黑、取消拉黑、黑名单 | 用户操作菜单、隐私设置 | 可直接对接 |
| 分范围屏蔽 | 屏蔽设置页 | 可保存和查询；各业务模块是否消费见第 1.3 节 |
| 直播间收藏、取消收藏 | 直播间详情收藏按钮 | 可直接对接，仅支持 `live_room` |
| 最近观看与观看记录管理 | 最近观看页、清除历史 | 可直接对接，仅支持 `live_room` |
| 关注主播的在线直播 | 关注页、直播发现页 | 可直接对接 |
| 房间可见性过滤 | 直播列表、直播详情 | 已由现有直播接口自动执行，前端无需单独判断权限 |

### 1.2 管理后台可对接

管理接口只允许后台角色 `888` 使用，适合制作社交运维页面：

- 核对和修复指定账号的关注计数；
- 查看指定账号的黑名单；
- 扫描“拉黑后仍存在关注”的异常数据；
- 查看 Outbox 事件状态、积压和失败原因；
- 重放失败事件；
- 查看 Worker、事件和投递渠道健康状态。

这些属于运维能力，不应出现在普通 APP 用户端，也不提供管理员代替用户关注、拉黑或收藏的接口。

### 1.3 当前建议暂不接入

| 功能 | 当前边界 | 前端处理 |
|---|---|---|
| 开播提醒实际推送 | 订阅偏好接口已经完成，但项目当前没有站内信、WebSocket、系统推送等真实投递渠道 | 可以暂不展示提醒开关；即使展示，也不能向用户承诺会收到通知 |
| `content` 屏蔽 | 屏蔽数据可保存，内容流尚未消费该规则 | 内容流接入前可暂不展示 |
| `recommendation` 屏蔽 | 屏蔽数据可保存，推荐模块尚未消费该规则 | 推荐模块接入前可暂不展示 |
| `direct_message` 屏蔽 | 屏蔽数据可保存，私信模块尚未消费该规则 | 私信模块接入前可暂不展示 |
| 短视频收藏/历史 | 没有真实短视频资源适配 | 不传 `short_video`，否则返回 `46007` |
| 回放收藏/历史 | 没有真实回放资源适配 | 不传 `live_replay`，否则返回 `46007` |
| 受保护 HLS 播放 | 需要真实媒体网关、源站和播放器联调；当前产品不需要 | 暂不调用 `room/playback`，不开发 Token 刷新和媒体 Header 注入 |
| CDN、FLV、WebRTC 鉴权 | 尚未接入真实媒体链路 | 当前前端不处理 |

## 2. 通用调用约定

### 2.1 地址与鉴权

- APP 社交接口前缀：`/api/v1/app/social`
- 管理后台接口前缀：`/api/v1/admin/social`
- APP 接口请求头：`Authorization: Bearer <APP access token>`
- 管理接口沿用现有后台 `x-token`，通过项目的 `@/utils/request.js` 发起请求。
- APP Token 与后台 Token 完全分离，不能混用。
- 社交模块默认启用，APP 和后台社交路由随服务启动注册；不再提供 `social.enabled` 总开关。

所有普通接口使用统一响应：

```json
{
  "code": 0,
  "data": {},
  "msg": "成功"
}
```

业务失败通常仍是 HTTP 200，前端必须判断 `code`。登录失效使用 HTTP 401。

### 2.2 公开编号

目标用户使用下面两种写法之一，并且只能填写一个字段：

```json
{
  "accountNo": "10000001"
}
```

```json
{
  "anchorNo": "100001"
}
```

`accountNo`、`anchorNo`、`roomNo`、`sessionNo` 都按字符串处理。前端不能提交数据库内部 `id`，也不能在请求中提交操作者 `userId`；操作者只由 APP JWT 确定。

直播间详情 `GET /api/v1/app/live/room/detail?roomNo=...` 返回主播关联账号的公开资料：

```json
{
  "userInfo": {
    "accountNo": "10000001",
    "nickname": "用户昵称",
    "avatar": "https://example.com/avatar.png",
    "signature": "用户签名",
    "gender": 0,
    "countryCode": "MY",
    "anchorNo": "100001",
    "isLive": true,
    "roomNo": "100001",
    "sessionNo": "202610270001"
  },
  "anchorInfo": {
    "anchorNo": "100001",
    "fansCount": 12
  }
}
```

直播间内的关注、取消关注、拉黑、屏蔽和开播订阅统一优先使用 `userInfo.accountNo`。也可以使用 `anchorInfo.anchorNo`，但服务端会把两者解析为同一个 `live_account.id`，前端不能把它们当成两个社交对象。

`userInfo` 是公开资料，不包含当前观众的关系状态。登录用户进入直播间后调用：

```text
GET /api/v1/app/social/follow/status?accountNo=<userInfo.accountNo>
Authorization: Bearer <APP access token>
```

使用返回数组中的第一项初始化 `following`、`blocked`、`blockedByMe`、`mutedScopes` 和 `subscribed`。匿名用户不调用该接口；点击关注时先进入登录流程。若 `userInfo.accountNo` 等于当前登录账号的 `accountNo`，前端隐藏对自己的关注、拉黑和订阅操作。

### 2.3 资源编号

当前前端只使用直播间资源：

```json
{
  "resourceType": "live_room",
  "resourceId": "100001"
}
```

其中 `resourceId` 是公开 `roomNo`。接口枚举中虽然保留 `live_replay` 和 `short_video`，当前生产服务没有注册这两类资源适配器。

### 2.4 游标分页

新社交列表使用游标，不使用 `page/pageSize`：

```text
?limit=20&cursor=<上一次响应的 nextCursor>
```

响应结构：

```json
{
  "list": [],
  "nextCursor": "",
  "hasMore": false
}
```

- `limit` 默认 20，范围 1—100；
- 首次请求不传 `cursor`；
- 后续请求原样传回 `nextCursor`，不能解析或自行拼接；
- 资源或用户被实时过滤时，可能出现 `list=[]` 但 `hasMore=true`，此时仍应使用新的 `nextCursor` 继续加载；
- `hasMore=false` 或 `nextCursor` 为空时结束分页。

### 2.5 幂等变更

关注、取消关注、拉黑、取消拉黑、屏蔽、收藏和订阅等变更统一返回：

```json
{
  "code": 0,
  "data": {
    "changed": true
  },
  "msg": "成功"
}
```

`changed=false` 仍是成功，表示服务端本来已经处于目标状态。前端应按目标状态更新界面，不要把它显示成失败。

## 3. APP 接口清单

### 3.1 关注关系

| 方法 | 路径 | 请求 | `data` | 前端用途 |
|---|---|---|---|---|
| POST | `/follow` | `Target` | `Change` | 关注 |
| POST | `/unfollow` | `Target` | `Change` | 取消关注，同时关闭该目标的开播提醒 |
| GET | `/follow/status` | Query `Target` | `Relationship[]` | 单目标详情状态，返回数组 |
| POST | `/follow/batch-status` | `{"targets":[Target]}` | `Relationship[]` | 批量初始化列表按钮，最多 100 个目标 |
| GET | `/follow/counters` | Query `Target` | `Counters` | 查询关注数、粉丝数 |
| GET | `/following/list` | `cursor, limit` | `UserList` | 本人的关注列表 |
| GET | `/followers/list` | `cursor, limit` | `UserList` | 本人的粉丝列表 |
| GET | `/follow/mutual` | `cursor, limit` | `UserList` | 本人的互关列表 |
| GET | `/follow/common` | Query `Target + cursor + limit` | `UserList` | 本人与指定目标的共同关注 |

`Relationship` 字段：

```json
{
  "accountNo": "10000001",
  "anchorNo": "100001",
  "nickname": "主播昵称",
  "avatar": "https://example.com/avatar.png",
  "signature": "用户签名",
  "gender": 0,
  "countryCode": "MY",
  "isLive": true,
  "roomNo": "100001",
  "sessionNo": "202610270001",
  "following": true,
  "followsYou": false,
  "mutual": false,
  "blocked": false,
  "blockedByMe": false,
  "mutedScopes": [],
  "subscribed": false
}
```

- `following`：当前登录用户是否关注对方；
- `followsYou`：对方是否关注当前登录用户；
- `mutual`：双方是否互相关注；
- `blocked`：任意一方存在拉黑；
- `blockedByMe`：是否由当前登录用户主动拉黑；
- `mutedScopes`：当前登录用户对目标设置的屏蔽范围；
- `subscribed`：当前登录用户是否开启该主播的开播提醒偏好。

`Counters` 示例：

```json
{
  "accountNo": "10000001",
  "followingCount": 12,
  "fansCount": 98
}
```

账号封禁或注销后可能从可见列表中隐藏，但关系事实仍可能计入计数。因此不能用列表当前长度替代 `followingCount` 或 `fansCount`。

### 3.2 拉黑

| 方法 | 路径 | 请求 | `data` | 前端用途 |
|---|---|---|---|---|
| POST | `/block` | `Target` | `Change` | 拉黑目标 |
| POST | `/unblock` | `Target` | `Change` | 取消拉黑 |
| POST | `/block/batch-status` | `{"targets":[Target]}` | `Relationship[]` | 批量查询关系状态 |
| GET | `/block/list` | `cursor, limit` | `UserList` | 本人黑名单 |

拉黑会在同一事务内解除双方已有关注，并关闭双方开播提醒。取消拉黑不会自动恢复关注。前端在拉黑成功后应同步重置关注和提醒状态。

### 3.3 分范围屏蔽

| 方法 | 路径 | 请求 | `data` |
|---|---|---|---|
| POST | `/mute` | `Target + scope` | `Change` |
| POST | `/unmute` | `Target + scope` | `Change` |
| POST | `/mute/batch-status` | `{"targets":[Target]}` | `Relationship[]` |
| GET | `/mute/list` | `cursor, limit` | `UserList` |

`scope` 可选值：

- `content`：内容；
- `live_notification`：开播提醒；
- `recommendation`：推荐；
- `direct_message`：私信。

屏蔽不会解除关注，也不等同于拉黑。当前只有数据保存、查询及开播提醒过滤逻辑已具备；由于没有真实推送渠道，产品当前可以暂缓此部分 UI。

### 3.4 直播间收藏

| 方法 | 路径 | 请求 | `data` |
|---|---|---|---|
| POST | `/favorite` | `Resource` | `Change` |
| POST | `/unfavorite` | `Resource` | `Change` |
| POST | `/favorite/batch-status` | `{"resources":[Resource]}` | `FavoriteStatus[]` |

`FavoriteStatus`：

```json
{
  "resourceType": "live_room",
  "resourceId": "100001",
  "favorite": true
}
```

`GET /api/v1/app/social/favorite/list` 已删除，返回 HTTP 404，前端应移除收藏列表调用及对应入口。收藏、取消收藏和批量收藏状态接口继续保留；不可见资源的批量状态返回 `favorite=false`。

### 3.5 最近观看

| 方法 | 路径 | 请求 | `data` |
|---|---|---|---|
| POST | `/history/open` | `Resource + requestId` | `WatchSession` |
| POST | `/history/report` | `HistoryReport` | `WatchAccepted` |
| GET | `/history/list` | `cursor, limit, resourceType=live_room` | `HistoryUserList` |
| POST | `/history/progress/batch` | `{"resources":[Resource]}` | `ResourceList` |
| POST | `/history/delete` | `Resource` | `{}` |
| POST | `/history/clear` | 无请求体 | `{}` |

历史列表改为“最近观看的用户”，列表项是第 4 节的 `UserPublicProfile`，再增加记录字段：

```json
{
  "accountNo": "10000001",
  "nickname": "用户昵称",
  "avatar": "https://example.com/avatar.png",
  "signature": "用户签名",
  "gender": 0,
  "countryCode": "MY",
  "anchorNo": "100001",
  "isLive": false,
  "resourceType": "live_room",
  "resourceId": "100001",
  "createdAt": 1793000000000,
  "lastWatchedAt": 1793000100000
}
```

`history/list` 不再返回 `title/coverUrl/durationMs/positionMs/completed`，当前只支持 `live_room`（可省略该参数）。用户卡片用 `accountNo` 查看公开资料；删除记录仍传原 `resourceType/resourceId`。正在直播且有查看权限时会同时返回 `isLive=true`、`roomNo` 和 `sessionNo`。历史仍按原观看资源去重、按最近观看时间排序，并过滤不可用账号或当前无权访问的资源。

`history/progress/batch` 的 `ResourceList` 进度契约保持原样；它和历史展示列表用途不同，不应共用同一前端 DTO。

进入直播间时调用 `history/open`：

```json
{
  "resourceType": "live_room",
  "resourceId": "100001",
  "requestId": "3e7f4d50-4f04-4df7-82a9-4ac505fc1a01"
}
```

`requestId` 使用项目已有 `CreateUUID` 生成。响应：

```json
{
  "sessionToken": "d40ef8c0-304c-4dbf-8878-64a8d52cb0c7",
  "sessionVersion": 1793000000001,
  "epoch": 0,
  "positionMs": 0,
  "completed": false
}
```

`sessionVersion` 是服务端生成的递增版本，不保证从 1 开始，前端不能自行计算。上报时必须原样携带这三个会话字段，并在当前会话内递增 `sequence`：

```json
{
  "resourceType": "live_room",
  "resourceId": "100001",
  "sessionToken": "d40ef8c0-304c-4dbf-8878-64a8d52cb0c7",
  "sessionVersion": 1793000000001,
  "epoch": 0,
  "sequence": 1,
  "positionMs": 0,
  "completed": false,
  "checkpoint": false
}
```

直播间资源的 `positionMs` 固定使用 0，`completed` 固定使用 false。普通上报返回 `persisted=false` 表示已经接受并进入短期缓冲，不是失败；离开页面等需要确认落库的时机可发送 `checkpoint=true`。若返回 `46010`，说明其他设备开启了更新的观看会话，当前页面应重新调用 `history/open`，不要继续重放旧 Token。

### 3.6 关注主播的在线直播

| 方法 | 路径 | 请求 | `data` |
|---|---|---|---|
| GET | `/live/following-online` | `cursor, limit` | `OnlineList` |

列表项复用 `UserPublicProfile`，并保留直播间标题和封面：

```json
{
  "accountNo": "10000001",
  "nickname": "用户昵称",
  "avatar": "https://example.com/avatar.png",
  "signature": "用户签名",
  "gender": 0,
  "countryCode": "MY",
  "isLive": true,
  "roomNo": "100001",
  "anchorNo": "100001",
  "sessionNo": "202610250001",
  "title": "直播间标题",
  "coverUrl": "https://example.com/cover.png"
}
```

后端已经过滤非直播状态、不可用主播、双向拉黑以及当前用户无权访问的房间，前端点击后直接使用 `roomNo` 进入现有直播间详情流程。

### 3.7 开播提醒偏好

以下接口已经可调用，但当前没有实际消息投递渠道：

| 方法 | 路径 | 请求 | `data` |
|---|---|---|---|
| POST | `/live/subscribe` | `Target` | `Change` |
| POST | `/live/unsubscribe` | `Target` | `Change` |
| POST | `/live/subscription/batch-status` | `{"targets":[Target]}` | `Relationship[]` |
| GET | `/live/subscription/list` | `cursor, limit` | `UserList` |

开启提醒要求当前用户已经关注目标。取消关注会关闭提醒；重新关注不会强制重新开启用户曾主动关闭的提醒。产品当前不需要推送功能时，前端可以完全不接这组接口。

## 4. 用户公开资料统一结构

后端只定义一份 `UserPublicProfile`：`server/model/common/response/user_public_profile.go`。`live.AccountPublicProfileResp` 和 `social.User` 是其类型别名；关系、历史等 DTO 嵌入该类型，JSON 字段保持平铺。

```typescript
interface UserPublicProfile {
  accountNo: string
  nickname: string
  avatar: string
  signature: string
  gender: number
  countryCode: string
  anchorNo?: string
  isLive: boolean
  roomNo?: string
  sessionNo?: string
}
```

适用范围：用户公开详情 `GET /api/v1/app/live/user/public-profile?accountNo=...`（需要 APP Bearer）、直播间详情 `userInfo`、关系批量状态、关注/粉丝/互关/共同关注/黑名单/屏蔽/订阅列表、历史列表，以及关注中的在线主播列表。

- `nickname/avatar/signature/gender/countryCode` 来自用户账号公开资料，不能改用主播身份资料。
- `anchorNo` 仅在存在正常且审核通过的主播身份时返回。
- `isLive` 表示**当前观众可访问的有效直播**。离线、无权查看、房间/场次异常或主播不可开播时为 `false`，并省略 `roomNo/sessionNo`。不要将其解释为全平台无条件公开的在线状态。
- 公开资料不包含手机号、登录用户名、密码、实名、生日、内部主键或 Token。`following` 等关系状态由 `Relationship` 扩展，不属于公开资料。
- 关系 `UserList.list` 在公开资料之外有 `createdAt`，屏蔽列表额外有 `scope`，订阅列表额外有 `enabled`。
- `HistoryUserList.list` 在公开资料之外有 `resourceType/resourceId/createdAt/lastWatchedAt`。
- `OnlineList.list` 在公开资料之外有 `title/coverUrl`，`isLive` 为 `true`。
- 所有列表外层仍为 `{list,nextCursor,hasMore}`；时间均为毫秒。共同关注、互关采用有界候选过滤，空页也可能有下一页。

前端应定义上述公共类型，并通过交叉类型或继承增加各页面字段，避免维护多份用户资料定义。

## 5. 管理后台接口清单

| 方法 | 完整路径 | 请求 | `data` | 用途 |
|---|---|---|---|---|
| GET | `/api/v1/admin/social/counter/check` | Query `Target` | `CounterAudit` | 核对数据库计数与事实数量 |
| POST | `/api/v1/admin/social/counter/repair` | `Target + confirm + reason` | `CounterAudit` | 持锁重算计数 |
| GET | `/api/v1/admin/social/block/list` | Query `Target + cursor + limit` | `UserList` | 审计指定账号黑名单 |
| GET | `/api/v1/admin/social/relationship/check` | `cursor, limit` | `UserList` | 扫描异常关系 |
| GET | `/api/v1/admin/social/event/list` | `cursor, limit` | `EventList` | 查看事件状态 |
| POST | `/api/v1/admin/social/event/replay` | `eventId, confirm, reason` | `Change` | 重放 failed 事件 |
| GET | `/api/v1/admin/social/health` | 无 | 动态健康数据 | 查看积压、指标和渠道状态 |

修复请求必须明确填写：

```json
{
  "accountNo": "10000001",
  "confirm": "REPAIR",
  "reason": "历史数据核对后修复"
}
```

事件重放必须明确填写：

```json
{
  "eventId": "d40ef8c0-304c-4dbf-8878-64a8d52cb0c7",
  "confirm": "REPLAY",
  "reason": "依赖恢复后人工重放"
}
```

`CounterAudit`：

```json
{
  "accountNo": "10000001",
  "followingCount": 12,
  "fansCount": 98,
  "actualFollowing": 12,
  "actualFans": 98,
  "mismatch": false
}
```

`EventList.list` 中包含 `eventId`、`eventType`、`status`、`createdAt`、`attempts`、`nextAttemptAt` 和脱敏后的 `lastError`。状态值为 `pending`、`done`、`failed`、`skipped`。只有 `failed` 事件允许重放。

健康接口的 `events[]` 每种状态最多统计 1000 条；`countIsLowerBound=true` 表示总量大于 1000，界面显示“1000+”，不能当作精确总数。`sampleOldestCreatedAt` 只是样本最早时间，不能当作全局积压最早时间。此设计避免健康轮询触发全表计数。

健康接口中的 `deliveryChannelConfigured=false` 是当前没有真实推送渠道的正常状态，不应在用户端显示为系统故障。管理页面可将它标记为“未配置”。

## 6. 错误码与前端处理建议

| code | 含义 | 建议处理 |
|---:|---|---|
| 0 | 成功 | 使用 `data` 更新页面 |
| 7 / HTTP 401 | 未登录、Token 无效或普通业务失败 | 401 进入现有重新登录流程；code 7 展示 `msg` |
| 46001 | 参数错误 | 停止重试并检查前端参数 |
| 46002 | 账号不可用 | 刷新页面或移除对应用户项 |
| 46003 | 不能对自己执行该操作 | 不展示对应操作入口 |
| 46004 | 存在拉黑冲突 | 刷新关系状态，提示先取消拉黑 |
| 46005 | 需要先关注 | 关闭提醒开关并提示先关注 |
| 46006 | 资源不存在或当前无权访问 | 从当前列表移除或返回上一页 |
| 46007 | 资源类型尚未接入 | 当前不要提交短视频或回放类型 |
| 46008 | 限流或系统背压 | 禁止立即循环重试，稍后再试 |
| 46009 | Redis、数据库等依赖暂不可用 | 保留当前 UI 状态并提示稍后重试 |
| 46010 | 观看会话冲突 | 重新调用 `history/open` 建立新会话 |

Worker 不健康或积压触发背压时，拉黑、取消关注/拉黑/屏蔽/订阅/收藏、删除/清空历史仍可执行，但仍受 Redis 写限流约束；Redis 不可用时写操作继续拒绝。

客户端限额为每个用户每 60 秒读请求 600 次、写请求 60 次；批量请求最多 100 项，请求正文最大 32 KiB。列表页应优先使用批量状态接口，避免逐卡片请求。

## 7. 前端实现建议

如果在当前仓库内实现前端，建议：

1. API 封装放在 `web/src/api/`，文件使用 `kebab-case`，请求统一复用 `@/utils/request.js`；
2. APP 请求封装显式添加 `Authorization: Bearer <APP token>`，不要复用管理后台的 `x-token`；
3. 使用 `web/src/utils/format.js` 已有的 `CreateUUID` 生成观看 `requestId`；
4. 关系卡片列表先收集最多 100 个目标，再调用批量状态接口，不要产生 N+1 请求；
5. 变更接口成功后以目标状态更新本地数据，同时后台刷新计数；
6. 游标只作为不透明字符串保存，列表筛选切换时清空旧游标；
7. 对 `46008`、`46009` 使用有上限的延迟重试，不做无限重试；
8. 所有公开编号在 TypeScript/JavaScript 中保持字符串，毫秒时间使用项目已有日期工具展示。

建议的第一期页面范围：

- 用户/主播详情的关注按钮、粉丝数和关注数；
- 关注、粉丝、互关、共同关注和黑名单列表；
- 直播间收藏按钮；
- 最近观看列表、单条删除和清空；
- “关注的主播正在直播”列表。

开播提醒、短视频、回放以及受保护媒体播放不进入当前前端一期范围。

## 8. 联调前置条件

开始联调前由后端确认：

- 后端以 `disable-auto-migrate: false` 启动，并已由 GORM 自动创建或更新社交表；
- Redis 可用，社交 Worker 会随服务自动启动；
- MySQL 和 Redis 可用；
- 联调账号状态正常，并准备至少两个 APP 账号及一个已审核主播；
- Swagger 中可以看到 `SocialApp` 和 `SocialAdmin` 分组。

前端不需要执行数据库迁移，也不需要启动推送渠道、CDN 或媒体网关来联调本期社交页面。

