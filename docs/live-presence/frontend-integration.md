# 前端对接与既有功能兼容

## 已修改的前端

- 当前仓库 `web/src/view/live/room/index.vue`、`session/index.vue`：按当前页批量查询统计，每 10 秒轮询；离开/隐藏页面暂停，迟到响应不覆盖新页面。房间与场次原有状态字段继续使用。
- `web/src/view/live/presence/PresencePanel.vue`：房间当前场次及场次详情的只读统计和运行状态。
- 实际观看端 `/Users/tabby/Desktop/IM/H5Player`：`src/modules/module_live/pages/live/live_room.vue`、`presence/controller.js`、`presence/api.js`；已接进房、播放/可见性心跳、退出、账号切换、断线恢复、点赞、真实观众资料与人数。
- `/Users/tabby/Desktop/IM/codes/web_live` 不属于这条观看链路，未重复接入。

现有送礼、钱包、收益、提现、守护、榜单及关注接口不变；presence 不接受礼物金额、余额、主播编号或客户端自报观看时长。播放地址仍由原有详情/权限链路负责，统计故障不会由本模块强制停止已获合法地址的播放器。

## 路径与身份

以下路径包含当前 `/api` 前缀；若部署更改 router-prefix，以部署配置为准。统一响应 `{ code, data, msg }`，成功 `code=0`。同时检查 HTTP 状态和业务码，不能将所有 HTTP 200 当作业务成功。所有统计响应禁止缓存。

会员使用 APP `Authorization: Bearer <appJWT>`。游客先 POST `/api/v1/app/live/presence/guest`，正文必须为 `{}`，取得 `guestToken` 后放在 `X-Presence-Guest`。游客身份有效 24 小时，只能进入允许游客访问的公开直播；游客不能点赞。JWT 错误不会自动降级为游客。不要混用后台 `x-token` 与 APP JWT。既有 Origin 白名单范围内已支持 X-Presence-Guest 预检，并通过 CORS 暴露 Retry-After；不需要放宽 Origin。

新接口严格校验正文：字段名称必须精确、所有 DTO 字段必填、拒绝重复键/未知键/null/尾随数据；正文最多 4 KiB，query 最多 1 KiB。Token 只存在内存，不能放 URL、埋点、错误上报或长期本地存储。

| 方法与路径（`/api/v1/app/live` 后缀） | 请求 | 说明 |
| --- | --- | --- |
| POST `/presence/guest` | `{}` | 签名游客身份 |
| POST `/presence/open` | `roomNo, requestId` | 成功取得直播详情和访问权限后调用 |
| POST `/presence/heartbeat` | `presenceToken, sequence, playing, visible` | 服务端返回心跳建议间隔 |
| POST `/presence/leave` | `presenceToken, sequence, reason` | 尽力发送，TTL 是异常退出兜底 |
| POST `/room/like` | `presenceToken, requestId, sequence, count` | 仅会员，有效在线连接必需 |
| GET `/presence/stats` | `roomNo` | 查询近实时统计 |
| GET `/presence/viewers` | `roomNo, cursor?, limit?` | 在线会员公开资料 |
| GET `/presence/active-viewers` | `roomNo, cursor?, limit?` | 当前在线会员近期活动 |

`requestId` 为 8～64 位字母、数字、下划线或连字符；重试必须使用原键原参数。进房新键创建新连接，同键返回同一连接；已过期连接需要新键重新进房。`presenceToken` 绑定身份、房间、场次、连接和恢复代际。

`sequence` 和 `count` 是规范十进制正整数字符串，禁止小数、指数、前导零。心跳与退房共享递增序列，从 `"1"` 开始；点赞使用独立递增序列。序列不超过 `9007199254740991`，前端使用 BigInt。重试同一心跳必须保持相同序号和 playing/visible，重复心跳不续期、不重复计时；乱序或同序号不同参数拒绝。

```json
{"roomNo":"目标房间编号","requestId":"open_唯一请求编号"}
```
```json
{"presenceToken":"内存中的凭证","sequence":"1","playing":true,"visible":true}
```
```json
{"presenceToken":"内存中的凭证","requestId":"like_唯一请求编号","sequence":"1","count":"5"}
```

退房 reason 仅支持 `user_leave / room_change / account_change / page_hidden / page_close`。同一账号其他设备的连接不受影响。重复关闭返回已关闭状态，不重复扣人数。关闭页面请求可能丢失，不能据此认定服务端立即离线。

## 响应数据口径

所有新增数量和累计时长均为十进制**字符串**；时间戳为毫秒数字；开关为 bool。不得 Number/parseInt 转换累计大整数。原有 live DTO 未整体改类型，以各自原契约为准。

