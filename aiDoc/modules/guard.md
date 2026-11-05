# guard 模块

完整说明见 docs/guard。Router/API/Service/Model 各自分层，根 enter.go 注册，initialize 组合 Provider 和 Worker。cmd/guard-migrate 离线备份迁移/验证。

guard 只管理权益，不直接改钱包、不反查 gift。gift 通过 model/guard 的 GrantInput/CompensationInput 传入不可变来源与方案快照；source_type 当前为 gift_benefit / gift_refund，未来可扩展 wallet_purchase 而无需修改会员/来源/补偿核心表。

同方案续期、不同等级独立保留。退款只撤回对应来源未使用时间。membership 及 grant 的状态/补偿缓存可从原始事实重放；原始来源与补偿不可改删。ledger/audit 只追加。正常操作发现投影与不可变流水不一致失败关闭，重建为独立受控审计动作。

APP 复用统一公开用户 DTO，无内部用户 ID 或财务字段。后台精确角色/方法/路径授权，root ID 1 沿用例外；写操作永久 requestId，不能手工改到期时间。售卖开关默认 false；表核验、方案、Provider、三类 Worker 和一致性就绪后方可开启。
