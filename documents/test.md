请基于我当前项目代码，实现「直播主播 LiveAnchor」相关客户端接口和后台管理接口。

## 一、总体要求

请先完整阅读当前项目中已有的：

* LiveAnchor model
* user / sys_user 相关 model
* router
* api
* service
* request / response DTO
* middleware
* JWT / 登录用户获取方式
* 权限控制方式
* Swagger 写法
* global response 写法
* GORM 使用规范

然后严格按照当前项目已有代码风格实现。

不要重新设计项目架构，不要引入新的框架，不要修改现有公共基础设施。

### 必须遵守以下规则

1. 只允许使用：

  * GET
  * POST

2. 客户端主播接口统一：

```text
/live/anchor/*
```

3. 后台主播管理接口统一：

```text
/live/anchor_admin/*
```

4. 后台接口必须接入当前项目已有管理员 JWT + RBAC 权限体系。

5. 客户端接口必须从当前登录用户 JWT / context 获取 user_id。

禁止客户端自己提交 user_id 来操作主播数据。

例如：

```text
POST /live/anchor/profile/update
```

只能修改当前登录用户自己的主播资料。

6. 必须按照项目现有规范拆分：

```text
router
api
service
model
request
response
```

不要把数据库操作直接写在 api/controller 中。

7. 所有接口必须编写完整 Swagger 注释。

Swagger 至少包括：

```text
@Tags
@Summary
@Description
@Accept
@Produce
@Param
@Success
@Failure
@Router
@Security
```

8. 所有参数必须进行校验。

9. 数据库更新必须避免：

```go
db.Save()
```

这种全字段覆盖更新。

优先使用：

```go
Updates(map[string]interface{}{})
```

或者明确指定字段。

防止客户端修改不应该修改的字段。

10. 所有后台管理修改操作必须明确白名单字段。

11. 所有接口统一使用项目已有：

```text
response.Ok
response.OkWithData
response.FailWithMessage
```

等统一返回结构。

12. 不要重复创建已有公共代码。

13. 如果当前项目中已经存在 pagination、JWT claims、operatorId、IP 获取、日志等公共方法，直接复用。

---

# 二、客户端接口

实现以下接口：

```text
GET  /live/anchor/info
POST /live/anchor/apply
GET  /live/anchor/apply/status
POST /live/anchor/profile/update
GET  /live/anchor/detail
GET  /live/anchor/live/check
GET  /live/anchor/pk/check
```

---

# 三、GET /live/anchor/info

获取当前登录用户自己的主播信息。

user_id 必须从 JWT 中获取。

逻辑：

```text
当前登录用户
    ↓
获取 user_id
    ↓
查询 live_anchor.user_id
```

如果当前用户不是主播：

不要直接返回数据库错误。

返回正常业务响应，例如：

```json
{
  "isAnchor": false,
  "anchor": null
}
```

如果已经是主播：

```json
{
  "isAnchor": true,
  "anchor": {}
}
```

具体返回字段根据现有 LiveAnchor model 和安全要求设计。

以下字段不能返回给普通客户端：

```text
管理员备注
风控内部数据
审核人
内部审核备注
后台操作信息
其他纯后台字段
```

---

# 四、POST /live/anchor/apply

主播申请接口。

当前登录用户申请成为主播。

user_id 必须从 JWT 获取。

禁止前端传递 user_id。

需要检查：

```text
1. 用户是否存在
2. 是否已经存在主播
3. 是否已经提交申请
4. 当前申请状态是否允许重新申请
5. 必填资料是否完整
```

必须防止重复申请。

即使客户端并发调用，也不能生成重复主播记录。

依赖：

```text
live_anchor.user_id UNIQUE
```

进行最终数据库唯一性保护。

申请成功后进入待审核状态。

例如：

```text
audit_status = pending
```

具体枚举值必须读取现有 LiveAnchor model/constants，不要自行重复定义。

