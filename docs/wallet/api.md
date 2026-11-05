# 钱包API契约

默认根路径`/api`，以实际`system.router-prefix`为准。统一响应`{code,data,msg}`，成功code=0；鉴权失败HTTP401；参数及业务失败HTTP200+470xx。金额、版本、管理员ID均为字符串，不要转为JavaScript Number；时间为毫秒整数。所有写请求使用`Content-Type: application/json`，最大16KiB，拒绝未知及重复JSON键、尾随JSON；查询拒绝重复和未知参数。请求数据库执行期限8秒；正文读取也受此期限限制。入口超长query/token或已知超大正文为HTTP400；超额并发为HTTP503、code=47012、Retry-After: 1，不进入业务处理器。客户端按退避策略重试，写请求必须保留原编号和参数。

## APP本人接口

`Authorization: Bearer <APP token>`。不得复用后台x-token或Cookie。

| 方法 | 路径 | 参数 |
| --- | --- | --- |
| GET | /v1/app/wallet/balance | currencyCode可选，仅COIN |
| GET | /v1/app/wallet/ledger | currencyCode、limit（默认20，1—100）、cursor |

余额示例：

```json
{"code":0,"data":{"currencyCode":"COIN","availableBalance":"100","frozenBalance":"0","version":"1","status":"normal","updatedAt":1793174400000},"msg":"成功"}
```

流水返回`{list,nextCursor,hasMore}`，按钱包版本倒序。每项包含ledgerNo/currencyCode/walletVersion/changeAmount/balanceBefore/balanceAfter/businessType/businessNo/originalLedgerNo/createdAt。APP没有reason/operatorId/evidenceRef。首次不传cursor，以后原样传nextCursor；hasMore=false停止。游标绑定钱包，不支持跨账号复用；新写入的流水需刷新首页，不会插入旧页序列。

## 后台

`x-token: <管理端 token>`，服务端角色权限见配置。管理页面位于`web/src/view/wallet/`，菜单和API元数据由初始化逻辑幂等同步。

| 方法 | 路径 | 权限与参数 |
| --- | --- | --- |
| GET | /v1/admin/wallet/account/detail | 读权限；accountNo或phone至少一个、currencyCode |
| POST | /v1/admin/wallet/account/status | 复核权限；accountNo或phone至少一个、currencyCode、status(normal/disabled)、reason |
| GET | /v1/admin/wallet/ledger/list | 读权限；accountNo或phone至少一个、currencyCode、limit、cursor；额外提供operatorId、operatorDisplay及内部reason |
| POST | /v1/admin/wallet/adjustment/create | 申请权限；见下例 |
| POST | /v1/admin/wallet/adjustment/approve | 复核权限；orderNo、reason |
| POST | /v1/admin/wallet/adjustment/reject | 复核权限；orderNo、reason |
| POST | /v1/admin/wallet/adjustment/reverse | 申请权限；requestId、原orderNo、reason、evidenceRef可选；只创建申请 |
| GET | /v1/admin/wallet/adjustment/list | 读权限；page/pageSize/accountNo/phone/orderNo/status/kind/sourceType/externalOrderNo |
| GET | /v1/admin/wallet/adjustment/detail | 读权限；orderNo必填 |
| GET | /v1/admin/wallet/account/check | 读权限；accountNo或phone至少一个、currencyCode、afterVersion、afterOrderId、afterTransactionId、limit |
| GET | /v1/admin/wallet/transaction/trace | 读权限；no，或businessType+businessNo+kind；afterRefundId可选 |

后台用户定位支持公开账号或完整E.164手机号（例如`+60123456789`）精确查询。两者同时提供时必须属于同一用户，否则按账号不存在处理；手机号只用于定位当前用户，账务事实仍绑定稳定的内部用户ID和公开账号快照。余额响应会返回解析后的`accountNo`和当前绑定`phone`，便于后台再次确认目标。订单、流水和审计保留原始管理员ID，同时提供`createdByDisplay`、`approvedByDisplay`、`rejectedByDisplay`或`operatorDisplay`，格式为`昵称 [ ID ]`；历史管理员被物理删除时回退为`管理员 [ ID ]`。

创建：

```json
{"requestId":"recharge_20261028_0001","sourceType":"manual","externalOrderNo":"BANK-RECEIVER1-20261028-0001","accountNo":"10000010","currencyCode":"COIN","amount":"100","reason":"人工充值，已核对资金到账","evidenceRef":"upload:private-evidence-id"}
```

