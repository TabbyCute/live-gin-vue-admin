# 后台超级 root

后台 `sys_users.ID=1` 固定为超级 root。正常登录后，菜单、按钮、角色权限和各业务操作权限均默认开放，无需为该用户分配权限。普通管理员的授权规则不变；APP 用户 ID=1 不属于后台 root。

钱包充值/冲正、礼物分成/提前终止、兑换规则/收益结算仍使用现有申请和审核入口，root 可由本人完成两步，无需其他管理员参与。所有操作记录真实创建人、审核人和原因。登录身份、账号有效性、账务一致性、业务状态、额度限制以及未接入的出款功能开关继续生效。

## 旧数据库升级

旧数据库中的审核触发器也会阻止自审。`auto-migrate is disabled` 是正常提示；随后出现 `legacy review trigger requires root migration` 表示旧约束尚未升级，无需开启全量自动迁移。

已有完整表结构时，使用专用命令仅升级下面四个触发器，不调用 AutoMigrate、不修改表字段或业务数据。在 `server` 目录执行：

```sh
# 默认只读预检，显示数据库目标和待执行语句数量
go run ./cmd/root-admin-migrate -config config.prod.yaml
# 使用有触发器权限的配置；备份文件所在目录须已存在，文件不得已存在
go run ./cmd/root-admin-migrate -config config.prod.yaml -apply -backup <备份文件绝对路径>
```

备份使用 0600 权限保存原触发器完整定义、DEFINER、SQL_MODE、字符集和升级语句。全部旧/新定义必须与代码精确匹配（忽略空白），备份成功后才执行；可重复执行。命令完成后核验礼物/收益结构及收益事实覆盖，输出 `startup checks passed` 后可重启服务。

只有尚未初始化完整模块时，才使用原有模块迁移命令：

```sh
go run ./cmd/gift-migrate -config <迁移配置文件>
go run ./cmd/earning-migrate -config <迁移配置文件> -migrate
```

迁移将以下表的 UPDATE 触发器升级为 `_root_v2` 版本：

- `anchor_revenue_rule`
- `anchor_rule_termination`
- `anchor_settlement_rule`
- `anchor_settlement`

新触发器先安装，再删除精确匹配的旧版本，不会出现无约束窗口。遇到未知旧触发器会报错并保留保护，需核对数据库版本。迁移可重复运行，不修改已有订单、余额或审核记录。无需补写 root 的权限记录。

发布后刷新页面或重新登录，加载全部菜单。root 可以读取客户及其他管理员的 AI 工作流会话；编辑会话保留原所属人。
