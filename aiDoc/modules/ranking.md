# ranking 模块

完整说明见 docs/ranking。独立只读投影，禁止修改 wallet/gift/earning/payout 财务事实。来源为 gift_revenue_ledger 和礼物订单不可变快照。

财富按用户净礼物消费，魅力按主播原始净礼物价值；不引用充值、余额或分成。礼物榜 anchor/room/session；财富 platform；魅力 platform/category（礼物分类）；day/week/month/all，Asia/Kuala_Lumpur，退款归原送礼周期。

RevenueNo 永久去重，代际收据防重复并补迟提交的较小源 ID。checkpoint 行锁串行批次，批量读订单/桶，原子追加事件/收据/桶/进度。新代际重建保留旧榜，完成后切换。不依赖 Redis。所有聚合金额/排名/数量为字符串，固定宽度排序键避免精度损失。

APP JWT、公共 DTO、私密房间访问限制、分页/时间/范围白名单。管理员独立精确权限和请求幂等审计；没有直接改分入口。cmd/ranking-migrate 提供只读预检、备份迁移、回填与核验。模块启动与旧结构兼容发布策略见 docs/guard/deployment.md。
