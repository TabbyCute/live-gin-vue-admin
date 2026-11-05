# 前端变更对接（2026-11-02）

后续新增的后台“提交打款中心”、出款列表及详情契约见[待出款订单前端对接](order-frontend-integration.md)。本页APP绑定/提现契约保持有效，H5无需新增实际付款动作。

这是提现申请的必填字段升级。统一响应 `{code,data,msg}`，成功仅 `code=0`。APP 使用 `Authorization: Bearer <APP token>`，不能使用后台 x-token。路由按本项目 `/api/v1/app/...`，以实际部署的全局 router prefix 为准。所有响应 `Cache-Control: no-store`，时间为 Unix 毫秒数字，金额、账户版本、total 为十进制字符串。

## 新增收款账户接口

| 请求 | 参数 | data |
| --- | --- | --- |
| GET `/api/v1/app/payout/payee/list` | page默认1、pageSize默认20最多100；可选currency、status=active/disabled | `{page,pageSize,total,list}` |
| POST `/api/v1/app/payout/payee/create` | 下表列出的资料与requestId | 安全Payee DTO |
| POST `/api/v1/app/payout/payee/set-default` | `{payeeAccountNo}` | 安全Payee DTO |
| POST `/api/v1/app/payout/payee/disable` | `{payeeAccountNo}` | 安全Payee DTO |

安全Payee DTO **仅**包含 `payeeAccountNo, method, currency, maskedAccount, status, version, isDefault, createdAt, updatedAt`。例如 `maskedAccount="****1234"`；`status="active"` 只代表已绑定，UI不得显示“已实名认证”“渠道认证成功”。无接口读取完整收款资料，无编辑、删除、恢复或解密接口。

create 的所有字段为字符串；公共必填为 `requestId,method,currency,accountName,account`。

| method | 附加字段 | 规则 |
| --- | --- | --- |
| bank_card | bankCode | 标识 `[A-Z][A-Z0-9_]{1,15}`，例如 ICBC；卡号12—19位ASCII数字；不接受空格、分隔符、信用卡校验结果声明 |
| alipay | 无 | 6—128字节且至少6字符的实际账户标识；邮箱形式或可带前导+的6—24位数字；无空白/控制字符；不擅自改大小写 |
| wechat | identifierType固定openid；appId | account为16—128位 `[A-Za-z0-9_-]`；appId为wx+16位十六进制字符；禁止将普通微信号当成可代付账号 |

`accountName` 为1—128字节收款资料，不代表实名认证通过；完整资料仅写入绑定接口，保存后只显示脱敏结果。不能放入 URL、浏览器持久缓存、监控埋点、console、错误详情或前端日志。绑定表单关闭/切换账号/组件销毁即清除敏感状态。完整资料只保留在该表单的临时内存中用于原请求重试。

requestId 为8—64位 `[A-Za-z0-9_-]`，创建前固定。相同请求与同一份资料返回原绑定（包括已停用状态）；变更参数或换请求号重复绑定返回49104。超时后保留当前表单原请求重试，或者关闭表单并刷新列表核对；不能自动换键反复新增。后端永久防重，关闭表单不会删除服务器已提交的绑定。首次同币种无默认账户时，新绑定自动设默认。

JSON只接受单个对象、精确字段名，拒绝未知/重复键、null、尾随值、错误类型及非UTF-8。禁止提交 ownerNo/ownerType/anchorNo/userId/id/version/status/isDefault/验证结果。POST不接受query。请求体8KiB，query512字节，鉴权头8KiB，预算8秒；APP单进程16并发、固定4096桶、每秒补1、突发10；429/503遵守Retry-After。

## 提现申请变更

原接口 `POST /api/v1/app/earning/withdrawal/apply` 改为：

```json
{
  "requestId": "withdraw_20261102_001",
  "currency": "CNY",
  "amount": "20450",
  "payeeAccountNo": "pa_0123456789abcdef0123456789abcdef"
}
```