申请成功仅表示pending，余额不变。`requestId`为8—64位大小写字母、数字、下划线或横线；区分大小写。amount为无前导零的正整数字符串；不接受小数、科学计数、正负号、空格或JSON数字。reason必须为3—500字符，不含控制字符和首尾空白。evidenceRef最多500字符，每笔人工充值均必填，引用已经上传的凭证；钱包不主动下载URL、不执行第三方HTTP请求。凭证存储及访问权限沿用部署方文件系统，复核人员需实际核验内容。

审核：

```json
{"orderNo":"wa_0123456789abcdef0123456789abcdef","reason":"已核对收款账户、交易号和到账金额，确认通过"}
```

订单返回：orderNo、requestId、sourceType、externalOrderNo、kind(grant/reverse)、accountNo、currencyCode、amount、reason、evidenceRef、status、originalOrderNo、reversalOrderNo、ledgerNo、createdBy/createdByDisplay、approvedBy/approvedByDisplay、rejectedBy/rejectedByDisplay、reviewReason、createdIp、reviewedIp及各毫秒时间。创建及审核IP取TCP真实对端，不信任X-Forwarded-For；经网关时记录网关IP，可与网关请求日志关联。

详情为`{order,ledger,audit,issues}`；未入账ledger=null，audit按发生顺序列出操作、管理员、IP、原因、时间。issues非空表示本单请求摘要、审计或流水事实异常；即使已入账单的流水缺失仍返回其现存订单与审计供追查，不能将HTTP成功等同于账务完整。全面核验仍需运行三游标对账。审核错误时只返回安全业务错误信息，不向调用方返回数据库错误详情。

订单列表保留项目`{page,pageSize,total,list}`：默认1/20，pageSize最大100，offset最大10000。status仅pending/credited/rejected/reversed；kind仅grant/reverse；不支持keyword模糊扫描、任意排序或无界导出。total与列表可能因并发提交轻微变化，财务核验以单据详情和对账为准。

对账按三个游标分批：首次`afterVersion=0&afterOrderId=0&afterTransactionId=0`，默认limit=100，最大1000；每次同时把`nextVersion`、`nextOrderId`、`nextTransactionId`传入对应after参数，直到hasMore=false。结果包含consistent、issues、checked（流水数）、checkedOrders、checkedTransactions、snapshotVersion。每批使用REPEATABLE READ只读事务取得一致的MVCC快照，不持资金行锁；首次开户并发也读取同一快照中的订单和钱包状态。连续多批期间可能有新入账，snapshotVersion会推进，不代表跨全部页面的冻结快照。必须从0开始并累计每批issues；单独检查最后一页不代表历史全部正确。这里只核查，不自动修改账或重放余额。order_request_hash表示申请摘要异常；order_audit表示订单创建/审核审计缺失、重复或身份/时间/原因不一致；legacy_source_unverified表示历史真实资金标识尚未核验（已驳回旧单除外），不等同于余额合计错误。所有钱包响应带Cache-Control: no-store。

### 消费与退款追踪

内核没有对外扣款、退款接口。内部Go契约见[扣款与消费退款](debit-refund-core.md)。本人流水新增`CONSUME`（负数）、`CONSUME_REFUND`（正数），`businessNo`为`wt_`内部交易号，退款的`originalLedgerNo`关联原扣款流水。不向APP返回业务订单号、操作者、IP、TraceID或内部原因。

后台追踪支持以下两种互斥查询：

```text
GET /v1/admin/wallet/transaction/trace?no=wt_<32位小写十六进制>
GET /v1/admin/wallet/transaction/trace?businessType=shop&businessNo=order-123&kind=debit
```

`no`接受wl_流水号、wt_交易号、wa_人工单号。业务查询kind仅debit/refund；必须提供全部三个字段，区分大小写。响应为`{ledger,transaction,original,refunds,nextRefundId,hasMore,audit,adjustment,issues}`，不适用的对象为null或省略：

- ledger：本次流水，与管理端流水DTO一致。
- transaction / original / refunds项：transactionNo、kind、businessType、businessNo、accountNo、currencyCode、amount、refundedAmount、originalNo、ledgerNo、actorType、actorId、actorDisplay、actorIp、traceId、reason、createdAt。
- original：查询退款时返回原扣款；refunds：该原消费下的退款，按ID正序，每页50条。保持查询条件不变，把nextRefundId作为afterRefundId继续查询；hasMore=false停止。退款合计以refundedAmount为准，不能只加总第一页。
- audit：当前交易审计；人工单查询的adjustment为原有`{order,ledger,audit}`详情。
- issues：消费交易完整性异常返回`transaction_integrity`，页面应警示并保留现存事实。完整核验使用account/check，新增异常transaction_ledger、transaction_audit、transaction_refund分别表示交易与流水、审计、退款关系异常。

