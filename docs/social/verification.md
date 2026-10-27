# 验收记录（2026-10-25）

最新2026-10-27变更及实测见[公开资料统一与生产审查](production-review-20261027.md)和[本次原始数据](load-result-20261027.json)。下文保留2026-10-25历史测量，不能作为最新契约；新增验证摘要见文末。

## 已执行检查

- gofmt；`git diff --check`。
- 社交、直播、模型、API、Router、initialize、middleware 的定向 go test。
- 社交/直播/Router/模型/middleware 的 race 测试。
- 隔离 MySQL 8.0.46、Redis 7.4 的集成与并发测试；GORM AutoMigrate 在真实 MySQL 执行。
- 定向 go vet 与 `go build ./...`。
- 重新生成 Swagger。
- `go test ./...`、`go vet ./...` 已执行，但全库未全绿，现有问题见下文，不能把定向通过描述成全库通过。

宿主：Go 1.24.5，darwin/amd64，VirtualApple @2.50GHz；Docker Desktop，MySQL/Redis独立测试容器，环回地址，数据库使用临时文件系统。没有修改 live_dev/live_pro 数据。

## 容量测量

下表为最后一轮未同时运行 race 的 Service 级测量，不包含HTTP、JWT、真实资源表大规模关联或外部推送网络耗时；测试账号/资源适配器是受控fixture。不能外推为生产容量或30万DAU达标。原始结果见 `load-result.json`。

| 场景 | 样本/并发 | QPS | P95 ms | P99 ms | 错误率 |
|---|---:|---:|---:|---:|---:|
| 1000个不同用户关注同一主播 | 1000 / 32 | 131.00 | 263.551 | 329.149 | 0 |
| 100目标批量关系状态，Redis禁用回源 | 400 / 16 | 878.90 | 23.768 | 31.033 | 0 |
| 1000粉丝数据的100条游标页 | 300 / 16 | 2884.51 | 9.963 | 12.625 | 0 |
| 严格观看checkpoint | 100 / 16 | 1510.55 | 12.257 | 13.108 | 0 |
| 同热点主播取消后重新关注（一组两操作） | 1000 / 32 | 71.32组 | 538.516 | 553.383 | 0 |
| 开播通知扇出20人/页，测试Sink | 5页 / 1 | 10.43页 | 96.695 | 96.695 | 0 |
| Redis同资源心跳聚合 | 1000 / 1 | 3776.29 | 0.291 | 0.558 | 0 |

心跳1000次最终仅一次持久化。通知样本仅100人、无网络推送，结果不代表渠道吞吐。

数据库连接池最大/实际打开32，WaitCount=0，连接池等待0ms。采样12次，MySQL CPU最高34.48%，容器内存约730MiB；Redis CPU最高2.02%，Redis INFO used_memory约1.39MiB、peak约1.55MiB。Docker CPU百分比按其自身口径，采样不是精确峰值记录。

慢查询阈值100ms，测量窗口记录2963条，最长约330.633ms；主要体现同一热点counter行锁等待，而非数据库CPU饱和。单热点同步准确计数明确是瓶颈。第一版不能通过异步计数绕开正确性；上线前必须按真实主播粉丝分布、机器、HTTP鉴权、读写比例扩大样本再确定SLO。若实测超过该热点上限，下一版需要有事务/分片不变量证明的分片计数设计，不能只加缓存宣称解决写热点。

SQLite纯逻辑 Benchmark（不代表MySQL生产QPS）：Follow+Unfollow约597739 ns/op，BatchStatus（三目标）约180508 ns/op，HistoryCheckpoint约118688 ns/op；包含大量GORM分配。完整重跑命令：

```sh
cd server
./tests/social/run.sh
# 只跑本地逻辑测试：
go test ./service/social ./service/live ./api/v1/social ./router/social ./model/social ./initialize
# 单独生成更长的benchmark样本：
go test ./service/social -run '^$' -bench . -benchmem -benchtime=10s
```

MySQL集成仅接受 `social_test_*` 库；会清理该库内测试表。Redis测试只接受compose约定的127.0.0.1:16389、DB15。生产压测必须另建独立环境，不得复用真实用户JWT或生产库。

## 索引与实际 EXPLAIN

真实 EXPLAIN 已保存在 load-result.json。当前样本小且1000条关注几乎都指向同一主播，优化器对粉丝页和订阅扇出选择PRIMARY范围扫描；不能把小样本计划写成已验证百万粉丝最优。关注出边单用户仅一条时选择唯一索引并有小范围filesort。没有凭经验强制索引。

