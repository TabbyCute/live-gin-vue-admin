# PostgreSQL 初始化

在 Navicat 中选中一个已经创建的空数据库，完整执行 `init.sql`，再填写服务端数据库连接并启动项目。

- 脚本只接受空的 public schema；使用事务，任何错误都会回滚，不能用于更新已有数据库。
- 初始账号为 `admin / 888888`，密码存储为 bcrypt 哈希；角色 888 为超级管理员，拥有脚本内全部菜单及 API 权限。
- 不再创建演示账号和演示角色。上线前修改默认管理员密码。
- 已移除网页初始化接口。配置 `system.disable-auto-migrate: true` 时，主程序、插件及 Casbin 都不会自动修改表结构。
- 插件文件生成、AI 代理、技能下载、权限缓存刷新需要登录及相应 API 权限。
- `sys_ignore_apis` 是 API 同步时的忽略列表，不是鉴权白名单；接口鉴权由路由中间件和 Casbin 决定。

## 密钥配置

支持服务端环境变量覆盖配置文件：

| 环境变量 | 配置项 |
| --- | --- |
| `GVA_JWT_SIGNING_KEY` | `jwt.signing-key` |
| `GVA_PGSQL_PASSWORD` | `pgsql.password` |
| `GVA_REDIS_PASSWORD` | `redis.password` |

生产环境应由部署平台注入独立的随机密钥，不应提交真实密码和密钥到 Git。修改 JWT 密钥后旧令牌失效，需要重新登录。`web/.env` 的 `VITE_*` 会暴露给浏览器，只放项目名称等公开配置。

代码修改及初始化 SQL 不会清理已有数据库中的演示账号、历史日志，也不会为已有角色自动追加新 API 权限。已有数据库需要单独处理，不要重复导入 init.sql。

已有数据库可执行 `security_permissions.sql`，补齐本次新增鉴权涉及的管理员权限，并重启后端使权限缓存刷新。该脚本可重复执行，不修改表结构、账号或历史日志；演示账号禁用和历史日志脱敏应另行确认后处理。

## 验证

`init_test.go` 仅允许连接名称以 `codex_init_audit_` 开头的空临时数据库，避免误操作业务库。设置 `GVA_INIT_TEST_DSN` 后，在 server 目录执行：

```sh
go test ./migrations/postgresql -v -count=1
go test ./middleware ./plugin/auto/initialize
go build ./...
```

测试覆盖失败回滚、重复导入保护、模型字段、菜单组件引用、关联完整性、管理员密码及权限、匿名/无权限调用拦截，以及禁用 DDL 后的插件和 Casbin 启动。测试完成后删除自行创建的临时数据库。
