# 可靠事件投递

后端model/service/api/router使用eventdelivery分组；initialize仅验证，cmd/backend-migrate显式增量迁移。internal/eventcapture连接gift、earning付款终态与presence的原事务通知。五张event_delivery表保存不可变安全负载、消费状态、尝试、重试审计及修复游标。

后台入口web/src/view/operations/event-delivery/index.vue，只有当前有效root访问。新业务事务内原子入队；历史补扫各来源独立回绕。Adapter默认nil，通知blocked；未来IM接入实现service/eventdelivery.Adapter，持久幂等键和有效Ack才能标记delivered。只有安全状态提示，私人payout按用户路由，不把财务数据广播房间。

使用数据库队列和有限并发，不需要单机引入MQ。租约fencing、退避、最多8次自动尝试/10次管理员代次，不承诺网络exactly-once。永久事实不自动删除，容量增长与归档需另行验收；旧来源补扫恢复时间取决于环扫周期。完整契约及本地验证见docs/backend-hardening/event-delivery.md。
