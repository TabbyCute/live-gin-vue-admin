# 客户端礼物对接文档

适用范围：APP / H5 / 小程序的礼物分类、礼物面板、送礼与本人消费记录。服务前缀为 `/api/v1/app/gift`，须通过 HTTPS。使用客户端登录返回的 `Authorization: Bearer <APP JWT>`；后台的 `x-token` 不能用于这些接口。

## 接口顺序

| 场景 | 请求 | 关键参数 |
| --- | --- | --- |
| 打开礼物面板 | `GET /category/list` | `page=1&pageSize=100` |
| 加载当前直播间礼物 | `GET /item/list` | `roomNo`、`categoryNo`、`page`、`pageSize` |
| 送礼 | `POST /send` | JSON：`roomNo`、`giftNo`、`quantity`、`requestId` |
| 恢复未确认结果 | `GET /result` | `requestId` 或 `no`，只能选一个 |
| 本人消费历史 | `GET /order/list` | `page`、`pageSize`，可加 `status`、`from`、`to` |

所有业务响应为 `{ "code": 0, "data": ..., "msg": "操作成功" }`；只有 `code === 0` 才是业务成功。401 需要重新登录；429、503、断连和超时需按下面的重试流程处理。响应采用 `Cache-Control: no-store`。

分页为 `{ list, total, page, pageSize }`，`total` 是十进制字符串，空列表是 `[]`。`pageSize` 最大 100，`page` 最大 10000。分类、礼物按排序值及内部稳定次序返回；客户端不要自行按数据库 ID 排序。

`/item/list` 的 `roomNo` 非空时会校验直播间可访问资格；最终送礼仍在事务内再次验证主播、房间和当前场次。**主播不能给自己送礼**，服务端返回 `48012`。前端可以隐藏自送按钮，但服务端限制始终生效。

## 礼物面板

分类使用 `categoryNo`、`name`、`iconUrl`。礼物使用：

| 字段 | 类型 | 用途 |
| --- | --- | --- |
| giftNo / categoryNo | string | 公开配置编号 |
| name / description | string | 展示文字；按纯文本渲染 |
| iconUrl | string | HTTPS 图片 |
| animationType | string | none / svga / mp4 / lottie |
| animationUrl | string | 对应动画资源；none 时为空 |
| coinPrice | string | 每件 COIN 整数价格 |
| maxQuantityPerOrder | string | 单次件数上限，同时不得超过 10000 |
| isPremium | boolean | 贵重礼物展示标识，不代表拥有守护权益 |

2026-11-04 已接入独立守护。`benefitStatus=enabled` 仅说明该礼物配置附赠权益，消费成功后仍须查询 [守护状态接口](../guard/api.md) 确认异步履约；不能仅凭送礼成功响应展示“已开通”。生产开关和已启用礼物见 [部署记录](../guard/deployment.md)。基础初始化礼物为草稿，补齐真实 HTTPS 图片、核定价格并启用后才出现在 APP；启用分类下没有已启用礼物时，面板可以隐藏空分类。

资源加载失败应降级到静态图标；不把返回的文字当 HTML 执行。动画播放不参与扣款成功判定。IM 尚未接入，送礼响应不是“所有观众均已收到动画”的证明。

## 送礼请求与金额精度

```json
{
  "roomNo": "从直播详情取得的公开编号",
  "giftNo": "从礼物列表取得的公开编号",
  "quantity": "2",
  "requestId": "客户端生成并保存的唯一请求键"
}
```

编号必须使用服务端实际返回值。`requestId` 为 8–64 位 ASCII 字母、数字、下划线或短横线，UUID 字符串可直接使用；区分大小写。

`quantity` 必须是无前导零的正整数字符串，不接受数字类型、`01`、小数、负数或科学计数法。禁止提交 `price`、`amount`、`anchorId`、`sessionId`、钱包编号、分成等额外字段；重复 JSON 键、未知字段和多段 JSON 都会被拒绝。

金额、数量和版本都以十进制字符串传输。JavaScript 用 `BigInt` 或可靠的十进制整数库计算展示值，禁止先转换为 `Number`；JSON 请求中再转回字符串：

```js
const quantity = '2'
const displayedTotal = (BigInt(gift.coinPrice) * BigInt(quantity)).toString()
// displayedTotal 仅用于展示，不发送给后端；实际价格以服务端下单时快照为准。
```

