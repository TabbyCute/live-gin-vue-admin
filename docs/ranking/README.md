# 礼物榜、财富榜和魅力榜

ranking 是独立的只读投影，不修改钱包、礼物、主播收益或出款数据。旧管理接口 `/gift/ranking` 保留原有 UTC 统计含义；新榜单采用显式 `ranking.timezone`，默认 `Asia/Kuala_Lumpur`，同一投影代际禁止混用时区。

| 榜单 | 主体 | 范围 | 分值 |
| --- | --- | --- | --- |
| 礼物榜 | 送礼用户 | anchor/room/session，可选 giftNo | 原始礼物 COIN − 成功退款 COIN |
| 财富榜 | 送礼用户 | platform | 全平台净礼物消费，排除充值、余额、收益及提现 |
| 魅力榜 | 主播 | platform/category（礼物分类） | 原始礼物净价值，不按主播分成金额计算 |

每种范围生成日、周、月、总榜。周一开始，退款永远冲减原送礼日期所属期间。数据来自 gift_revenue_ledger 与 gift_order 的不可变维度/价格快照，不读当前礼物价格。净值为零的主体不占榜位。同分按公开编号升序，名次唯一连续。

## 投影设计

- ranking_applied_event：RevenueNo 和 source ID 永久唯一，保留原送礼时间和消费维度。
- ranking_score_bucket：完整维度唯一键、精确十进制金额/件数、65 位补零排序键与版本；不经过 float64/JavaScript Number。
- ranking_generation_receipt：每代际独立来源收据，原 applied_event 不删。
- ranking_checkpoint：消费端、最大已见 source ID、当前代际、时区、处理时间、错误与退避。
- ranking_rebuild_job：任务号、代际、进度、计数、失败原因、开始/完成时间。
- ranking_admin_permission / ranking_audit_log：精确权限、永久请求幂等和不可变审计。

Worker 在 checkpoint 行锁内按来源 ID 顺序批处理；订单批量读取，桶批量读取/写入，收据、桶和 checkpoint 同事务提交。使用“该代际没有处理收据”的条件补漏，不只用 `ID > checkpoint`：MySQL ID 分配顺序不等于事务提交顺序。崩溃回滚后可重试，已提交批次保持永久去重。checkpoint 表示最大已见 ID，完整性应同时看 lagCount=0 和对账结果。

历史重建使用新代际，保留旧榜查询，分批从原始礼物事实重放；追平后原子切换。旧代际和去重记录不删除。重建期间新流水也进入影子代际；切换后的补漏继续处理迟提交来源。负分/金额异常失败关闭，保留错误与进度，绝不归零掩盖问题。Redis 不是必要依赖。

入口：[接口](api.md)、[发布](deployment.md)、[测试](testing.md)。
