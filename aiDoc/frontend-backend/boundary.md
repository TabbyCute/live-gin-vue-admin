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
- SRS/可信统计接口前缀为 `/api/v1/app/live/hook`，必须使用 `X-Live-Hook-Token`，不使用 APP 或后台 JWT。
- 所有直播业务时间戳均为毫秒；直播时长字段为 `durationMs` / `totalLiveDurationMs`。
- 场次状态值固定为：`0`准备中、`1`直播中、`2`结束中、`3`已结束、`4`已取消、`5`失败。
- 房间状态与直播状态是两个独立字段：`status` 表示是否允许使用房间，`liveStatus` 表示当前运行阶段。
- `publishToken` 只在准备开播响应中返回一次，前端不得持久化展示；服务端只保存哈希。
- 礼物统计字段 `giftCount`、`giftCoinAmount`、`giftUserCount` 是汇总快照，不代替礼物和钱包流水。
