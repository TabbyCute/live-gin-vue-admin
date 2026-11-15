# Go 工具链安全补丁（2026-11-12）

当前构建工具链从Go1.26.10升级至Go1.26.11，最低语言版本仍为1.26.0。该版本于2026-11-10发布；官方漏洞数据库把GO-2026-6859（HTTP/1 MaxBytesReader响应头竞态导致崩溃）的1.26分支修复点列为1.26.11。[Go官方发布历史](https://go.dev/doc/devel/release#go1.26.11)、[官方漏洞记录](https://vuln.go.dev/ID/GO-2026-6859.json)。

同步范围：`server/go.mod` toolchain、`server/Dockerfile` builder、根`Makefile`构建镜像、`.github/workflows/ci.yaml`三处Go版本、wallet/social漏洞扫描脚本、`aiDoc/relations/development-workflow.md`当前工具链基线。历史报告及H5相关文件未修改。必须用新工具链重建应用与迁移二进制；修改版本声明或重启旧二进制不会修复旧二进制携带的标准库。

实测`GOTOOLCHAIN=go1.26.11 go version`输出`go version go1.26.11 darwin/amd64`。使用相同工具链重建govulncheck v1.1.4并扫描`./...`，新报告不再命中GO-2026-6859，仅保留GO-2026-6911；另外6项导入包和2项模块报告没有符号可达路径。报告完整保存为`evidence/govulncheck-go1.26.11.log`，原扫描文件保留。扫描器因剩余可达漏洞返回状态3，**没有被标为全绿或加入忽略列表**。

GO-2026-6911的官方记录截至本次复核仍为所有版本受影响、无已知修复版本。现有`mcp/client`禁跟随HTTP重定向，并在RoundTripper中验证Location派生请求仍为原scheme/host，以防凭证发送到异源；Go1.26.11下`TestMCPRejectsRedirectAndLocationCredentialExfiltration` race回归通过（7.715s）。该措施限制已知重定向路径，不等于第三方库已有完整上游修复，也不代替管理员对初始MCP目标地址的信任判断。[官方GO-2026-6911](https://pkg.go.dev/vuln/GO-2026-6911)。

管理员依赖补丁、剩余风险和审计证据见[dependencies.md](dependencies.md)。最终应用构建、全套回归及部署由整体交付流程执行；本补丁工作未访问生产或SSH。
