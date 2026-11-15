# 主播公开主页一期

更新：2026-11-14。范围是观众访问的主播主页，以及主播投稿管理、后台图片审核；不替换主播自己的收入/提现工作台。

## 已实现

| 模块 | 行为 |
| --- | --- |
| 公开主页 | 主播名片、真实认证状态/既有等级、简介、内容分类、关注/粉丝数、互关关系、受权限控制的直播入口 |
| 多图相册 | 横向切换、页码、大图预览、按需读取与邻图预取；头像与相册分开；单张、空相册、失败占位 |
| 主播管理 | 多张上传、排序/首图、删除、待审/驳回/下架状态及私有原因、观众视角预览；简介和分类的窄字段编辑 |
| 互动 | 关注/取消、互关、共同关注、拉黑/解除入口；游客操作先登录并返回；保留普通账号资料页 |
| 贡献榜 | 当前日/月/总榜、分页，复用扣除退款后的礼物投影；金额和计数以十进制字符串传输 |
| 守护 | 按当前服务端时间展示有效会员与计划等级，支持高等级到期后切换到仍有效的低等级；公开权益文字白名单 |
| 分享 | 规范主页链接、复制、本地生成二维码；不携带当前查询参数或登录凭据 |
| 管理后台 | 筛选、鉴权图片预览、通过/驳回/下架、原因、冲突重载、未知提交结果的原请求重试 |

守护延续已有赠礼授予规则，没有新增直接购买。没有动态流、私信、粉丝团、直播预约、独立等级成长或礼物墙。内容标签来自真实启用的直播分类，不虚构尚不存在的标签目录。IM SDK 未接入，继续沿用既有独立实时事件适配边界，不展示可发送消息的假入口。

## 架构与数据边界

`Router → API → Service → Model`，通过各层 `enter.go` 注册。`anchorhome.PublicService` 负责公开聚合；图片命令和媒体由独立 `AlbumService` 管理。已有 live/social/ranking/guard 模块继续拥有其事实与规则。

主页、榜单和守护每次读取在同一个只读 RR 快照中完成目标可见性验证及数据查询。游客允许访问公开资料；提供了无效/撤销 Bearer 时必须拒绝，不能退回游客。双向拉黑、主播/账号封禁、删除和申请审核状态均查数据库。直播入口复用当前场次与房间权限，不根据主播历史场次推测在线。

公开 DTO 明确列出字段，不嵌入内部账号、收入、收款、风控、推流凭据、媒体存储路径或未审核图片信息。守护不暴露会员订单号、流水、购买时间、到期时间或任意 `benefitConfig`。仅提取 `publicBenefits: string[]` 中最多 8 条、每条最多 80 字的纯文本说明，页面按文本渲染。

四张新表：`anchor_home_album`、`anchor_home_photo`、`anchor_home_command`、`anchor_home_audit`。相册按 owner 行锁串行化变更；相册版本和照片审核版本分别 CAS。永久幂等记录保留成功结果；同一 requestId 不允许换内容。审计/幂等表有数据库 UPDATE/DELETE 保护触发器。投稿删除不移除审核证据。

简介编辑仅改 `live_anchor.signature/category_id`，不复用旧全字段覆盖接口。版本是这两项和主播编号的 SHA-256 内容版本；它不是单调序号，A→B→A 后旧 A 版本可再次匹配。相同目标内容重试无副作用，不同内容的并发编辑返回冲突。

## 媒体与资源保护

