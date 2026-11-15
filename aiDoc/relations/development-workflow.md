# 开发工作流

## GVA Helper / MCP 约束

如果当前环境可用 GVA Helper 或其他项目专用 MCP 工具，开发前应优先使用它获取项目级建议、约束和示例，再落地具体实现。

## Go构建与安全基线

2026-11-12安全复扫后，后端工具链固定Go1.26.11，模块最低语言版本1.26；CI、Makefile、Docker builder及当前扫描脚本已同步，修复GO-2026-6859（net/http HTTP/1 MaxBytesReader响应头竞态）。升级依赖或发布前须以当前go.mod/go.sum重新构建，不能仅重启旧二进制。漏洞复扫须用同一工具链构建扫描器，例如`GOTOOLCHAIN=go1.26.11 go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...`，并区分符号可达结果与仅存在于依赖中的报告。当前依据见docs/backend-hardening/go-security-20261112.md；2026-10-30历史报告保留原始工具链与证据。

## 推荐开发顺序

1. 先分析需求与接口
2. 先设计后端模型和请求结构
3. 再实现 Service 层业务逻辑
4. 再实现 API 层与 Router 层
5. 最后补齐 `initialize/`、插件入口或前端接入
6. 完成后进行联调与验证

## 前后端协作顺序

- 后端优先给出稳定接口
- 前端可基于 Mock 或 Swagger 并行开发
- 联调时以后端真实接口契约为准

## 分支策略

- `main`: 生产分支
- `develop`: 开发分支
- `feature/*`: 功能分支
- `hotfix/*`: 热修复分支

## 提交规范

建议使用语义化提交：

- `feat`
- `fix`
- `docs`
- `style`
- `refactor`
- `test`
- `chore`

推荐格式：`type(scope): description`

## 单机后端整体验证（2026-11-12）

`bash server/tests/backend/run.sh`固定使用localhost Docker MySQL/Redis/SRS，覆盖全量Go、核心race、负载、fuzz、vet/build和admin测试/构建，不运行H5。金融fixture会清空专用测试库，禁止改为生产DSN。`server/cmd/backend-migrate`是本轮唯一总迁移入口：默认只读；显式写入先私有全库备份，再只迁移新增三模块并验证旧资金/权限。当前结果与未关闭边界见`docs/backend-hardening/README.md`。
