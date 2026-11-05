# 后台接口与操作

前缀 `/api/v1/admin/earning`，使用后台 x-token。服务层每次查询数据库当前启用账号、角色归属和 `anchor_finance_permission` 的完整 path/method；菜单可见不等于接口可调用。初始仅角色 888 授权，后续财务角色需要单独配置精确操作权限。

## 操作顺序

1. 在“结算兑换规则”创建业务确认的币种、兑换比例及生效区间，由另一管理员审核通过。每个 sourceUnit/currency 的 active 区间不能重叠。未配置有效规则不能结算。
2. 从“主播管理”的“创建结算单”直接进入（自动锁定该主播），或在“收益结算”创建。默认最近7天、结束为打开弹窗时的当前秒，支持自选起止时间及一键更新到当前秒，不允许未来时间。兑换规则下拉分页查询已审核规则，展示名称、编号、兑换比例与有效期；当前未生效或已过期的规则禁止选择，提交时后端再次核验规则当前有效性（不要求收益产生期间完全落在兑换规则有效期内）。时间范围包含开始、不包含结束，只纳入最多1000条未结算事实；查看明细和合计后复核入账，超级root ID1可自审，其他管理员需另一人复核。不能提交任意结算金额或事实列表。超过1000条应缩小时间区间；同一毫秒集中超过上限时需要扩展分批选择能力，不能绕过限制。
3. “主播可提现账户”查看 available/frozen/debt 及不可变流水，执行完整对账。结算和退款分开记账，不覆盖历史。
4. 主播本人APP默认可申请提现并原子冻结余额；后台不能代填银行账号或创建任意主播提现。新申请选择本人有效payeeAccountNo并保存不可变脱敏快照，旧占位单不改写。
5. “提现审核”可先点“自动对账”查看快照。点击通过时服务端再次自动核验；异常不会放行。驳回附原因并释放冻结（先抵扣欠额）。
6. 审核通过显示approved，资金仍冻结。2026-11-02新增“提交打款中心”，复核通过后只创建唯一ready订单；列表/详情由payout模块提供，见[后台订单对接](../payout/order-frontend-integration.md)。没有实际付款。

## 路由

| 方法/后缀 | 用途 |
| --- | --- |
| GET overview | 全部/指定主播 COIN 收益与分币种余额汇总 |
| GET income/list | 统一收益事实，可按来源与 settled/pending 筛选 |
| GET exchange/list | 兑换规则列表 |
| POST exchange/create | 创建规则，返回 SettlementRule |
| POST exchange/review | 另一管理员审核规则 |
| GET settlement/list | 结算申请列表 |
| POST settlement/create | 生成预览，不直接入账户 |
| POST settlement/review | 另一管理员审核并原子入账 |
| GET settlement/detail?no=… | 结算、明细、审计和一致性结果 |
| GET account/list | 可提现账户列表 |
| GET account/ledger?anchorNo=…&currency=… | 不可变余额流水，分页 |
| GET account/check?anchorNo=…&currency=… | 深度一致性检查，返回 consistent/account/issue |
| GET withdrawal/list | 提现列表 |
| GET withdrawal/detail?no=… | 提现、最多100份最新对账快照、流水、审计 |
| POST withdrawal/review | 人工审核，批准前自动重新对账 |
| GET reconciliation/list | 全部对账记录分页，no 是提现单号 |
| POST reconciliation/run | 对指定提现单新建不可变系统对账快照 |

列表共同参数 page/pageSize/no/anchorNo/currency/sourceType/status/from/to/sort/desc。默认1/20，最多100条，页码<=10000。sourceType只用于income；income状态由唯一结算归属派生。exchange没有主播筛选；account没有status/no；sort只能createdAt/amount，不支持的组合返回49001。from/to毫秒左闭右开。

创建兑换请求：requestId/name/sourceUnit（COIN）/currency/coinUnits/minorUnits/roundingMode（floor）/effectiveFrom/effectiveTo。币种与整数比例必须明确，不默认人民币、不自动固定汇率。正整数金额参数是字符串，时间是毫秒数字。

创建结算请求：requestId/anchorNo/ruleNo/periodStart/periodEnd。所有审核请求统一 `{ "no": "业务单号", "approve": true, "reason": "核验原因" }`；approve必传且必须为JSON布尔，reason不超过500字节且不可空。对账请求 `{ "withdrawalNo": "业务单号" }`。

创建参数被永久绑定到 requestId 及创建人；超时复用原键原参数。审批只有相同复核人、决定和原因才作为原操作重放；不允许更换原因覆盖原审计。对账运行是重新检查，重复点击可以产生新快照；它不转移资金。

创建弹窗共用现有 `GET exchange/list?status=active&page=1&pageSize=20` 和 `POST settlement/create`；管理员需要对应的规则查询及结算创建操作权限。没有新增菜单、自动审批或APP财务写接口。弹窗在请求结果不明确时锁定原参数，关闭再从另一主播打开也保留原申请供重试；该内存状态不跨刷新/页面卸载保存，刷新后应先查结算列表再创建。真正重复入账由服务端不可重复的收益归属与复核事务阻止。

错误码见 [APP 接入文档](frontend-client-integration.md)。内部异常仅返回稳定业务错误，不返回 SQL、连接串或数据库堆栈。HTTP 处理预算8秒，后台最大16个并发财务请求，无无限等待队列。应用实例之外仍需要网关和目标环境容量验证。

2026-11-02：withdrawal/list新增仅后台可用的payeeAccountNo精确筛选；withdrawal/detail新增currentPayee，对比当前账户与申请快照。详情见docs/payout/frontend-integration.md。