- 只接受上传的 JPEG/PNG/WebP，拒绝 SVG、远程 URL 抓取和伪造文件内容；单文件 8 MiB、最多 20MP、单边最多 12,000 像素。
- 服务端解码、处理 JPEG 方向、重新编码去元数据；全图最长边 2048，缩略图最长边 480。昂贵处理默认最多 2 个并发，在数据库行锁外完成。
- 默认每主播最多 9 张，可配置 1–30；pending/rejected/removed 也占数量。默认滚动 24 小时最多 50 次上传，删除仍计数，防反复删除重传填满磁盘。
- 私有目录 0700、文件 0600；拒绝公开静态目录及符号链接绕过。`os.Root` 限制文件访问范围，不接受客户端文件路径。
- 每次媒体读取重新检查权限和审核状态，`private, no-store`、`nosniff`；下架后同 URL 和条件请求不再返回图片。已被观看者自行保存的副本无法撤回。
- H5/后台用带身份的 Blob 读取预览，身份切换、关闭或卸载时撤销 URL，旧请求不能回填新账号。
- 明确回滚仅清理本次生成的文件；提交结果不确定时保留文件，供原 requestId 查询/重放。每 5 分钟最多扫描 1,000 个媒体目录项，保留游标续扫；清理超过 24 小时的孤儿与删除超过 7 天的媒体，保留元数据审计。
- JSON/图片分别有容量和截止时间；游客 JSON 默认 180 次/分钟/来源、媒体 600 次/分钟/来源。复用可信代理解析和 HMAC 来源键；Redis 原子限流，未启用 Redis 时使用有上限的单机计数。要求 Redis 时故障关闭。

## 性能验证和已修正的问题

本机隔离 MySQL 8.0.46、开启 Go race detector，8 并发，仅表示此环境的测量，不是线上容量或 SLA。

| 场景 | 数据规模 | 已记录结果 |
| --- | --- | --- |
| 公开主页 | 2,000 个历史场次，160 次读取 | 约 10 条 SQL/次；P95 约 23ms |
| 贡献榜 | 1,000 人，50 人/页，80 次读取 | 修正前 P95 约 554ms；修正后约 38ms，实际时间见最终证据 |
| 守护 | 1,000 人，每人 32 段续期历史，50 人/页 | P95 约 198ms；仍按现行权益及不可变快照核验 |
| 图片净化 | 1920×1080 完整解码/缩略图 | 约 119.5ms/op、31MB/op，默认并发 2 |

贡献榜发现二进制投影键与旧账号索引排序规则不一致，曾导致相关查询扫描账号表。现启动读取实际账号列编码，只转换外侧投影键，保留账号索引与二进制精确身份比较；测试覆盖 `utf8mb4_0900_ai_ci`、`utf8mb4_unicode_ci` 及大小写伪变体。公开请求不执行全站收入流水积压扫描，`lagCount` 为空表示没有计算，`dataAsOf` 表示投影时间。

守护的当前区间筛选针对真实 MySQL 调整了 JSON_TABLE 查询结构，测试覆盖恰好到期、未来生效、低等级回落和快照被篡改。分页上限 50、最多 100 页，查询有截止时间。更大规模需要用实际业务数据复测，尤其是守护总数查询和长续期投影；不将本次千人测试外推为百万规模保证。

## 测试证据

- `evidence/go-final.jsonl`：全仓库 Go 回归，486 个顶层测试通过；其中 112 个依赖专用环境/显式开关的顶层测试跳过，不计作通过。
- `evidence/anchor-home-final-race.jsonl`：一期模块、API、Router、中间件、初始化、迁移工具的 race 回归，启用专用相册/公开读取/迁移 MySQL 库；92 个顶层测试通过、2 个显式跳过（另行运行的浏览器 harness 和 Redis 专用环境检查）。
- `evidence/public-final.log`：真实 MySQL 公开访问控制、净礼物退款、守护边界、12 客户端 CAS、负载与索引验证。
- `evidence/public-http-race.log`：伪造转发地址、限流并发、Redis 故障关闭、非法 Bearer、重复参数、null/缺失/越权字段。
- `evidence/core-regression-race.log`：既有钱包、礼物、守护、排行、收益、提现、社交、直播、可靠事件的本地 MySQL/Redis race 回归，335 个顶层测试通过、9 个额外压力开关跳过；包含本地模拟注册、充值、赠礼、结算和提现。失败首轮保留单独证据，最终结果以稳定重跑为准。
- `evidence/h5-build.log`、`swagger.log`、`go-vet.log`、`go-build.log`：构建与静态验证。
- H5 独立仓库 `documents/anchor-home-20261114/`：81 项测试、19 项隔离浏览器检查、10 组真实 Go HTTP + H5 联调、移动端截图；生产依赖审计无已知漏洞。
- 后台 `web/tests/anchor-home.test.mjs`：新增 11 项交互、Blob、身份/筛选竞态和幂等重试测试；后台全部 75 项测试及生产构建通过，见 `evidence/admin-tests.log`。
- `evidence/test-summary.json`：机器可读统计与逐项跳过清单。不同套件有重叠，不能将数量相加宣称独立测试总数。

