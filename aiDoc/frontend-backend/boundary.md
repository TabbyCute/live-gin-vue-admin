# 前后端边界说明

2026-11-04 guard/ranking 新契约见 `docs/guard/api.md`、`docs/ranking/api.md`：APP 只接收公开编号、公共用户 DTO 和字符串金额/排名；守护剩余时间由服务端计算。后台操作沿用永久 requestId、明确确认和服务端 actions，菜单不代替授权；不得前端修改排行榜分数或会员到期时间。礼物配置守护方案必须经后端就绪校验。

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
- 管理接口`/api/v1/admin/social`同时校验后台JWT、角色888及数据库当前账号可用性/角色归属，写修复需确认词及原因。
- 2026-10-30加固不改变DTO：JSON需完整单对象且不超过32KiB，query4KiB、Authorization8KiB上限；响应no-store。首次历史心跳/会话切换/缓冲过期校验本人数据库会话，失败46010。连续已验证心跳只聚合Redis，persisted=false不是持久化承诺；限流/背压失败需退避。
- 仅在受保护媒体网关就绪时prepare的旧playUrl为空，公开房间未启用媒体网关时沿用现有播放链路；受保护HLS需先调用现有room/playback，并为playlist/segment附加Authorization与X-Playback-Token。私密和仅关注者开播在媒体网关验收前禁止。
- 当前生产资源适配仅live_room；short_video/live_replay明确返回未接入。通知偏好可管理，实际投递渠道尚未注册。
- 管理health每状态count最多1000；countIsLowerBound=true表示总数大于1000，sampleOldestCreatedAt仅为样本时间。
- 完整契约与部署边界见docs/social/README.md、api.md及production-review-20261030.md。

## 钱包账务契约

- APP `/api/v1/app/wallet`仅本人Bearer，后台`/api/v1/admin/wallet`校验后台JWT及当前数据库身份；普通管理员默认888且申请与审核必须不同管理员，超级 root（sys_users.ID=1）免角色授权并可自审。
- 金额、版本、后台管理员ID为十进制字符串；毫秒时间戳为数字。当前币种仅COIN，不定义兑换率，不提供消费/支付接口。
- 订单列表沿用page/pageSize/total/list；不可变流水采用limit/cursor/nextCursor/hasMore，按钱包版本游标避免大偏移扫描。
- 创建申请和冲正使用稳定requestId；approve成功才变余额；超时复用原参数重试。冲正仅创建待审核申请，不直接扣减。
- APP流水隐藏原因、凭证和管理员。完整契约见`docs/wallet/api.md`，业务错误47001—47015。

- 系统人工充值创建必须带externalOrderNo及evidenceRef，sourceType仅manual（默认）；凭证统一大写、跨管理员永久唯一。订单DTO增加sourceType/externalOrderNo，列表支持精确凭证检索；状态接口不再隐式开户。

## 礼物接口（2026-10-31）

APP /api/v1/app/gift 使用独立JWT，送礼仅roomNo/giftNo/quantity/requestId；金额、数量、万分比、版本、total均为十进制字符串。后台 /api/v1/admin/gift 以精确操作权限鉴权。原SRS礼物数值字段不再公开，新giftSummary字符串统计且独立授权。完整契约见docs/gift/api.md。

2026-11-02：后台giftSummary新增grossQuantity（赠送总件数）、refundQuantity（退款件数），quantity保持扣退款后有效件数；grossCoin/refundCoin/netCoin为总价值/退款/有效价值COIN，均为十进制字符串。场次列表一页一次批量聚合，无权限或模块未就绪时省略giftSummary，不能按0展示。旧pendingEarningCoin字段保持兼容，实际为按订单分成计算的净收益（含已结算），后台改名为“主播礼物分成（含已结算）”；真正待结算余额使用earning/overview.pendingCoin。公开APP房间/场次DTO不新增这些后台字段。

2026-10-31复核修订：APP送礼/结果/历史仅返回AppOrder，隐藏内部钱包编号、分成/收入和权益内部快照，客服凭orderNo或requestId+账号核验；主播自送48012。rule/list增加termination对象，提前终止申请和复核分别授权，原规则status保持历史审核状态。草稿礼物可暂缺图标，启用必须真实HTTPS。客户端完整字段、精度与响应丢失恢复流程见docs/gift/frontend-client-integration.md。

## 主播收益与提现审核（2026-11-01）

