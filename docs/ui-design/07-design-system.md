# UI 设计系统规范

> 本规范分「源码现状 Token」与「重设计目标规范」。目标规范允许优化视觉，但不得改变业务语义、状态机和权限边界。

## 1. 品牌与设计方向

- 产品气质：可信赖的直播内容社区 + 克制的资金工具。直播区沉浸、有活力；钱包/收益区信息准确、低干扰；主播工作台专业、可操作。
- 当前临时品牌：`KPlayer`，主色为 Vant 蓝。正式品牌名、Logo、中文名、品牌字体和版权归属均为「待确认」。AI 稿使用可替换的品牌占位符，不制作不可逆新Logo。
- 视觉关键词：深色沉浸直播、清晰层级、轻质感卡片、蓝紫强调、危险操作克制、金融数据高可读。
- 避免：过度霓虹、玻璃层叠影响对比度、用动画暗示未确认交易成功、用绿色把“审核通过待出款”画成到账。

## 2. 颜色体系

### 2.1 现有源码 Token（必须兼容）

| 语义 | Light | Dark |
| --- | --- | --- |
| Primary | `#1989FA` | `#3F9CFF` |
| Primary hover | `#3F9CFF` | `#69B1FF` |
| Primary active | `#0570DC` | `#1989FA` |
| Page | `#F5F6F8` | `#0B0C0F` |
| Container | `#FFFFFF` | `#15171C` |
| Elevated | `#FFFFFF` | `#1C1E24` |
| Text primary | `#18191C` | `#F5F5F5` |
| Text secondary | `#61666D` | `#A1A5AD` |
| Text tertiary | `#9499A0` | `#6F737B` |
| Border | `#E5E7EB` | `#292C33` |
| Success | `#16A34A` | `#22C55E` |
| Warning | `#F59E0B` | `#F59E0B` |
| Danger | `#EF4444` | `#F87171` |

来源：`src/assets/common_v1/css/theme-light.css`、`theme-dark.css`。

### 2.2 目标语义用法

- Primary：主操作、选中、链接；同屏最多一个主要实心 CTA。
- Success：已付款、直播中、审核通过的主播身份；不能用于 `approved/待出款`。
- Warning：审核中、待出款、结果待确认、开播准备、额度/限流。
- Danger：驳回、停用、封禁、账务异常、删除/拉黑/结束直播确认。
- Neutral：已取消、已结束、已停用的历史展示。
- Live accent：可用 Primary→紫色 `#8B5CF6` 渐变，仅作直播封面/头像背景，不承担状态含义。
- 文字与背景满足 WCAG AA；状态必须同时有文案或图标，不单靠颜色。

## 3. 字体层级

字体栈沿用系统字体：`-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, Helvetica, Arial, sans-serif`。中文不得使用不可授权的商业字体。

| Token | H5 | PC | 字重/行高 | 用途 |
| --- | --- | --- | --- | --- |
| Display | 28 | 36 | 700 / 1.2 | 登录品牌、直播工作台主标题 |
| H1 | 24 | 28 | 700 / 1.3 | 页面核心数字/标题 |
| H2 | 20 | 22 | 650 / 1.35 | 卡片分组 |
| H3 | 17 | 18 | 600 / 1.4 | 列表标题 |
| Body | 15 | 15 | 400 / 1.6 | 主正文 |
| Body small | 13 | 13 | 400 / 1.5 | 说明、元数据 |
| Caption | 11–12 | 12 | 500 / 1.4 | 英文眉题、标签 |
| Amount | 30 | 36 | 700 / 1.15，tabular-nums | 钱包/收益余额 |
| Code | 12–13 | 13 | 500 / 1.5，monospace | 推流凭证、业务单号 |

数字大于容器时允许换行/缩放或横向滚动，禁止省略余额；业务单号可中部省略但详情需完整可复制。

## 4. 间距、尺寸、圆角、阴影

- 4px 基础网格；间距 Token：4/8/12/16/20/24/32/40/48/64。
- H5 页面左右 12–16；PC 内容区左右24–32，最大宽度按页面 900/960/1180。
- 触控目标最小44×44；PC紧凑按钮最小32高但键盘焦点清晰。
- 圆角：输入/小按钮8；普通按钮12；卡片14–16；主卡/弹层18–24；胶囊999。
- 阴影沿用：sm `0 1px 3px rgb(0 0 0/8%)`、md `0 4px 16px ...`、lg `0 8px 32px ...`；Dark 提高不透明度。普通列表优先边框，避免每卡重阴影。
- 边框1px；焦点环2px Primary + 2px外间距；危险焦点环用Danger。

## 5. 布局系统

- Breakpoint：源码真实切换点 767px；建议设计补充 `320–767` H5、`768–1199` tablet/small PC、`>=1200` PC。
- 主壳：全屏 `VisualViewport` 高度；禁止 body 滚动；业务内容区自己滚动。
- H5：单列、底部主Tab、底部弹层、safe-area padding。
- PC：居中内容；发现页多列卡片；个人中心卡片网格；开播台双列；登录左右分屏；底部主Tab可保留为居中胶囊（源码现状），若改侧栏属于信息架构变更，需确认。
- 横向滚动 Tab 显示边缘渐隐；长列表使用分页/游标，不无限累积 DOM。

## 6. 核心组件规范与 Variants

### 6.1 Button

Variants：Primary、Secondary、Text、Danger、Danger-outline、Icon、Pill。每种含 default/hover/pressed/focus/disabled/loading。Loading不改变按钮尺寸；禁用对比仍可读；危险实心只在确认对话框最终动作。

