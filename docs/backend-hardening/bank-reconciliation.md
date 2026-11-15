# 外部银行账单核对与本地支付模拟

## 信任边界

银行CSV是经授权管理员导入的**外部声明**，不是已认证的银行回调，也不是收款人银行到账证明。导入、运行核对、复核核对记录均不调用 `FinancialFinalize`，不修改提现/出款状态，不写钱包余额/收益账户/资金流水，不创建支付凭证。现有人工出款、私有回单、普通管理员双人复核和ID=1既有root例外保持原规则。

核对记录中的 `matched` 只表示两边保存的声明在指定字段上吻合。核对复核的 `acknowledge` 不是付款批准；差异只能 `investigate`，实际付款/未知结果处理仍回现有出款订单工作流，并提供既有规则要求的独立调查和新凭证。

## 输入及存储

接口为 `/v1/admin/payout/bank-reconciliation/{import,run,review,list,detail}`，项目路由前缀按部署配置添加。POST使用严格JSON：禁止未知/大小写别名/重复字段、null、尾随JSON、query参数；GET使用字段白名单且拒绝重复query值。统一 `{code,data,msg}` 响应；金额和用户ID输出字符串，时间为毫秒。

导入正文：

```json
{
  "requestId": "fixed-operation-key",
  "channelAccountNo": "pca_<32 hex>",
  "periodFrom": 1794441600000,
  "periodTo": 1794528000000,
  "declaredComplete": false,
  "csv": "bank_reference,company_account,recipient_account,currency,amount_minor,status,booked_at,direction\n..."
}
```

- CSV为UTF-8，固定八列，最多512KiB/1000条数据行；HTTP JSON上限1MiB（容纳转义）；最多2个同时在途请求；读取正文期限5秒、请求总预算15秒。
- 账期 `[periodFrom,periodTo)`，最长93天；按绝对毫秒比较，CSV时间必须为带时区RFC3339且位于账期。UTC与等价时区时间形成相同事实。
- `amount_minor` 为无前导零的正整数最小单位，不接受浮点、负数、溢出或COIN。`direction` 固定 `debit`。状态只接受 `settled/pending/failed/reversed`。银行导出格式须先在受控环境按银行定义转换，不能任意猜测金额单位或状态含义。
- `company_account` 必须与所选公司账户的加密完整账号精确一致；历史停用公司账户可用于核对历史记录。`recipient_account` 必须为完整银行卡号，服务端仅保存HMAC。仅有尾号/掩码的账单不能证明精确收款去向，当前导入明确拒绝。
- 流水规范化与人工出款保持一致：拒绝控制字符，去空白并转大写，使用既有公司账户+流水HMAC，最后4字符可显示。禁止凭金额猜测对应订单；同额不同流水是未匹配记录。
- CSV全字段均有明确语法校验，公式开头的流水、账号、金额及内嵌换行不能成为可接受事实。没有CSV原文下载或通用导出入口。
- 不保存CSV原文、完整公司/收款账号、完整银行流水或文件路径；导入批次摘要、单行内容摘要和收款账号摘要均采用既有fingerprint key派生HMAC。核对时重新验证行HMAC和批次集合HMAC。禁用请求体/SQL参数敏感日志，复用`FinancialSafeLogger`。

同公司+规范化银行流水永久唯一，内容包括金额、币种、收款去向、银行状态、记账时间。重复完全相同记录可出现在多个批次；同流水内容变化整批回滚并报49302，包括后续状态更正/撤销。当前不覆盖旧声明、不自动接受银行更正版本；需要保留原始银行材料、调查并通过既有人工出款流程处理。银行另发独立撤销流水可以导入为独立 `reversed` 声明，但不会被当成原付款退款或余额冲正。

批次摘要使用排序后的行HMAC集合和账户/账期/完整性声明；行顺序或等价时区变化不产生新业务事实。同一requestId永久绑定操作人+动作+原始参数摘要，换参数或换操作人返回冲突，即使目标批次相同。成功重试仍实时重新鉴权。所有批次、行、关联、核对运行、检查项、复核、请求凭据及审计均有数据库禁止UPDATE/DELETE的触发器。

## 双向核对与处理

按公司账户和流水精确查找attempt，再验证订单加密快照，检查金额、币种、收款人、银行状态和平台状态。核对在RR事务快照中完成，不持资金锁；最多读取1000导入行和1000个账期成功attempt。导入及复核用RC事务，导入串行锁定公司账户；记录和审计同事务提交，唯一键竞争失败时回滚后重试。

| result | 含义与操作 |
| --- | --- |
| `matched` | 金额、币种、收款账号、流水一致；银行settled且平台succeeded，可独立acknowledge |
| `unmatched_bank` | 银行记录无同公司同流水attempt；包括可能重复付款、手工遗漏、无关款项；只能调查 |
| `amount_mismatch` / `currency_mismatch` / `recipient_mismatch` | 对应核心字段不同，不能放行或修正账务 |
| `bank_pending` / `bank_failed` / `bank_reversed` | 银行声明状态不是settled；不能据此覆盖平台事实 |
| `platform_not_succeeded` | 银行settled但平台仍待确认/未知/失败等；导入不使其变paid |
| `missing_in_statement` | 完整性被声明为true，平台在账期payment_time内成功但该批次未见记录；仍只是差异 |
| `not_seen_partial` | 部分账单中未见平台成功记录，不代表漏款 |

银行记账时间与平台付款时间可能跨日，收费合并、付款拆分、批量付款和银行撤销可能有各自的编号规则。当前不自动净额抵销、不聚合同额交易；应扩大或更正账期调查，不能因缺行重付/取消。完整性是管理员声明，系统无法独立证明银行文件完整。

