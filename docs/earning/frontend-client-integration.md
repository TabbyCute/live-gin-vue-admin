# APP 主播收益接口接入

> 2026-11-03扩展：人工出款已支持frozen→paid、withdrawal_paid及提现paid/cancelled；此前“paid恒为0/出款未接入”的描述是阶段历史。当前行为以[人工出款前端契约](../payout/manual-frontend-integration.md)与[验证记录](../payout/manual-verification-20261103.md)为准。

前缀 `/api/v1/app/earning`。全部使用 APP `Authorization: Bearer <accessToken>`，不得使用后台 x-token。操作者与主播身份只取登录态，不传 anchorNo/userId/anchorId。仅本人主播可读；无主播身份返回业务错误，不返回他人数据。响应 `Cache-Control: no-store`。

## 金额和统一响应

统一 `{ "code": 0, "data": ..., "msg": "操作成功" }`；只有 code=0 才是成功。业务错误通常 HTTP 200；未登录 401、请求超限 400、限流 429、容量拒绝 503 仍需处理。HTTP 失败/超时不是“交易一定没发生”。

金额单位为明确币种的最小单位，例如已配置 CNY 时 amount="123" 表示 123 分；不得将 COIN 或不同币种互加。所有金额、余额、版本和 total 是字符串，时间为 Unix 毫秒数字。保留原字符串，计算用 BigInt/十进制库；不能使用 JS Number 计算金额。不要默认汇率，APP 不能计算或提交结算金额。

## 接口

| 方法/路径 | 参数 | data |
| --- | --- | --- |
| GET /overview | 无 | pendingCoin、settledCoin（COIN 字符串），anchorNo、balances、withdrawalEnabled、payoutEnabled |
| GET /balance | 无 | 与 overview 相同的对象；读取 balances 数组 |
| GET /settlement/list | page/pageSize；可选 currency/status/from/to/no/sort/desc | page/pageSize/total/list |
| POST /withdrawal/apply | 下述 JSON | AppWithdrawal |
| GET /withdrawal/list | page/pageSize；可选 currency/status/from/to/no/sort/desc | page/pageSize/total/list |
| GET /withdrawal/detail | no=提现单号 **或** requestId=原请求键，二选一 | AppWithdrawal |

分页默认 page=1/pageSize=20，pageSize<=100，page<=10000。from/to 为 createdAt 的左闭右开毫秒范围；sort 仅 createdAt/amount，desc 布尔，默认列表按 ID 降序。详情仅使用明确支持的参数；重复/未知 query 和未知 JSON 字段会失败。

`balances` 每项：currency、available、frozen、debt、paid、version（后五项字符串）。无账户时为空数组。pendingCoin 是尚未结算的净收益事实，不是可提现余额；可包含退款。提现申请默认开放，withdrawalEnabled正常为true；主播须通过本人身份、主播审核状态、运营提现权限、风险状态、余额和待处理退款校验，**不要求且不判断实名认证或主播认证状态**。payoutEnabled恒为false，表示真实付款尚未实现；后台已支持创建ready订单，不影响本APP契约。

AppSettlement 字段：settlementNo、periodStart、periodEnd、earningCoin、currency、amount、status、createdAt、approvedAt。status 为 pending/approved/rejected；仅 approved 已入账户。负数 amount 表示结算退款冲减。明细不暴露内部来源哈希、分成规则和管理员审计。

AppWithdrawal 字段新增payeeAccountNo/payeeMethod/payeeVersion，原有字段：withdrawalNo、requestId、amount、currency、accountMasked、status、createdAt、reviewedAt。status 为 pending_review/approved/rejected。approved 显示“审核通过”，不能显示“已到账”或“已提交出款”。只返回安全的内部收款业务编号，不返回真实姓名、完整账号、风控明细、钱包编号或内部主键。客服凭 withdrawalNo 查询后台。

## 创建和响应丢失恢复

```http
POST /api/v1/app/earning/withdrawal/apply
Authorization: Bearer <APP_TOKEN>
Content-Type: application/json

{"requestId":"withdraw_20261101_example_001","currency":"CNY","amount":"100","payeeAccountNo":"pa_0123456789abcdef0123456789abcdef"}
```

APP提现必须选择本人active且币种匹配的payeeAccountNo，不得上传完整账号、姓名、版本、风险结果或审核状态。服务端在冻结事务中核验并保存不可变脱敏快照，审核通过仍不打款，也不判断实名认证。账户绑定和旧版本兼容见[收款账户前端对接](../payout/frontend-integration.md)。

requestId 为 8—64 位 `[A-Za-z0-9_-]`，每次用户明确创建新申请时生成，持久保存原请求。amount 是无前导零的正整数字符串，上限 9223372036854775807；禁止小数、负数、科学计数或 JSON number。currency 是已配置的 3—8 位大写币种代码。

申请成功时 available 减少、frozen 增加；整个交易原子提交。超时/断网/503/49003：先按原 requestId 查询详情，未发现时以**原 requestId、原金额、原币种、原payeeAccountNo**重试。原键仍可用于恢复已成功申请。不能换键自动重发，也不能将“结果未知”显示为确定失败。双击按钮在请求期间禁用；页面关闭/重开仍应保留未确定请求。审核驳回释放冻结，但会先抵扣欠额，余额可能不回到原数值。

## 错误与限额

| code | 意义与处理 |
| --- | --- |
| 49001 | 参数格式/金额溢出错误，修正后明确发起新请求 |
| 49002 | 无权限/越权；重新确认登录身份 |
| 49003 | 服务暂不可用或结果未知；查询原请求后退避重试 |
| 49004 | 幂等参数、结算归属或状态冲突；不得换键盲目重试 |
| 49005 | 主播状态、运营提现权限、收款快照或风险状态不允许；不表示实名认证失败 |
| 49006 | 本人记录不存在；不能用于判断他人订单是否存在 |
| 49007 | 余额不足、欠额或未处理退款；刷新账户 |
| 49008 | 提现服务初始化异常；提示稍后重试并上报技术人员 |
| 49009 | 对账异常；交由财务核查，客户端不可绕过 |
| 49010 | 无有效兑换规则/冲突；交由运营配置 |
| 49011 | 收益来源未接入可信核验 |
| 49012 | 同步核验容量超限；交由运维/财务处理 |
| 49013 | 请求频率超限；按 Retry-After 退避 |

请求体上限 64KiB，query 2KiB，鉴权头 8KiB，请求处理预算 8 秒。APP 单进程并发上限32，固定4096桶限流，单桶每秒补2次、突发5次；桶碰撞只会更保守，不能依此宣称跨实例全局配额。429/503 遵守 Retry-After 并加随机退避；不要自动高频刷新列表。生产仍需要网关总量/身份限流及应用实例容量验收。
