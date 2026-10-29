# 钱包前端客户端对接文档

本文面向 APP、H5、小程序等用户端前端。客户端仅能查询当前登录用户本人的 COIN 余额与流水，不能传入其他用户账号，也不能创建充值、审核、修改钱包状态或直接修改余额。

## 1. 接口概览

默认服务根路径为 `/api`，客户端钱包路径如下；部署环境应以实际网关域名和 `system.router-prefix` 为准。

| 功能 | 方法 | 路径 |
| --- | --- | --- |
| 查询本人余额 | GET | `/api/v1/app/wallet/balance` |
| 查询本人流水 | GET | `/api/v1/app/wallet/ledger` |

请求使用独立 APP JWT：

```http
Authorization: Bearer <app-access-token>
```

不要使用管理后台的 `x-token`、Cookie 或管理员 JWT。服务端从已验证的 APP JWT 中读取本人账号，不接受 `accountId`、`accountNo` 等选人参数。

统一响应：

```ts
interface ApiResponse<T> {
  code: number
  data: T
  msg: string
}
```

- HTTP `200` 且 `code === 0` 表示成功。
- HTTP `401` 表示登录失效或令牌类型错误，应进入统一重新登录流程。
- HTTP `200` 且 `code !== 0` 表示参数或业务失败，不能当作成功数据渲染。
- 不要根据 `msg` 文本判断业务；展示可使用 `msg`，程序分支使用 HTTP 状态和 `code`。
- 钱包接口返回 `Cache-Control: no-store`。不要在网关、Service Worker或客户端持久缓存中缓存资金响应；页面状态可暂存当前用户数据，并在切换账号时清空。

## 2. 金额与时间规则

所有余额、变动金额和钱包版本都是十进制字符串，必须全程按字符串保存：

```ts
type IntegerString = string

interface WalletBalance {
  currencyCode: 'COIN'
  availableBalance: IntegerString
  frozenBalance: IntegerString
  version: IntegerString
  status: 'normal' | 'disabled'
  updatedAt: number
}
```

禁止使用 JavaScript `Number`、`parseInt` 或浮点数保存和计算余额，数值超过 `2^53 - 1` 后会丢失精度。只做展示时可以对字符串插入千分位；需要前端整数运算时使用 `BigInt`，并确认目标运行环境支持：

```ts
const formatInteger = (value: string) => {
  const negative = value.startsWith('-')
  const digits = negative ? value.slice(1) : value
  return `${negative ? '-' : ''}${digits.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}`
}

const canAfford = (balance: string, required: string) => BigInt(balance) >= BigInt(required)
```

当前 COIN 只定义为整数钱包单位，不定义法币兑换率和小数位。`updatedAt`、`createdAt` 是 Unix 毫秒时间戳；值为 `0` 表示尚无对应时间。

## 3. 查询本人余额

```http
GET /api/v1/app/wallet/balance?currencyCode=COIN
Authorization: Bearer <app-access-token>
```

`currencyCode` 可省略，当前只接受 `COIN`。成功示例：

```json
{
  "code": 0,
  "data": {
    "currencyCode": "COIN",
    "availableBalance": "1200",
    "frozenBalance": "0",
    "version": "3",
    "status": "normal",
    "updatedAt": 1793174400000
  },
  "msg": "成功"
}
```

尚未发生过充值的正常用户返回虚拟零余额：

```json
{
  "currencyCode": "COIN",
  "availableBalance": "0",
  "frozenBalance": "0",
  "version": "0",
  "status": "normal",
  "updatedAt": 0
}
```

读取余额不会自动开户。`status === "disabled"` 表示钱包当前停止资金变动，但历史余额和流水仍可展示。客户端不应自行把停用状态改成正常，也不要隐藏已有余额。

## 4. 游标查询本人流水

首次请求：

```http
GET /api/v1/app/wallet/ledger?currencyCode=COIN&limit=20
Authorization: Bearer <app-access-token>
```

后续请求原样传回上一页的 `nextCursor`：

```http
GET /api/v1/app/wallet/ledger?currencyCode=COIN&limit=20&cursor=<nextCursor>
```

参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| currencyCode | 否 | 当前仅 `COIN`，默认 `COIN` |
| limit | 否 | 1—100，默认 20 |
| cursor | 否 | 不透明游标；只能原样传回，不能解析、修改或跨用户复用 |

响应类型：

```ts
interface WalletLedgerItem {
  ledgerNo: string
  currencyCode: 'COIN'
  walletVersion: IntegerString
  changeAmount: IntegerString
  balanceBefore: IntegerString
  balanceAfter: IntegerString
  businessType: 'ADMIN_GRANT' | 'ADMIN_REVERSE' | string
  businessNo: string
  originalLedgerNo?: string
  createdAt: number
}

interface WalletLedgerPage {
  list: WalletLedgerItem[]
  nextCursor: string
  hasMore: boolean
}
```