如果当前系统的设计是：

```text
申请时直接创建 LiveAnchor
```

则按照该方式实现。

如果项目已经存在 AnchorApply 独立申请表，则使用已有设计。

不要擅自创建新的申请表。

---

# 五、GET /live/anchor/apply/status

获取当前用户主播申请状态。

返回至少表达：

```text
是否申请
审核状态
拒绝原因
主播ID
```

例如：

```json
{
  "applied": true,
  "auditStatus": 1,
  "rejectReason": "",
  "anchorId": 10001
}
```

具体字段命名遵循当前项目 JSON 命名规范。

---

# 六、POST /live/anchor/profile/update

主播修改自己的公开资料。

user_id 从 JWT 获取。

禁止修改敏感字段。

客户端允许修改的字段请根据 LiveAnchor model 判断，原则上只允许主播个人公开资料，例如：

```text
nickname
avatar
cover
intro
gender
birthday
country_code
tag_ids
```

如果 model 实际字段名称不同，以当前 model 为准。

禁止客户端修改：

```text
user_id
anchor_no
audit_status
status
live_permission
pk_permission
recommend
recommend_weight
agency_id
signed
risk
cert_status
remark
created_at
updated_by
audit_by
```

以及任何管理字段。

必须使用字段白名单。

不要直接：

```go
db.Model(&anchor).Updates(req)
```

应该显式构建更新字段。

例如：

```go
updates := map[string]interface{}{
    "nickname": req.Nickname,
    "intro":    req.Intro,
}
```

---

# 七、GET /live/anchor/detail

查询主播公开详情。

建议参数：

```text
anchor_id
```

如果当前项目主播对外主要使用 anchor_no，也可以支持：

```text
anchor_no
```

请优先遵循当前项目已有 ID 设计。

只能返回公开字段。

不能返回：

```text
风控
后台备注
审核备注
签约内部信息
运营内部字段
管理员操作记录
```

---

# 八、GET /live/anchor/live/check

判断主播是否允许开播。

当前登录用户调用。

需要综合判断：

```text
是否主播
审核是否通过
主播账号状态
是否允许直播
认证状态（如果当前业务要求）
风控状态
```

返回不要只给 bool。

建议结构：

```json
{
  "allow": true,
  "code": "OK",
  "message": ""
}
```

失败例如：

```json
{
  "allow": false,
  "code": "ANCHOR_DISABLED",
  "message": "主播账号已禁用"
}
```

可能业务状态：

```text
NOT_ANCHOR
AUDIT_PENDING
AUDIT_REJECTED
ANCHOR_DISABLED
LIVE_DISABLED
RISK_BLOCKED
CERT_REQUIRED
OK
```

如果项目已经有业务错误码系统，则接入现有系统，不要重复设计。

注意：

这个接口只是「业务资格检查」。

不要在这里创建直播间、生成推流地址或者修改直播状态。

---

# 九、GET /live/anchor/pk/check

判断当前主播是否允许参与 PK。

逻辑与 live/check 类似。

至少判断：

```text
是否主播
审核是否通过
主播状态
live_permission
pk_permission
risk
```

返回：

```json
{
  "allow": true,
  "code": "OK",
  "message": ""
}
```

这个接口以后会用于：

```text
单主播 SRS 直播
      ↓
主播请求进入 PK
      ↓
检查 PK 权限
      ↓
进入 LiveKit PK
```

因此这里只负责主播 PK 资格判断。

不要在这里直接操作 SRS 或 LiveKit。

---

# 十、后台管理接口

实现：

```text
GET  /live/anchor_admin/list
GET  /live/anchor_admin/detail

POST /live/anchor_admin/audit
POST /live/anchor_admin/status/update
POST /live/anchor_admin/permission/update
POST /live/anchor_admin/profile/update
POST /live/anchor_admin/recommend/update
POST /live/anchor_admin/agency/update

POST /live/anchor_admin/signed/update
POST /live/anchor_admin/risk/update
POST /live/anchor_admin/remark/update
POST /live/anchor_admin/cert/update
```

