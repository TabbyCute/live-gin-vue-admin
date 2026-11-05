# 榜单 APP/后台对接

APP 前缀 `/api/v1/app/ranking`，全部要求独立 APP Bearer JWT；后台前缀 `/api/v1/admin/ranking/admin`，后台 x-token 和独立精确操作权限。

## APP 接口

`GET /gift`、`GET /wealth`、`GET /charm`。

| 参数 | 含义 |
| --- | --- |
| period | day/week/month/all，默认 day |
| periodKey | 日 YYYY-MM-DD，周填周一 YYYY-MM-DD，月 YYYY-MM，总榜 all；缺省服务器当前期间 |
| scopeType/scopeNo | gift 必填 anchor/room/session 与对应公开编号；wealth 固定 platform 且无编号；charm 支持 platform 或 category+礼物分类编号 |
| giftNo | 仅礼物榜可选，留空表示全部礼物 |
| page/pageSize | 默认 1/20，page≤10000、pageSize≤100 |

示例：

```http
GET /api/v1/app/ranking/gift?scopeType=anchor&scopeNo=1100003&period=week
GET /api/v1/app/ranking/wealth?period=all&page=1&pageSize=20
GET /api/v1/app/ranking/charm?period=month&periodKey=2026-11
```

响应：

```json
{"code":0,"data":{
  "list":[{"rank":"1","score":"18446744073709551614","quantity":"200","subjectNo":"10000010","subjectType":"user","user":{"accountNo":"10000010","nickname":"用户","avatar":"","signature":"","gender":0,"countryCode":"","isLive":false}}],
  "total":"1","page":1,"pageSize":20,"rankType":"wealth","period":"all","periodKey":"all","timezone":"Asia/Kuala_Lumpur","scopeType":"platform","scopeNo":"","giftNo":"",
  "myRank":{"rank":"1","score":"18446744073709551614","quantity":"200","subjectNo":"10000010","subjectType":"user","user":{"accountNo":"10000010","nickname":"用户","avatar":"","signature":"","gender":0,"countryCode":"","isLive":false}},
  "generatedAt":1793500000000,"dataAsOf":1793499999000,"lagCount":"0"
},"msg":"操作成功"}
```

金额、件数、排名、total、lagCount 都是十进制字符串。使用 BigInt/十进制库或直接展示字符串；不要转 Number。myRank 是当前 JWT 用户的名次（魅力榜为其主播身份），未上榜时 null，不受当前页限制。公开资料复用 UserPublicProfile，资料失效不暴露内部 ID。房间/场次礼物榜继续校验当前用户的直播访问权限；不能借榜单绕过私密房间或拉黑。

dataAsOf 是最近一次追平核验的时间，generatedAt 是本响应生成时间；lagCount>0 表示异步投影尚未追平，不能将榜单显示当成支付回执。当前实现每次查询读取 MySQL，缓存删除不影响结果。

## 后台接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /board | 同上参数，另传 rankType=gift/wealth/charm；不返回管理员个人排名 |
| GET | /status | checkpoint、lagCount、脱敏错误和实际管理权限 actions |
| GET | /job/list | page/pageSize，历史重建任务 |
| POST | /rebuild | `{requestId,scope:"all"}` 创建可恢复的新代际任务，同一时间只允许一个重建 |
| POST | /reconcile | `{requestId,scope:"all"}` 对比来源/投影，记录核验审计；不修改分数 |

不存在直接修改分值的入口。普通管理员需要 `/ranking/admin/<path>` + HTTP 方法的独立授权；ID 1 沿用项目超级管理员规则。请求键永久去重，超时或断连保持原键重试。错误、安全预算与[守护接口](../guard/api.md#安全与错误)一致。
