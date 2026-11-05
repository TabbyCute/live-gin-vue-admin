# 榜单发布

共用受控迁移流程见[守护与榜单发布](../guard/deployment.md)。榜单迁移入口为 `server/cmd/ranking-migrate`，默认只读，写入需先完成本地测试与私有备份。来源只读，禁止把回填脚本当成调整财务事实的工具。

监控 ranking_checkpoint 的 lastError/dataAsOf、来源缺失收据数量 lagCount、重建任务状态/processedCount/errorCount；不能把自增 ID 中的合法空洞当成遗漏，也不能只用最大 ID 判断追平。

排序索引为 idx_ranking_board（代际、类型、周期、范围、礼物、补零分值倒序、公开编号升序）。批次批量读订单和旧桶，批量写入更新及去重收据。消费异常回滚全部桶/收据/checkpoint，错误脱敏持久化并退避；业务送礼不等待榜单 Worker。

重建在新代际写入，旧代际保留；追平后切换，历史去重事实不删。同步核验读取原始 gift_revenue_ledger + gift_order，与所有当前代际桶逐项比较，还检查来源数量和积压；当前交互核验上限 10000 事件。高于该容量需先扩展异步/分段核验，不可以返回虚假的 consistent=true。

生产执行记录见[部署结果](../guard/deployment-result-20261104.json)，测试见[验证记录](testing.md)。
