# 客户端账号注册登录与独立鉴权

## 基本信息

- 提出日期：2026-09-21
- 当前状态：`done`
- 需求类型：客户端账号 / 登录鉴权
- 优先级：高
- 需求文件：`aiDoc/memory/business/done/live-account-authentication.md`

## 用户原始意图摘要

在 `live_account` 模块实现字段简单的客户端账号注册、登录和当前账号信息能力，并在 `jwt_app` 中完成客户端鉴权；客户端与管理后台鉴权体系完全独立，且客户端不用 Cookie，只使用 token。

## 影响范围

- 后端 Model：新增 `live_account` 客户端账号表模型。
- 后端 Service：新增注册、登录和按 ID 查询账号逻辑。
- 后端 API：新增客户端注册、登录和当前账号信息接口。
- 后端 Router：区分公开与客户端 JWT 私有路由，同时支持 v1/v2 现有版本入口。
- 中间件：实现 `JWTAppAuth`，仅接受 `Authorization: Bearer <token>`。
- 配置：新增独立 `jwt-app` 密钥、有效期和签发者配置。
- 数据库初始化：将 `live_account` 加入自动迁移与首次初始化表清单。

## 已确认约束

- 客户端账号不复用 `sys_users`。
- 客户端 JWT 不复用管理后台 JWT 密钥、签发者、Claims、`x-token`、Cookie、黑名单或 Casbin。
- 客户端登录仅在 JSON 响应中返回 token，不写 Cookie。
- 客户端受保护接口仅从 `Authorization: Bearer <token>` 读取 token。
- 密码使用 bcrypt 哈希存储，接口响应不返回密码哈希。

## 接口契约

- `POST /api/v1/app/live/user/register`
- `POST /api/v1/app/live/user/login`
- `GET /api/v1/app/live/user/info`
- v2 路由沿用相同相对路径与行为。

## 完成结果

- 已完成独立账号模型、请求/响应 DTO、Service、API、Router、JWT 工具与中间件。
- 已增加账号注册登录及独立 JWT 的自动化测试。
- 已同步 Swagger 注释与前后端鉴权契约说明。
