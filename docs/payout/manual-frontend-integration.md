# 人工出款前端对接（2026-11-03）

本契约接续绑定、提现选账和 ready 订单，不替换原 APP 接口。管理后台源码已增加公司账户页、出款操作、私有凭证和敏感资料弹窗；H5Player 已增加 paid/cancelled 展示与筛选、已付款累计值。人工出款不自动调用银行，`succeeded` 表示人工独立核验后完成内部记账，不能解释为系统已经查询银行到账。

## 基础约定

后台前缀 `/api/v1/admin/payout`，沿用后台 JWT。每次操作均重新查询当前启用用户、有效角色和 `anchor_finance_permission` 精确路径/方法。菜单可见不等于接口授权。ID=1 为超级 root；888 仅保留此前三个ready接口权限；23项新增人工权限不自动授予任何普通角色，须显式授权。

JSON 成功结构 `{code:0,data,msg}`，分页 `{page,pageSize,total,list}`；金额、账号内部ID、版本、total 均为十进制字符串，禁止转浮点金额。时间为毫秒整数。所有 POST 都带固定 `requestId`（8—64 位 ASCII 字母、数字、下划线或横线），严拒未知字段、重复键、大小写替代键、null、拼接 JSON、查询串和超大正文。客户端不能传金额、币种、操作者、复核者或收款账号来改变订单。

普通请求正文≤8KiB，服务端8秒预算/8并发；凭证上传 JSON≤3MiB、图片原文≤2MiB，15秒/2并发。普通后台请求每实例每身份桶突发10、每秒补1；敏感查看和上传各路由突发2、每10秒补1，固定4096桶避免身份map无界增长。网关须承担多实例总限流。超时、网络中断、503/429使用原参数原键重试；业务失败展示固定错误，不回显底层响应或请求体。响应 `X-Request-ID` 用于定位日志。

`GET /capabilities` 返回当前用户的精确能力map（键如 `attempt/review-success`）。订单详情的 `allowedActions` 在权限基础上再检查当前状态、领取人、独立复核、凭证存在与存储可用性。前端依此显示按钮；提交时服务端再次检查。权限更新后应重新获取。

## 订单查询

原 `POST /order/create {requestId,withdrawalNo}` 仍只创建 ready，冻结不变。`GET /order/list` 原参数兼容，status 支持 ready/processing/pending_confirmation/succeeded/failed/unknown/cancelled。新增精确筛选 `channelAccountNo`、`operatorId`、`reviewerId`、`paymentReference`；流水号查询必须同时提供公司账户编号，服务端使用标准化HMAC指纹。原 `payoutNo/withdrawalNo/anchorNo/currency/payeeMethod/from/to/page/pageSize` 保留。attempt筛选匹配历史任何一次尝试，不只最新一次。

`GET /order/detail?payoutNo=...` 新增：

| 字段 | 含义 |
|---|---|
| attempts | 最多100次尝试，升序sequence；包括脱敏公司账号、操作/复核人、真实支付流水、时间、凭证编号与哈希；不含密文/指纹 |
| attemptEvents | 不可变尝试事件，不返回内部完整状态快照 |
| evidence | 私有凭证安全元数据：evidenceNo/attemptNo/sha256/mime/size/createdBy/createdAt；无路径或URL |
| investigations | 最近200条调查，含新证据编号；`investigationsTruncated` 表明截断 |
| reconciliations | 最近20条内部对账快照、哈希和结论 |
| check | consistent/issues/checkedAt；问题是固定分类代码 |
| allowedActions | 结合权限与当前状态的操作map |
| history | 最多512条连续状态历史（含sequence/attemptNo/requestId/原因/操作者） |
| audit | 最近200条脱敏审计；`auditTruncated` 表明截断 |

`consistent` 现在表示内部账务证据链一致。当前绑定是否停用独立看 `currentPayee`：银行已经付款后，停用绑定不妨碍依据不可变快照完成复核和记账。初始 `creationReconciliation` 永久保留原审核事实。

## 公司账户

`GET /channel-account/list`：page默认1、最多100；pageSize默认20、最多100；可筛currency、active/disabled。响应仅脱敏账号和平台限额。

| POST 路径 | requestId之外的字段 |
|---|---|
| /channel-account/create | channelCode=`manual_bank`, accountName, institutionName, currency, account, singleLimit, dailyLimit |
| /channel-account/enable | channelAccountNo, reason |
| /channel-account/disable | channelAccountNo, reason |

限额使用最小货币单位字符串，dailyLimit≥singleLimit>0。完整公司账号仅创建时输入，6—34位大写ASCII字母/数字，不允许密码、验证码等字段。资料创建后不可编辑/删除；更换资料应新建账户并停用旧账户。UTC日累计领取金额用于每日限额，释放/失败/取消不退还当天额度，不代表银行真实余额。第一期manual_bank只向bank_card收款方式支付。

## 人工出款操作

