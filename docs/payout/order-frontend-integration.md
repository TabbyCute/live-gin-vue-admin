# 打款中心前端对接（2026-11-02）

> 此文保留绑定/ready阶段记录；人工出款、paid、取消、权限与迁移以 [当前人工出款文档](manual-operations.md) 和 [前端契约](manual-frontend-integration.md) 为准。

本阶段在现有绑定、提现选账及审核上增加唯一的待出款订单。管理后台已实现二次确认、列表和只读详情；H5继续使用已实现的绑定与提现流程，无新增付款接口。APP接入说明仍以[收款账户及提现契约](frontend-integration.md)为准。

## 管理后台接口

鉴权使用后台 `x-token`；APP Bearer token不能调用。默认路径如下，以部署全局router prefix为准。统一响应 `{code,data,msg}`，成功仅 `code=0`；响应 `Cache-Control: no-store`，`X-Request-ID`用于诊断。菜单可见性不能代替服务器权限校验。

| 方法和路径 | 请求 | data |
| --- | --- | --- |
| POST `/api/v1/admin/payout/order/create` | 下述两个字符串字段，不接受query | 安全Order |
| GET `/api/v1/admin/payout/order/list` | 下表筛选 | `{page,pageSize,total,list}` |
| GET `/api/v1/admin/payout/order/detail` | `payoutNo` | 下述详情结构 |

```json
{"requestId":"payout_submit_20261102_001","withdrawalNo":"wd_0123456789abcdef0123456789abcdef"}
```

示例编号不是实际数据。`requestId`为8—64位 `[A-Za-z0-9_-]`，创建确认时固定；`withdrawalNo`为`wd_`加32位小写十六进制。不能提交金额、币种、主播、账户、版本、状态、风险判断或付款结果。只接受单个JSON对象，拒绝未知/重复键、null、错误类型、尾随数据和非UTF-8。请求体8KiB、query512字节、鉴权头8KiB，服务预算8秒；后台单进程最多8个在途请求、固定4096身份桶、每秒补1令牌/突发10，429/503按Retry-After重试。

| 列表字段 | 规则 |
| --- | --- |
| page / pageSize | 默认1/20；page 1—10000，pageSize 1—100 |
| payoutNo / withdrawalNo | 完整业务编号精确匹配 |
| anchorNo | 1—40位字母/数字/下划线/连字符，精确匹配 |
| currency | 3—8位大写字母，精确匹配 |
| payeeMethod | bank_card / alipay / wechat |
| status | 本期仅ready |
| from / to | Unix毫秒，`createdAt >= from AND createdAt < to`；均提供时from小于to |

空筛选字段应省略，禁止重复query键和未知字段。列表按ID倒序；count和当前页在同一只读一致性事务中获取；页面切换不承诺跨请求快照。

安全Order字段：`payoutNo,businessType,businessNo,anchorNo,amount,currency,payeeAccountNo,payeeVersion,payeeMethod,accountMasked,payeeSnapshotHash,status,createdBy,createdAt,updatedAt`。`businessType=anchor_withdrawal`、`businessNo=withdrawalNo`、`status=ready`。金额为最小货币单位十进制字符串，`createdBy`和版本为字符串，时间为毫秒数字，分页`total`为字符串。金额不经过Number浮点换算；列表显示原最小单位并标明币种。快照哈希是专属加密封装的SHA256，不是可枚举账号的明文哈希。

详情data：

```text
order: Order
withdrawal: 原后台安全提现DTO
currentPayee: {account: 安全Payee|null, state, matchesSnapshot, usable}
history: [{eventNo,payoutNo,fromStatus:"",toStatus:"ready",actorId,createdAt}]
audit: [{eventNo,payoutNo,action:"order.create",actorId,traceId,snapshot,createdAt}]
consistent: 当前账务和账户核验是否通过
checkedAt: 当前核验时间（毫秒）
creationReconciliation: 创建时核验快照（安全JSON字符串）
```

audit.snapshot为安全Order JSON字符串。currentPayee各状态沿用绑定文档。创建时核验事实不可变；后来账户停用会使当前consistent=false，而原订单保持ready。数据库/密钥服务不可用返回错误，不能把它显示为“已核验”。详情没有编辑、删除、取消、重新打款、确认成功/失败或流水号输入能力；接口不存在上述动作。

## 提现审核页衔接

原后台 `earning/withdrawal/list` 和 `earning/withdrawal/detail` 的提现数据新增可选 `payoutNo,payoutStatus`。由批量查询/详情查询payout_order投影得到，不写回提现表；未提交时省略。现有 `payeeRef,payeeMethod,accountMasked,payeeVersion,accountName`安全快照与详情currentPayee继续复用。后台不能更换账户。

按钮显示要求：`status=approved`、`reconciliationStatus=passed`、`payeeRef`是真实`pa_`业务编号且尚无payoutNo。点击“提交打款中心”后展示提现号、主播、金额/币种、方式、脱敏账户和版本，并提示“本操作只创建待出款订单，不会实际打款”。服务端始终重做权限和财务核验，不信任按钮判断。

创建成功显示“已提交打款中心”、payoutNo和“待出款”，刷新提现列表后禁止再次提交。资金仍冻结，提现仍approved；不得显示“已到账”。`overview.payoutEnabled=false`继续表示**实际付款不可用**，不能用这个字段隐藏本期后台创建ready订单的能力。H5不新增payoutNo显示，approved仍只表示审核通过、待出款。

相同requestId+withdrawalNo返回原订单；同一withdrawalNo换requestId也只返回原订单，并永久登记该新键；任何已使用requestId换withdrawalNo返回49204。二次确认弹窗冻结原请求，提交期间锁定，网络结果不明确时保留原参数重试；也可关闭弹窗后按withdrawalNo查询订单列表。即使关闭后换键重开，服务端业务唯一约束也防止重复订单。历史订单账户后来停用不影响查询或幂等恢复，但阻止新订单；恢复原订单不等于重新授权付款。

独立一级“打款中心”→“出款订单”；动态菜单组件`view/payout/order/index.vue`。root用户ID=1沿用完整权限，初始角色888同步两个菜单和三个API精确权限；普通角色不自动授权。需要给普通角色开放时，沿用金融模块的`anchor_finance_permission`精确路径/方法授权及用户角色归属校验，仅赋菜单不足以调用接口。

## 错误与安全

| code | 客户端行为 |
| --- | --- |
| 49201 | 参数无效，修正输入 |
| 49202 | 无当前后台操作权限 |
| 49203 | 服务/存储/加密完整性暂不可用；先按提现号查询，再原参数重试 |
| 49204 | 幂等键已绑定其他提现，不自动换键掩盖冲突 |
| 49205 | 提现、账户或账务核验未通过，刷新详情并排查 |
| 49206 | 出款单不存在 |
| 49101 | 公共请求大小或鉴权头保护拒绝 |
| 49103 | 公共并发保护暂不可用，按Retry-After重试 |
| 49107 | 限流，遵守Retry-After |

鉴权失败沿用项目统一登录处理。前端使用固定安全错误文本，不展示底层异常。列表/详情采用请求序列校验，切换令牌或卸载时使旧响应失效并清理数据。使用Vue文本插值/白名单快照组件，无v-html。完整账号、姓名、密文、nonce、key ID、fingerprint和数据库内部主键均不出现在API、页面、日志或审计。
