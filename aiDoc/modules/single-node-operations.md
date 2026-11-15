# 单机运行可观测性

internal/telemetry只记录固定路由模板和固定worker名字，最多512路由加overflow，不以账号/房间/错误内容作为指标标签。HTTP记录次数/5xx/429与503/耗时桶；worker记录运行、失败、连续失败、最近成功和停滞。重启会清零，没有外部指标存储或告警通知接收方。

service/system/operations.go实时核验root，在3秒预算内检查DB/Redis、连接池、堆/goroutine及有界出款积压。API GET /v1/admin/system/operations无参数且no-store，web/src/view/system/OperationsPanel.vue必要交互展示。数据库查询异常显示未知，不作成功计数。

日志Cutter保持文件句柄，按天/64MiB轮转，每级别保留3份当日旧文件，清理只扫描直接日期目录且每小时最多一次，新旧日志根目录及当日日志目录均收紧到0700，当前文件0600。标签仅结构化字段，不成为动态路径；拒绝非普通文件。部署日志目录必须私有、防不可信本地用户写入；非分布式安全存储。

错误数据库副本只保存level/caller及服务端rt_UUID trace，不复制原始错误/请求数据。数据库持久化最多2并发、500ms；写失败不递归日志。正常应用文件日志依然可能包含其他模块自定义字段，不能声称全项目无任何敏感内容。SRS hook URL/param整体脱敏。

SIGTERM进入draining后ready返回503，HTTP最多等待15秒，再按模块有界停止worker并Sync日志。业务数据库连接在worker结束前保持可用。上线需让服务管理器的stop超时覆盖HTTP及各worker退出预算，监控进程退出和资源上限；Compose的server已设置stop_grace_period:60s。
