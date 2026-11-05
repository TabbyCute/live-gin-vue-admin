# 礼物 HTTP 接口

统一`{code,data,msg}`；分页`{list,total,page,pageSize}`，total、金额、数量、万分比和版本使用十进制字符串；时间使用毫秒时间戳，sortOrder为有界整数。所有APP响应隐藏数据库内部ID。Swagger已同步，可从服务内Swagger查看完整请求模型。

## APP

前缀`/api/v1/app/gift`，全部使用独立APP JWT：`Authorization: Bearer <token>`。不接受管理员token。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /category/list | 仅启用分类，page/pageSize；稳定sort_order ASC,ID ASC |
| GET | /item/list | 仅启用礼物且分类启用；categoryNo、name、minAmount、maxAmount、page/pageSize；roomNo非空时检查当前直播访问资格 |
| POST | /send | roomNo、giftNo、quantity、requestId，其他JSON字段拒绝 |
| GET | /result | no=订单号或requestId=幂等键，二选一，仅本人 |
| GET | /order/list | 本人消费记录，筛选和分页，不接受他人内部用户ID |

送礼请求示例（编号需使用实际配置）：

```json
{"roomNo":"room0001","giftNo":"gift0001","quantity":"2","requestId":"client_request_000001"}
```

requestId为8—64位ASCII字母、数字、`_`或`-`，区分大小写。同用户同key同roomNo/giftNo/quantity返回原单，即使当前礼物价格已变或场次已结束。不同参数返回48004。数量必须是无前导零的正整数字符串，最大10000并受礼物每单上限限制。金额只能来自服务端定价；价格、amount、anchorId、sessionId等伪造字段直接拒绝。

HTTP超时、断连、503均不能解释为“未扣款”。先保留原参数，再用原key查询或重试。APP无退款或任意金额扣款入口。

主播自送返回48012。送礼、结果查询和消费历史统一返回显式 `AppOrder` DTO：公开订单号、请求键、主播/房间/场次编号、礼物展示快照、数量、消费额、退款额、状态和时间。内部钱包编号、分成规则/比例、预计主播及平台收入、权益内部快照仅后台可见。客服通过orderNo关联后台钱包明细；完整字段与重试流程见[客户端对接文档](frontend-client-integration.md)。

## ADMIN

前缀`/api/v1/admin/gift`，后台JWT `x-token`；启用管理员+真实角色成员+gift_admin_permission完整路径/方法三重检查。查询、配置、退款、分成创建、分成审核分别授权，菜单可见不等于有权执行。

| 方法 | 路径 | 请求与行为 |
| --- | --- | --- |
| GET | /category/list、/item/list | 后台含草稿/停用配置；状态、名称、分类、价格范围筛选；返回目录version |
| POST | /category/save | 新建不传version；更新必须传旧version和完整配置 |
| POST | /item/save | 新建不传version；更新必须传旧version和完整配置；资源URL仅HTTPS |
| POST | /sort | kind=category或item；numbers完整编号集合；item需categoryNo；version=目录版本 |
| POST | /delete | kind、no、该条配置version；被使用/产生订单禁止删除 |
| GET | /order/list | 下表筛选；稳定排序 |
| GET | /order/detail | no；返回订单、退款、收入、pending收益、权益补偿、广播事件及consistent/issues |
| POST | /refund | orderNo、quantity、requestId、reason；同一操作保留原key和参数 |
| GET | /ranking | dimension、period、from/to、筛选、分页、白名单排序 |
| GET | /rule/list | status、page/pageSize，按ID DESC |
| POST | /rule/create | name、scopeType、scopeNo、priority、anchorShareBps、effectiveFrom/To；创建pending版本 |
| POST | /rule/review | ruleNo、approve；不同管理员复核，重叠范围拒绝生效 |
| POST | /rule/terminate | ruleNo、requestId、reason；对已批准且未到期规则发起提前终止，永久幂等 |
| POST | /rule/terminate/review | terminationNo、approve；申请人以外的授权管理员复核，通过后即时停止新订单匹配 |

规则创建和批准时都核验分类/礼物/主播是否存在。分类或礼物被任何历史规则引用后禁止物理删除，使用停用管理。rule/list 的 `status` 表示原审核状态，新增 `termination` 表示最新终止申请（可为null），包含编号、原因、pending/approved/rejected、申请/复核人和时间。原规则的有效期和审批快照保持不变；有效结束边界是原effectiveTo与已批准终止时间的较早者。新的同范围版本只能在该边界之后生效。

订单筛选：no、accountNo（精确公开账号）、anchorNo、roomNo、sessionNo、categoryNo、giftNo、status、from/to、minAmount/maxAmount、refunded=yes/no；状态succeeded/partial_refund/refunded。page最多10000、pageSize最多100。

订单sort：createdAt/amount/refund/quantity，desc布尔；礼物sort：price/sortOrder。排序不接受SQL列名或表达式，始终有ID次级排序。

排行dimension：sender/anchor/room/session/gift/category/platform；period：day/week/month/custom；自定义必须from/to且不超过93天；按anchorNo/roomNo/sessionNo/categoryNo/giftNo筛选。sort：gross/refund/net/earning/quantity/orders/users；按编号稳定次级排序。窗口按UTC解释，默认净额降序。

退款成功响应含refundNo和walletRefundTransactionNo。客户端不要把旧订单的estimatedAnchorEarningCoin理解为退款后余额；它是送礼时原始快照，净额以详情事实/统计为准。退款窗口网络失败后保留原key；关闭或刷新浏览器后必须先从原订单详情核查结果，禁止盲目再次发起新退款。

## 安全和错误

拒绝重复JSON键、未知字段、多JSON值、非法UTF-8、重复查询键和未知查询参数。请求体上限64KiB，URL query上限2048字符，JWT头上限8192字符。APP与ADMIN分别128/32在途上限，无等待队列；8秒服务预算。APP固定4096槽哈希限流，单槽10次/秒、突发20；碰撞可保守限流。多实例网关仍需按账户/IP统一限流。容量拒绝503、限流429及Retry-After；非法/业务错误沿用统一响应code。响应Cache-Control=no-store。

48001参数无效；48002无权限；48003依赖/超时，请原key查询；48004幂等或版本冲突；48005当前状态不可用；48006无本人记录；48007超退；48008守护售卖开关、Provider、方案或有效期未就绪；48009账务不一致；48010分成规则无效/复核冲突；48011限流；48012主播自送。钱包业务错误保留470xx语义，不泄露SQL、连接信息或堆栈。守护与公开榜单接口见 [guard](../guard/api.md)、[ranking](../ranking/api.md)。

新增`giftSummary`只对具有`/gift/ranking GET`权限的后台查询返回。旧live_session的giftCount/giftCoinAmount/giftUserCount不再通过HTTP返回为财务指标，请使用giftSummary中的字符串金额。
