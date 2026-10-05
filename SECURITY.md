# Security Policy

## Reporting a Vulnerability

Please report security issues to qimiaojiangjizhao@gmail.com


http://127.0.0.1:18080/TB_LIVE/5100008.m3u8
http://127.0.0.1:18080/TB_LIVE/5100008.flv

http://127.0.0.1:12985/rtc/v1/whep/?app=TB_LIVE&stream=5100008






ffmpeg \
-re \
-stream_loop -1 \
-i "./2.mp4" \
-c copy \
-f flv \
""

/proxy/pages/income
/api/agent/center/income/overview


/proxy/pages/income-record
/shortVideo/api/agent/center/income/record/page


/shortVideo/api/agent/reward/calculator/calculate

十六、你现在最大的缺失：http_hooks
十七、我建议你暂时不要开 on_play

每个页面都需鉴权使用公共的组件user_auth_change处理；
部分页面要有个加载中的状态占位，防止数据加载成功后切换各种状态导致页面闪动；
在余额宝模块，状态管理中的获取余额的函数多次被高频触发，请优化；
Apis文件的apiCode不能重复，如果有重复请帮我修改；
余额宝部分使用的接口不能劫持接口请求报错，有公共的报错提示方便我通过apiCode找到报错位置；

[tb_live_module:test]2026-10-03 17:27:31.003	[34minfo[0m	/Users/tabby/codes/go/live-gin-vue-admin/server/api/v1/live/live_hook.go:81	SRS Hook 原始参数	{"hook": "publish", "path": "/api/v1/app/live/hook/publish", "clientIp": "127.0.0.1", "contentType": "application/json", "rawBody": "{\"server_id\":\"vid-n7u7nd0\",\"service_id\":\"o8r7k842\",\"action\":\"on_publish\",\"client_id\":\"8o4s9m8r\",\"ip\":\"172.18.0.1\",\"vhost\":\"__defaultVhost__\",\"app\":\"TB_LIVE\",\"tcUrl\":\"rtmp://127.0.0.1:12935/TB_LIVE\",\"stream\":\"5100008\",\"param\":\"?pt=v1_tKWGDvZmTIXbRMsE5cd8KIppOjgu32H-qanVnPhtSQrmpqhxv_X7o3pO2uKOkQ2-oRyp1OglBIA1DPTdGhGJVD4kNRRGvJOBx2VDVDw3MU8SelJfjPDelgPTmYfX6s7YW-44FhAskzlHKSv2VBzs0qq_fes5m9V-lI-sGiXFD-EPk_aDtjaFDZ759XtkY5usI4s3tsDp5iLxNGHrH40SAg7LsjS-0Biu7R8ki9iv1VczA-aXOw2aJf1O16BAeJhGA6ba7VKo771Zqw3NsrwyeP7I22mZq796QhFLfTuLT_e7IME5Rm6C2v3FAk-CdjCxeU_aone2_yQw6Z22HxEm_mA0y69g5vJyjJlYO8ch\",\"stream_url\":\"/TB_LIVE/5100008\",\"stream_id\":\"vid-5bmj3ei\"}"}
[tb_live_module:test]2026-10-03 19:33:04.880	[34minfo[0m	/Users/tabby/codes/go/live-gin-vue-admin/server/api/v1/live/live_hook.go:81	SRS Hook 原始参数	{"hook": "unpublish", "path": "/api/v1/app/live/hook/unpublish", "clientIp": "127.0.0.1", "contentType": "application/json", "rawBody": "{\"server_id\":\"vid-n7u7nd0\",\"service_id\":\"o8r7k842\",\"action\":\"on_unpublish\",\"client_id\":\"8o4s9m8r\",\"ip\":\"172.18.0.1\",\"vhost\":\"__defaultVhost__\",\"app\":\"TB_LIVE\",\"tcUrl\":\"rtmp://127.0.0.1:12935/TB_LIVE\",\"stream\":\"5100008\",\"param\":\"?pt=v1_tKWGDvZmTIXbRMsE5cd8KIppOjgu32H-qanVnPhtSQrmpqhxv_X7o3pO2uKOkQ2-oRyp1OglBIA1DPTdGhGJVD4kNRRGvJOBx2VDVDw3MU8SelJfjPDelgPTmYfX6s7YW-44FhAskzlHKSv2VBzs0qq_fes5m9V-lI-sGiXFD-EPk_aDtjaFDZ759XtkY5usI4s3tsDp5iLxNGHrH40SAg7LsjS-0Biu7R8ki9iv1VczA-aXOw2aJf1O16BAeJhGA6ba7VKo771Zqw3NsrwyeP7I22mZq796QhFLfTuLT_e7IME5Rm6C2v3FAk-CdjCxeU_aone2_yQw6Z22HxEm_mA0y69g5vJyjJlYO8ch\",\"stream_url\":\"/TB_LIVE/5100008\",\"stream_id\":\"vid-5bmj3ei\"}"}



请先参考我和gpt的对话，我后面会按照他的思路做：


我还想加一个录播功能，主播可以在直播间播放录播，说白了就是添加一个视频地址







请仅帮我完成下面这一步；
我希望一步一步的来请先帮我实现。请重新校验我的配置文件的live，有新增改动，下面的需要用到的尽可能的在面去取；
请先帮我在/session/prepare实现publishToken的生成。
publishToken由{
"ver": 1,
"scope": "live:publish",
"token_id": "82fd10bcfa8c4e28",
"session_no": "LS_20260923_00001",
"app": "tb_live",
"vhost": "push.xxx.com",
"stream": "A_1000009",
"credential_version": 1,
"iat": 1790204400,
"nbf": 1790204340,
"exp": 1790290800
}组成；把这个用hook-token加密进行加密，然后后面会在回调里面解密；
同时帮我在pushUrl中拼好推流token；



2. SRS Hook 和统计接口没有应用层鉴权
   [hook 路由 (line 7)](/Users/tabby/codes/go/live-gin-vue-admin/server/router/live/live_hook.go:7) 全部属于公开路由，只在注释里写了“部署层 IP 白名单”，代码并没有真正校验。
   其中：
- publish 还有 pt 验证，安全性相对较好。
- unpublish 没有共享密钥或签名验证。
- session/stats 可以根据场次号直接覆盖观看、点赞、礼物金额等数据：[live_room.go (line 552)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:552)。
  你现在直接把后端 8888 暴露给公网时，并不存在所谓“部署层白名单”。应给 Hook 增加独立共享密钥/HMAC 校验，或者使用只允许 SRS 访问的内部监听端口。

  

4. SRS 崩溃或重启会产生永久“假直播”
   当前定时任务只处理：
- 准备超时；
- 已收到 unpublish 并进入重连窗口的场次；
- 结束中的场次。
  参见 [live_room.go (line 578)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:578)。
  如果 SRS 崩溃、容器强制重启或 Hook 请求丢失，数据库中的场次仍然是：
  status=直播中
  reconnect_deadline_at=0
  现有扫描任务永远不会处理它，公开列表和后台会持续显示正在直播。
  需要增加“数据库与 SRS 实际流状态对账”：
- 周期查询 SRS /api/v1/streams；
- DB 显示直播中但 SRS 连续多次不存在时，进入断流重连窗口；
- 服务启动和 SRS 重启后立即执行一次对账；
- 为避免网络抖动误结束，建议连续 2～3 次缺失后再处理。




6. 统计快照可能倒退或丢失最终数据
   当前收到什么值就覆盖什么值：[live_room.go (line 614)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:614)。
   因此：
- 较旧的延迟请求可能覆盖较新的统计。
- 场次刚完成结算后到达的最后一次统计会被拒绝。
- 主播累计观看量可能使用了非最终快照：[live_room.go (line 1818)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:1818)。
  建议给统计请求增加 sequence 或采集时间，只接受更高版本；累计字段使用单调更新规则。结束流程最好先冻结统计、获取最终快照，再做最终结算。


5. 公开直播列表的 pageSize 没有限制
   接口只判断大于零：
- [live_room.go (line 224)](/Users/tabby/codes/go/live-gin-vue-admin/server/api/v1/live/live_room.go:224)
  查询代码也没有最大值限制：
- [live_room.go (line 1637)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:1637)
  请求 pageSize=1000000 可能造成大查询、大对象分配和大响应，是直接的性能攻击面。应限制为 50 或 100，并限制最大页码，长期改为游标分页。


------------
一、上线前必须解决的问题
1. 私密和仅关注者直播实际上没有媒体权限控制
   数据库/API 虽然通过 visibility 隐藏直播间，但 SRS 的 FLV、HLS、RTMP 播放地址没有鉴权：
- [srs.conf (line 69)](/Users/tabby/codes/go/live-gin-vue-admin/srs/srs.conf:69) 直接开放 HTTP-FLV。
- [srs.conf (line 80)](/Users/tabby/codes/go/live-gin-vue-admin/srs/srs.conf:80) 直接开放 HLS。
- 没有 on_play、播放 token、签名 URL 或 CDN 鉴权。
- stream_name 是稳定房间号，知道地址的人可以绕过业务 API 直接播放。
  这意味着“私密”“仅关注者”目前只是列表不可见，不是真正不可观看。
  建议使用：
- 后端签发短时播放 token；
- SRS on_play 或 Nginx/CDN 鉴权；
- token 绑定用户、房间、场次、协议和过期时间；
- 生产播放必须走 HTTPS/WSS/RTMPS。
  这是当前最高优先级问题。
2. Hook 和统计写入入口缺少应用层鉴权
   Hook 路由完全公开注册，只依赖注释里所说的“部署层 IP 白名单”：
- [live_hook.go (line 7)](/Users/tabby/codes/go/live-gin-vue-admin/server/router/live/live_hook.go:7)
- [live_hook.go (line 104)](/Users/tabby/codes/go/live-gin-vue-admin/server/api/v1/live/live_hook.go:104)
  其中 publish 还有 pt 校验，但：
- unpublish 没有签名；
- session/stats 没有签名；
- 知道 sessionNo 就可能伪造观看、点赞、礼物等统计；
- IP 白名单一旦因反向代理、Docker 网络、云安全组配置错误而失效，接口会直接暴露。
  建议增加 HMAC 签名：
  signature = HMAC-SHA256(secret, timestamp + nonce + rawBody)
  同时校验时间窗口、nonce 防重放，再配合内网访问/IP 白名单。

[//]: # (3. 请求体没有大小限制，存在内存和日志放大风险)

[//]: # (   Hook 会先完整读取请求体：)

[//]: # (- [live_hook.go &#40;line 71&#41;]&#40;/Users/tabby/codes/go/live-gin-vue-admin/server/api/v1/live/live_hook.go:71&#41;)

[//]: # (  没有 http.MaxBytesReader。攻击者可以发送巨大请求体造成内存压力。如果打开 log-srs-hook-raw-body，还会把完整 pt 和任意超大字段写入日志。)

[//]: # (  后台操作日志也会完整读取请求和响应：)

[//]: # (- [operation.go &#40;line 31&#41;]&#40;/Users/tabby/codes/go/live-gin-vue-admin/server/middleware/operation.go:31&#41;)

[//]: # (  建议：)

[//]: # (- Hook 请求体上限约 32～64 KiB；)

[//]: # (- 普通 JSON 上限约 1 MiB；)

[//]: # (- 永远对 pt、Authorization、密码脱敏；)

[//]: # (- SRS 错误响应日志只保留前 1～4 KiB。)
4. 生产配置里存在弱密钥和明文凭据
   [config.prod.yaml (line 3)](/Users/tabby/codes/go/live-gin-vue-admin/server/config.prod.yaml:3) 中存在：
- 很弱的后台 JWT 密钥；
- 明文数据库和 Redis 凭据；
- APP JWT 仍是占位式密钥；
- publish-token-key、SRS 地址等正式配置仍为空。
  如果这些配置曾推送到远程仓库，仅修改文件不够，必须立即轮换数据库、Redis、JWT、推流密钥，并考虑清理 Git 历史。
  同时当前 /health 永远返回 ok：
- [router.go (line 69)](/Users/tabby/codes/go/live-gin-vue-admin/server/initialize/router.go:69)
  即使 SRS 配置为空、数据库表没迁移、推流密钥不可用，负载均衡仍会认为服务健康。建议区分：
- /health/live：进程存活；
- /health/ready：数据库、配置、迁移版本、关键依赖可用。
5. 公开直播列表的 pageSize 没有限制
   接口只判断大于零：
- [live_room.go (line 224)](/Users/tabby/codes/go/live-gin-vue-admin/server/api/v1/live/live_room.go:224)
  查询代码也没有最大值限制：
- [live_room.go (line 1637)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:1637)
  请求 pageSize=1000000 可能造成大查询、大对象分配和大响应，是直接的性能攻击面。应限制为 50 或 100，并限制最大页码，长期改为游标分页。
6. 后台直播接口只有登录校验，没有接口级权限
   后台路由仅挂了 JWT：
- [router_system.go (line 17)](/Users/tabby/codes/go/live-gin-vue-admin/server/initialize/router_system.go:17)
- [live_room_admin.go (line 11)](/Users/tabby/codes/go/live-gin-vue-admin/server/router/live/live_room_admin.go:11)
  动态菜单只控制“是否显示按钮”，不能阻止直接请求。任何有效后台账号理论上都可以查看直播详情、修改房间、强制结束直播。
  还需要注意后台 JWT 中管理员禁用状态检查目前被注释：
- [jwt_system.go (line 47)](/Users/tabby/codes/go/live-gin-vue-admin/server/middleware/jwt_system.go:47)
  建议给每个读写接口增加 Casbin/API 权限校验，并恢复管理员状态/token version 检查。
  二、直播流程的极限边界问题

[//]: # (1. prepare 成功但响应丢失，主播拿不到推流凭证)

[//]: # (   当前流程是：)

[//]: # (1. 创建 Session；)

[//]: # (2. 生成 pt；)

[//]: # (3. 数据库只保存 pt 哈希；)

[//]: # (4. 事务提交；)

[//]: # (5. 返回明文 pt。)

[//]: # (   见 [live_room.go &#40;line 273&#41;]&#40;/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:273&#41;。)

[//]: # (   如果事务已经提交，但移动网络在响应返回前断开，客户端重试会得到“已有活动场次”，而服务端也无法从哈希恢复原始 pt。只能结束当前准备场次或等待 120 秒超时。)

[//]: # (   建议给 prepare 增加幂等键，或者允许主播对“尚未 publish 的 Preparing 场次”安全轮换推流凭证。)
2. 最终统计可能少数据或被旧数据覆盖
   统计接口直接覆盖所有字段，没有：
- 消息序号；
- 采集时间；
- 幂等事件 ID；
- 单调递增限制；
- 最大值校验；
- 最终快照确认。
  见 [live_room.go (line 640)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:640)。
  因此：
- 较旧的统计消息晚到，可能把新数据覆盖小；
- 主播结束后立即结算，统计服务最后一批数据可能还没送达；
- 伪造或异常的大数值可能导致主播累计字段溢出或结算失败；
- 礼物金额不能只依赖这个快照接口。
  建议统计请求增加 eventSeq 和 observedAt，使用条件更新；累计字段使用单调逻辑；结束时先关闭统计写入并拉取最终快照，礼物以独立账本为准。
3. SRS 重启后的新连接可能暂时被拒绝
   直播中且没有进入重连窗口时，新 on_publish 的 SRS 连接 ID 必须与旧连接一致：
- [live_room.go (line 511)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:511)
  SRS 突然重启且没有成功发送 on_unpublish 时，新 SRS 会生成新的 stream/client/server ID。主播马上重推可能被后端拒绝，必须先等待周期对账连续缺失三次后进入重连窗口。
  小规模约等待 15 秒；直播数量多时可能等待几分钟。
  当前实现只有“后端启动时立即对账”，没有真正的“SRS 重启事件立即对账”。建议增加 SRS 实例启动通知，或按 server_id 检测实例代际变化。
4. 超长直播断线后可能无法重连
   正式配置推流 token 有效期 24 小时。连接不断时可以继续直播，但直播超过 24 小时后断线，重新触发 on_publish 会因为 token 过期而失败。
   还存在密钥轮换问题：滚动部署时不同实例使用不同 publish-token-key，会随机拒绝正在重连的主播。
   建议：
- 提供活动场次推流凭证刷新接口；
- token 使用 key_id；
- 密钥轮换期间同时支持旧、新密钥；
- 明确定义最大直播时长。
5. 多 SRS 节点目前没有真正支持
   配置只有一个 SRS HTTP API 地址：
- [srs.go (line 4)](/Users/tabby/codes/go/live-gin-vue-admin/server/config/srs.go:4)
  Session 虽然保存了 srs_server_id，但查询和主动断流没有用它选择节点：
- [live_room.go (line 1321)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:1321)
  如果以后 SRS 前面放普通负载均衡：
- 推流在 SRS-A；
- 查询可能落到 SRS-B；
- 后端误判流不存在；
- 主动结束也可能找不到发布者。
  因此当前架构实际上只支持“单逻辑 SRS”。要扩容，需要建立 server_id -> SRS API endpoint 路由或统一的媒体控制层。
6. SRS 长时间故障会出现安全优先但可用性下降
   当前设计在 SRS API 报错时不会累计“流缺失”，这是正确的，可以防止误结束。
   代价是：
- 已断流的房间可能长期显示直播中；
- Ending 场次可能无限重试；
- current_session_id 一直占用，主播无法重新 prepare；
- 没有失败上限、死信状态或人工恢复入口。
  建议保留“不盲目结算”的安全原则，同时增加：
- media_state=unknown/degraded；
- Ending 最长滞留告警；
- 人工强制确认媒体已停止的审计接口；
- SRS 恢复后优先处理积压；
- 不要简单到达重试次数后自动结算。
7. streamInfo 不是持续的当前快照
   取得尺寸时会写入 streamInfo，但直播中持续对账只更新 last-seen 和缺失次数：
- [live_room.go (line 1017)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:1017)
  如果主播中途从 1080p 切到 720p、改变编码器或音频参数，前端看到的 streamInfo 会一直是旧值。
  可以在对账快照发现内容变化时更新；只在字段变化时写库，避免每 5 秒写一次。
8. Hook 字段兼容性仍有边界
   模型把 SRS ID 字段定义成可选，但只要数据库里已有字段，回调缺少该字段就匹配失败：
- [live_room.go (line 621)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:621)
  某些 SRS 版本、插件或代理如果漏掉 client_id/server_id，on_unpublish 会被静默忽略，只能等待对账发现。
  应明确 SRS 7.0.165 的必需字段合同，并为缺字段建立有优先级的安全匹配策略和告警。
9. 依赖各机器本地时间
   token、租约、准备截止、重连截止、对账时间都依赖 time.Now()。多后端实例时如果时钟漂移，会出现：
- token 提前过期；
- 一个实例提前抢租约；
- 准备或重连被过早结束；
- 重复扫描。
  至少需要 NTP/chrony 和时钟漂移告警；更严格时，关键租约可以基于数据库时间。

[//]: # (  三、性能与容量结论)

[//]: # (  当前任务不会无限创建 goroutine：每个任务有进程锁，主动断流有固定 worker，失败重试写入数据库。因此小规模下不会出现明显的内存累积。)

[//]: # (  真正的风险是“队列和状态延迟累积”。)

[//]: # (  默认批量 100、对账间隔 5 秒、缺失阈值 3 时：)

[//]: # (  活动直播数	完整轮询一遍	理论确认断流	加 20 秒重连窗口)

[//]: # (  100	5 秒	约 15 秒	约 35 秒)

[//]: # (  1,000	50 秒	约 150 秒	约 170 秒)

[//]: # (  5,000	250 秒	约 750 秒	约 770 秒)

[//]: # ()
[//]: # ()
[//]: # (所以配置注释里“3 次 × 5 秒约 15 秒”只在活动直播不超过一个批次时成立。)

[//]: # (另外：)

[//]: # (- SRS 流列表每页 100，最多 100 页：[client.go &#40;line 230&#41;]&#40;/Users/tabby/codes/go/live-gin-vue-admin/server/internal/srs/client.go:230&#41;。)

[//]: # (- 接近 10,000 路时，一次快照可能需要约 100 次 HTTP 请求，而且必须在总计 3 秒上下文超时内完成。)

[//]: # (- 达到 10,000 条整页数据时会直接报“超过最大分页范围”。)

[//]: # (- 每个后端实例都会独立拉取全量 SRS 列表，多实例会成倍放大流量。)

[//]: # (- 对账每个候选场次还会执行独立事务和行锁，100 场/5 秒约为每秒 20 个对账事务；开播突发时 readiness 最高可接近每秒 100 个场次事务。)

[//]: # (- 主动断流默认 8 worker、100 场、单请求 3 秒，SRS 完全卡住时一批最坏约需 ceil&#40;100/8&#41; × 3 ≈ 39 秒。期间后续周期会被锁跳过，不会叠加 goroutine，但会形成 DB 积压。)

[//]: # (- [srs.conf &#40;line 3&#41;]&#40;/Users/tabby/codes/go/live-gin-vue-admin/srs/srs.conf:3&#41; 当前 max_connections=1000，发布者、播放器和其他连接共享容量，不能理解成“支持 1000 个主播外加无限观众”。)

[//]: # (  建议规模达到几百路以前就做：)

[//]: # (1. 单实例定时任务 Leader 选举；)

[//]: # (2. SRS 快照共享缓存，一轮只拉一次；)

[//]: # (3. readiness、reconcile、stop 分开配置批量大小；)

[//]: # (4. 存在流批量更新，只有状态切换才逐场事务；)

[//]: # (5. 游标分页或按 stream ID 批量查询；)

[//]: # (6. 对 100、500、1,000、5,000 路分别压测。)
   四、当前实现做得好的地方
   以下设计是正确且值得保留的：
- prepare 通过主播行锁和分类共享锁解决权限竞态：[live_room.go (line 232)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/live_room.go:232)。
- 数据库生成列唯一索引保证同一房间只有一个活动场次：[live_room_session.sql (line 79)](/Users/tabby/codes/go/live-gin-vue-admin/documents/live_room_session.sql:79)。
- on_publish 不会直接改成直播中，必须确认 SRS active 且宽高有效。
- 延迟、重复 Hook 有连接身份匹配和状态条件保护。
- SRS 查询失败不会误累计断流次数。
- 生命周期扫描与 SRS 断流任务分离，避免 SRS 故障拖住准备/重连超时。
- 主动结束先可靠写入 Ending，再尝试 SRS，具备幂等重试。
- worker 数、扫描批量、重试退避均有部分上限。
- 结束结算通过状态条件防止主播累计数据重复增加。
  五、测试与当前构建状态
  我执行了：
  go test ./service/live ./internal/srs ./api/v1/live -count=1 -cover
  结果：
- internal/srs：通过，覆盖率 84.8%；
- api/v1/live：通过，但覆盖率只有 6.6%；
- service/live：失败，覆盖率输出为 64.6%。
  失败点是 [publish_token_test.go (line 70)](/Users/tabby/codes/go/live-gin-vue-admin/server/service/live/publish_token_test.go:70)：测试只替换 Base64 最后一个字符，而非规范 Base64 的不同尾字符可能解码成相同字节，所以测试偶发/稳定失败。生产 OnPublish 还会比较原始 token 哈希，因此目前不等同于直接绕过，但建议解码使用严格模式，并让测试直接篡改密文字节。
  当前全量构建还存在明确阻断：
- [gorm_biz.go (line 5)](/Users/tabby/codes/go/live-gin-vue-admin/server/initialize/gorm_biz.go:5) 有未使用 import；
- [gorm_biz.go (line 10)](/Users/tabby/codes/go/live-gin-vue-admin/server/initialize/gorm_biz.go:10) 的 AutoMigrate() 没有传模型。
  所以当前工作树还不能作为“已通过构建和测试”的生产版本。
  推荐修复顺序
1. 立即轮换并移出生产凭据，修复构建和红色测试。
2. 实现播放鉴权，保护 SRS HTTP API，生产使用 TLS。
3. 给 Hook/统计接口增加 HMAC、重放保护、请求体限制和限流。
4. 修复公开列表无限 pageSize、后台接口权限和管理员状态校验。
5. 解决最终统计握手、事件顺序和金额账本问题。
6. 增加 prepare 幂等、SRS 重启快速恢复、推流凭证刷新。
7. 再实现多实例 Leader/共享 SRS 快照和多 SRS 节点路由。
8. 使用 MySQL 8 + SRS 7.0.165 做真实并发、断网、重启、超时和大规模压测，而不只依赖 SQLite 单元测试。