| 字段 | 含义 |
| --- | --- |
| `memberCount` / `onlineCount` | 当前有效连接中的去重登录账号数，两字段相等 |
| `guestCount` | 当前有效签名游客身份数 |
| `totalOnlineCount` | 会员数 + 游客数 |
| `connectionCount` | 有效连接数，多个设备分别计数 |
| `peakOnlineCount` | 本场会员与游客合计在线峰值 |
| `viewCount` | 本场成功创建连接次数；同键重试不增加 |
| `viewerCount` | 本场累计去重身份数，包含游客；游客转会员会成为另一身份 |
| `likeCount` | 服务端实际接受的点赞总数 |
| `totalWatchDurationMs` | 去重身份时长之和，含游客；不等于连接时长简单求和 |
| `updatedAt / checkpointAt / finalizedAt` | 读取时间、成功落库时间、最终汇总时间，0 表示尚无 |
| `stale` | 在线状态不可确认；在线字段可能是 `""`，必须显示“—/未知” |
| `dataLost` | 此场曾发生 Redis 状态丢失，恢复后仍保留此标记 |
| `pendingCount / cleanupLagMs` | 待落库记录与事件总量、最早过期连接的清理延迟 |
| `workerHealthy` | 最近成功检查点在 60 秒内；尚未建立检查点或已最终结束不据此单独告警 |

open/heartbeat/leave/like 另返回 `connectionNo, heartbeatIntervalSeconds, expiresInSeconds, connectionDurationMs, viewerSessionDurationMs, closed`；open 返回 `presenceToken`；like 返回 `acceptedCount`。closed=true 时结束该会话。默认 15 秒心跳、45 秒 TTL，使用响应值而非前端写死。

列表返回 `{list:[{user,lastActiveAt,lastActivityType}],nextCursor,hasMore}`。user 复用现有 UserPublicProfile，游客不出现在资料列表中。列表不含 IP、设备、presenceToken、connectionNo、订单或资金信息。游标不透明且绑定场次代际；最多 100 条、默认 20 条。排序和在线状态会随活动更新，翻页允许最终一致，客户端按账号去重；被过滤时当前页可以为空但 hasMore=true。活跃类型可能为 entered/heartbeat/liked/follow/gift，不代表财富榜。

## 退避与前端状态机

| HTTP / 业务码 | 行为 |
| --- | --- |
| 400 / 49501 | 修正参数，不无限重试 |
| 403 / 49502 | 清理旧连接，新 requestId 重新 open |
| 403 / 49503，或 JWT 401 | 停止上报，刷新身份/房间权限；不要匿名绕过 |
| 400 / 49504 | 幂等参数或序列冲突，检查重复控制器；不要盲目加序号补记未知结果 |
| 429 / 49505、49507 | 按 Retry-After 退避；后者也可能是 Worker/积压背压 |
| 503 / 49506、49508 | 依赖不可用或状态损坏；指数退避，继续原有合法播放链路 |

H5 控制器每次只执行一个会话修改请求，原键原序列重试，2～60 秒抖动退避，并尊重 Retry-After；点赞队列最多 100，按最多 10 个聚合，避免每点一次请求一次。默认账号限额 10/秒、120/分钟，单次协议上限 20，但不能超过实际限流配额。失败不凭本地点击数增加服务端点赞总数。切房、切账号、卸载、pagehide 会尽力结束旧连接，旧异步 open 的响应不能覆盖新连接。

## social history 与旧客户端

旧 `/social/history/open|report` 保留原语义。新版直播页移除独立的 history 心跳，由 presence 内部按确定键打开最近观看，并在检查点按原 history token/version/epoch 更新最近观看时间；不另写观看秒数，不复用两套 sequence。用户删除历史后，不会被旧 checkpoint 重新创建。其他页面原有 history 使用不变。

旧客户端未接 presence 时不会被凭 SRS 或旧 history 推算成在线观众。因此灰度发布期间新统计仅覆盖已接入客户端。不要同时启用旧直播 history 报告器和新版 presence 控制器。

旧 `/api/v1/app/live/hook/session/stats` 认证成功后固定返回 HTTP 410 / 49509；不再允许外部覆盖观众及礼物统计，部署时停止旧统计推送。SRS publish/unpublish 等生命周期 Hook 不受影响。

## 后台接口

POST `/api/v1/admin/live/presence/stats`，后台 `x-token`，正文例：

```json
{"roomNos":["房间编号"],"sessionNos":[]}
```

两个数组都必须提供，合计 1～100 个；返回 data 数组，通过 roomNo/sessionNo 建索引，不依赖返回顺序。当前后台使用共享 request 封装的 `/live/presence/stats` 相对路径，基础前缀由该封装处理。

仅固定后台 root 或当前有效角色 888 可读，菜单显示不能代替服务端权限。已有房间无当前场次返回当前在线 0；不存在的房间不制造占位记录；Redis 故障时保留已落库累计值，同时将当前在线标记未知。后台不提供任意改统计、改资金或查看网络标识的接口。
