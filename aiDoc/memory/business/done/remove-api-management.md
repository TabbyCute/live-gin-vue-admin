# 删除独立 API 管理功能

## 状态

已完成。

## 需求

- 从管理后台删除“api管理”菜单和页面。
- 删除 API 管理相关的前端请求封装、后端 CRUD/同步接口及其 Router、API、Service 请求响应结构。
- 删除新库初始化中的 API 管理菜单、接口元数据、Casbin 策略和 API 忽略表初始化。
- 存量数据库启动时清理上述菜单、接口元数据、权限策略和 `sys_ignore_apis` 表。

## 保留边界

- 保留 `sys_apis` 表和 `SysApi` 模型，作为角色接口授权、插件、代码生成和版本数据的底层权限元数据，不再作为独立管理功能暴露。
- 角色管理和版本管理需要的只读接口清单迁移到 `POST /casbin/getAllApis`。
- 代码生成回滚和插件卸载仍可通过后端内部方法删除其产生的接口权限元数据。

## 影响范围

- 删除 `web/src/view/superAdmin/api/api.vue` 和 `web/src/api/api.js`。
- 删除 `/api/*` API 管理路由及其后端分层实现。
- 删除 MCP `create_api` 工具；API 权限元数据改由模块初始化或生成流程维护。
- Swagger、菜单初始化、接口初始化和 Casbin 初始化同步更新。