| POST 路径 | requestId之外的字段与规则 |
|---|---|
| /order/start | payoutNo, channelAccountNo, reason；仅ready |
| /order/retry | 同start；仅failed，新建递增sequence的attempt |
| /order/release | attemptNo, reason, confirmNoTransfer=true；本人processing且无任何凭证、流水和银行提交标记 |
| /attempt/fail-unpaid | 同release；确认尚未进行转账，本次尝试failed |
| /attempt/bank-submitted | attemptNo, reason；已向银行提交后立即记录，即使还没拿到回单，随后禁止释放 |
| /attempt/submit | attemptNo, paymentReference, paymentTime, evidenceNo, operatorNote, confirmRealTransfer=true |
| /attempt/review-success | attemptNo, reason, confirmExternalResult=true |
| /attempt/review-failure | 同上；财务明确核验未付款成功 |
| /attempt/review-unknown | 同上；无法确定结果，继续冻结 |
| /attempt/resolve-unknown | attemptNo, investigationNo, decision=success/failure, reason, confirmExternalResult=true；独立调查人员提供本人新调查和新上传凭证 |
| /order/investigate | payoutNo, note, evidenceNo（可空；用于解决结果的调查必须有凭证） |
| /order/cancel | payoutNo, reason, confirmNoTransfer=true；ready或所有尝试明确未付款的failed |
| /order/reconcile | payoutNo, reason；追加内部对账事实，不修改余额 |

原因/说明3—500字节，拒绝控制字符；不要放账号、姓名等敏感资料。常见长数字、邮箱和URL自动脱敏。真实流水规范化为大写并移除Unicode空白，6—128位ASCII字符（字母、数字及`./_-`），拒绝UUID形式；同公司账户下永久唯一，失败后也不能复用。规范化不代表真实性验证，真实性依赖人工查验银行原始记录。支付时间须不早于领取、不晚于当前时间一分钟。提交之后流水、支付时间和原凭证不可变。

复核人不得是操作者；仅ID1例外且审计记录 `superRootException=true`。普通“复核失败”不能处理unknown。成功原子扣冻结增paid、提现paid及withdrawal_paid流水；失败保持冻结；取消先用冻结抵debt再返回available。终态不可重开。余额不足、账务异常、缺失/损坏凭证均会拒绝，不能绕过校验手动改状态。

## 明文查看与私有凭证

收款资料流程：`POST /order/reveal-payee-grant {requestId,targetNo:payoutNo,reason}` → `{grantNo,token,expiresAt}`（45秒）→ `POST /order/reveal-payee {requestId:新键,grantNo,token}`。后者仅当前领取人、订单processing、两项精确权限都有效时成功。使用后不能重放明文；网络丢响应时重新申请能力，不缓存旧明文。公司账号对应 `/channel-account/reveal-grant` 与 `/channel-account/reveal`，targetNo为公司账户编号。没有宣称MFA；这是实时权限检查和一次性能力控制。

成功敏感响应仅弹窗内存展示，最长30秒；关闭、卸载、页面停用、退出或切换账号立即清除，丢弃迟到响应。不得进入列表缓存、URL、localStorage/sessionStorage、埋点、错误日志或复制到持久表单。

`POST /evidence/upload {requestId,attemptNo,mime,content}`，content为PNG/JPEG原始字节的纯base64（不带data URL）；原图≤2MiB、宽高≤4096、像素≤400万。只允许本人处理中尝试或有解决unknown权限的独立调查人。返回evidenceNo/evidenceHash，上传后原图不可覆盖；每次尝试最多10份。普通列表详情没有可公开访问的图片链接。

`POST /evidence/read {requestId,evidenceNo,reason}`：独立evidence/read与order/detail权限；成功返回图片二进制，失败是统一JSON。响应no-store/nosniff/CSP sandbox/attachment。客户端按MIME识别成功，创建临时Blob URL用于图片展示，在关闭/登出时revoke；不得保存为公开对象URL。

## H5兼容及错误码

H5继续只显示本人脱敏收款账户、申请金额、提现状态、available/frozen/debt/paid；无公司账户、尝试、银行回单或明文能力。提现approved期间由后台payout状态处理；新增paid=`人工复核已付款`、cancelled=`已取消·冻结已释放`，旧请求字段不变。

49201参数、49202权限、49203暂不可用/原请求重试、49204幂等冲突、49205状态或账务异常、49206订单不存在、49207同人复核、49208查看能力失效、49209私有凭证不可用、49210公司账户/限额、49211缺调查证据。底层存储错误只给固定错误码，后台不要展示原异常。49201为确定的参数拒绝，可修改后用新键提交；网络错误/503/结果不明确仍锁定原参数原键。401/429/503遵循原鉴权和重试规则。

旧`payoutEnabled=false`仅表示自动外部支付尚未接入，不控制人工后台按钮，也不能用于拒绝paid余额或终态。公司账户选择已支持每页100条翻页，覆盖1000条历史账户上限。
