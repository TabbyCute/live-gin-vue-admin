# 后台固定超级 root 管理员

- 日期：2026-11-01
- 用户要求：ID 为 1 的后台用户拥有默认完整功能和权限，任何操作不需要另行授权或多人参与。
- 实现：统一 `model/system.IsSuperAdmin`，仅依据可信后台用户 ID；覆盖角色中间件、菜单按钮、严格角色树、客户数据范围、AI 工作流会话、API Token 角色、社交、礼物、收益及原有钱包 root 策略。
- 自审：钱包充值/冲正、礼物分成/提前终止、收益兑换/结算允许同一 root 创建并审核；原审核接口及幂等重试保留。对账规则和数据库触发器同步接受 root 自审。
- 前端：完整动态菜单，统一 root 按钮判断，直播高权限操作入口开放；提示文字说明 root 可独立复核。
- 边界：登录和身份验证保留，APP 身份不共享 root；不取消数据合法性、业务状态、金额限制、账务审计或尚未接入的功能开关。
- 升级：旧库先执行新版 `gift-migrate`、`earning-migrate -migrate`，升级四个审核触发器；无需为 root 补角色或授权记录。升级方法见 `docs/root-admin.md`。
- 验证：覆盖 root 无角色访问、全部菜单按钮、普通管理员拒绝自审、root 自审及幂等、结算对账、旧触发器重复升级；前端 27 项回归与生产构建通过，后端全量测试通过；本地临时 MySQL 8 的礼物/收益 root 自审、旧触发器升级及结算对账验证通过。
- 初次交付仅修改本地代码与构建产物；后续用户报告启动出现 `legacy review trigger requires root migration`，并明确要求远程处理。
- 2026-11-01 后续处理：使用 `config.prod.yaml` 指向的 `live_prod` 完成四个审核触发器的定向升级；未运行 AutoMigrate，未更改表字段、业务记录或重启服务。原触发器备份：`/Users/tabby/.codex/backups/live-gin-vue-admin/root-review-live_prod-20261101-remote-upgrade.json`（0600）。
- 增加 `cmd/root-admin-migrate`：默认只读预检；`-apply -backup` 先核验全部新旧定义、保存备份，再安装新约束和移除旧约束，可重复执行。临时 MySQL 验证备份失败零写入、重复执行及完整结构校验通过；远程礼物、收益和收益覆盖启动校验均通过。
