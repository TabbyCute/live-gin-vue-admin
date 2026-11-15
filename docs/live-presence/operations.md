# 运维、审计与恢复

## 日常观察与排障

后台房间列表回答“哪个主播当前在播、游客多少、会员多少”；详情的统计面板回答“数据是否新鲜、是否最终完成、是否积压”。场次状态继续以 live_session/SRS 生命周期服务为准，presence 不能把在线人数当成开播信号。

排障先记录 roomNo、sessionNo、时间范围、HTTP 状态、业务码；不要收集 JWT、presenceToken、原始 IP 或完整请求正文。服务日志 `live presence operation failed` 提供 code/sessionNo/failureCount，首条及每 100 条采样，避免依赖故障造成日志风暴。典型 code：redis_operation、state_corrupt、worker_failed、checkpoint_failed、source_projection_failed（以 failure 调用点为准）。没有将原始 SQL、DSN、Redis key 或凭证拼入对外错误。presence 使用独立静默 SQL logger，主请求日志对相关路径 query 脱敏。

建议接入现有监控平台的指标/告警：HTTP 49506/49508、429、请求耗时、60 秒无成功 checkpoint、pendingCount 持续增长、cleanupLagMs 超过两倍 TTL、结束场次长时间 finalizedAt=0、Redis OOM/evicted_keys/used_memory/CPU/命令延迟、MySQL 事务耗时/锁等待。当前交付包含运行字段和结构化日志，**未替用户配置外部监控平台或通知渠道**。Redis/MySQL/应用主机应做时钟同步。

依赖故障下只显示“未知”或持久累计快照，不把人数填成 0。已结束且完成汇总的场次当前在线 0 是确定值。Worker 没有建立检查点或场次已经封存时，不用 workerHealthy=false 单独推断故障。

仅使用只读数据库账号进行调查；以下 SQL 的 `?` 必须通过参数绑定：

```sql
SELECT session_no,room_no,epoch,revision,updated_at,attempt_at,finalized_at,
       data_lost,last_error,view_count,viewer_count,like_count,total_watch_duration_ms
FROM live_presence_checkpoint WHERE session_no=?;

SELECT visit_no,viewer_id,joined_at,last_seen_at,left_at,status,exit_reason,
       watch_duration_ms,like_count,revision
FROM live_viewer_visit WHERE session_no=? ORDER BY joined_at,visit_no LIMIT 100;

SELECT event_id,event_type,source_type,source_no,occurred_at,payload
FROM live_room_event WHERE session_no=? ORDER BY occurred_at,event_id LIMIT 100;
```

用户编号和访问时间为内部业务数据，应按调查权限与保留政策使用；不直接把 SQL 结果当成 APP DTO 返回。日常客服不需要原始网络标识。表增长应按业务保留政策归档；目前未开启自动永久删除访问事实或审计事件。

## 钱款去向排查

观众统计不是账本。先从 `live_room_event` 的 `source_type='gift'` / `source_no` 取得真实 giftOrderNo，或直接用既有礼物订单页按房间/场次查单，再沿项目已有链路核对：

`gift_order → wallet_transaction / wallet_ledger → anchor_income_fact / anchor_earning_ledger → anchor_settlement / allocation → withdrawal → payout_order → payout_attempt / event / evidence / reconciliation`。

具体关联键与权限沿用 gift、earning、payout 文档及后台详情。presence 不复制金额，不自动确认 paid，不把礼物展示事件当成扣款成功依据。礼物退款和原始账务仍由原模块处理；房间事件是历史关联，不因退款删除。没有发布成功的 gift_order 不会被投影成 room.gift.sent。关注事件同样必须来自已提交 social outbox。

## Redis 丢失与恢复

1. 确认服务连接的是正确 Redis/DB/Cluster，排除错误配置、网络故障和主从切换暂态。检查 MySQL 已持久数据和日志，保存受影响场次号。
2. 完整丢失会标记 dataLost/redis_state_lost，停止自动重建。服务重启本身不会清空 Redis；不要使用 FLUSHALL、FLUSHDB、KEYS 批量删生产键或清空 MySQL。
3. 只有确认该场次全部 11 个状态键均已不存在，才可通过有数据库及 Redis 权限的私有运维 CLI 操作。命令先备份受影响表并进行资金只读摘要校验，再在事务中推进 epoch、保留累计基线、关闭旧 visit，并写 `room.presence.recovered` 事件记录原因/旧新代际。没有公开 HTTP 恢复接口。

```bash
# 从 server/ 执行；示例参数需替换成经核实的真实场次，不要把密钥填入 reason。
go run ./cmd/presence-migrate \
  -config /私有路径/生产配置.yaml \
  -recover-session 真实场次编号 \
  -reason '已核实该场次Redis全部状态丢失，批准重新收集在线状态' \
  -backup-dir /私有绝对路径/备份
```

4. 任一原键仍存在即拒绝恢复，工具不会替操作人删键。部分键损坏需先查原因/评估可信备份；不要为通过命令而删除剩余数据。命令无自动“强制恢复”选项。
5. 下一次新 open 从已落库基线初始化，旧 token 失效；旧游客/会员仍可通过新连接重新收集。dataLost 永久保留，未落库窗口不能还原。近期观看历史的原有事实不删除。
6. 核对新 checkpoint、当前人数、事件审计、最终汇总，并保存 CLI JSON 输出及私有备份摘要。若原场次已终态，Worker 会完成基线封存。

恢复操作只修复统计可用性，不能修补、冲销或确认任何资金事实。金额争议须使用已有对账/调查流程。

## Worker 与积压

多实例争抢同场 15 秒租约，事务中另用 epoch/revision fencing；退出时取消任务并等待最多 12 秒。Worker 通过数据库检查点发现待处理场次，不扫描 Redis 全库。失败场次更新 attempt_at 后轮转，避免一直阻塞其他房间。

SQL 失败不会确认 Redis 待办；Redis 确认失败可重跑已落库批次。事件队列满时先落库排空，再继续过期清理。若已有持久事实且 Redis 整场不存在，Worker 不会假装成功最终汇总，需要上述受控恢复。依赖长期失败会停止统计写入；播放器是否还能播放由原媒体服务决定。

不要以增加连接上限代替修复慢 MySQL、热点 Redis 或积压。当前 Worker 每批固定 100，单热门房间必须验证消费速度能持续超过新增脏记录速度；无严格端到端持久化 SLA 的场景不能宣称每次 leave 都已同步落库。
