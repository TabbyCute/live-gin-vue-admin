# Anchor Home

观众侧主播公开主页一期：公开资料/直播入口、相册、贡献榜和守护；配套主播投稿管理及后台审核。当前交付与数据库状态以 `docs/anchor-home/README.md` 为准。

- 服务边界：`server/service/anchorhome/PublicService` 聚合 live/social/ranking/guard 的公开事实；`AlbumService` 处理私有媒体、CAS、永久幂等和不可变审核记录。
- HTTP：`/v1/app/anchor-home/{profile,supporters,guards,photo}` 可选 APP Bearer；`settings`、`album/*` 必须 APP Bearer；`/v1/admin/anchor-home/album/*` 使用后台 x-token 并复查当前管理员角色。
- 新表：`anchor_home_album/photo/command/audit`。启动只验证；显式 `cmd/anchor-home-migrate` 在备份后创建缺表及指定元数据，拒绝隐式修改既存不兼容结构。
- 所有公开数据采用字段白名单；照片待审/驳回原因、内部 ID、资金/风控/联系方式和推流凭据不可进入公开 DTO。
- 主页读取以只读 RR 快照保持目标可见性与数据一致；媒体逐次验证/no-store；社交/直播授权不使用展示缓存。
- 索引排序规则：排名键为二进制；账号列可能为旧 CI 编码。`ranking.DiscoverAccountEncoding` 在启动检测实际列编码，仅转换查询外侧键，不转换索引列，并保留精确身份比较。
- 相册版本是安全整数 CAS；简介/分类版本是 SHA-256 内容 CAS，不能混用。业务 requestId 是幂等键；HTTP traceId 是排障标识。
- H5 源码位于独立仓库 `/Users/tabby/Desktop/IM/H5Player/src/modules/module_anchor_home`；通过 `deploy/build-h5.sh` 生成同源部署资产。普通账号资料和原有收益流程保留。
- 后续 IM 接入复用现有独立实时适配边界。当前不提供私信发送、守护直接购买或未实现的成长规则。

本地测试：`server/tests/anchorhome/compose.yaml` 只绑定 127.0.0.1:13339；各测试使用固定、不同的 `anchor_home_test_*` 数据库并在建立连接前验证目标。真实浏览器 harness 必须显式 opt-in，默认测试不会启动常驻服务。
