# MCP 独立开发辅助服务

MCP 入口是 `server/cmd/mcp`；工具、到业务后台的 HTTP 调用和托管进程管理位于 `server/mcp`，客户端封装位于 `server/mcp/client`。主业务进程与仓库 Compose 不默认启动 MCP，也不默认映射其 8889 端口。当前初始化没有注册 `InitAutoCodeRouter`；不能把已有 MCP 管理 API 定义视为主服务已开放的路由。

独立进程和托管进程统一使用 `ResolveMCPListenAddr`，`mcp.listen-host` 未填、空值或空白值默认 `127.0.0.1`，端口默认 8889。显式设置 `0.0.0.0` 可监听所有 IPv4 接口，`::` 可启用 IPv6 通配监听；IPv6 地址使用不带方括号的配置值，由 `net.JoinHostPort` 组装监听地址。`mcp.base_url` 是客户端连接地址，与监听地址分开配置，使用 IPv6 时应配置例如 `http://[::1]:8889/mcp`。

```yaml
mcp:
  listen-host: 127.0.0.1
  addr: 8889
  base_url: http://127.0.0.1:8889/mcp
  upstream_base_url: http://127.0.0.1:8888
  auth_header: x-token
  request_timeout: 15
```

独立 MCP 没有全局协议鉴权闸门，初始化、工具枚举和部分纯文本辅助工具可匿名调用。涉及后台数据或操作的工具从请求上下文取 token，再交由业务后台验证。回环监听只是缩小网络暴露面；明确启用外部访问时，仍需网关鉴权、网络访问控制和后台权限边界，不能将这个开发辅助服务直接当成公开业务 API。

出站 HTTP 的三条路径（MCP transport、工具到后台、健康检查）统一使用绑定初始 scheme/host/port 的 HTTP client，并拒绝自动重定向，避免服务返回恶意 Location 后向其他目的地发送请求或凭证。正常同源 JSON-RPC 与业务接口请求保留。初始目标地址仍由受信配置决定，此机制不提供 DNS/IP 固定或完整 SSRF 地址白名单；本项目当前没有启用 MCP OAuth，未来接入其发现/令牌客户端时必须另行检查。

依赖告警 [GO-2026-6911](https://pkg.go.dev/vuln/GO-2026-6911) 当前没有已修复版本，静态扫描仍会报告。官方条目描述 Location SSRF/凭证泄漏，但其引用的 [Go 漏洞库 issue](https://github.com/golang/vulndb/issues/6911) 与 [上游 issue](https://github.com/mark3labs/mcp-go/issues/1164) 描述 JSON-RPC 重复键、大小写/Unicode 折叠导致的解析差异，来源内容不一致。本地验证覆盖 Location 威胁；没有声称修复上游 JSON 解析差异。将来若在 MCP 前置网关增加基于原始 JSON 字段的工具白名单，必须使网关和服务采用一致且拒绝重复/非规范字段的规则，或采用经确认的上游修复。

本地安全回归：`env -u MCP_TEST_URL go test -race ./mcp/... ./cmd/mcp -count=1 -v`。真实 `httptest` 回放覆盖 301/302/303/307/308、跨主机、同主机不同端口、HTTPS 降级、请求/通知/关闭、正常 RPC、后台 token 和健康检查；外部目标的请求与凭证计数均须为零。监听测试覆盖缺省、显式外部值和 IPv6。测试不连接真实外部 MCP。