人工与消费的管理员均显示`昵称 [ ID ]`；用户显示`用户 [ 公开账号 ]`，任务显示`系统 [ 服务标识 ]`。普通流水中的operatorId=0表示非管理员，不能当作超级管理员或使用它替代actorType/actorId。

## 错误码

| code | 含义 | 处理 |
| --- | --- | --- |
| 47001 | 参数无效 | 修正请求 |
| 47002 | 管理员/角色无权限 | 核对当前账号状态及角色 |
| 47003 | 账号不存在/不可入账 | 核对用户状态 |
| 47004 | 订单不存在 | 核对单号 |
| 47005 | 状态冲突/幂等键冲突 | 查询原单，不能盲目新建 |
| 47006 | 普通管理员自审禁止 | 交给另一管理员；固定顶级超级管理员userId=1不受此项限制 |
| 47007 | 单笔/每日/待审单数量/退款拆分数量超限，或余额溢出保护 | 核对额度，禁止拆单绕过 |
| 47008 | 人工充值凭证缺失 | 新申请提供凭证 |
| 47009 | 钱包停用 | 核查停用原因 |
| 47010 | 可用余额不足 | 拒绝扣款或冲正，禁止改余额 |
| 47011 | 账务不变量异常 | 停止该钱包写操作并对账排查 |
| 47012 | 服务/数据库异常或提交结果未知 | 查询原单，使用原请求编号/原审核内容重试 |

## 资金防重与升级契约

`sourceType`省略时为`manual`，当前人工入口只接受`manual`；不能填`alipay`、`wechat`来绕过审核或伪装渠道回调。自动支付以后必须有独立支付订单和验签流程。`grant/ADMIN_GRANT`保留为兼容的内部枚举，当前创建入口语义已明确为系统人工充值。

`externalOrderNo`是**实际资金凭证号**，由收款渠道、收款账户和真实交易流水号按运营统一规则组成，不能用随机UUID、requestId或自编流水冒充。1—128位ASCII，首位字母/数字，后续允许字母、数字、`.`、`_`、`:`、`/`、`-`；统一转大写，拒绝空白、Unicode和控制字符。数据库`(source_type, external_order_no)`永久唯一，不包含管理员、用户或金额，因此换人、换用户、换金额、换requestId仍返回47013。不能通过改写前缀重复使用同一真实资金凭证；审核人须核验真实到账与凭证标准化是否正确，系统不连接银行核验真实性。

`requestId`仍用于接口重试。相同管理员、相同requestId、相同规范化请求返回原订单；修改凭证号也属于内容冲突，返回47005。响应丢失时查原单或按原参数重试，不要新建业务凭证。可以通过`adjustment/list?externalOrderNo=...`按唯一资金标识查询，默认为manual，命中联合唯一索引。

已驳回或已冲正的资金标识也不释放。当前没有修改核心字段、原单重开或“换凭证再次入账”的修账接口；错误申请先驳回并进入人工资金核查流程。确需更正同一真实资金的收款人或金额时，必须单独设计带原单关联和复核的更正流程，不能伪造新凭证规避防重。

钱包状态操作不开户。无钱包返回47014；正常充值审批首次开户和余额写入同一事务，并额外写`wallet_open`审计（审计对象`wallet:<id>`）。重复设置同一状态不写虚假变更记录；设置状态或开户时审计失败则整个事务回滚。

| 新增code | 含义 | 处理 |
| --- | --- | --- |
| 47013 | 资金凭证已有申请 | 按sourceType/externalOrderNo查询原单，禁止绕过防重 |
| 47014 | 钱包尚未开户 | 不以状态接口开户；真实充值审批时原子开户 |
| 47015 | 历史申请无真实资金标识 | 核查后驳回旧待审单，不直接批准 |
| 47016 | 累计消费退款超过原消费金额 | 查询原消费和成功退款，禁止新单绕过 |

迁移后的`legacy`是历史缺少资金标识的记录，`externalOrderNo`只是原内部订单号，**不代表真实资金凭证**。`reversal`为内部冲正命名空间，以冲正单号区分；两者均不能通过人工充值API创建。旧数据升级与上线核查见deployment.md。
