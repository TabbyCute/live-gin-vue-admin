# payout 主播收款账户与人工出款

入口 `router/payout → api/v1/payout → service/payout → model/payout`，由各层enter注册；初始化 `initialize/payout*.go`，独立迁移CLI `cmd/payout-migrate`。当前实现本人绑定、提现选账、ready订单及人工出款闭环；不接外部银行API、回调或自动银行账单。

绑定资料AES-256-GCM加密、独立HMAC指纹防重；APP仅本人访问安全DTO，每主播100条历史绑定，默认槽唯一，停用不可恢复。绑定不代表渠道认证或实名匹配。提现在earning事务内重新验证有效账户，将不可变脱敏快照、冻结、提现及审计一起提交；旧payout_center_pending事实保留且禁止出款。完整绑定资料仅独立高权限的当前领取人短时单次查看，45秒授权能力，实时验证用户/角色/权限；无MFA伪装。前端关闭、30秒到时、退出或身份变化清空明文。

订单状态ready/processing/pending_confirmation/succeeded/failed/unknown/cancelled，单笔提现唯一订单，每次领取/重试新建不可覆盖attempt，最多100次；活动槽和成功槽唯一。processing释放要求本人未标记银行提交、未留流水、未上传凭证。unknown禁止重试/取消/释放，必须独立调查及新的私有凭证后解决；成功/取消不可重开。公司账户加密且敏感资料创建后不可变，active/disabled受审计，最多1000条；manual_bank只支付bank_card，UTC日累计领取额度不代表银行余额且不退还。

所有POST严格JSON、正文/并发/时间上限、固定错误码和永久requestId绑定操作者/动作/参数摘要。流水标准化指纹在同公司账户永久唯一。凭证原始PNG/JPEG≤2MiB，以AES-GCM写入显式私有目录，文件和目录fsync后提交元数据；os.Root限制路径，普通API没有文件路径/公共URL，单独审计接口授权读取。没有生产Mock或可匿名读取凭证的路由。

权限源 `anchor_finance_permission` 精确路径/方法；当前启用后台用户及角色实时复核，只有ID1 root绕过精确授权并可同人复核，例外明确审计。迁移保留既有888角色三项ready接口权限与菜单，注册23项新人工能力但不自动授权给任何普通角色。后台 `/capabilities` 与详情allowedActions驱动按钮，服务端再次检查。财务角色授权必须通过受控变更维护金融权限表；普通Casbin菜单可见不能代替金融授权。

资金写入仍唯一归属earning的move/post/CAS及版本链。payout通过初始化注入FinancialGuard/Lock/Check/Finalize回调，避免包循环。锁序finance control共享锁→主播财务锁→订单→attempt→公司账户/日额度或查看凭证。成功同事务frozen减、paid增、withdrawal_paid流水、提现paid、连续状态历史、审计、不可变terminal事件和内部对账；取消仅ready/明确未付款failed，优先抵debt后回available。真正转账后绑定停用不阻止按原快照确认。自动内部对账不是银行对账，terminal事件暂未配外部投递器。

数据库CHECK/唯一索引/证据链触发器同步执行状态机，普通ORM/SQL不能随意改状态、金额或删除历史。迁移识别精确已知旧保护；MySQL DDL可重入且最终核验，SQLite跨表DDL在事务内暂停已知触发器后完整恢复。历史paid=0流水只补0；历史ready订单不产生虚假后续事件。迁移前私有备份，维护期间禁止财务业务写入。

后台独立“打款中心 / 出款订单、公司出款账户”，包含领取/凭证/独立复核/调查/重试/取消/对账。H5保持本人DTO与提现四参数协议，新增paid/cancelled和paid余额；不展示公司银行账户或回单。旧payoutEnabled=false仅保留自动外部支付能力语义，不能隐藏人工操作或把已有paid误判无效。

当前契约、部署、可追溯查询和实测边界见 `docs/payout/manual-frontend-integration.md`、`manual-operations.md`、`manual-verification-20261103.md`。可复现脚本 `server/tests/payout/run.sh` 只接本机隔离MySQL，严禁用生产跑故障注入、负载或伪造付款。