示例编号仅用于说明，不是可使用的账户。amount以最小单位计，CNY为分，仍用BigInt转换；不能提交银行卡号、accountName、payeeVersion、riskFlags或审核状态。未传payeeAccountNo的新申请返回49001；不属于本人、停用、币种不符或不可用返回49005。不会自动使用默认账户补齐缺参。

APP提现列表/详情/申请响应增加安全字段 `payeeAccountNo,payeeMethod,payeeVersion`，并保留原 `withdrawalNo,requestId,amount,currency,accountMasked,status,createdAt,reviewedAt`。payeeAccountNo为内部公开业务编号，不是数据库ID；旧占位单该字段为空，原accountMasked继续显示。不要对旧单自动补绑。pending_review/approved/rejected状态未变，approved只显示“审核通过·待出款”。

选择列表仅包含本人、active、币种匹配的绑定。默认账户仅初选，可自行更换；没有账户先引导绑定。确认弹窗需冻结金额、币种与账户选择，请求结束前禁用重复点击。提交后持久保存的恢复数据只含 `requestId,currency,amount,payeeAccountNo,createdAt`，按登录账号隔离。

超时/断网/5xx/49003/49004后先GET withdrawal/detail?requestId=原键；若尚无结果，只能以原四项参数重试。不同账户、金额或币种使用相同键返回49004。已提交账户后来停用，原申请结果仍可恢复，但不能创建新单，也不能修改旧单账户。不要因更换默认账户而改写待确认参数。

H5保留原v1本地待确认记录兼容读取。旧记录缺少payeeAccountNo时只查询，不自动补账。仅在服务端明确49006且距请求至少30秒后，经用户显式确认清除本地未创建请求，再重新绑定并填写新申请。多实例升级须避免旧/新后端混跑提现写请求；不允许将历史已创建单按该流程换账户。

H5使用同页面并发锁、不可变待确认参数、账号/令牌代际校验，并在浏览器支持Web Locks时让提交、恢复和本地旧请求清理共用跨标签互斥。清理持久记录前匹配原四项参数，迟到请求不能删掉另一页面新保存的请求。服务端仍以requestId及事务核验为权威；不支持Web Locks的旧浏览器应避免多窗口操作，生产需验收目标浏览器。

## 管理后台变化

原提现列表增加收款方式、脱敏账户及版本。新增可选 `payeeAccountNo` 精确筛选（仅后台 withdrawal/list），使用独立数据库索引，便于从一个收款绑定追查提现单。APP不能使用该筛选查询他人资料。

原 withdrawal/detail 增加：

```json
{
  "currentPayee": {
    "account": null,
    "state": "legacy_placeholder",
    "matchesSnapshot": false,
    "usable": false
  }
}
```

account存在时为同一个安全Payee DTO。state可能active/disabled/missing/legacy_placeholder/unverifiable/unavailable；matchesSnapshot比较不可变资料，usable还要求当前active且密文完整。此信息仅供展示，人工批准仍在写事务内重新执行全部账务和账户检查。后台没有改绑、解密、实际打款或历史单补选按钮。

## 错误处理

| code | 含义 |
| --- | --- |
| 49101 | 参数、大小或JSON结构无效 |
| 49102 | 当前APP账号/主播不可用或无权限 |
| 49103 | 存储、密钥/密文完整性或临时服务问题；查询结果后原参数重试 |
| 49104 | 已绑定或幂等参数冲突，刷新本人列表，不可自动换键 |
| 49105 | 账户不存在/不属于本人/已停用/不适用（不暴露他人是否存在） |
| 49106 | 已达每主播100条历史绑定上限 |
| 49107 | 限流，遵守Retry-After |

沿用earning的490xx错误码。收款接口和提现接口的 `X-Request-ID` 是服务端诊断编号，不能取代客户端幂等requestId；报障记录该响应头和withdrawalNo，不记录收款原文。