### 6.2 Field

Variants：文本、密码、手机号、验证码组合、金额、Textarea、Select、Radio、Date、File。状态：empty/filled/focus/error/disabled/read-only/loading。Label H5可顶置或固定82px，但长文案不截断；错误保留一行高度减少跳动。密码提供显隐且默认隐藏；金额使用 `inputmode=decimal`，显示单位而不写进输入值。

### 6.3 Navigation

- Navbar：54px，左右槽最小72，中间单行省略；H5可选顶部safe area；返回/右操作44px点击区。
- 主Tabbar：三等分，54px+底部safe area；active为Primary soft+2px指示；支持键盘 role=tab。
- 内容Tabs：分段/横滑两型；active文字+背景/下划线双提示；切换后保留焦点。
- 面包屑：只用于PC深层页的可选增强，源码无组件，标「待确认」；不可替代返回。

### 6.4 Card/List/Table

- LiveCard：16:9或接近现有封面、状态左上、观看量右上/底部、作者和标题叠层；broken image有品牌渐变。
- ProfileCard：头像、主文案、关系/状态；按钮与整卡点击分离。
- FinanceCard：大数字、币种、冻结/已付/debt、更新时间；金额对齐，状态不只颜色。
- OrderCard：对象+状态+金额+时间+业务号；退款显示总额/已退/净额。
- PC仍以卡片/列表为主；当前用户端没有密集业务表格。若设计桌面表格，必须保持H5卡片等价字段和状态。

### 6.5 Status Tag

Variants：info/success/warning/danger/neutral/disabled/live。文字示例：直播中、审核中、待出款、已付款、已取消、已停用、结果待确认。Tag最小高度22，图标12，文本12。

### 6.6 Modal/Popup/Drawer

- H5：底部 Popup 圆角20–24，最大高度85dvh，内部滚动+底部safe area；危险确认使用系统风格Dialog。
- PC：普通表单居中对话框宽480–640；详情/复杂管理可右抽屉560–720；焦点陷阱、Esc、遮罩关闭规则明确。
- 状态：open/confirming/error/disabled；提交失败留在弹层；敏感表单关闭前若有未确认请求需明确提示或按源码保留策略。

### 6.7 Toast/Notice

- Toast：短成功/轻错误，2–3秒；屏幕阅读器 `role=status`。
- Inline Notice：持久错误、结果未知、账务/资格状态；含标题、解释、requestId（若安全）、操作。
- Network Banner：离线/恢复；不要遮挡Navbar与safe area。

### 6.8 Loading/Empty/Error

- 首屏用骨架或现有Lottie；局部按钮Spinner；列表加载更多保留数据。
- 空状态含业务原因和真实可用下一步；不虚构充值/购买等动作。
- 错误态含安全文案、重试；资金写入把「结果未知」做成warning而非danger。
- 图片：头像首字、礼物🎁、封面品牌渐变；重试按钮只用于可重试受信资源。

### 6.9 Media Player

Variants：idle/loading/ready/manual-play/playing/buffering/paused/ended/error/unsupported；全屏；静音/音频授权；封面兜底。控制层在触控后短暂显示；`prefers-reduced-motion` 降低动效。直播不可拖动，点播进度控件仅C-P16演示。

### 6.10 SensitiveCredential

用于推流Token、Push URL、Stream Key、收款账号输入：默认遮罩；显示/隐藏；复制成功Toast；长内容等宽字体；PC不在hover中自动暴露；App截图防护是否启用待原生确认。

## 7. 图标与素材

- 现状：Vant icon、内联SVG、emoji、`hero.png`、Lottie loading、公共 `icons.svg`。
- 目标：统一24px线性图标，1.75–2px描边；状态图标实心/线性规范一致；禁止混用emoji作为正式主图标（礼物失败占位可保留🎁）。
- 头像/封面/礼物图片：`object-fit:cover`，加载懒加载；业务文本按纯文本渲染；远程资源仅允许受信HTTP(S)，礼物生产要求HTTPS。
- Logo与品牌插图需在品牌确认后替换；不要继续沿用第三方示例图片或协议链接。

## 8. 动效与无障碍

- 页面主Tab 190–220ms淡入水平位移；普通颜色180–200ms；弹层200–280ms；成功动画不超过3s且可跳过。
- `prefers-reduced-motion: reduce` 时取消位移、自动循环和非必要庆祝动画。
- 所有按钮、Tab、输入有可见焦点；语义 `nav/main/section/dialog/status/alert`；图片有业务alt或装饰空alt。
- 对话框打开后焦点移入，关闭回触发按钮；全屏相册支持左右键/Esc；键盘顺序符合视觉顺序。
- H5支持200%字体不遮挡关键操作；至少320px宽可用。

## 9. 主题规则

- 默认主题来自环境配置，无效回Dark；无用户选择时跟随系统；手动选择后持久化。
- 设计稿必须同时给Light/Dark的关键页面：登录、首页、直播间、钱包、收益、开播台、弹层、错误/空状态。
- 直播间操作层始终深色半透明，不随Light变成浅底，以保证画面上可读。
- 原生App若接入系统主题，启动到首屏不得闪烁错误主题。

## 10. Design Tokens 交付格式

AI设计平台应输出变量：`color/*`（light/dark modes）、`type/*`、`space/*`、`radius/*`、`shadow/*`、`size/touch-min`、`breakpoint/*`、`motion/*`；组件必须用变量绑定，不在单页硬编码。所有组件建立 size×tone×state×platform variants，并提供开发标注。

