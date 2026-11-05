# gift 礼物模块

- 路径：server/{model,service,api/v1,router}/gift，通过enter.go注册；前端web/src/api/gift.js及view/live/gift。
- gift_order、gift_refund_order、gift_revenue_ledger为礼物金额事实；SRS hook已禁止写入gift统计。live详情使用新giftSummary，列表批量聚合，财务摘要独立授权。
- 资金仅调用wallet.DebitInTx/RefundInTx，不修改钱包内核、不创建第二用户余额。资金与订单/收入/待结算收益/事件同事务，退款追加负向事实。
- MySQL8生产、SQLite开发。整数COIN和基点；HTTP大整数为字符串，APP不暴露内部ID；同用户+requestId永久幂等，退款key全局唯一。
- 配置与规则版本CAS、目录控制行、触发器和唯一/CHECK约束；锁顺序用户→订单→目录→主播→房间→场次→钱包。仅明确回滚的数据库死锁/锁超时重试整个事务。
- gift_admin_permission独立于已移除的Casbin；默认角色888，查询/配置/退款/规则创建/复核分离，live原有查询权不能绕过礼物汇总权限。
- 无GuardService时guard-enabled=true拒绝启动，权益配置只能disabled；发件箱pending不伪造IM成功。
- 迁移：initialize/gift*.go、cmd/gift-migrate，启动严格校验。保留已有未提交改动及钱包基线。
- 文档：docs/gift/README.md、api.md、accounting.md、deployment.md、testing.md。真实MySQL测试入口server/tests/gift/run.sh，前端npm run test:gift。
- 主播用户ID与发送者一致时禁止新送礼（48012）。APP送礼/查询/历史使用AppOrder显式字段白名单，只保留客服可用的orderNo/requestId；内部钱包与分成字段仅授权后台可见。
- 退款和详情复用逐笔检查：退款单→钱包交易→钱包流水→收入/收益冲减；总额相同但关联错配也拒绝。批量读取避免N+1。
- 规则创建/批准检查scope对象，历史规则引用阻止分类/礼物物理删除。anchor_rule_termination提供双人复核提前终止，原规则与订单快照不改。
- 2026-11-04 接入独立 guard：结构化 GuardService、同事务 BenefitGrant、事务外 ProcessBenefits 与 guard_task 租约恢复；guard 按来源撤回未使用时长。旧 ProcessBenefitCompensation 仅兼容回归，不再注册生产 Worker。生产开关仍关闭，发布状态见 docs/guard/deployment.md。
- ranking 独立消费收入事实并批量投影；不得阻断送礼或依据主播分成计算公开榜单。旧 /gift/ranking 保留 UTC 财务口径，新 APP 榜单明确 Asia/Kuala_Lumpur。
- 显式seed-catalog补齐3分类6草稿且不覆盖现有配置；catalog-only仅数据写入。config/inspect-catalog支持目标核对。MySQL启动结构核验使用6次批量查询。
