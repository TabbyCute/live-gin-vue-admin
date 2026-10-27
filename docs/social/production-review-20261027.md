# 社交模块公开资料统一与生产审查（2026-10-27）

本次在现有项目中完成代码修改、真实 MySQL/Redis 验证和前端契约更新。没有部署线上服务，也没有访问或修改生产数据库。推送渠道、短视频/回放、CDN 媒体链路不属于本次需求。

## 1. 已交付接口变化

公开资料唯一结构：`server/model/common/response/user_public_profile.go` 的 `UserPublicProfile`。`live.AccountPublicProfileResp`、`social.User` 均为该类型的别名，不再各自维护字段定义。

| 公开字段 | 语义 |
|---|---|
| accountNo | 用户账号公开编号；关注、拉黑等操作仍针对账号 |
| nickname/avatar/signature/gender/countryCode | 来自用户账号的公开资料 |
| anchorNo | 可选，正常且审核通过的主播身份编号 |
| isLive | 当前观众可访问的有效直播状态 |
| roomNo/sessionNo | 仅在isLive=true时返回的直播入口 |

关注、粉丝、互关、共同关注、黑名单、屏蔽、订阅、批量关系状态、用户公开详情、直播间详情的userInfo、历史列表、关注中的在线主播均复用该结构。JSON字段平铺；没有返回内部主键、手机号、登录用户名、密码、实名、生日或Token。

`isLive`结合账号可用性、主播审核/权限/风险、房间状态、当前场次及其关联、房间可见性、关注和双向拉黑判定。无查看权限时返回false并隐藏入口。展示结果是查询时快照，点击进入直播间仍执行当前权限检查。

需要前端适配的变化：

1. `GET /api/v1/app/social/history/list` 返回 `HistoryUserList`：公开用户资料加 `resourceType/resourceId/createdAt/lastWatchedAt`。不再返回房间标题、封面或进度字段；原资源键用于删除记录。目前仅支持live_room，省略类型时默认该类型。
2. `history/progress/batch` 保持原进度契约，与历史用户卡片分开定义。
3. `GET /api/v1/app/social/favorite/list` 已从Router、API、Service及Swagger删除，返回404。收藏、取消收藏及批量状态仍保留，数据表不删除。
4. `live/following-online` 增加公共用户字段，保留title/coverUrl和直播编号。
5. 管理health的每状态count最多1000；countIsLowerBound=true表示总数大于1000，sampleOldestCreatedAt只表示样本最早时间，界面应显示“1000+”。
6. 共同关注、互关、后台异常关系扫描允许空页且hasMore=true，客户端按nextCursor继续，不要根据list长度判断结束。

完整示例见[前端对接说明](frontend-integration.md)。当前社交接口38个：APP31、后台7。

## 2. 审查发现与修复

| 问题 | 本次修改及效果 |
|---|---|
| 用户资料重复定义且关系列表缺少资料、直播状态 | 统一公开DTO，批量查询账号与有效直播；直播详情继续使用白名单转换 |
| 给每个用户单查直播容易产生N+1 | 1人和100人公开资料均最多2条SQL；列表只取当前页用户 |
| MySQL可能通过主播索引读取历史场次 | 当前场次关联固定使用PRIMARY，按room.current_session_id点查；实际EXPLAIN验证 |
| 多条关系查询可能拼出不同事务时刻的关注/拉黑状态 | 双向关注、拉黑、屏蔽和订阅合并为单条UNION ALL快照；拉黑时关系状态不显示关注/订阅 |
| 缓存可能带出旧资料或旧直播状态 | 每次重新读取公开资料及直播权限，再替换缓存载荷中的用户字段；关系版本仍用于隔离旧缓存回填 |
| 热门counter每次锁定前都执行重复upsert | 先按主键升序锁现有行，仅缺失时创建并重读；保留跨实例数据库锁和同事务准确计数 |
| 稀疏共同关注/互关或异常关系查询可能扫描大量候选 | 先按索引取limit+1候选，再对最多100条执行批量交集/拉黑检查；按候选推进游标 |
| 重开已有历史仍重复COUNT和容量清理 | 仅新增历史行时执行容量检查，保留每人1000条上限 |
| 删除历史后重开版本可能重新从低值开始，旧Redis版本压制新上报 | 用户counter新增history_version；在用户锁下单调生成会话版本，删除/清空不重置 |
| Lua重编码大整数可能损失精度 | Redis保留Go生成的原始JSON；测试覆盖JS最大安全整数sequence |
| 多Worker可读取同一历史队列并重复访问MySQL | 增加15秒分片租约，单次处理最多10秒；数据库token/version/epoch/sequence与CAS确认继续保证旧任务不能覆盖新会话 |
| 慢通知处理可能阻塞健康心跳和历史刷盘 | 使用固定四个有界循环，通知、历史、审计健康、清理分开运行，退出时等待结束 |
| Worker故障导致用户无法撤销关系或删除历史 | 拉黑和取消/删除类操作免除Worker健康/背压前置条件，仍执行Redis写限流及事务审计 |
| health轮询全表统计、清理排序代价随事件量增长 | health按状态采样；过期事件按(status,updated_at,id)索引分批锁定，MySQL SKIP LOCKED减少多实例互抢 |
| Outbox对象长度小于现有sessionNo上限 | object_no由varchar(64)扩展至varchar(96) |

