# 前台客户端完整页面清单

## 1. 页面 ID 规则

- `C-Pxx`：正式业务/系统页面。
- `C-Sxx-yy`：页面内弹层、对话框、操作面板或独立状态面。
- `C-Dxx`：调试/演示路由，不生成正式产品设计稿。
- 平台：H5、PC 为当前同一响应式 Web 应用；Android/iOS 表示后续设计适配，不表示已有原生代码。

## 2. 路由页面

| ID | 模块/页面 | 路由 | 平台 | 进入条件/角色 | 布局与展示 | 业务操作/跳转 | 数据依赖 | 关键状态/设计要求 | 源码 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| C-P00 | 应用主壳 | `/` | H5/PC；App待确认 | 全部 | 全屏动态 viewport；主 Tab 页面采用内容区+底部三 Tab | 首页/榜单/我的；前后方向动效 | Router、system Store | safe area、键盘/地址栏高度、reduced motion | `src/App.vue`、`layouts/app-main.vue`、`module_system/pages/home/home.vue` |
| C-P01 | 登录/注册 | `/login?mode=` | 全平台 | 仅游客；已登录跳安全 redirect | H5 单列表单；PC 左品牌视觉+右表单；登录/注册双 Tab | 登录后返回原页；协议外链 | user auth APIs | required/expired/verifyFailed；表单校验、短信倒计时、提交忙态、注册成功自动登录失败 | `module_system/pages/login/index.vue` |
| C-P02 | 直播发现首页 | `/home` | 全平台 | 游客可见；关注/看过需登录 | 顶部主分类+热门子分类；直播卡片网格/列表；底部主 Tab | 切换关注/热门/新秀/广场/看过；卡片进直播间或资料 | live room/category、social online/history | 首载、下拉刷新、分页、空、错误、图片降级；私有 Tab 登录引导 | `module_live/pages/live/live_home.vue` |
| C-P03 | 主播热度榜 | `/rank` | 全平台 | 全部 | 日/周/月 Tab、名次/头像/分类/热度列表、底部主 Tab | 切周期 | 当前本地静态数据 | 必须显示“数据能力待接入”的设计注记；不可做实时数据承诺 | `module_live/pages/live/live_rank.vue` |
| C-P04 | 个人中心 | `/user` | 全平台 | 登录用户 | 资料卡、钱包/礼物/收益/主页入口、社交/直播统计、创作者中心、账号信息、退出 | 进入 C-P09/12/13/14/15/10/11/08；退出确认 | user/social/wallet/earning | 子模块独立加载/失败；普通/审核中/驳回/主播差异；账号状态标签 | `module_live/pages/user/main_user.vue` |
| C-P05 | 直播间 | `/live/room/:roomNo` | 全平台 | 游客可看公开房；互动需登录 | 四层结构：状态、背景、播放器、操作；顶栏主播/观众；底部功能栏 | 关注、收藏、点赞、拉黑、送礼、退出；主播资料/礼物历史/钱包 | room detail、presence、social、gift、wallet、HLS | 房间/播放器全状态、音频同意、presence降级、自己房间禁送礼、场次结束 | `module_live/pages/live/live_room.vue` |
| C-P06 | 主播公开主页 | `/anchor/:anchorNo` | 全平台 | 游客可见；互动需登录 | 相册、身份卡、直播入口、贡献榜、守护、分享 | 关注/回关、共同关注、拉黑、进直播、管理本人主页 | anchor-home + social | 加载/不可访问/空相册/分区失败；登录返回原页；拉黑后隐藏 | `module_anchor_home/pages/anchor-home.vue` |
| C-P07 | 用户资料 | `/user/profile/:kind(account\|anchor)/:publicNo` | 全平台 | account资料要求登录；anchor旧地址由守卫重定向C-P06 | 头像昵称签名、关注粉丝、关系操作、直播入口、账号字段 | 关注/共同关注/拉黑/进直播 | public-profile、social | 自己不显示关系操作；加载/失败；被拉黑/不存在 | `module_social/pages/social_profile.vue`、`module_live/routes.js` |
| C-P08 | 社交与内容 | `/user/social/:section?` | 全平台 | 登录用户 | 顶部横向 Tab；用户/历史卡片；游标分页 | 关注/取消/回关/拉黑；历史进直播/删除/清空；点卡进资料 | social APIs | following/followers/mutual/common/history/blacklist 变体；空/错误/分页；清空/拉黑确认 | `module_social/pages/social_center.vue` |
| C-P09 | 账号设置 | `/user/set` | 全平台 | 登录用户 | 账号概览；个人资料表单；修改密码；账号认证只读信息 | 保存资料、改密 | user profile/password | 字段级错误、保存/改密忙态、token轮换失败；手机号/实名不可改 | `module_live/pages/user/user_set.vue` |
| C-P10 | 主播申请 | `/user/to-anchor` | 全平台 | 登录用户 | 宣传头；身份加载/错误；申请状态时间线或申请表；分类底部选择 | 提交、重新申请、取消编辑、进开播页、刷新 | anchor info/apply/category | 无记录、审核中、通过、驳回、身份失败；驳回原因与进度 | `module_live/pages/user/user_to_anchor.vue` |
| C-P11 | 主播开播台 | `/user/live` | 全平台 | 登录且 `applyStatus=2`；否则重定向C-P10 | 资格卡、直播间设置、当前场次、推流凭证、OBS指南、最近直播 | 保存/恢复房间、生成/恢复Key、刷新Push URL、结束、复制凭证 | live check/room/session/category | 资格拒绝/封禁截止、无房间/关闭、等待推流/直播/结束、凭证丢失、幂等恢复、10s轮询 | `module_live/pages/user/user_live.vue` |
| C-P12 | 我的钱包 | `/user/wallet` | 全平台 | 登录用户 | COIN余额卡、冻结、版本/更新时间、流水列表 | 刷新、下拉、重试、加载更多 | wallet balance/ledger | 余额和流水独立失败；停用/账务异常/503冷却；无流水 | `module_wallet/pages/wallet_home.vue` |
| C-P13 | 礼物消费记录 | `/user/gifts` | 全平台 | 登录用户 | 待确认警示；状态筛选；订单卡片；分页 | 查询/安全恢复、复制订单号、刷新/更多 | gift result/order、wallet refresh | 全部/成功/部分退款/全退；图片降级；结果未知与退避 | `module_gift/pages/gift_history.vue` |
| C-P14 | 主播收益 | `/user/earning` | 全平台 | 登录且已审核主播入口；路由本身仅auth | 余额、汇总、结算、待确认提示、收款管理、提现筛选/列表 | 申请提现、恢复/重试、收款管理、详情、刷新/更多 | earning/payee/realtime bridge | 多币种、debt/frozen/paid、能力禁用、所有提现终态、HTTP轮询 | `module_earning/pages/earning_home.vue` |
| C-P15 | 主播主页管理 | `/user/anchor-home/manage` | 全平台 | 登录且已审核主播 | 相册网格与审核标签；公开介绍；观众预览 | 上传、排序、设首图、删除、重试、保存介绍、私有预览 | anchor-home album/settings | 8MB/20张、pending/approved/rejected/removed、拒绝原因、CAS冲突、结果未知 | `module_anchor_home/pages/anchor-home-manage.vue` |
| C-P16 | 通用播放器演示 | `/play` | 全平台 | 公开 | URL输入、播放画面、控制、填充模式、信息与事件日志 | 加载URL、播放/暂停、切模式、刷新/清日志 | 本地HLS | 开发工具，不纳入正式IA；若保留需受控入口 | `module_player/pages/play/index.vue` |
| C-P17 | 电影占位 | `/movies`、`/moives` | 全平台 | 登录用户 | 仅文本 `mo` | 无 | 无 | 非生产页面；不得生成电影业务 | `module_player/pages/moives/index.vue` |
| C-P18 | 关于占位 | `/about` | 全平台 | 全部 | 仅文本 `about` | 无 | 无 | 正式内容、许可证、版本、客服入口待确认 | `module_system/pages/about/index.vue` |
| C-P19 | 404 | 任意未知地址 | 全平台 | 全部 | 标题、说明、返回首页 | 返回C-P02 | i18n | 明确路由不存在，不暴露技术详情 | `module_system/pages/not-found/index.vue` |