| 索引 | 查询模式 |
|---|---|
| uk_social_follow_pair | 双向状态、幂等唯一约束、共同/互关EXISTS |
| idx_social_follow_out(user_id,id DESC) | 本人关注按id稳定游标 |
| idx_social_follow_in(target_user_id,id DESC) | 粉丝按id稳定游标 |
| uk_social_block_pair / idx_social_block_list | 双向拉黑事实、本人黑名单游标 |
| uk_social_mute_scope / idx_social_mute_list | scope唯一状态、本人屏蔽游标 |
| uk_social_favorite_resource / list / type | 收藏唯一、全类型及类型过滤游标 |
| uk_social_history_resource / recent / retention | 资源进度唯一、last_watched_at+id游标、保留期清理 |
| uk_social_subscription_pair / list / fanout | 订阅唯一、本人列表、target+enabled+id扇出 |
| uk_social_event_id / business | 事件标识与业务事件去重 |
| idx_social_event_claim | status+next_attempt_at+id租约领取，实测命中 |
| idx_social_event_audit | 内置关系审计批量消费 |
| idx_social_event_retention | 已完成事件分批保留期清理 |
| uk_social_consumer_delivery | event+consumer+user去重及按event清理 |
| counter PRIMARY(user_id) | 顺序行锁、原子条件计数、持锁修复 |

继续容量评估：在独立库构造多主播Zipf分布、至少百万边，ANALYZE TABLE 后对首/中/尾页执行 EXPLAIN ANALYZE；记录examined rows、filesort、buffer pool命中、锁等待和连接等待。采集请求QPS/P95/P99/失败率和CPU/内存后再决定是否修改索引。

## 40项异常验收映射

自动化测试在 service/social、service/live/live_access_test.go、router/social、api/v1/social。下表标“实验”的内容是明确待真实环境执行，不是已经通过。

| # | 场景 | 自动化或复现实验 |
|---:|---|---|
| 1 | 1000请求同时关注 | TestMySQLConcurrency，真实MySQL，唯一边和计数核对 |
| 2 | 重复关注 | TestRelationshipStateMachine，changed=false |
| 3 | 关注取消交错 | TestMySQLConcurrency及状态机，最终事实/计数一致 |
| 4 | 关注同时被拉黑 | TestMySQLConcurrency，最终无冲突边 |
| 5 | 双方互相拉黑 | MySQL并发测试与状态机；两向拉黑保留、两向关注清除 |
| 6 | 中途失败 | TestRollbackCountInvariantAndRepair，outbox写失败注入后全事务回滚 |
| 7 | 唯一键冲突 | 并发测试由数据库唯一约束兜底；1062分类测试不盲目重试 |
| 8 | 死锁 | TestTransactionRetryClassificationAndCancellation注入1213；实验：两个SQL会话相反顺序锁counter制造真实死锁，检查重试指标及上限 |
| 9 | 锁等待超时 | 注入1205；实验：会话A持有counter锁，B设置短innodb_lock_wait_timeout，验证4次以内结束 |
| 10 | Redis完全不可用 | TestRedisOutageFailsClosedButRelationshipsRemainCorrect |
| 11 | Redis旧值回填 | TestRedisStaleFillCannotOverwriteNewVersion，真实Redis |
| 12 | 提交成功响应丢失 | 状态幂等测试；实验：反向代理提交后丢弃响应，再以同目标重试，计数/事件不增加 |
| 13 | 不存在用户 | TestValidationMuteAndAutoSubscription |
| 14 | 封禁用户 | 同上及TestLiveAccessClosedPrivateDisabledAndDBFailure |
| 15 | 封禁主播 | 统一权限过滤；实验：后台封禁已直播主播，检查详情/播放/收藏拒绝及既有直播收尾 |
| 16 | 关闭场次 | TestLiveAccessClosedPrivateDisabledAndDBFailure |
| 17 | 关注者进入 | TestLiveAccessFollowsBlocksAndCopiedPlayback |
| 18 | 取消后播放 | 同上，旧凭证重新校验失败 |
| 19 | 拉黑后旧URL | 同上，既有签名不能绕过新关系 |
| 20 | 复制HLS给他人 | 同上验证用户绑定；真实网关/播放器切片链实验待执行 |
| 21 | 大量粉丝页 | 1000粉丝实测；百万级分布EXPLAIN/压测待执行 |
| 22 | 同毫秒多条边 | ID作为唯一稳定排序键，不依赖时间排序；Cursor测试 |
| 23 | 分页期间取消 | TestCursorStableHiddenAndVersionKeys |
| 24 | 多设备观看 | TestHistoryDevicesOutOfOrderRewindClearDelete |
| 25 | 心跳乱序 | 同上及Redis聚合测试 |
| 26 | 快速订阅开关 | 状态机/AutoSubscription；实验并发交错订阅/取消后核对最后事务状态 |
| 27 | SRS重连 | 既有live_room测试通过；首次开播事件只在Preparing分支，RecordLiveStarted唯一键测试 |
| 28 | 结束与通知并发 | 每次投递前校验适配；实验：阻塞Sink，结束场次再释放，检查后续收件人跳过；已发消息不承诺撤回 |
| 29 | Worker中途崩溃 | TestOutboxCrashLeaseRecoveryAndStaleCompletion，过期租约重领、旧持有者提交失败 |
| 30 | 重复消费 | TestOutboxDedupFailureReplayAndRevocation、TestDeliveryReceiptFailureIsAtLeastOnce |
| 31 | 时间不一致 | TestUTCTimeDriverLocationDoesNotChangeInstant、MySQL不同loc测试；实验对两实例偏移时钟，检查播放凭证拒绝/任务延迟，运维必须NTP |
| 32 | 历史计数错误 | TestRollbackCountInvariantAndRepair；持锁修复不覆盖并发增量 |
| 33 | 资源删除后收藏 | TestSocialRoomResourceChecksAccess与批量过滤；真实短视频删除尚无模块 |
| 34 | 清空后旧缓冲 | TestRedisAggregationClearFenceAndQueueBound，真实Redis |
| 35 | 整数上限 | TestCounterAndSessionOverflowFailWithoutPartialChange及参数上限、条件更新 |
| 36 | 连接池耗尽 | 测量pool统计；实验把MaxOpenConns设1并占住连接，调用带超时ctx的写入，期望context超时、无部分状态 |
| 37 | Redis热key | 同资源1000心跳测量与队列上界测试；分片单热点/RedisCluster大压测待执行 |
| 38 | 依赖超时重试 | 事务重试分类、幂等状态机；提交结果未知不在服务端无条件重试 |
| 39 | 新旧计数并写 | 已把已知主播DTO读取改到权威表；上线停旧写者，回退SQL仅停写执行，不能双权威 |
| 40 | 直接媒体地址 | API/凭证路径测试及默认上线门禁；真实防火墙/CDN/SRS端口扫描与拉流实验待执行 |

