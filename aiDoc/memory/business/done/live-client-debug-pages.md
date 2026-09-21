# 客户端账号与主播调试页

## 基本信息

- 提出日期：2026-09-21
- 当前状态：`done`
- 需求类型：客户端接口调试工具
- 需求文件：`aiDoc/memory/business/done/live-client-debug-pages.md`

## 用户原始意图摘要

在 `test/live` 下完成两个无依赖、极简、方便调试的 HTML 页面：账号页用于注册和登录，主播页用于从申请开始调试当前全部主播客户端操作。

## 完成结果

- `html-1注册账号.html` 支持注册、登录、Token 保存/清空和当前账号查询。
- `html-2申请主播.html` 支持申请/重申、申请状态、当前主播资料、资料更新、公开详情、开播资格和 PK 资格检查。
- 主播调试页只使用对外 `anchorNo` 查询公开详情，不读取或展示 `LiveAnchor` 数据库主键。
- 两页仅使用原生 HTML/CSS/JavaScript 和 `fetch`，通过 `localStorage` 共用 API 基址与客户端 Bearer Token。
- 请求结果直接显示 HTTP 状态和格式化 JSON，用于本地调试，不作为生产页面。
