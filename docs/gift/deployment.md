# 部署和权限

生产基线为MySQL8/InnoDB（约束必须实际启用），SQLite仅用于开发验证。自动测试始终只连接127.0.0.1:13327上的独立gift_test_测试库。用户本轮另外明确授权在prod远程库添加基础分类和礼物；业务导入与测试隔离，不向生产写入测试资金或订单。

## 安装

1. 备份、核验目标实例，使用专用DDL账号。在目标发布窗口执行下面命令；不要把凭证写入命令行历史或文档。
2. 已有wallet/live/social必须先完成其迁移；礼物迁移不会创建/更改钱包核心结构或余额。
3. 如果关闭auto-migrate，必须先离线迁移再启动服务。启动始终验证礼物表列、索引、CHECK、InnoDB和不可变触发器定义；缺失/不兼容则拒绝启动。
4. 系统已有角色和菜单时执行`-metadata`幂等补齐直播子菜单/API/超级管理员权限；正常启动也会补齐。没有默认业务分成或汇率。

```bash
cd server
# GIFT_MIGRATION_DSN 由部署系统秘密注入，不应写进仓库。
go run ./cmd/gift-migrate -metadata
go run ./cmd/gift-migrate -verify
```

迁移命名锁使用专用连接，数据库名参与锁名；所有操作有5分钟上限，释放锁失败会淘汰连接。重复执行不会删除财务事实。当前版本从无gift表安装及重复执行已测试；将来结构升级需版本化DDL并评估不可变触发器兼容性，不应自动删除保护触发器来绕过失败。

本次升级增加 `anchor_rule_termination`，并为权益补偿增加尝试次数、错误原因、租约、更新时间和完成时间。补偿UPDATE使用v2触发器：先安装新保护，再移除内容完全匹配的旧版禁止更新触发器；未知旧定义会失败关闭。SQLite/MySQL均测试已有pending补偿行的保留与重复升级。迁移在发布窗口执行；不要让新代码在旧结构上接收业务请求。

## 基础目录导入

普通结构迁移不自动插入业务目录。显式执行：

```bash
cd server
# 只读检查目标和现有目录；连接凭证从指定配置读取，不显示密码。
go run ./cmd/gift-migrate -config config.prod.yaml -inspect-catalog
# 首次安装：仅迁移礼物及待结算收益表；不修改钱包表。
go run ./cmd/gift-migrate -config config.prod.yaml
# 已存在目录表时只做草稿数据导入，不执行DDL和权限同步。
go run ./cmd/gift-migrate -config config.prod.yaml -seed-catalog -catalog-only
go run ./cmd/gift-migrate -config config.prod.yaml -inspect-catalog
```

`-config` 与 `GIFT_MIGRATION_DSN` 二选一，避免环境变量意外覆盖目标。`-verify` 和 `-inspect-catalog` 不允许与写入选项组合。目录导入在一个事务内锁定gift_control，按公开编号补齐；已有名称、价格、状态和资源完全保留。仅在新增记录时递增目录版本并追加一条catalog/seed审计。

| 分类 | 编号 | 礼物及草稿价格（COIN） |
| --- | --- | --- |
| 基础心意 | basic_affection | basic_rose 玫瑰 1；basic_heart 爱心 10 |
| 热场应援 | basic_support | basic_cheer 加油棒 30；basic_cake 庆祝蛋糕 100 |
| 豪华礼遇 | basic_premium | basic_rocket 火箭 1000；basic_crown 皇冠 5000 |

礼物均为draft、每单最多99件、无动画、无守护权益；图标为空，不使用虚构CDN。后台允许保存未配置图标的草稿，启用必须填写真实HTTPS图标。运营核定价格、上架，并由两位管理员创建/批准真实分成规则后方可送礼。初始化不创建默认分成，不产生消费、收益或测试钱包。生产导入不等于部署新后端/前端；应用发布及菜单同步按正常发布流程执行。

运行账号应只授予必要的SELECT/INSERT/受限UPDATE，不应具备DROP/TRIGGER/ALTER权限。需要菜单补齐的启动流程应由单个发布作业先执行，避免多个节点并发首次创建元数据；业务Pod使用`disable-auto-migrate`并在元数据就绪后启动。表结构迁移有命名锁；菜单同步另通过目录控制行锁串行化。

## 权限

新`gift_admin_permission`按authority_id+path+method唯一，初始仅角色888。每次服务入口验证sys_users.enable、sys_user_authority真实成员和精确权限，不依赖菜单隐藏。

- 查询：`/gift/category/list`、`/gift/item/list`、`/gift/order/list`、`/gift/order/detail`、`/gift/ranking`、`/gift/rule/list`，GET。
- 配置：`/gift/category/save`、`/gift/item/save`、`/gift/sort`、`/gift/delete`，POST。
- 退款：`/gift/refund POST`。
- 分成创建：`/gift/rule/create POST`；分成审核：`/gift/rule/review POST`。
- 提前终止申请：`/gift/rule/terminate POST`；终止复核：`/gift/rule/terminate/review POST`，两项独立授权，申请人不能自审。

项目已移除通用API权限管理，本阶段不恢复旧Casbin。给其他角色授权需受控数据库变更工单、双人复核及审计；仅分配菜单不会授予操作权限。撤销角色成员或具体操作授权立即影响后续请求。分成规则必须由另一名具备review权限的启用管理员审核；超级管理员也不能自审。

## 配置与外部服务

```yaml
gift:
  guard-enabled: false
```

未接入真实 GuardService 或完整就绪检查失败时设置 true 会拒绝启动。2026-11-04 的实现及启用顺序见 [守护部署](../guard/deployment.md)：先迁移与发布、启动并核验 Worker、配置方案，最后开启销售及具体礼物权益。UI 可保存关闭配置。IM Worker 仍未接入，不能将 pending 事件伪装成 delivered；结算/提现与公司出款分别见 earning/payout 的独立文档，gift 不直接出款。

## 运维检查

- 对48009及`gift invariant violated`错误告警，停止相关人工退款操作并核查订单详情、钱包完整流水和数据库变更。
- 监控8秒预算超时、429/503、数据库锁等待/死锁、连接池饱和、慢查询和错误率。请求IP取TCP对端，不相信未经配置的转发头；真实源IP由可信网关日志关联。
- 对账按订单号分页扫描，详情提供订单/退款/收入/收益/钱包交易金额核对；钱包深层流水与版本仍以wallet/check为准。不得把当前礼物详情核对称为未来提现自动对账。
- 逐笔核验包括退款单号、钱包TransactionNo/LedgerNo、BusinessNo、原扣款引用、账户/币种/金额、收入数量和收益冲减引用。发现错链即consistent=false，并禁止继续退款或按幂等键返回账务成功；不是只比较总额。
- `gift_event_outbox WHERE delivery_status='pending'`在未接IM阶段积压是已知状态，应建立容量保留和接入计划；上线IM后按最老事件年龄报警，Worker必须在钱包事务外投递。
- 生产环境大数据量排行需要检查EXPLAIN与查询超时；当前没有财务缓存，无Redis依赖，按<=93天窗口聚合。历史全量统计需要单独容量评估。
- 业务核心使用数据库原子性；目标环境必须测试MySQL主从切换、提交响应丢失、备份恢复与多实例重试，明确RPO/RTO。
- 部署前处理管理后台现有依赖告警，不能将本机功能测试等同于企业级生产验收。
