# 钱包账务模块

- 分层：`server/router/wallet` → `server/api/v1/wallet` → `server/service/wallet` → `server/model/wallet`，全部通过enter.go组装。
- 持久化：独立wallet_account/wallet_ledger/wallet_adjustment_order（source_type+external_order_no防资金重复），加wallet_daily_quota及wallet_audit_log。整数金额，独立于live_account；没有删除接口/软删除。
- 核心写路径：adjustment.go内创建、复核、驳回、冲正；余额、连续版本、额度、流水、审计、订单同事务；posting.go提供内部postLedger，调用方承担业务验证和事务提交。普通管理员强制双人审核；固定顶级超级管理员userId=1允许自审，但不绕过其他校验。
- 查询及核查：query.go；本人安全DTO、后台账号/手机号精确定位、管理员昵称与ID批量展示、版本游标、双游标分批只读对账。详情及每批对账采用RR只读快照，不持资金行锁；integrity.go复核订单、原始流水和创建/审核审计，审核成功重试也验证提交事实。钱包不使用Redis余额缓存。
- 初始化：initialize/wallet.go迁移、服务依赖及API元数据；wallet_schema.go以6次批量查询严格核验MySQL运行环境与结构；wallet_migration.go通过专用连接命名锁串行钱包迁移/菜单同步，不加锁表。cmd/wallet-migrate供外部DDL作业使用。
- 配置：config.Wallet启动快照；零值采用安全默认，角色与额度在所有实例保持一致；固定顶级超级管理员userId=1不受钱包角色列表限制，但仍须为启用用户。
- 支持：生产MySQL8/InnoDB；SQLite仅开发测试。业务扩展须扩展统一记账不变量及对账，禁止独立UPDATE余额。
- 文档入口：`docs/wallet/README.md`、`api.md`、`deployment.md`、`testing.md`、`production-review.md`。
- 管理前端：`web/src/view/wallet/{introduction,account,ledger,recharge}`，统一API在`web/src/api/wallet.js`；requestScope.js防止旧响应覆盖新账号，写操作固定确认参数，充值新建重新解析当前账号/手机号。用户列表批量显示余额并提供带账号跳转，返回列表刷新余额；动态菜单由`syncWalletMetadata`补齐。`npm run test:wallet`运行组件逻辑竞态回归。客户端契约见`docs/wallet/frontend-client-integration.md`。