导入人、核对运行人及出款操作人不能复核相关检查项；ID=1现有root例外允许同人，但`rootException=true`单独留痕。复核锁定检查项和最新attempt行，要求版本未变；陈旧报告必须重新运行核对。每条检查项只能接受一次独立复核记录，参数固定重试可返回原结果。调查原因按现有风格脱敏长数字、邮箱和URL。列表和详情只返回脱敏字段，详情只读取最新核对运行，最多2000检查项，历史运行保存在库中。

## 权限和管理员入口

沿用后台JWT、启用用户/当前角色实时校验及 `anchor_finance_permission` 精确路径/方法；APP JWT不能访问。精确权限为 `/payout/bank-reconciliation/import|run|review` POST及`list|detail` GET。迁移注册菜单和API元数据，**不给普通角色隐式授予金融权限**。通过既有受控授权流程赋予所需权限及菜单后，在“打款中心 / 银行账单核对”上传标准CSV、运行核对、查看差异并独立处理。列表可见不代表可操作。

页面只把待提交CSV保存在当前内存，成功、退出、身份变化或离开页面即清空；失败保留原请求键和参数供重试。没有localStorage/sessionStorage明文保存。未知结果先查批次，再决定重新导入。

## 迁移及回滚

专属CLI `server/cmd/bank-reconcile-migrate`：

```sh
# 默认只验证；DSN由本地安全环境注入，不写入输出。
go run ./cmd/bank-reconcile-migrate
# 或明确读取选定配置，不能与DSN环境变量并用。
go run ./cmd/bank-reconcile-migrate -config /private/config.yaml -migrate -sync-metadata
```

环境变量为 `BANK_RECONCILE_MIGRATION_DSN`。`-migrate` 只新增八张 `payout_bank_*` 表、16个禁止改删触发器以及 `payout_attempt(channel_account_no,status,payment_time)` 查询索引。现有表事实不改写，索引兼容旧应用。MySQL索引使用`ALGORITHM=INPLACE,LOCK=NONE`，DDL元数据锁等待10秒；失败显式返回，不能静默降级为阻塞建表。复用财务迁移命名锁，DDL可重入；未知同名触发器/不兼容字段或约束拒绝启动。启动仅验证，不隐式DDL。MySQL校验包含InnoDB、二进制排序、字段类型/空值性、主键/唯一索引、CHECK定义、触发器准确内容及范围查询索引。

**CLI本身不包含全库备份、资金前后摘要或自动恢复。** 生产维护须由总迁移工作流先完成私有备份、资金事实摘要，执行后验证摘要一致、权限和接口可用。局部失败按报错修复后重跑CLI，禁止删除已有核对证据来“恢复”。回滚应用不要求删除新增表/索引；无资金变更可逆向冲账。任何清表/降级脚本均未提供。

## 本地渠道模拟和资金测试

`server/internal/testpay` 是本地测试边界的可替换Channel接口（Submit/Query/Verify）和内存HMAC签名模拟器。构造器强制显式`tcp(127.0.0.1:port)`和`bank_test_`/`earning_test_`数据库名前缀；生产初始化、路由和控制台均不引用它。不是支付宝/微信/银行实际SDK，没有生产假充值/假付款入口。真实渠道接入仍需商户配置、渠道签名/查单规范、持久化inbox、真实银行证据和独立上线评审。

模拟事件只在验证签名、固定订单身份/金额/币种之后进入测试sink。sink复用现有钱包人工单和独立管理员审核，使用永久外部订单号/请求键；不绕过人工双审，不直接写余额。pending或乱序旧pending不入账；超时丢响应后查原单，不重建单号；重复成功通知与查询补偿由同一现有钱包幂等约束确保只记一次账。

本地MySQL测试文件 `server/service/earning/local_channel_bank_test.go` 只复用原安全fixture，地址限定127.0.0.1:13328且库名前缀earning_test_；测试会重建该指定隔离库，不能给生产配置。使用独立 `earning_test_bankflow`，不共享其他并行套件的库。

覆盖：新注册/登录→主播申请/审核→签名模拟充值确认、签名错误/重复/乱序/丢响应/查单补偿→送礼及幂等部分退款→收益结算→本人提现→私有模拟回单及人工独立双审→paid只一次→外部双向银行核对。额外验证非root自审、root留痕、并发8次导入、跨actor同键冲突、即时撤销权限、陈旧报告、错公司/收款人/金额/币种/状态、公式/多行/重复流水、账期结束边界和等价时区、1000/1001行限额、脱敏及不可变/漂移/重入迁移。无外部银行网络调用；SRS独立容器的实际开播验收由总链路报告覆盖，本测试使用隔离发布事实。

测试命令（DSN仅本地安全环境注入）：

```sh
go test -race ./service/earning -run '^TestMySQL(LocalChannelToBankReconciliation|BankDiscrepancyMatrix)$' -count=1 -v
go test -race ./api/v1/bankreconcile ./router/bankreconcile ./internal/testpay -count=1
go test ./api/v1/bankreconcile -run '^$' -fuzz FuzzBankStrictJSON -fuzztime=5s -parallel=2
```

实测数值、最终测试结果由本次交付记录附上；小规模本地数据的响应时间不代表生产吞吐承诺。

本次本地记录见 `docs/backend-hardening/evidence/bank-reconciliation-local-tests.log`：MySQL全链与差异矩阵race通过（25.56s）；1000行导入186.5ms/核对101.1ms；API/router/simulator race、真实TCP慢体和并发拒绝、10136次短fuzz均通过。MySQL在单条attempt小样本上选择旧唯一索引，新范围索引在候选集合中；不把该小样本计划或耗时当作生产负载保证。并发导入、运行核对及复核均额外覆盖8路同requestId，等待锁后的重放重新鉴权并解析已提交receipt。