全部必须管理员登录。

---

# 十一、GET /live/anchor_admin/list

主播分页列表。

使用当前项目已有 PageInfo / pagination 结构。

支持合理的筛选条件。

根据 LiveAnchor model 实际存在字段实现，例如：

```text
anchor_id
anchor_no
user_id
nickname
mobile
country_code
gender
status
audit_status
signed
risk
cert_status
agency_id
recommend
created_at range
```

不要为了筛选条件额外增加数据库字段。

支持模糊搜索的字段根据实际 model 实现，例如：

```text
anchor_no
nickname
```

分页查询注意：

```text
Count
Limit
Offset
Order
```

默认：

```text
id DESC
```

列表接口不要加载无意义的大字段。

如果有关联用户信息，需要避免 N+1 查询。

---

# 十二、GET /live/anchor_admin/detail

获取主播后台完整信息。

参数：

```text
anchor_id
```

返回后台需要的主播完整资料。

如果需要关联：

```text
用户信息
机构信息
审核信息
```

请使用合理方式查询。

不要造成大量重复 SQL。

---

# 十三、POST /live/anchor_admin/audit

主播审核。

请求建议：

```text
anchor_id
audit_status
audit_remark
```

只允许：

```text
通过
拒绝
```

具体状态值必须读取现有 constants。

审核时记录：

```text
审核人
审核时间
审核备注
```

如果 LiveAnchor 当前已有这些字段就更新。

如果 model 没有对应字段，不要擅自新增数据库字段，先在最终结果中说明。

必须检查状态流转是否合法。

例如已经审核通过的主播，不应该被重复执行「通过申请」。

---

# 十四、POST /live/anchor_admin/status/update

控制主播账号状态。

例如：

```text
正常
禁用
```

只修改主播 account/status 字段。

不要顺带修改：

```text
直播权限
PK 权限
审核状态
```

不同职责必须解耦。

---

# 十五、POST /live/anchor_admin/permission/update

主播功能权限控制。

用于修改例如：

```text
live_permission
pk_permission
```

建议请求：

```json
{
  "anchorId": 10001,
  "livePermission": true,
  "pkPermission": true
}
```

如果当前 model 是 int 状态，则遵循 model。

不要为了 API 修改现有 model 类型。

---

# 十六、POST /live/anchor_admin/profile/update

后台修改主播资料。

与客户端：

```text
/live/anchor/profile/update
```

不同。

管理员可以修改更多主播基础资料，但仍然禁止通过该接口修改：

```text
审核状态
主播状态
直播权限
PK权限
推荐状态
机构
签约状态
风险状态
认证状态
```

这些字段必须通过各自独立接口维护。

---

# 十七、POST /live/anchor_admin/recommend/update

主播推荐控制。

根据当前 LiveAnchor model 更新：

```text
recommend
recommend_weight
```

如果只有 recommend 字段，则只处理已有字段。

推荐权重必须校验合理范围。

例如：

```text
>= 0
```

不要擅自增加上限，除非项目 constants 已经有定义。

---

# 十八、POST /live/anchor_admin/agency/update

修改主播所属机构/公会。

建议：

```text
anchor_id
agency_id
```

如果：

```text
agency_id = 0
```

项目允许表示解除机构关系，则支持解除绑定。

更新之前检查：

```text
主播是否存在
机构是否存在
```

如果当前项目还没有 agency 表，则只实现 LiveAnchor 已有能力，并在完成报告说明依赖。

---

# 十九、POST /live/anchor_admin/signed/update

修改主播签约状态。

只负责 signed 相关状态。

例如：

```json
{
  "anchorId": 10001,
  "signed": true
}
```

如果 model 中不是 bool，按照实际字段类型处理。

不要在这里修改机构。

