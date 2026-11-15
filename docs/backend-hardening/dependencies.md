# 依赖安全复核（2026-11-12）

本轮处理当前Go标准库修复和admin依赖的兼容安全补丁。没有把漏洞扫描结果等同于可利用性证明，也没有将剩余告警隐藏为“全部通过”。H5及历史审计报告未读取或修改。

## Go

Go1.26.11已成为go.mod、Docker、Makefile、CI和当前扫描脚本的构建基线，修复GO-2026-6859；官方发布与复验记录见[go-security-20261112.md](go-security-20261112.md)。[Go官方发布历史](https://go.dev/doc/devel/release#go1.26.11)、[GO-2026-6859官方记录](https://vuln.go.dev/ID/GO-2026-6859.json)。

新govulncheck符号可达结果只剩1项：

| 依赖/风险 | 当前处理与剩余边界 |
| --- | --- |
| `github.com/mark3labs/mcp-go@v0.41.1` / GO-2026-6911：Location重定向SSRF/凭证泄露 | 官方仍无已知修复；现有HTTP client拒绝自动重定向，transport拒绝任何异源Location请求；race回归已通过。扫描器仍报告可达，不做忽略。初始MCP目标由受信管理员配置，未宣称防住所有SSRF类别。[官方报告](https://pkg.go.dev/vuln/GO-2026-6911) |

另有6项导入包报告和2项所需模块报告未发现被本应用调用的漏洞符号。它们仍保留在工具结果中，不能据此证明依赖无风险。上游最新发行页未给出此MCP告警的已修复版本，因此不为消除版本数字而跨主版本替换SDK。[上游发行页](https://github.com/mark3labs/mcp-go/releases)。

## Admin 已采用的安全补丁

仅对当前admin包和锁文件定向更新；传递依赖使用“旧版本→修复版本”的窄范围pnpm overrides，未强制替换不兼容的主版本。

| 依赖 | 本次版本 | 官方依据 |
| --- | --- | --- |
| qs | 6.16.0 → 6.16.1 | [字符串化DoS修复](https://github.com/advisories/GHSA-6pq8-jhm2-2vcw) |
| source-map-js | 1.2.1 → 1.2.2 | [索引source map事件循环DoS](https://github.com/advisories/GHSA-68fv-2mgg-jv7q) |
| postcss-selector-parser | 6.1.4 → 6.1.5 | [选择器二次复杂度修复](https://github.com/advisories/GHSA-rj75-hqrm-r3gf) |
| vite | 8.3.0 → 8.3.3 | [开发服务器文件边界修复](https://github.com/advisories/GHSA-vfpm-58rq-9qcg) |
| launch-editor | 2.14.1 → 2.14.2 | [文件存在性边界修复](https://github.com/advisories/GHSA-587h-7wmj-h5pm) |
| shell-quote | 1.10.0 → 1.11.0 | [命令引用注入修复](https://github.com/advisories/GHSA-pqg4-j6r4-53mv) |
| follow-redirects | 1.16.0 → 1.16.2 | [跨realm凭证头保护修复](https://github.com/advisories/GHSA-mrwp-f3q8-3g9p) |
| body-parser | 1.20.8 → 1.20.9 | [压缩正文尾随字节DoS修复](https://github.com/advisories/GHSA-27r3-jxmh-p828) |
| fast-uri | 3.1.8 → 3.1.9 | [URI scheme注入修复](https://github.com/advisories/GHSA-333x-w4r6-6crp) |
| joi | 17.13.8 → 17.13.9 | [邮件地址拆分DoS修复](https://github.com/advisories/GHSA-4528-x5fx-h6hg) |
| node-forge | 1.4.0 → 1.4.3 | [TLS状态异常修复](https://github.com/advisories/GHSA-j8rw-jp99-435v) |

Vite补丁带动其声明的Rolldown和对应平台绑定升级；这是同一构建工具依赖闭包变化，不是新增业务组件。锁文件以`pnpm install --frozen-lockfile --ignore-scripts`验证。qs的普通编码及公告中不可调用toString输入通过公共API回归。最终admin生产构建与现有组件测试由总验收流程复跑。

## 遗留风险边界

| 路径/风险族 | 本次未作跨主版本替换的原因 |
| --- | --- |
| form-create设计器 → wangeditor4 | XSS告警无修复版本，需要评估编辑器替换及内容信任边界，不能声称升级现有4.x即修复。[公告](https://github.com/advisories/GHSA-g7mw-5cq6-fv82) |
| vue3-sfc-loader / Vue CLI → Vue2模板编译器 | 已有Vue2编译器告警无受支持修复，需要迁移旧加载/编译链。[公告](https://github.com/advisories/GHSA-g3ch-rx76-35fx) |
| 同一旧编译链 → PostCSS7 | 修复版本位于PostCSS8；直接全局override可能破坏旧编译器，需单独迁移与功能验收。[公告](https://github.com/advisories/GHSA-fxqj-rqcc-2cmp) |
| echarts5 | 安全修复要求6.1+，涉及图表与vue-echarts兼容验证。[公告](https://github.com/advisories/GHSA-fgmj-fm8m-jvvx) |
| 设计器格式化链 → glob10 / minimatch9 | 修复要求glob14和minimatch11，保留为单独兼容迁移事项。[glob公告](https://github.com/advisories/GHSA-3g3m-2fjr-6pw4)、[minimatch公告](https://github.com/advisories/GHSA-g3hv-75wg-m249) |
| 旧Vue CLI开发链 → webpack-dev-server4、middleware5、serialize-javascript6、cross-spawn5、uuid8等 | 当前应用使用Vite，但旧CLI依赖仍安装，不能等同于无可利用性；修复涉及跨主版本或清理旧CLI链，应单独验证。生产环境不应启动这些开发服务器；这是部署边界而非移除漏洞。 |

## 最终扫描结果

- Go：标准库GO-2026-6859消失，保留MCP的1项符号可达告警；扫描真实返回状态3。
- Admin生产依赖：10项（3 high、7 moderate、0 critical），原生产快照为13项（4 high、9 moderate）。剩余集中于上表的旧编辑器/编译器/图表与格式化链。
- Admin全部依赖：22项（6 high、15 moderate、1 low、0 critical）；首批三个补丁之后保存的全依赖快照为62项（21 high、30 moderate、10 low、1 critical），第二批限定安全补丁消除了40项。node-forge、Vite和shell-quote已不在最终告警中。
- pnpm audit两种模式仍因遗留告警返回非零，未使用忽略列表或改写报告。

## 证据

- `evidence/GO-2026-6859.json`、`evidence/GO-2026-6911.json`：从官方vuln.go.dev获取的记录快照。
- `evidence/govulncheck-go1.26.11.log`：新Go工具链扫描，保留真实失败状态。
- `evidence/admin-dependency-audit-20261112.json`：本次admin生产依赖审计。
- `evidence/admin-all-dependency-audit-20261112.json`：包含开发依赖的最终完整审计；`evidence/admin-all-dependency-audit-before-safe-patches-20261112.json`保留第二批补丁前快照。

所有扫描反映执行时的漏洞数据库。admin audit是依赖存在性报告，不是浏览器/服务端符号可达分析；剩余高危、无fix或跨主版本项目需要独立跟进，不能包装为安全认证。
