# earning 主播收益、结算与提现审核

礼物分成基础使用anchor_revenue_rule和anchor_earning_ledger。2026-11-01补齐统一收益事实、兑换规则/结算双人复核、独立可提现账户、申请冻结、自动对账与人工审核。入口server/router/earning → api/v1/earning → service/earning → model/earning；初始化initialize/earning.go，独立CLI cmd/earning-migrate，前端web/src/view/live/earning六页。

分成优先级anchor > gift > category > global；同scope有效期不重叠；规则新建版本、创建复核分离。没有有效规则禁止送礼；无任何默认比例。收益最初pending，退款追加reversal，采用净额累计舍入；从未写入用户COIN钱包。

规则创建/批准核验scope对应对象。anchor_rule_termination以独立事实保存提前终止申请与另一位管理员复核；原active审核状态不改，查询返回termination，选规则排除已终止规则。冲突检查使用原截止时间与终止时间的较早者。已引用分类/礼物保留公开编号，防止删除后重建误用旧规则。

COIN不是法币。新结算按显式整数兑换规则入独立账户，余数按主播/原规则跨批保留，退款追加负向调整；不足记debt。source_type/source_entry_no和income_no结算归属永久防重。当前仅gift注册可信来源核验，未来bounty/donation/guard/fan_club/charge可按同事务接入契约扩展。

提现申请无全局配置开关，安装DatabasePayeeProvider并要求本人已绑定的有效payeeAccountNo；提现申请不接收客户端完整收款资料，强制主播审核状态、运营提现权限、风险、余额和账务核验，但不读取或判断用户实名认证、主播认证或收款实名。审核通过status=approved且金额保持冻结；后台可“提交打款中心”创建唯一ready订单。内部ValidatePayoutInTx复用buildReconciliation只读计算与财务锁，payout事务保存创建核验快照，不新写earning对账或修改提现事实。2026-11-03由payout人工出款流程接续；领取前重新核验有效绑定及不可变版本，真实付款后依据固定快照完成独立复核。成功在同一事务frozen减、paid增、withdrawal_paid流水、提现paid及终态事件/审计；取消仅ready或明确未付款failed，解冻优先抵debt。paid纳入move/post/CAS、四余额版本链与深度核验，paid/cancelled不可变。每结算1000事实、同步历史核验20000保护上限，超限禁止放行；高流水账户需要后续核验检查点方案。文档与生产菜单执行记录见docs/earning/README.md。

2026-11-02收款账户绑定、事务内选账、旧占位单隔离、后台去向筛选及H5对接见docs/payout/README.md。
