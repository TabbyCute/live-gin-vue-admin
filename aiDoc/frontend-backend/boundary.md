# 前后端边界说明

## 归属边界

- 后端负责路由、参数校验、业务逻辑和响应结构
- 前端负责页面流程、交互体验、本地状态和展示层
- 共同行为通过明确的 API 契约协作，不通过隐式约定耦合

## 契约规则

- 保持统一响应结构：`{ code, data, msg }`
- 保持统一分页结构：`{ page, pageSize, total, list }`
- 字段名不要随意漂移
- 前后端字段类型必须保持一致
- 后端必须提供完整而准确的 Swagger 接口说明
- 前端接口调用应以实际 Swagger 与后端实现为准

## 变更规则

- 涉及破坏性接口调整时，要先写清楚变更范围
- Swagger 或其他接口说明必须与真实实现一致
- 前端接口封装应继续放在 `web/src/api/` 或 `web/src/plugin/<name>/api/`
- 可复用逻辑优先复用 `web/src/utils/` 现有能力

## 完成前检查

跨前后端改动结束前，至少确认以下几点：

1. 后端响应结构仍然满足前端预期
2. 前端仍在使用正确的字段名和数据类型
3. 若契约发生了长期变化，对应说明已经补到 `aiDoc/`

## 客户端账号鉴权契约

- 客户端账号接口前缀为 `/api/v1/app/live/user`，兼容版本前缀为 `/api/v2/app/live/user`。
- 注册、登录使用 JSON body；登录成功后在统一响应的 `data.token` 中返回客户端 access token。
- 客户端不使用 Cookie，只通过 `Authorization: Bearer <token>` 访问受保护接口。
- 客户端 token 不得写入或复用管理后台的 `x-token` Header/Cookie。
- 客户端账号、JWT 密钥、签发者、Claims 和鉴权中间件均与管理后台独立。
- 登录响应的 `expiresAt` 是毫秒时间戳；账号响应字段为 `id`、`username`、`nickname`、`avatar`、`status`、`createdAt`。

## 直播间与场次契约

- 客户端直播接口前缀为 `/api/v1/app/live/room`；本人操作使用 APP Bearer Token，公开直播列表和详情不要求登录。
- 客户端不得接收或提交直播间、场次、主播的数据库主键，只使用 `roomNo`、`sessionNo`、`anchorNo`。
- 后台直播间和场次接口前缀分别为 `/api/v1/admin/live/room`、`/api/v1/admin/live/session`，后台可以使用内部主键执行精确操作。
- SRS/可信统计接口前缀为 `/api/v1/app/live/hook`，由部署层真实来源 IP/CIDR 白名单保护，不使用 APP 或后台 JWT；`publish` 还会解密并完整校验 pt。
- 所有直播业务时间戳均为毫秒；直播时长字段为 `durationMs` / `totalLiveDurationMs`。
- 场次状态值固定为：`0`准备中、`1`直播中、`2`结束中、`3`已结束、`4`已取消、`5`失败。
- 房间状态与直播状态是两个独立字段：`status` 表示是否允许使用房间，`liveStatus` 表示当前运行阶段。
- `publishToken` 是 `v1_` 开头的加密 pt，只在准备开播响应中返回一次，前端不得持久化展示；服务端只保存最终 Token 的哈希。
- 主播端 `POST /api/v1/app/live/room/session/end` 不传请求体或 `sessionNo`；服务端根据 APP JWT 结束当前登录主播的活动场次。
- 礼物统计字段 `giftCount`、`giftCoinAmount`、`giftUserCount` 是汇总快照，不代替礼物和钱包流水。
- 公开详情 `GET /api/v1/app/live/room/detail` 始终返回房间资料和完整 `anchorInfo`；`latestSession` 有活动场次时表示当前场次，否则表示最近一次已结束直播，从未完成直播时固定返回空对象 `{}`。
- 公开详情同时返回主播关联账号的公开 `userInfo`；`userInfo.accountNo` 是关注、拉黑、屏蔽和开播订阅的首选目标编号。`anchorInfo.anchorNo` 仍可作为社交目标兼容入口，两者最终解析到同一个 `live_account.id`，不得形成两套关系。
- `userInfo` 不承载当前观众的关系状态。登录用户进入直播间后使用 `GET /api/v1/app/social/follow/status?accountNo=<userInfo.accountNo>` 初始化关注等状态；匿名用户不调用受保护的社交状态接口。
- 公开详情不返回房间/场次内部主键、房间状态原因、断流重连控制字段、失败原因、结算时间或礼物金额。

## 社交关系契约

- APP `/api/v1/app/social` 使用独立APP Bearer，操作者只取JWT；目标使用accountNo/anchorNo二选一，资源使用公开编号。
- 公开用户资料唯一结构为 `model/common/response.UserPublicProfile`；live/social使用类型别名及嵌入复用，JSON字段平铺。账号字段为accountNo/nickname/avatar/signature/gender/countryCode，主播身份anchorNo可选；isLive表示当前观众可访问的有效直播，只有true时提供roomNo/sessionNo。禁止返回账号实体或私密字段。
- 关注/粉丝/互关/共同关注/黑名单/屏蔽/订阅、关系批量状态、历史、关注中在线主播、用户详情和房间详情userInfo共用该结构。历史list新增资源键及观看时间，移除旧房间卡片/进度字段；progress/batch保持原契约。
- 2026-10-27删除GET `/api/v1/app/social/favorite/list`（404），其余收藏操作保留；前端应移除收藏列表入口。
- 新列表采用limit/cursor/nextCursor/hasMore；过滤后空页仍可能有nextCursor。旧PageInfo接口不改为游标。
- 所有新社交时间与进度为毫秒；业务错误46001—46010，统一code/data/msg。
- 管理接口`/api/v1/admin/social`同时校验后台JWT与角色888，写修复需确认词及原因。
- 仅在受保护媒体网关就绪时prepare的旧playUrl为空，公开房间未启用媒体网关时沿用现有播放链路；受保护HLS需先调用现有room/playback，并为playlist/segment附加Authorization与X-Playback-Token。私密和仅关注者开播在媒体网关验收前禁止。
- 当前生产资源适配仅live_room；short_video/live_replay明确返回未接入。通知偏好可管理，实际投递渠道尚未注册。
- 管理health每状态count最多1000；countIsLowerBound=true表示总数大于1000，sampleOldestCreatedAt仅为样本时间。
- 完整契约与部署边界见docs/social/README.md、api.md及production-review-20261027.md。

## 钱包账务契约

- APP `/api/v1/app/wallet`仅本人Bearer，后台`/api/v1/admin/wallet`校验后台JWT及当前数据库角色；默认888，申请与审核必须不同管理员。
- 金额、版本、后台管理员ID为十进制字符串；毫秒时间戳为数字。当前币种仅COIN，不定义兑换率，不提供消费/支付接口。
- 订单列表沿用page/pageSize/total/list；不可变流水采用limit/cursor/nextCursor/hasMore，按钱包版本游标避免大偏移扫描。
- 创建申请和冲正使用稳定requestId；approve成功才变余额；超时复用原参数重试。冲正仅创建待审核申请，不直接扣减。
- APP流水隐藏原因、凭证和管理员。完整契约见`docs/wallet/api.md`，业务错误47001—47015。

- 系统人工充值创建必须带externalOrderNo及evidenceRef，sourceType仅manual（默认）；凭证统一大写、跨管理员永久唯一。订单DTO增加sourceType/externalOrderNo，列表支持精确凭证检索；状态接口不再隐式开户。
