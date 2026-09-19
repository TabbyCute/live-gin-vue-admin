# 正式与测试环境配置分离

## 基本信息

- 提出日期：2026-09-19
- 当前状态：`done`
- 需求类型：环境配置 / 启动流程 / 文档
- 优先级：高
- 需求文件：`aiDoc/memory/business/done/environment-config-separation.md`

## 用户原始意图摘要

将正式环境和测试环境的配置、数据库与启动流程完全分开，并提供从首次初始化到日常启动的完整文档。

## 影响范围

- 后端：分离正式/测试 YAML 配置和 Makefile 启动目标
- 前端：复用 Vite development/production 模式，文档化启动和构建方式
- 文档：新增 `README-CUSTOM.md` 完整操作手册
- 插件 / 模块：无业务逻辑修改

## 涉及对象

- 模块：后端启动、Viper 配置、前端构建
- 接口：`/init/checkdb`、`/init/initdb`、`/health`
- 页面：数据库初始化页、登录页
- 配置：`server/config.prod.yaml`、`server/config.dev.yaml`、`web/.env.development`、`web/.env.production`

## 已确认约束

- 正式配置文件为 `server/config.prod.yaml`，MySQL 数据库为 `live_pro`。
- 测试配置文件为 `server/config.dev.yaml`，MySQL 数据库为 `live_dev`。
- 启动命令必须通过 `-c` 明确选择配置文件。
- 正式与测试环境使用不同的 Redis DB、日志目录和上传目录。
- 两份 YAML 使用完全相同的字段、顺序和注释，只保留环境配置值差异。
- 正式环境禁用启动时 AutoMigrate，测试环境允许 AutoMigrate。
- 首次初始化必须先使 `db-name` 为空，再通过初始化页插入基础数据。

## 当前进展

- 已将正式环境配置重命名为 `config.prod.yaml`，对应 `live_pro`。
- 已将测试环境配置重命名为 `config.dev.yaml`，对应 `live_dev`。
- 已将 Gin `release` 模式映射到正式配置，将 `debug` / `test` 模式映射到测试配置。
- 已统一 `config.dev.yaml` 与 `config.prod.yaml` 的完整字段、顺序和注释，并补齐当前配置结构体中的缺失字段。
- 已分离 Redis DB、日志目录和本地上传目录。
- 已在 Makefile 新增测试/正式后端启动和前端启动/构建目标。
- 已在 `README-CUSTOM.md` 记录完整初始化、启动、构建、验证、重置和排错流程。

## 后续待办

- 正式部署时将真实密钥迁移到仓库外配置或 Secret 管理系统。
- 如需同机同时运行两套环境，再分离后端端口与前端代理端口。

## 更新规则

- 同一需求始终维护在同一个文件中。
- 新信息优先补充到对应段落，不要另起一份重复记录。
- 只有需求状态变化时，才在 `active/` 与 `done/` 之间移动文件。