复现一期模块测试：仓库根目录运行 `bash server/tests/anchorhome/run.sh`，说明见 `server/tests/anchorhome/README.md`。脚本只使用固定的本地临时数据库，包含 race 和 5 秒 EXIF fuzz。依赖升级后的图片/API/路由/提现定向复测记录在 `evidence/image-upgrade-regression.log`。

测试全部使用临时合成身份和独立本地数据库。真实支付、短信、银行和 IM 外部服务不在本次测试中调用。未连接 SSH，未操作真实资金。

## 依赖安全扫描与剩余风险

本次 `govulncheck v1.9.0` 发现 WebP VP8L 解码可被构造图片触发过量内存分配；已将 `golang.org/x/image` 从 v0.44.0 升至官方修复版本 v0.45.0，并通过图片、HTTP、提现相关 race 回归及真实 MySQL 模块脚本。依据：[GO-2026-6222](https://pkg.go.dev/vuln/GO-2026-6222)。升级前后原始结果分别在 `evidence/govulncheck-before.log` 与 `evidence/govulncheck.log`。

最终扫描仍报告 **1 项可达依赖告警 GO-2026-6911**，不能表述为零漏洞扫描。Go 官方条目列出 MCP transport 跨源重定向的 SSRF/凭证泄漏，检查当日未列出已修复版本；应用为 MCP transport 提供限制同源的 HTTP client，并拒绝重定向，额外保护工具到后台的带凭证请求及健康检查。依据：[GO-2026-6911](https://pkg.go.dev/vuln/GO-2026-6911)。这些应用侧防护不能替代上游修复，也不能消除静态依赖告警。扫描另列 6 项导入包和 1 项依赖模块告警，未发现当前可达调用；后续升级或增加调用时必须重新扫描。

真实 MCP transport 完成 45 组 Location 攻击回放（跨主机/同主机异端口/HTTPS 降级，5 种重定向状态，request/notification/close），另有 10 组后台带凭证跳转回放，目标端请求和凭证接收计数均为 0；正常初始化、工具列表及关闭链路保持可用。`go test -race ./mcp/... ./cmd/mcp` 和定向 vet 通过，原始记录在 `evidence/mcp-security-race.log`。这些检查不覆盖初始配置地址或 DNS 信任改变，也不覆盖未启用的 OAuth 发现/令牌请求。

独立及托管 MCP 的 `listen-host` 缺省/空白现在只监听 `127.0.0.1`，显式配置才允许外部监听，兼容 IPv6。主业务与 Compose 不会默认启动该进程。独立 MCP 尚无全局协议鉴权，若显式向外部开放仍需网关及后台权限控制；不能把回环默认当作协议鉴权。详见 `aiDoc/modules/mcp.md`。

该条目链接的 [Go 漏洞库 issue #6911](https://github.com/golang/vulndb/issues/6911) 和 [SDK issue #1164](https://github.com/mark3labs/mcp-go/issues/1164) 与条目正文存在范围差异，涉及 JSON-RPC 字段大小写/重复键与网关解析差异。当前未发现项目使用原始 JSON 字段白名单作为独立授权网关；后台权限仍由 JWT 及业务权限校验。**本次没有修复 SDK 的 JSON 解析行为**，后续增加此类网关或 OAuth 时需要独立复核，不能据重定向测试声称相关协议风险全部关闭。

## 部署和排障

新增模块启动只验证结构，不执行 DDL。唯一显式迁移入口：

```sh
cd server
go run ./cmd/anchor-home-migrate -config /absolute/path/config.prod.yaml -verify
go run ./cmd/anchor-home-migrate -config /absolute/path/config.prod.yaml -migrate -metadata -backup-dir /absolute/private/backup-dir -progress
```

写入前：命名锁、旧表结构预检、固定范围一致性 SQL 备份、文件 fsync 与 SHA256；备份目录 0700、SQL 0600。只创建缺失相册表、空表所需保护及指定菜单/API元数据；既存列、索引、约束不兼容时拒绝，不自动 ALTER 修复。故障中断后的合法部分建表可安全续跑。已有金融/相册事实和全部既有元数据精确比对，账号/主播旧表额外只做全行 proof、不写入，也不声称其已被额外完整备份。真实 SQL 还原已在隔离库验证。

本次远程操作状态：只读预检在 connection 阶段失败。TCP 可建立，但没有收到 MySQL 握手；尚未完成远程备份或迁移，未执行任何生产 DDL/DML。详见 `evidence/prod-preflight.json` 和 `prod-network-check.json`。连接恢复后，继续上面的备份/迁移/复验；不要跳过验证直接启动新版本。

用户确认允许外网直连后再次重试：目标仍为配置中的 `140.210.12.162:3306`。CLI 预检再次在 connection 阶段失败；默认路由为本机 `utun8`，3 次原始 TCP 探针均快速建立连接，但各等 10 秒仍收到 0 字节 MySQL 握手。仅对诊断 socket 设置 `IP_BOUND_IF` 绑定 `en0`，分别等待 5 秒和 20 秒，结果均为 TCP 超时，绑定本身成功。没有改系统代理、VPN、路由或生产配置，也没有发送认证或写库请求。网卡绑定不保证排除所有系统内容过滤，这些结果不足以断言服务器宕机、凭证错误或由 VPN 单独导致。证据：`evidence/prod-retry-preflight.json`、`prod-retry-network.jsonl`、`prod-retry-direct-probe.jsonl`、`prod-retry-direct-probe-20s.jsonl`。已询问是否允许 SSH 只读检查服务器监听、映射及防火墙；未获新授权前继续遵守用户此前不连接 SSH 的要求。

原生发布的 prod 私有媒体默认 `../shared/anchor-home`，不随版本目录替换；Docker 默认 `/var/lib/tb-live/anchor-home`，Compose 已增加独立持久卷。数据库备份不能替代私有媒体备份；备份/恢复时需要同时保留媒体目录。生产应监控磁盘余量、图片失败率、接口延迟、数据库/Redis连接、投影时间和清理 worker 最近成功时间。具体告警通道与业务容量仍需真实部署环境验证。

用响应 `X-Request-ID` 查 `traceId` 日志。图片命令日志同时包含经过长度/格式校验的业务 `requestId`，可对应 `anchor_home_command` 与 `anchor_home_audit`；不记录正文、图片、Token、审核原因或收款资料。`anchor_home_media_cleanup` 接入既有运营面板 worker 指标。重试未知提交结果必须保留原 requestId，不能生成新编号；版本冲突先重新读取。

本次交付是源代码、编译产物和本地验证结果；没有把应用发版到远程服务器，也不将其表述为已完成线上生产验收。

本轮创建的七个隔离测试 Compose 项目已清理，原有 `tb-live-srs` 容器保持运行。复现脚本可重新创建临时环境，清理记录见 `evidence/local-container-cleanup.log`。