APP `/api/v1/app/earning` 全部本人Bearer，后台 `/api/v1/admin/earning` 精确操作授权。金额/版本/total字符串，时间毫秒数字。APP仅安全DTO；客服保留settlementNo/withdrawalNo/requestId。提现详情支持no/requestId二选一以恢复响应丢失；原键原参数重试才可防重。

2026-11-02：主播列表与收益结算共用创建弹窗，主播入口固定anchorNo；兑换规则沿用exchange/list分页下拉，要求查询权限，提交仍需settlement/create权限。默认结束为当前整秒、开始为其前7天，UI和后端拒绝未来范围；时间线按收益事实created_at左闭右开，不按场次结束时间。创建不自动批准，金额不由前端提交。

提现pending_review→approved/rejected，approved仅由可信人工出款事务进入paid/cancelled。申请无全局开关，符合本人主播、运营提现权限、风险和账务条件时默认可用，不判断实名认证或主播认证；APP提现只传requestId/currency/amount/payeeAccountNo，不得在提现请求上传完整收款资料。approved继续冻结，不等于已打款；payoutEnabled固定false保留自动外部支付未接入的兼容语义，后台仍可提交打款中心创建ready订单。新单使用payout数据库账户不可变脱敏快照；旧“待出款中心核验”单保留且禁止出款，人工领取前再次核验账户状态及版本。独立结算币种不能与COIN混算，欠额抵扣优先，规则无默认值。客户端文档docs/earning/frontend-client-integration.md；运营/扩展/部署边界见docs/earning/README.md。

## 后台超级 root 契约（2026-11-01）

- 后台 `sys_users.ID=1` 是固定超级 root；身份来自后台登录 JWT，不依赖角色 ID、角色归属、菜单分配或操作权限表。APP 账号 ID=1 不获得后台权限。
- root 默认获得全部已注册菜单与按钮，包括后续新增菜单；严格树形角色模式不限制 root 的角色查询、菜单分配和用户角色设置。客户数据及 AI 工作流会话管理不受角色/所有者范围限制。
- 钱包、礼物、社交和收益管理均允许 root 访问；充值/冲正、分成规则/提前终止、兑换规则/结算均支持 root 自行复核，无需另一位管理员。仍使用现有创建和审核接口，审核决定、原因、幂等键及真实操作人照常记录。
- 登录身份验证、账号有效性、参数校验、业务状态机、账务一致性、金额限制及未接入功能的关闭状态继续生效；这次修改仅豁免权限分配和多人限制。普通管理员维持原权限规则。
- 旧库须使用新版礼物及收益迁移命令升级四个审核触发器，再启动新版服务。迁移先安装新约束，再删除内容精确匹配的旧约束，可重复执行；不更改历史业务数据。

## 主播收款账户契约

APP `/api/v1/app/payout/payee`提供list/create/set-default/disable；仅APP本人身份，拒绝客户端ownerNo/anchorNo。敏感收款资料仅create写入，采用AES-GCM；DTO只允许payeeAccountNo/method/currency/maskedAccount/status/version/isDefault/createdAt/updatedAt。active代表绑定，不是渠道认证。提现新增必填payeeAccountNo，重试四项参数固定；后台详情新增currentPayee及后台withdrawal/list支持payeeAccountNo精确筛选。完整变更及旧客户端兼容见docs/payout/frontend-integration.md。

后台`/api/v1/admin/payout/order/create|list|detail`使用x-token和独立金融精确权限。创建只传requestId/withdrawalNo，返回唯一ready订单；同键异提现冲突、同提现异键返回原单并永久占用键。列表分页total/金额/创建人/版本字符串，时间毫秒，详情常规字段脱敏，另以精确权限开放人工操作和一次性明文查看。后台withdrawal投影新增payoutNo/payoutStatus，APP结构不变。二次确认后仍冻结；禁止客户端提交金额/账户/版本/状态；独立复核成功必须走受控人工出款接口，不能修改订单状态字段。独立一级打款中心菜单，当前契约见docs/payout/manual-frontend-integration.md。

2026-11-03人工出款：payout七状态与attempt六状态，23项新增精确能力默认不授予普通角色，ID1同人例外明确审计。公司账户、流水、私有凭证、调查、复核及内部对账由后台操作；APP继续仅本人安全DTO并增加paid/cancelled状态接受与paid余额展示。敏感查看是45秒单次授权，前端30秒清空；证据走授权POST二进制读取，无公共URL。所有写操作requestId永久绑定，未知网络结果原参数重试，49201确定参数失败允许修正后新键重发。历史记录不可覆盖、paid成功与取消释放必须原子入账，unknown不可重试或释放。