既有不变量继续保留：JWT决定操作者、APP与后台身份分离、后台角色888、关系与计数与Outbox同事务、唯一约束兜底、仅对死锁/锁等待超时有限重试、参数与批量上限、敏感写Redis故障拒绝、所有权限从数据库事实判定。

## 3. MySQL查询与索引证据

公开资料：账号和主播公开字段1条批量SQL；有效直播与权限最多1条批量SQL。没有主播时不发直播SQL。普通关注/粉丝页由1条有界关系候选SQL加最多2条资料SQL组成，不执行总量COUNT。历史页额外批量检查原资源可见性，不按每个用户循环查询。

实际MySQL8.0.46、1001账号/1000主播/1000房间/1000场次的100人批量直播查询计划：

| 数据 | 使用索引 | 访问方式 |
|---|---|---|
| live_account | PRIMARY | range，100个目标 |
| live_anchor | uk_user_id | eq_ref |
| live_room | uk_live_room_anchor | eq_ref |
| live_session | PRIMARY | eq_ref，当前场次 |
| 两个方向social_block | uk_social_block_pair | eq_ref |
| 关注者权限social_follow | uk_social_follow_pair | eq_ref |

该样本没有ALL全表扫描。完整计划和测量在[原始结果](load-result-20261027.json)。这不是百万粉丝或多年历史数据分布的执行计划证明。

## 4. 已执行验证

- 真实MySQL/Redis定向测试：`service/social`、`service/live`、`api/v1/social`、`api/v1/live`、`router/social`、`initialize`、`model/social`全部通过。
- `go test -race ./service/social ./service/live ./router/social ./model/social`通过。
- 定向`go vet`通过，`go build ./...`通过。
- 重新生成Swagger，确认38条社交路由、历史新响应、各用户DTO字段一致，favorite/list不再注册。
- `git diff --check`通过。既有全仓测试问题见[原验收记录](verification.md#全库既有检查失败)，本次不将定向测试描述成全仓测试通过。

新增/扩展回归覆盖：公开/私密/仅关注者直播、双向拉黑、主播权限撤销、结束/准备中/场次关联错误、封禁账号、资料敏感字段排除、历史只读本人、公开详情与关注/在线/历史相同资料、1人与100人查询数量、关系缓存命中后资料与直播刷新、删除重开及大整数上报、分片租约互斥、无Worker时隐私操作仍受限流、稀疏互关的空页游标、有界健康样本、真实MySQL计划和并发状态机。

复现：`cd server && ./tests/social/run.sh`。脚本使用独立本机端口13316/16389和social_test_*数据库；退出清理测试容器。真实资料测试用单独数据库，避免Go多个package并行测试互相清表；Redis的live/social测试分别用DB14/DB15。

## 5. 本机负载样本与实际边界

Go1.24.5 darwin/amd64，Docker Desktop，MySQL8.0.46/Redis7.4.11，临时数据库，Service调用，不含HTTP/JWT、跨机网络或生产数据规模。最终资料测量没有与race测试同时执行。

| 场景 | 请求/并发 | QPS | P95 ms | P99 ms | 错误 |
|---|---:|---:|---:|---:|---:|
| 真实账号/主播/直播表，关注列表100人 | 400/16 | 1336.21 | 17.67 | 19.86 | 0 |
| 100个目标关系状态，无Redis，受控用户适配 | 400/16 | 1305.38 | 15.79 | 24.66 | 0 |
| 同一主播1000个用户首次关注，受控用户适配 | 1000/32 | 103.54 | 618.27 | 691.12 | 0 |
| 同热点主播取消后重新关注，每请求两操作 | 1000/32 | 87.94组 | 397.25 | 501.21 | 0 |
| 历史同步checkpoint，受控资源适配 | 100/16 | 1660.53 | 12.06 | 12.54 | 0 |
| Redis同资源聚合心跳，串行调用 | 1000/1 | 4146.92 | 0.26 | 0.31 | 0 |

社交关系压测使用受控用户/资源适配器；只有首行包含真实live表关联，不能混淆。热点数据结果表明，同一主播的同步准确粉丝计数仍存在行锁串行化；本次减少了不必要SQL，但不以缓存或异步计数掩盖写入瓶颈。跨机器、百万关系、多主播分布以及完整HTTP链路仍需按实际部署和SLO验容量，不能据此承诺任意规模或后续永久无需优化。

普通历史心跳仍是短期缓冲，Redis丢失或60秒过期会损失未落库进度；需要持久化确认时使用checkpoint。列表展示与后续点击间可能发生关系/直播变化，实际进入时必须重新鉴权。推送未来接入仍按至少一次定义。这些是明确的系统语义，不应对前端承诺更强保证。

## 6. 部署与兼容

- 沿用单数snake_case表、snake_case字段、camelCase DTO和九张社交表，没有新造用户或主播关系表。
- GORM自动迁移新增`social_user_counter.history_version bigint unsigned NOT NULL DEFAULT 0`，扩展`social_outbox_event.object_no`至96字符；已有关系和收藏数据保留。
- 正式更新应先备份、确认DDL权限和迁移窗口，再让全部实例使用同版代码。尤其历史会话生成算法变更时避免长时间新旧版本混跑；旧会话凭证仍按数据库当前会话校验。
- 前端同步历史DTO、删除收藏列表调用、复用公开资料类型、识别health下界。服务重新部署后接口变化才会在线上生效。
- 本次只更新工作区并在隔离容器验证，未触发发布、线上重启或生产迁移。
