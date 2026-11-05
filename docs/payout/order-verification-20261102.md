# 待出款订单交付与验证（2026-11-02）

## 实际完成范围

复用既有收款绑定、H5提现选账和审核重新核验，完成后台“提交打款中心”确认、唯一ready订单、独立一级“打款中心 / 出款订单”、筛选和只读详情。新增四表保护、专属加密快照、永久请求登记、初始历史/安全审计、后台提现关联投影和追踪日志。没有实际付款、Mock成功、paid、frozen→paid或历史单补选。

后端主要入口：`service/payout/order.go`、`order_crypto.go`、`service/earning/payout_order.go`、payout API/router/model、`initialize/payout*.go`和`cmd/payout-migrate`。后台：`view/payout/SubmitOrderDialog.vue`、`view/payout/order/index.vue`、现有提现FinancePage。Swagger三类文件已生成；aiDoc模块、跨栈契约与业务需求索引已更新。

H5目录`/Users/tabby/Desktop/IM/H5Player`现有账户管理、默认选账、无账户引导、固定原四参数恢复及防重逻辑继续使用，本阶段完整复验；没有伪造H5已接入真实付款。

## 执行证据

[结构化验证结果](order-validation-20261102.json)、[测试输出摘录](order-validation-excerpt-20261102.txt)。完整脚本`PAYOUT_H5_DIR=/Users/tabby/Desktop/IM/H5Player bash server/tests/payout/run.sh`退出0；临时本机MySQL三库均由脚本清理，补充测试的临时库也已清理。远程业务库没有用于压测或故障注入。

| 验证 | 结果 |
| --- | --- |
| 全仓Go测试 | `go test ./... -count=1`通过，含SQLite和真实MySQL |
| Go竞态 | payout/earning API、router、service及middleware通过；最终增量另跑race |
| 并发 | 同单32并发（相同和不同请求键）只得一个主订单；同键不同提现一成功一冲突；停用与创建竞争不绕过校验 |
| 幂等 | 原键原参数返回原单；同单换键返回原单并永久登记；二次键不能再绑定其他单；停用后原单可恢复 |
| 资金守恒 | 创建、重复请求、并发及失败后frozen/paid/提现/审核事实不变；不新写earning对账 |
| 前置核验 | pending_review/rejected、对账异常、占位账户、版本不符、冻结流水/余额异常拒绝；既有APP本人归属/状态/币种及审核测试继续通过 |
| 原子回滚 | order/history/audit/request每个写入点注入失败，四表均无半笔；查询数据库失败正确报暂不可用；取消context/缺加密依赖不落单 |
| 加密 | 绑定与订单不同密文/AAD；改金额、来源、目标、核验快照、复制绑定密文、缺历史key均拒绝；随机源故障报错，不降级明文 |
| 数据库保护 | SQLite/MySQL下有效processing状态/正金额改写也被不可变触发器拒绝；直接INSERT processing被ready触发器拒绝；订单删除、历史/审计/请求登记修改删除拒绝 |
| 越权/暴露 | APP与后台JWT隔离、已停用/缺失管理员、角色冒用、普通用户无权限；没有成功/失败/派发/attempt/取消/删除/解密路由 |
| 输入边界 | 未知/重复JSON键、金额/账户/版本/状态伪造、POST query、错误类型、非法页码/时间/状态、超大body/query/header拒绝；no-store、限流/预算保护 |
| 敏感信息 | DTO/审计/日志不含账号原文、真实姓名、密文、nonce、key ID或fingerprint；列表SELECT只取安全字段 |
| 列表/详情 | 精确组合筛选和时间半开区间、分页、提现关联投影、当前状态核验、历史事实保持不变 |
| 菜单/迁移 | SQLite/MySQL重复迁移、顶级菜单父子关系、888三个权限；root ID1不依赖角色；普通角色未自动授权；额外校验列类型/NULL/默认值/主键及CHECK精确表达式，防止只凭同名约束跳过必要迁移 |
| Fuzz | PayeeJSON、OrderJSON、PayeeValidation、EarningJSON各10秒通过 |
| 静态检查/构建 | `go vet ./...`、`go build ./...`通过；迁移增量另做vet/build |
| 管理后台 | 40项通过：收益/结算/新订单19项，礼物/钱包/root21项；生产构建通过 |
| H5 | 49项通过，生产构建通过 |

组件测试执行实际Vue编译后的setup逻辑，覆盖固定请求、防双击、丢响应原参数重试、错误固定文案、账号切换、列表及详情旧响应丢弃；不等于目标浏览器人工验收。Node24的VM测试按现有环境以`--jitless`运行相关组件套件，HTTP套件保留WebAssembly。构建仍有现有大chunk提示。

## 有限负载

真实MySQL、500条收益事实、16并发、64次提现：P50 856ms / P95 875ms / 最大894ms，资金核验全部通过。随后同规模创建64笔ready订单：耗时3863ms，P50 958ms / P95 966ms / 最大995ms，64笔均唯一、余额完全不变。

这是本机有限数据集与并发量，不能用来承诺生产TPS或大历史数据延迟。同步财务核验20000事实保护上限、大量主播、持续压测、多实例、网络分区和故障切换尚未在目标环境验收。

## 远程数据库

使用既有生产配置连接live_prod，首次结构迁移及菜单权限同步成功。新增四表均0行，原有收款表仍0行；没有创建收款资料或业务出款单。菜单核验：2个菜单、顶级父菜单、888有2个菜单/3个操作权限、3个API。

迁移前后五张财务表SHA256内容摘要一致：提现1条、可提现账户1条、账户流水2条、结算1条、收益事实7条；礼物6条、钱包账户1条计数不变。重复迁移及最终只读验证均成功；最终核验于2026-11-02 22:48:24（Asia/Kuala_Lumpur；UTC 14:48:24）完成，含列定义、主键和CHECK精确表达式检查。原始记录由[远程执行记录](order-remote-migration-20261102.json)保存，无需用户再次手工迁移当前远程库。

曾发生一次重复AutoMigrate超过CLI默认5分钟维护预算，命令以context deadline退出，没有写入业务事实。之后增加已完整验证结构的无DDL快捷返回及1—30分钟可配置维护预算，并通过SQLite/MySQL重入回归；远程以15分钟维护预算重跑。HTTP请求预算仍为8秒。MySQL DDL非整体事务，不能将超时当作自动回滚，最终必须核验已落库结构及摘要。

## 依赖扫描与发布边界

订单实现重新执行govulncheck，0个当前可达漏洞；另报告7个导入包及2个依赖模块中未调用的漏洞，见[原始扫描](order-go-vulnerability-20261102.txt)。本阶段没有改变前端依赖，沿用同日绑定阶段锁文件扫描：后台生产4高10中，全量1严重20高30中7低；H5生产0，全量4高3中。原始证据见[绑定阶段依赖扫描](verification-20261102.md#依赖安全检查结果与未关闭项)。不能称全部依赖无漏洞或整个平台达到生产安全验收。

未发布/重启线上应用，未注入真实密钥，未执行真实付款或在远程提交订单。数据库完成不意味着运行进程采用新代码；发布需完成密钥注入、同步后端/前端、重新登录菜单、目标浏览器验收、网关/APM脱敏与限流、HA、备份/密钥恢复演练及既有依赖整改。功能契约见[前端对接](order-frontend-integration.md)，发布及后续扩展见[运维文档](order-deployment.md)。
