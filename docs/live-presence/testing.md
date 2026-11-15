# 验证记录与验收边界

## 实际运行环境

2026-11-06，本机 macOS/Go 1.26.10/Node 24，Docker 隔离 Redis 7.4、MySQL 8.0（tmpfs 测试库）。Redis 只绑定回环 16479、测试 DB15；MySQL 只绑定回环 13436。集成测试拒绝非回环地址及非 presence_test DSN，每例创建随机独立数据库后删除。测试未清空真实 Redis、未在远程正式库运行压力/破坏性故障测试；本次创建的两个测试容器已删除。

新增复现入口 `server/tests/presence/run.sh` 使用独立 Compose 项目、回环默认 18479/15436，EXIT/INT/TERM 清理自己创建的容器与卷；可通过 PRESENCE_REDIS_PORT/PRESENCE_MYSQL_PORT 改测试端口。脚本不会读取 config.prod.yaml，依赖 Docker、Go 和可获取的测试镜像。脚本语法和 Compose 配置已校验；本次实际执行的是等价的隔离容器命令与测试命令。

```bash
bash server/tests/presence/run.sh
# 管理端
cd web
node --experimental-vm-modules --test tests/presence.test.mjs
pnpm run build
# 实际观看端
cd /Users/tabby/Desktop/IM/H5Player
node --experimental-vm-modules --test tests/*.test.js
pnpm run build:prod
```

## 通过的验证

- Go 全仓 `go test ./... -count=1`；真实 Redis/MySQL presence 集成、增量迁移两次、私有备份内容与权限。
- `go test -race` 覆盖 service/live、api/v1/live、router/live、middleware，并对最后修改的恢复审计、队列排空和迁移命令补跑。
- 两个 fuzz 各 10 秒、并行 2：凭证解析 2651 次、严格 JSON 21767 次，无崩溃；这不是穷尽安全证明。
- `go vet ./...`、`go build ./...`、Swagger 重新生成。
- H5 全套 57 项通过（其中 presence 8 项）；管理端 composable 测试通过；两个前端生产构建通过。现有后台其他模块仍有 ES2015 与 BigInt 目标不匹配的构建警告，本功能数量处理要求支持 BigInt 的现代浏览器；未改写那些既有模块。

覆盖内容：进房幂等与不同连接、多设备去重、游客与会员区分、乱序/重复心跳、暂停/隐藏与最大时长、观看并集、TTL与并发清理、场次结束最终汇总、账号/关系/房间权限、旧凭证与代际、点赞并发/配额/幂等、游标隐私、坏键类型、Lua 整数边界、Redis 网络黑洞、MySQL 写入失败后重试、满队列排空、Worker 退出、Redis 丢失恢复审计、历史版本防倒退、gift/social 已提交事实投影、退役外部统计入口。

HTTP 层包含 APP/后台身份隔离、管理员当前权限、重复与未知 JSON 键、大小/头/query 限制、依赖故障错误、无敏感错误返回；CORS 在既有 Origin 白名单内允许 X-Presence-Guest 并暴露 Retry-After，未知 Origin 仍拒绝。前端包含晚到响应、切号、指数退避、原键原序号重试、过期重进、连续点赞不拖延心跳、状态刷新不能越过 Retry-After。

## 本机负载数据

以下是一次隔离运行的实测值，Redis 已有测试热身数据。open-state/heartbeat-state 直接测 Redis 状态机，**不包含 HTTP、JWT、权限 SQL、代理、TLS 或跨机网络开销**。另有真实 MySQL 的 20 个同键并发完整 Service.Open 验证幂等、10 个并发 checkpoint 验证落库与终态；不能用状态机数据替代完整生产接口吞吐。

| 场景 | 数量/并发 | 总耗时 | P95 | P99 |
| --- | --- | --- | --- | --- |
| 单热门房间 open 状态机 | 1000 / 32 | 117.43 ms | 6.41 ms | 11.07 ms |
| 心跳状态机 | 1000 / 32 | 92.07 ms | 5.16 ms | 7.61 ms |
| 在线列表，真实 MySQL 公开资料 | 10 页×100 / 串行 | 未单独汇总 | 14.34 ms | 14.34 ms |
| 点赞 Service 突发 | 1000 / 32 | 71.20 ms | 4.30 ms | 6.65 ms |
| 16 房间 open 状态机 | 160 / 32 | 24.54 ms | 9.98 ms | 11.34 ms |
| MySQL 批量 checkpoint | 1001 visits / 12 次有界批次 | 259.56 ms | — | — |
| 过期清理 | 1001 / 每批≤100 | 24.31 ms | — | — |

点赞突发接受 4、拒绝 996（限流或乱序），验收的是高频请求不能突破账户额度，不是 1000 次都成功。串行列表仅 10 样本，P99 等于最慢样本，不足以评估稳定尾延迟。

该轮 Redis INFO：used_memory=4055240 字节（3.87 MiB）、used_memory_peak=5312560 字节（5.07 MiB）；累计 used_cpu_sys=2.041441s、used_cpu_user=14.352160s。这是测试实例启动以来的累计 CPU，**不是该轮 CPU 百分比**。完整输出见 `evidence/load.log`。同场全部状态键及全局 admission 是实际热点；本轮未部署真实 Redis Cluster。

批量 UPSERT/批量历史同步前，同规模约 2.46 秒，优化后本轮约 0.26 秒；不同轮次环境存在波动，该对比用于说明减少数据库往返的效果。

## 仍需目标环境验收

- 长时间稳定负载、完整 HTTP/JWT 路径、多个进程/主机、单房极限与全局 admission 热点；配置上限不是容量承诺。
- 实际 Redis Cluster 拓扑/故障转移、跨机房、恢复时间、备份恢复演练；当前验证 Lua 键同 hash tag，不宣称完成 Cluster 故障实测。
- 实际 SRS/CDN 播放权限与防盗链、浏览器/手机后台调度、网络切换；本次未进行真实主播推流和线上付费交易。
- 多年 outbox/event 规模下的反连接投影扫描、访问事实归档、外部告警平台与运营值班流程。

远程迁移验收独立于应用发布：两轮 financialTablesUnchanged=true、结构 verified=true，应用尚未发布。不得根据本地通过就称线上已经采集在线人数，也不承诺 Redis 丢失情况下历史零损失。
