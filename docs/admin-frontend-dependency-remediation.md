# 管理后台前端依赖安全整改清单

更新时间：2026-10-30  
当前状态：待后续专项处理  
适用范围：`web/` Vue 管理后台

## 1. 范围说明

本清单记录 `pnpm audit --prod` 对管理后台生产依赖的扫描结果。当前共有11条告警：4条高危、7条中危，实际涉及7个包和5条主要依赖链。PostCSS 7.0.39重复命中5条不同公告，因此“11条告警”不等于“11个不同组件”。

这些告警不属于钱包Go服务、MySQL账务表或APP/H5钱包接口。钱包管理页面没有直接使用富文本编辑器、动态SFC加载器或ECharts，但它们与钱包页面处于同一个管理后台应用、同一个管理员登录环境，仍应作为管理后台整体上线项处理。

Axios已从1.8.2升级到1.21.0并退出本清单。结构化扫描原始记录见[frontend-dependency-audit-20261030.json](wallet/frontend-dependency-audit-20261030.json)。

## 2. 十一条告警明细

| 序号 | 严重度 | 包及当前版本 | 风险 | 实际依赖路径 | 项目使用位置 | 建议处置 |
| ---: | --- | --- | --- | --- | --- | --- |
| 1 | 高 | `wangeditor 4.7.15` | 链接处理可能产生XSS | `@form-create/designer → @form-create/component-wangeditor → wangeditor` | 系统工具/表单设计器 | 升级可移除旧wangeditor的表单设计器版本，或删除/替换该富文本组件；该旧包没有可直接升级的安全版本 |
| 2 | 高 | `postcss 7.0.39` | 恶意`sourceMappingURL`可能读取本地文件 | `vue3-sfc-loader → @vue/component-compiler-utils → postcss` | 自动代码生成的动态组件预览 | 优先移除或替换`vue3-sfc-loader`旧链路，不能只强制覆盖PostCSS大版本 |
| 3 | 高 | `postcss 7.0.39` | Previous Source Map路径穿越及文件泄露 | 同上 | 同上 | 与第2项一起处理 |
| 4 | 高 | `source-map-js 1.2.1` | 特制Source Map可导致事件循环阻塞、DoS | `@unocss/transformer-directives → css-tree → source-map-js` | UnoCSS构建链 | 将解析链升级到使用`source-map-js >= 1.2.2`的版本，并验证生产构建 |
| 5 | 中 | `postcss 7.0.39` | 换行符解析异常 | `vue3-sfc-loader`旧链路 | 自动代码生成的动态组件预览 | 与第2项一起处理 |
| 6 | 中 | `postcss 7.0.39` | CSS输出未正确转义`</style>`，可能产生XSS | 同上 | 同上 | 与第2项一起处理 |
| 7 | 中 | `postcss 7.0.39` | Source Map文件读取问题修复不完整 | 同上 | 同上 | 与第2项一起处理 |
| 8 | 中 | `vue-template-compiler 2.7.16` | 动态模板编译可能产生客户端XSS | `vue3-sfc-loader → vue-template-compiler` | 自动代码生成的动态组件预览 | 移除旧Vue 2编译器链，改用与当前Vue 3一致且受维护的预览方案 |
| 9 | 中 | `postcss-selector-parser 6.1.4` | 特制选择器可能造成二次复杂度CPU消耗 | `vue3-sfc-loader → @vue/component-compiler-utils` | 自动代码生成的动态组件预览 | 升级到`>= 6.1.5`；最好随整个旧SFC链一起移除 |
| 10 | 中 | `echarts 5.5.1` | Lines系列Tooltip渲染存在XSS风险 | 管理后台直接依赖，`vue-echarts`共享 | 首页/仪表盘图表 | 升级到`echarts >= 6.1.0`并回归所有图表配置、Tooltip和主题 |
| 11 | 中 | `qs 6.16.0` | 特制数组序列化参数可能触发异常、造成DoS | 管理后台直接依赖 | 当前`src/`未发现实际导入 | 确认无运行时调用后删除；若需保留则升级到`>= 6.16.1` |

补充说明：项目的公告富文本使用`@wangeditor/editor 5.1.23`，第1项告警针对的是表单设计器间接携带的旧包`wangeditor 4.7.15`，两者不是同一个npm包，整改时不要误删公告编辑器。

## 3. 按依赖链归并后的工作量

| 整改项 | 可关闭的告警 | 优先级 | 主要风险 |
| --- | ---: | --- | --- |
| 替换`vue3-sfc-loader`旧动态编译链 | 7条：2高、5中 | P0 | 自动代码预览功能可能需要调整实现 |
| 清除表单设计器携带的旧`wangeditor` | 1条高危 | P0 | 需确认表单设计器中的富文本字段兼容性 |
| 升级`source-map-js`所在构建链 | 1条高危 | P0 | 需要完整生产构建验证 |
| ECharts升级到6.1.0或更高 | 1条中危 | P1 | 大版本升级，需要回归图表API和Tooltip |
| 删除或升级`qs` | 1条中危 | P1 | 当前疑似未使用，预计改动较小 |

## 4. 推荐处理顺序

1. 先处理`qs`和`source-map-js`，它们范围较小，可快速减少两条告警。
2. 专项替换`vue3-sfc-loader`。这是最大收益项，一次关闭7条告警；重点回归`web/src/view/systemTools/autoCode/picture.vue`。
3. 处理`@form-create/designer`携带的旧`wangeditor`，重点回归表单设计器的保存、加载、预览和富文本字段。
4. 升级ECharts，逐一检查仪表盘图表、Tooltip、主题、缩放和窗口尺寸变化。
5. 重新生成锁文件、执行完整审计和构建，确认没有因强制覆盖产生多个冲突版本。

不建议直接使用不受控的`pnpm audit --fix --force`或只在锁文件里强制覆盖PostCSS 7到8。`vue3-sfc-loader`依赖的是旧编译器接口，盲目跨大版本覆盖可能让动态组件预览在运行时失效。

## 5. 验收清单

依赖整改完成后执行：

```bash
cd web
pnpm install --frozen-lockfile
pnpm audit --prod
pnpm run test:wallet
pnpm run build
```

还需人工回归：

- 登录、退出、令牌过期和无权限访问。
- 系统工具中的表单设计器：新建、编辑、保存、重新加载、预览以及富文本字段。
- 自动代码生成页面：动态预览、生成失败提示、恶意模板或CSS输入的拒绝行为。
- 仪表盘全部图表：数据刷新、Tooltip、空数据、超长文本和窗口缩放。
- 公告富文本：编辑、回显、只读展示和危险HTML清理。
- 钱包介绍、钱包账户、余额流水、人工充值单和交易溯源页面。

最终验收标准：

- `pnpm audit --prod`不再出现本清单11条公告；新增公告必须逐项评估。
- `pnpm-lock.yaml`提交并在CI中使用冻结安装。
- 管理后台构建、钱包测试和上述人工回归全部通过。
- 对确实无法立即升级的依赖，必须记录可达性分析、临时隔离措施、负责人和完成日期，不能仅以“钱包页面未使用”为由关闭风险。

## 6. 当前相关文件

- 直接依赖声明：`web/package.json`
- 冻结版本：`web/pnpm-lock.yaml`
- 表单设计器：`web/src/view/systemTools/formCreate/index.vue`
- 动态SFC预览：`web/src/view/systemTools/autoCode/picture.vue`
- 图表封装：`web/src/components/charts/index.vue`
- 公告富文本：`web/src/plugin/announcement/form/info.vue`
- 钱包封版报告：`docs/wallet/production-review-20261030.md`