## 3. 弹层、对话框和操作面

| ID | 所属页 | 名称/进入条件 | 内容与操作 | 状态要求 |
| --- | --- | --- | --- | --- |
| C-S01-01 | C-P01 | 协议校验 | 用户协议、隐私政策外链；未勾选阻止提交 | 行内错误+Toast；外链新窗口；正式URL待确认 |
| C-S04-01 | C-P04 | 退出确认 | 退出后需重新登录；取消/退出 | 退出按钮危险色；提交中禁重复 |
| C-S05-01 | C-P05 | 礼物面板 | 分类、礼物网格、余额、数量/预设、总价、赠送、历史、钱包 | 目录加载/空/错；选中；余额不足；请求中；待确认恢复；自送禁用 |
| C-S05-02 | C-P05 | 礼物庆祝动画 | 静态图、MP4/Lottie；资源失败回退 | 仅本人成功反馈；不能写“全房已广播” |
| C-S05-03 | C-P05 | 主播拉黑确认 | 拉黑解除关注并隐藏主页 | 确认/取消/失败；账号切换取消旧操作 |
| C-S05-04 | C-P05 | 未登录引导 | 关注/收藏/点赞/送礼触发 | 跳 C-P01 并携安全 redirect |
| C-S05-05 | C-P05 | 音频授权 | 继续静音/同意并开启声音 | 浏览器拒绝时保留静音播放和可重试 |
| C-S05-06 | C-P05 | 播放/房间状态面 | 进入中、不可播放、手动播放、缓冲、失败、结束 | 操作层不能被加载层遮挡；错误有重试/退出 |
| C-S06-01 | C-P06 | 相册全屏预览 | 左右滑/键盘、计数、关闭 | 只加载相邻；图片失败可重试；焦点回收 |
| C-S06-02 | C-P06 | 分享面板 | 二维码、只读链接、复制、系统分享 | 生成/分享失败降级；关闭回焦 |
| C-S08-01 | C-P08 | 清空历史确认 | 不可恢复说明 | 取消/确认/成功空态/失败保留列表 |
| C-S09-01 | C-P09 | 日期选择（当前原生 date input） | 生日≤今天 | App设计需用原生日期选择器；H5键盘遮挡处理 |
| C-S10-01 | C-P10/P11 | 分类选择器 | 层级扁平列表、加载、确认/取消 | 底部弹层；空/错误；长列表搜索为待确认 |
| C-S11-01 | C-P11 | 结束直播确认 | 结束后推流地址失效 | 危险确认、提交中、成功/失败 |
| C-S11-02 | C-P11 | 凭证显示/复制 | 显示/隐藏敏感值、复制单项 | 默认遮罩；防截图能力不存在；离页清理说明 |
| C-S14-01 | C-P14 | 申请提现 | 币种、金额、全部、收款账户、余额、冻结说明 | 校验、无账户、确认框、提交中、成功说明、结果未知 |
| C-S14-02 | C-P14 | 提现详情 | 金额、状态、时间、脱敏账户、单号/请求号 | 加载、旧请求迟到丢弃、关闭、失败Toast |
| C-S14-03 | C-P14 | 收款账户管理 | 列表、默认、停用、新增三种方式 | 加载/空/错、敏感字段、确认、原参数重试、100上限 |
| C-S15-01 | C-P15 | 私有照片预览 | 原图、关闭 | 需登录 Blob；关闭/换账号释放 URL |
| C-S15-02 | C-P15 | 删除照片确认 | 审核/历史保留说明 | CAS冲突刷新；结果未知保留原操作 |

## 4. 调试/演示路由

| ID | 路由 | 用途 | 生产要求 |
| --- | --- | --- | --- |
| C-D01 | `/test` | Vant、Lottie、语言、主题测试 | 从正式导航和公网路由隔离 |
| C-D02 | `/test2` | 下拉刷新/列表测试 | 同上 |
| C-D03 | `/test_video` | HLS 播放器能力与日志 | 同上；避免任意URL成为安全暴露面 |
| C-D04 | `/test_video_swiper` | 100条短视频虚拟滚动测试 | 同上；不生成产品短视频设计 |

