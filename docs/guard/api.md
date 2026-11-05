# 守护客户端和后台对接

统一 `{code,data,msg}`，成功 `code=0`。APP 前缀 `/api/v1/app/guard`，仅 `Authorization: Bearer <APP token>`。管理员前缀 `/api/v1/admin/guard`，使用后台 `x-token`。不能混用两套账号。

## APP

| 方法 | 路径 | 参数/响应 |
| --- | --- | --- |
| GET | /plan/list | page/pageSize（默认 1/20，最大 100）；仅启用方案，建议天数和 version 为字符串 |
| GET | /status | anchorNo 必填；查询 JWT 用户在此主播下的守护，分页 list 无记录时为空 |
| GET | /mine | page/pageSize、anchorNo、status=active/expired；仅本人，不接受 accountNo 或内部 userId |
| GET | /anchor/members | anchorNo 必填、page/pageSize；主播下当前有效会员，公开资料通过统一批量 DTO 返回 |

会员响应示例（编号和时间仅说明格式）：

```json
{"code":0,"data":{"page":1,"pageSize":20,"total":"1","list":[{
  "membershipNo":"gm_example","user":{"accountNo":"10000010","nickname":"用户","avatar":"","signature":"","gender":0,"countryCode":"","isLive":false},
  "anchorNo":"1100003","activePlanNo":"monthly","activePlan":{"planNo":"monthly","name":"月守护","level":1,"priority":10,"benefitConfig":"{}","version":"1"},
  "startedAt":1793500000000,"expiresAt":1796092000000,"remainingMillis":"2592000000","remainingDays":"30","status":"active","generatedAt":1793500000000
}]},"msg":"操作成功"}
```

没有生效方案时 `activePlan=null`、`activePlanNo=""`、`remainingMillis="0"`、`remainingDays="0"`、`status="expired"`。使用 activePlan 的历史快照展示当前权益，不能用最新 plan/list 覆盖已发放权益。时间为服务器计算的毫秒时间戳，剩余时长为十进制字符串。remainingDays 由服务器按 24 小时向上取整（不足一天显示 1 天），不用于延长实际到期时间。客户端计时只用于展示，进入需校验权益的功能时仍向服务端查询。

`user` 复用 UserPublicProfile，只有 accountNo/nickname/avatar/signature/gender/countryCode 及当前访问者可进入的直播资料。账号不可公开时资料可能为空；不能回退展示数据库用户 ID。没有手机、邮箱、实名认证、钱包交易号、主播分成或平台收入。

送礼沿用 `POST /api/v1/app/gift/send`：只提交 roomNo、giftNo、quantity、requestId。不能提交方案、天数、用户或价格。成功表示消费事务成功，守护异步履约；客户端稍后查询 `/status`，不能因暂未到账重新生成送礼 requestId。可保留 gift orderNo 用于客服核验。

## 管理后台

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /plan/list | 分页、status；含 guardEnabled 和实际写权限 actions |
| POST | /plan/save | 新建/修改/启停/排序统一入口，更新必须带原 version |
| GET | /member/list、/member/detail | anchorNo/accountNo/membershipNo/status/page/pageSize；detail 按会员号查询 |
| GET | /grant/list、/compensation/list、/ledger/list | membershipNo/page/pageSize，保留来源和完整审计 |
| GET | /task/list | 分页任务与脱敏失败原因 |
| POST | /task/retry | requestId、number=taskNo；仅失败任务可人工重试 |
| POST | /reconcile、/rebuild | requestId、number=membershipNo；核验/从不可变来源重建，不能指定任意到期时间 |

保存方案：

```json
{"requestId":"admin_request_001","planNo":"monthly","name":"月守护","level":1,"priority":10,"defaultDurationDays":"30","benefitConfig":"{}","status":"disabled","sortOrder":0,"version":""}
```

建议天数 1～3650；礼物的每件天数单独配置。方案编号 1～40 位字母/数字/下划线/短横线。requestId 8～64 位同字符集合，永久唯一；相同操作者和参数重试返回原结果，改变参数返回 49003。未知结果保持原请求键，不能修改载荷后继续用同一键。

普通管理员每次读取当前启用账号、真实角色成员和精确操作授权；ID 1 沿用固定超级管理员例外。页面 actions 仅辅助展示，后端始终再次鉴权。

## 安全与错误

49001 参数无效；49002 无权限；49003 幂等/版本/租约冲突；49004 服务暂不可用；49005 数据核验异常；49006 记录不存在；49007 限流。未迁移的兼容发布返回 HTTP 503，不会出售守护。

拒绝未知/重复参数、重复 JSON 键、多 JSON 值、非法 UTF-8 和超大请求。正文 64 KiB、query 2048 字符、认证头 8192 字符；APP/后台各 64/16 在途请求，8 秒预算，无等待队列；APP 账号固定槽限流 10 次/秒、突发 20。响应 no-store，错误不泄露 SQL、地址、凭证或堆栈。跨实例限流需部署网关落实。