## 安全审查与修复

已实际修复并复测：

- 两端稳定数据库锁替代本地mutex；计数更新条件防下溢，异常回滚。
- 幂等插入不依赖clientFoundRows参数推断成功；唯一约束兜底。
- 旧缓存回填使用双方version构造键；账号资料每次重新可见性过滤。
- 独立APP身份，不使用请求userId；全路由匿名拒绝与管理员角色矩阵测试。
- 观看版本/序号/epoch与删除令牌检查，防旧缓冲恢复。
- Worker lease fencing、有限失败、过期直播跳过；外部投递故障窗口如实按至少一次定义。
- 只接受受约束资源类型/编号和媒体路径，签名校验绑定身份及场次；未知路径默认拒绝。
- 私密/关注者房间所有新开播入口加媒体门禁，阻断只限制前端的上线方式。
- UTC datetime类型隔离旧项目loc=Local影响；SQL与模型命名/索引一致。
- 新Swagger路径与旧后台BasePath兼容；反向代理不向SRS转发播放签名query。
- 新Worker无逐用户goroutine；队列分片容量、保留期、限流与积压背压均有上限。

## 全库既有检查失败

`go test ./...`：mcp/client/client_test.go 动态Fatalf；utils/json_test.go 多处动态Errorf；service/system/auto_code_package_test.go 动态Logf，触发Go1.24默认vet检查。utils/ast测试引用不存在的server/plugin/gva文件，随后panic。该测试还向gorm_biz.go注入无关import，已恢复测试前内容。

`go vet ./...`：上述测试问题及service/system/sys_auto_code_sqlite.go既有未命名结构体字面量。没有为让全库表面通过而跳过测试或修改无关业务。本次范围定向vet/build/test/race结果另列，不混淆。

## 未验证与上线风险

真实SRS/HLS播放、CDN逐切片验权、源站隔离、终端续期、媒体缓存与协议降级尚未实测。短视频/回放、真实通知渠道未接入。当前后台只有API，无新增管理页面。测试没有证明1万主播/30万DAU、百万粉丝、跨机房或跨RedisCluster容量。实例延迟桶与业务计数已接入后台health和现有Zap，尚未新增Prometheus/OpenTelemetry采集系统；跨实例P95/P99需现有日志/监控平台汇总，不能直接平均各实例分位数。

## 2026-10-27 增补验证

统一UserPublicProfile、历史用户卡片、删除favorite/list、实时直播权限过滤及本次SQL/历史/Worker优化已完成。真实MySQL8.0.46、Redis7.4.11定向测试及race通过；定向vet和go build ./...通过，Swagger已重新生成，社交接口为38个。未再次宣称全仓go test ./...或go vet ./...通过；历史问题记录保留。

真实账号/主播/房间/场次表的100人关注页：400请求、并发16，QPS1336.21，P95 17.67ms、P99 19.86ms、零错误。实际计划当前场次走PRIMARY；资料查询1人和100人均最多2条。此为本机Service小样本，不含HTTP/JWT及生产规模。

本次新增断言覆盖公开资料复用、敏感字段排除、直播权限与状态、缓存命中后资料刷新、删除重开历史单调版本、JS安全整数精度、分片刷盘租约、Worker故障时撤销操作限流、有界交集游标及健康采样。逐项修复和部署兼容事项见[本次审查](production-review-20261027.md)。
