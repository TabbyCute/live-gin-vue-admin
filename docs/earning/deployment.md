# 迁移、菜单与开放条件

> 2026-11-03扩展：人工出款已支持frozen→paid、withdrawal_paid及提现paid/cancelled；此前“paid恒为0/出款未接入”的描述是阶段历史。当前行为以[人工出款前端契约](../payout/manual-frontend-integration.md)与[验证记录](../payout/manual-verification-20261103.md)为准。

使用独立 `server/cmd/earning-migrate`。它不调用消费钱包迁移，不启动 HTTP 服务，不创建默认兑换规则、测试账户或测试资金。已有 `gift-migrate/main.go` 保持本任务开始时内容，避免混入此前回退过的启动逻辑。

从仓库 server 目录执行（凭证留在配置文件，不放入命令行或日志）：

```sh
go run ./cmd/earning-migrate -config config.prod.yaml
# 先做本机验证；以下为有写入的显式步骤：
go run ./cmd/earning-migrate -config config.prod.yaml -migrate
go run ./cmd/earning-migrate -config config.prod.yaml -backfill
go run ./cmd/earning-migrate -config config.prod.yaml -verify
go run ./cmd/earning-migrate -config config.prod.yaml -metadata-only
go run ./cmd/earning-migrate -config config.prod.yaml
```

默认命令只读菜单/API/权限；`-verify`只读核验14张收益相关表、唯一索引、CHECK、触发器正文及历史礼物收益覆盖。`-metadata-only`与迁移/回填不能组合。可替代配置文件使用环境变量 EARNING_MIGRATION_DSN，两种来源互斥。工具单连接池、连接5秒、读写60秒、全程5分钟预算。

`-migrate` 是显式新增/核验本模块结构，复用现有财务迁移互斥锁；MySQL要求8.0.16以上、InnoDB、utf8mb4_bin及严格模式。不删除/重建已有财务事实；发现不兼容保护触发器正文即失败。MySQL DDL不具备完整事务回滚，需要发布前真实备份和恢复演练，不用删表作为回滚方案。

`-backfill`按历史 anchor_earning_ledger 的 ID 顺序，以永久source键镜像真实礼物收益/退款。已有镜像会核验而不重复记账；不改钱包余额、原订单或历史分成。不自动结算。运行旧版本应用期间可能继续产生未镜像事实，正式切换前应暂停写流量、补齐回填并verify，再启动新版本；启动发现缺口会失败关闭，不静默漏结算。

菜单名称 earning-overview / earning-exchange / earning-settlement / earning-account / earning-withdrawal / earning-reconciliation，父级 liveManagement。组件路径为 `view/live/earning/<kind>/index.vue`，六页真实存在。同步追加父/子菜单角色888授权与17个精确接口权限；重复执行不新增重复菜单。它不撤销现有角色权限、不变更现有礼物目录。

## 发布与提现开放

发布后端及 web 构建后，重新登录后台以重新加载菜单。数据库菜单已写入不意味着线上进程和静态资源已经更新。此任务不重启现有服务或执行实际打款。

提现申请没有全局配置开关。2026-11-02改为安装数据库收款账户提供方，新申请必须选择本人有效payeeAccountNo；部署前执行payout迁移并通过secret环境变量配置AES与独立HMAC密钥，缺失时启动失败。没有实名认证条件，历史占位单不补绑。部署次序、密钥及轮换边界见[收款账户部署](../payout/deployment.md)。

兑换规则须业务自行确认，再双人复核；工具不写入示例汇率。完整收款资料及真实姓名不进入APP或后台页面；两端均仅展示安全脱敏资料。后台已能创建ready订单，迁移与独立菜单见[订单部署](../payout/order-deployment.md)；当前没有自动派发队列和付款成功入口。

## 容量与运维边界

每单结算<=1000事实；同主播/币种同步完整核验的历史账户流水、已批准结算、提现及结算事实各有20000上限，礼物证明批量子表也有20000行保护。达到任一上限会停止相关操作，不会跳过核验。**该历史上限不能通过“把下一笔结算拆小”消除**。高流水主播上线前需要可信核验检查点/归档方案并重做容量、故障与恢复验收；当前并非无限生命周期账本的容量交付。不要手工删除历史事实来规避。

HTTP 预算8秒，APP32/后台16在途上限；APP固定内存桶限流是单进程防护，跨实例全局频控应在网关/共享限流层验证。锁等待、死锁率、49003/49009/49012、异常对账、待审核冻结时长和回填缺口应纳入监控。断网后查询原幂等请求，不能用新请求号重试资金操作。

目标部署仍需给出主播历史规模、请求速率与延迟SLO，实测峰值、资源饱和、MySQL/应用多实例故障切换、网络分区、备份恢复、RPO/RTO和恢复后资金守恒。本机测试不能证明这些生产项目已完成。


## 本次已执行结果

2026-11-01远程live_prod：迁移、真实历史覆盖回填和只读核验均成功。六菜单ID58—63挂在父菜单7（直播管理），角色888获得全部六页与17条精确操作权限。重复同步的菜单/授权/API/权限/事实计数完整回读相同。原始礼物收益及新收益、规则、结算、可提现账户、流水和提现记录计数均为0；没有测试资金。见[结构化执行记录](production-metadata-20261101.json)。前后端应用未发布、未重启。