价格可能在打开面板后更新；此版本请求不包含价格锁定字段。成功后应使用响应中的 `unitPrice` 和 `grossCoinAmount` 更新消费展示。

## 三个订单接口使用同一个公开 DTO

`/send`、`/result` 和 `/order/list` 的每条订单都只包含以下字段：

```ts
type AppGiftOrder = {
  orderNo: string                  // 客服核验首选编号
  requestId: string                // 客户端本次操作的永久幂等键
  anchorNo: string
  roomNo: string
  sessionNo: string
  categoryNo: string
  categoryName: string
  giftNo: string
  giftName: string
  iconUrl: string
  animationUrl: string
  animationType: 'none' | 'svga' | 'mp4' | 'lottie'
  unitPrice: string
  quantity: string
  grossCoinAmount: string
  refundedCoinAmount: string
  status: 'succeeded' | 'partial_refund' | 'refunded'
  createdAt: number                // Unix 毫秒
}
```

消费净额为 `BigInt(grossCoinAmount) - BigInt(refundedCoinAmount)`。退款不修改原件数、单价或名称快照。当前没有 APP 退款入口，退款由获授权后台执行。

APP 不再返回 `walletTransactionNo`、`walletLedgerNo`、`revenueRuleNo`、分成比例、预计主播收益、平台收入和 `benefitSnapshot`。客服让用户复制 **orderNo** 即可在后台消费记录中找到完整钱包关联；尚未获得订单号时，可提供 **requestId + 本人公开账号** 进行核验。不要让用户提供 JWT、密码或内部数据库 ID。

## 超时、断网、重复点击和恢复

1. 每次用户明确发起的新送礼，生成一个新 `requestId`，将它和固定的 `roomNo/giftNo/quantity` 按当前登录账号保存到持久存储，再发请求。
2. 按钮进入提交中状态，禁止并行创建第二个请求键。重试始终使用原参数和原键。
3. `code=0` 后记录 `orderNo`，清除本次待确认状态，刷新钱包显示；同一 `orderNo` 的成功动画只播放一次。
4. 超时、断连、HTTP 503 或 `48003` 表示结果未知。保留待确认状态，先用原 `requestId` 查询，再以原参数重试；指数退避并设置上限，429 尊重 `Retry-After`。
5. 查询返回 `48006` 也可能是原请求尚未提交完成，不能据此生成新键。仍可用原键重试，数据库唯一约束保证只扣款一次。
6. APP 重启后恢复待确认请求；切换账号时隔离待确认记录，不能替其他账号重试。登录过期后登录原账号再查询。
7. 用户明确再次赠送另一笔时才生成新键。`48004` 需要检查本地参数是否被修改，不要靠更换请求键消除冲突。

同一用户、同一键和同一参数返回原单，即使原场次已结束或当前价格改变；不再扣款。查询始终限制本人，即使知道他人的 `orderNo` 也无法读取。

## 错误处理

| code / HTTP | 客户端处理 |
| --- | --- |
| 48001 | 参数或数量错误，检查类型、上限和请求字段 |
| 48002 | 无权限，停止操作 |
| 48003 / HTTP 503 / 超时断连 | 结果未知，原键查询或退避重试 |
| 48004 | 幂等参数或配置版本冲突，禁止盲目换键 |
| 48005 | 礼物、分类、主播或场次不可用，刷新状态 |
| 48006 | 当前本人查询不到记录，按待确认流程核验 |
| 48008 | 守护售卖、Provider、方案或有效期未就绪，不能承诺权益 |
| 48009 | 账务核查异常，保留 orderNo/requestId 并联系支持 |
| 48010 | 无有效分成规则或规则复核冲突，提示暂不可赠送 |
| 48011 / HTTP 429 | 限流，遵守 Retry-After，保留原键 |
| 48012 | 主播不能给自己送礼 |
| 470xx | 钱包业务错误，沿用钱包对接文档语义 |
| HTTP 401 | 重新登录原账号后恢复查询 |

客户端验收应覆盖：自己给自己送礼、重复点击、响应丢失、重启恢复、数量超过上限、余额不足、管理员停用礼物、场次结束、价格变化、退款后的历史显示、跨账号查询，以及 JS 安全整数范围以外的金额展示。后台接口及权限说明见 [api.md](api.md)，钱包接口见 [钱包客户端对接](../wallet/frontend-client-integration.md)。