签约和机构属于两个独立概念。

---

# 二十、POST /live/anchor_admin/risk/update

修改主播风险状态。

这是后台内部接口。

例如：

```text
正常
限制
封禁
```

具体枚举严格使用当前项目定义。

该接口只负责：

```text
risk
```

及当前 model 已经存在的风险相关字段。

不要通过这个接口直接修改：

```text
status
live_permission
pk_permission
```

是否允许直播，由：

```text
/live/anchor/live/check
/live/anchor/pk/check
```

根据多个状态综合判断。

这样可以保留完整业务状态。

---

# 二十一、POST /live/anchor_admin/remark/update

修改后台内部备注。

例如：

```json
{
  "anchorId": 10001,
  "remark": "重点主播，注意跨境开播情况"
}
```

这个 remark：

```text
只能后台读取
只能后台修改
```

绝对不能在任何客户端主播详情接口中返回。

需要限制最大长度，长度按照数据库字段容量设计。

---

# 二十二、POST /live/anchor_admin/cert/update

主播认证状态修改。

根据 model 现有设计更新：

```text
cert_status
```

如果还有：

```text
cert_type
cert_time
```

且 model 已存在，可以根据实际业务一并处理。

如果认证资料应该属于单独认证表，不要强行塞进 LiveAnchor。

优先遵循当前数据库模型。

---

# 二十三、Service 层要求

主播业务逻辑尽量集中到：

```text
LiveAnchorService
```

例如：

```go
GetMyAnchorInfo()
ApplyAnchor()
GetApplyStatus()
UpdateMyProfile()
GetPublicDetail()

CheckLivePermission()
CheckPkPermission()

GetAnchorList()
GetAnchorDetail()
AuditAnchor()
UpdateAnchorStatus()
UpdateAnchorPermission()
UpdateAnchorProfile()
UpdateAnchorRecommend()
UpdateAnchorAgency()
UpdateAnchorSigned()
UpdateAnchorRisk()
UpdateAnchorRemark()
UpdateAnchorCert()
```

具体方法名按照项目现有命名风格调整。

API 层只负责：

```text
参数绑定
参数校验
获取当前用户
调用 service
返回 response
```

不要把大量业务判断写在 API 层。

---

# 二十四、权限检查逻辑复用

请不要分别在：

```text
live/check
pk/check
```

写大量重复 if。

可以在 service 内部封装主播基础状态校验。

例如：

```go
checkAnchorBasePermission(anchor)
```

然后：

```text
CheckLivePermission
    ↓
基础主播状态
    ↓
live_permission

CheckPkPermission
    ↓
基础主播状态
    ↓
live_permission
    ↓
pk_permission
```

但是不要过度抽象。

保持代码简单可读。

---

# 二十五、并发和数据安全

重点检查：

### 主播申请

必须避免同一个 user_id 并发生成多个主播。

数据库唯一索引：

```text
user_id UNIQUE
```

必须作为最后一道保护。

### 后台更新

更新时必须：

```text
WHERE id = ?
```

同时检查：

```text
RowsAffected
```

主播不存在时返回明确错误。

### 更新字段

所有 update 接口使用字段白名单。

禁止用户通过 JSON 注入额外字段修改数据库敏感信息。

---

# 二十六、索引与性能

不要为了这批 API 随意新增大量索引。

首先检查 LiveAnchor 当前已有索引。

列表接口常见查询主要围绕：

```text
id
user_id
anchor_no
status
audit_status
agency_id
created_at
```

只有确认现有索引无法满足查询时才提出修改建议。

不要直接改数据库结构。

如果认为必须增加索引，请在最终报告中单独列出：

```text
建议增加的索引
原因
对应 SQL 查询
```

让我确认后再修改。

---

# 二十七、Swagger

所有接口必须在 Swagger 清晰区分：

```text
直播-主播
直播-主播后台管理
```

客户端例如：

```go
// @Tags LiveAnchor
```

