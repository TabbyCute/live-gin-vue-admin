# 银行账单核对

`router/bankreconcile → api/v1/bankreconcile → service/bankreconcile → model/bankreconcile`，各enter组装。专属初始化/迁移 `initialize/bank_reconcile*.go`、`cmd/bank-reconcile-migrate`。

只导入外部声明并比对人工payout事实，不设置paid、不写资金账本、不生成支付凭证。绑定公司账号、银行流水、收款银行卡HMAC、币种、整数金额、银行状态和账期；部分账单与声明完整账单分开。八张表追加且不可修改删除；永久requestId和公司+流水防重。精确金融权限复用payout实时校验；普通操作人独立复核，ID1原root特例留痕。详情输出脱敏，不保留CSV原文。

双向核对批量查询，1000行/512KiB，HTTP并发2/读取5s/总15s；历史成功attempt按账户+状态+付款时间索引反查未见记录。核对复核只能一致确认/标记调查，真实付款仍走原私有凭证和人工双审。

admin入口`web/src/view/payout/reconciliation/index.vue`，H5未读取或修改。仅本地测试引用`internal/testpay`可替换Channel和HMAC模拟器，真实资金全链复用现有wallet/gift/earning/payout，无生产模拟路由。

契约、迁移与限制见 `docs/backend-hardening/bank-reconciliation.md`。