示例：

```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "ledgerNo": "wl_0123456789abcdef0123456789abcdef",
        "currencyCode": "COIN",
        "walletVersion": "3",
        "changeAmount": "-100",
        "balanceBefore": "1300",
        "balanceAfter": "1200",
        "businessType": "ADMIN_REVERSE",
        "businessNo": "wa_0123456789abcdef0123456789abcdef",
        "originalLedgerNo": "wl_abcdef0123456789abcdef0123456789",
        "createdAt": 1793174400000
      }
    ],
    "nextCursor": "MToy",
    "hasMore": true
  },
  "msg": "成功"
}
```

流水按钱包版本倒序返回。`changeAmount > 0` 是入账，`changeAmount < 0` 是冲正或未来的扣减业务。当前已开放业务：

| businessType | 客户端文案建议 |
| --- | --- |
| ADMIN_GRANT | 人工充值 |
| ADMIN_REVERSE | 充值冲正 |
| 未知新值 | 余额变动 |

客户端必须允许未知 `businessType` 正常展示，避免后端以后增加消费等类型时页面崩溃。APP 响应不会返回管理员 ID、内部审核原因或资金凭证。

## 5. 推荐的状态管理和刷新策略

```ts
const wallet = reactive({
  balance: null as WalletBalance | null,
  ledger: [] as WalletLedgerItem[],
  nextCursor: '',
  hasMore: false,
  loadingBalance: false,
  loadingLedger: false
})
```

推荐行为：

1. 进入钱包页时并行请求余额和流水首页。
2. 下拉刷新时丢弃旧游标，重新请求余额及流水首页；新流水不会插入已经加载的旧分页中。
3. 加载更多期间禁用重复触发。同一个 `nextCursor` 不要并发请求多次。
4. 用户切换、退出登录或令牌失效时，清空余额、流水和游标，并使已经发出的旧请求失效；可使用请求序号，仅允许最新序号的响应更新页面，防止旧响应晚到后再次填入前一个用户的数据。
5. 页面回到前台时可刷新余额；不要高频轮询。若以后接入服务端推送，收到“钱包已更新”事件后仍应重新拉取权威余额。
6. 不要在客户端把流水累加结果当作权威余额，也不要乐观增加充值金额；只信任余额接口及服务器已提交的流水。

## 6. 错误处理

客户端通常会遇到以下业务码：

| code | 含义 | 客户端处理 |
| --- | --- | --- |
| 47001 | 参数无效 | 修复客户端参数，不要自动重复请求 |
| 47003 | 用户不存在或账号不可用 | 清理用户会话并提示联系客服 |
| 47011 | 账务不变量异常 | 停止金额相关交互，展示通用异常并提示联系客服 |
| 47012 | 钱包服务暂不可用 | 保留当前已展示数据，允许用户稍后手动重试 |

后台人工充值相关的 47002、47004—47010、47013—47015 不应从 APP 本人查询接口正常返回。仍应保留未知错误码兜底，不展示数据库错误或堆栈。

## 7. 安全要求

- 只通过 HTTPS 访问生产接口；不要把令牌、完整响应或钱包余额写入公开日志、埋点参数和崩溃截图。
- APP JWT 与后台 `x-token` 必须分开存储和发送。不要在查询参数中携带令牌。
- 不要尝试传 `accountNo`、`accountId`、`operatorId`、`reason` 等后台字段；接口会拒绝未知查询参数。
- 不要解析或持久化依赖游标内部格式。游标属于当前钱包和当前分页序列。
- 客户端只能展示充值结果，不能以“调用成功动画”“支付页返回”或本地缓存推断到账。后台审核完成后，重新查询余额才是最终结果。
- 金额文案应明确单位为 COIN，不显示未经产品定义的法币价值。

## 8. Axios 示例

```ts
import axios from 'axios'

const appApi = axios.create({ baseURL: '/api/v1/app', timeout: 8000 })

appApi.interceptors.request.use((config) => {
  const token = getAppAccessToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

export const getMyWalletBalance = () =>
  appApi.get<ApiResponse<WalletBalance>>('/wallet/balance', { params: { currencyCode: 'COIN' } })

export const getMyWalletLedger = (cursor = '', limit = 20) =>
  appApi.get<ApiResponse<WalletLedgerPage>>('/wallet/ledger', {
    params: { currencyCode: 'COIN', limit, ...(cursor ? { cursor } : {}) }
  })
```

项目管理后台的实现位于 `web/src/view/wallet/`，可作为状态、金额和游标展示的参考；客户端不能复用其中的后台充值、审核和状态接口。
