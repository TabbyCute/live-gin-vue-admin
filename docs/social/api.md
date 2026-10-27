# 社交接口契约

APP前缀 `/api/v1/app/social`，仅 GET/POST，Authorization Bearer；操作者没有请求 userId 参数。accountNo、anchorNo 二选一。批量 targets 最多100个，内部去重；资源参数为 `{resourceType:"live_room",resourceId:"公开roomNo"}`。正文32KiB；游标 limit 默认20、最大100，nextCursor为空代表结束。过滤不可见资源可能产生空页但 nextCursor 非空。

| 方法 | 路径 | 参数 / 结果 |
|---|---|---|
| POST | follow、unfollow、block、unblock | Target / changed |
| GET | follow/status | Target / 当前双方关系数组 |
| POST | follow/batch-status、block/batch-status、mute/batch-status、live/subscription/batch-status | targets / 完整双方关系数组 |
| GET | following/list、followers/list、follow/mutual、block/list、mute/list、live/subscription/list | cursor/limit / 本人关系列表 |
| GET | follow/common | Target + cursor/limit / 本人与目标的共同关注 |
| GET | follow/counters | Target / 全部事实关注数、粉丝数 |
| POST | mute、unmute | Target + scope / changed |
| POST | favorite、unfavorite | Resource / changed |
| POST | favorite/batch-status | resources / 收藏状态 |
| POST | history/open | Resource + requestId / sessionToken、sessionVersion、epoch、进度 |
| POST | history/report | Resource + sessionToken/version/epoch/sequence/positionMs/completed/checkpoint / accepted、persisted |
| GET | history/list | cursor/limit/resourceType（默认且仅支持live_room） / HistoryUserList：公开用户资料及直播状态、原资源键、最近观看时间 |
| POST | history/progress/batch | resources / 已持久化进度；单对象也使用一个元素的数组 |
| POST | history/delete | Resource / 空对象 |
| POST | history/clear | 无正文 / 空对象 |
| POST | live/subscribe、live/unsubscribe | Target / changed |
| GET | live/following-online | cursor/limit / 当前可访问的直播中主播 |

2026-10-27 契约调整：`GET favorite/list` 已删除（404）；收藏/取消/批量状态保留。历史列表展示用户，不再返回房间标题、封面或进度；`history/progress/batch` 保留原进度结构。

统一公开资料为 `model/common/response.UserPublicProfile`：`accountNo/nickname/avatar/signature/gender/countryCode/anchorNo?/isLive/roomNo?/sessionNo?`。关注、粉丝、黑名单、屏蔽、订阅、互关、共同关注、用户详情、房间详情userInfo、历史及关注中在线主播均复用该类型。字段平铺；只有当前观众可访问的有效直播才返回 `isLive=true` 和 `roomNo/sessionNo`。离线或无权访问时不暴露直播入口。

历史 `HistoryUserItem` 额外返回 `resourceType/resourceId/createdAt/lastWatchedAt`，通过资源键删除记录。在线 `OnlineRoom` 额外返回 `title/coverUrl`。更多完整示例见 `frontend-integration.md`。

关系返回只含公开资料与本人相关标志：following、followsYou、mutual、blocked、blockedByMe、mutedScopes、subscribed。blocked表示任意方向，未把无权访问账号的资料塞入DTO。已关闭/封禁账号可能从批量结果省略。

后台前缀 `/api/v1/admin/social`：全部后台JWT + 角色888 + 操作审计，普通管理员不能调用。

| 方法 | 路径 | 规则 |
|---|---|---|
| GET | counter/check | 单账号锁内核对，返回修复前事实差异 |
| POST | counter/repair | Target、confirm=REPAIR、reason至少4字符；同事务修复和审计事件 |
| GET | relationship/check | 游标扫描拉黑/关注并存异常；供进一步按账号核对 |
| GET | block/list | Target + cursor/limit，授权查看指定用户黑名单 |
| GET | event/list | 有界浏览事件状态、失败次数与脱敏错误码 |
| POST | event/replay | eventId、confirm=REPLAY、reason；只重放failed |
| GET | health | 每状态最多1000条样本数量、countIsLowerBound、sampleOldestCreatedAt、实例指标、渠道配置状态 |

| 主体 | 本人私有列表 | 查看普通公开计数 | 更改他人关系 | 管理核对/修复 | 观看私密直播 |
|---|---|---|---|---|---|
| 未登录 | 否 | 否（社交端点需登录） | 否 | 否 | 否 |
| 正常APP账号 | 是 | 是 | 否 | 否 | 仅主播本人 |
| 封禁/注销APP账号 | 否 | 否 | 否 | 否 | 否 |
| 普通后台角色 | 否 | 依既有后台 | 否 | 否 | 无播放特权 |
| 后台888 | 审计黑名单 | 是 | 无代操作接口 | 是 | 无播放特权 |

健康状态 `countIsLowerBound=true` 表示该状态总数大于1000，`sampleOldestCreatedAt` 只是样本最早时间。共同关注、互关、异常关系扫描先取最多limit个候选再过滤，允许空页但带nextCursor。

错误码：0成功；46001参数；46002账号不可用；46003自己；46004拉黑冲突；46005需关注；46006资源不可访问；46007类型未接入；46008限流/背压；46009依赖不可用；46010观看会话冲突。账号鉴权沿用401及原响应。其他业务错误不返回数据库原文。

媒体接口在现有 `/api/v1/app/live/room`：GET playback签发HLS凭证；GET playback/check供代理逐请求检查，后者遵循auth_request的204/403协议，不是普通业务JSON接口。完整参数和DTO以生成的 Swagger 为准。

关注例子：

```json
{"accountNo":"10000001"}
```

幂等返回例子：

```json
{"code":0,"data":{"changed":false},"msg":"成功"}
```

观看协议：先open，心跳复用响应里的token/version/epoch，sequence在当前会话内递增；重新open后使用新的服务端版本。positionMs可回退；直播不接受点播进度语义。checkpoint成功才可认为已落库。分页、收藏和进度响应不暴露live_room/live_session/live_anchor内部主键。