后台：

```go
// @Tags LiveAnchorAdmin
```

按照项目实际 Swagger Tag 命名规范调整。

每个接口 Swagger 必须明确：

```text
接口用途
参数
是否需要登录
返回值
错误情况
```

---

# 二十八、路由

最终路由必须清晰类似：

```go
liveGroup := Router.Group("live")

anchorGroup := liveGroup.Group("anchor")
{
    anchorGroup.GET("info", ...)
    anchorGroup.POST("apply", ...)
    anchorGroup.GET("apply/status", ...)
    anchorGroup.POST("profile/update", ...)
    anchorGroup.GET("detail", ...)
    anchorGroup.GET("live/check", ...)
    anchorGroup.GET("pk/check", ...)
}
```

后台：

```go
anchorAdminGroup := liveGroup.Group("anchor_admin")
{
    anchorAdminGroup.GET("list", ...)
    anchorAdminGroup.GET("detail", ...)

    anchorAdminGroup.POST("audit", ...)
    anchorAdminGroup.POST("status/update", ...)
    anchorAdminGroup.POST("permission/update", ...)
    anchorAdminGroup.POST("profile/update", ...)
    anchorAdminGroup.POST("recommend/update", ...)
    anchorAdminGroup.POST("agency/update", ...)
    anchorAdminGroup.POST("signed/update", ...)
    anchorAdminGroup.POST("risk/update", ...)
    anchorAdminGroup.POST("remark/update", ...)
    anchorAdminGroup.POST("cert/update", ...)
}
```

但必须优先适配当前 gin-vue-admin 项目的 RouterGroup 组织方式，不要机械照搬。

---

# 二十九、不要实现的内容

本次不要实现：

```text
直播间
开播
关播
SRS 推流
SRS API
推流密钥
推流地址
拉流地址
LiveKit
主播 PK 房间
弹幕
礼物
点赞
钱包
主播收益
结算
主播在线状态
Redis 在线主播
排行榜
推荐算法
```

这一阶段只完成：

```text
主播身份
主播资料
主播审核
主播状态
主播权限
主播认证
主播风控
主播签约
机构关联
后台管理
开播/PK资格检查
```

---

# 三十、代码完成后检查

完成代码后请自行检查：

```text
go fmt
go vet / 项目可执行的静态检查
go test ./...
go build
```

如果整个项目存在与本次修改无关的历史错误，请明确指出，不要为了通过 build 修改无关业务代码。

同时检查：

```text
路由是否重复
Swagger 是否完整
import 是否正确
request 是否重复
response 是否泄露后台字段
客户端是否能伪造 user_id
所有后台接口是否经过权限控制
所有状态字段是否做合法值校验
所有 update 是否使用白名单
是否存在 Save 导致全字段覆盖
是否存在 N+1
是否存在重复 SQL
```

---

# 三十一、最后给我输出修改报告

完成后请告诉我：

## 1. 新增文件

列出文件路径。

## 2. 修改文件

列出文件路径。

## 3. 新增接口

列出全部接口。

## 4. Request DTO

列出新增 request struct。

## 5. Response DTO

列出新增 response struct。

## 6. Service 方法

列出新增 service 方法。

## 7. 权限检查

说明客户端 user_id 如何获取。

说明后台管理员如何鉴权。

## 8. 数据库

说明：

```text
是否修改表结构
是否新增索引
是否新增 migration
```

默认情况下不要修改数据库结构。

## 9. 潜在问题

如果当前 LiveAnchor model 缺少某些业务字段，请列出来，不要自行增加。

## 10. 测试结果

告诉我：

```text
go test
go build
Swagger
```

执行结果。

---

最重要的一点：

请先阅读并理解当前项目已有代码，再实现。

不要根据这份需求重新造一套架构。

所有命名、错误处理、权限、数据库访问、Swagger、目录结构都应该尽可能和现有项目保持一致。


