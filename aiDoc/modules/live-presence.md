# Live Presence

- 职责：签名游客/APP会员进房、心跳、退出、去重在线、受限点赞、服务端时长、持久检查点与内部事件；没有 WebSocket/弹幕。
- 后端：server/router/live/live_presence.go → api/v1/live/live_presence.go → service/live/live_presence*.go、live_room_event.go → model/live/live_presence.go；初始化 initialize/live_presence.go，独立迁移 cmd/presence-migrate。
- 前端：web 的直播间/场次列表与 presence/PresencePanel；实际观看端 /Users/tabby/Desktop/IM/H5Player，web_live 不重复接入。
- APP路径 /api/v1/app/live/presence/*、/room/like；后台 /api/v1/admin/live/presence/stats，仅固定root或当前有效888角色。后台不能手动改统计。游客header X-Presence-Guest；CORS保留Origin白名单并支持此header、Retry-After。
- memberCount=onlineCount（已登录去重），guestCount按签名游客身份去重，totalOnlineCount为两者之和；新数量为十进制字符串，未知在线用空串+stale，不伪造0。
- Redis当前事实，MySQL访问/检查点/最终统计。默认15秒心跳/45秒TTL/30秒权限复核；多设备按保守最大有效时间线计时。Redis键有TTL与容量界限，同场hash tag；全局admission为单slot热点。
- presence独占观众统计、最终更新主播观众累计；SRS/live生命周期仍管开播/结束、时长；gift/wallet/earning/payout管资金唯一事实。旧hook/session/stats已退役HTTP410。
- social/history接口保留；新版直播页取消旧报告器，presence内部只更新最近观看，不追加第二套时长/sequence。已删历史不会被旧检查点重建。
- Worker有界租约与数据库epoch/revision fencing；SQL提交后CAS确认待办，队列满先排空。Redis全丢失必须受控恢复，保留dataLost和恢复审计原因，不能删真实键绕过检查。
- 2026-11-06远程live_prod已增量迁移、备份并证明资金摘要不变；应用待发布。完整契约/测试/运维见 docs/live-presence/README.md。
