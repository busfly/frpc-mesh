# 移动端（手机）适配设计

**日期**：2026-11-04　**范围**：仅前端（`web/`），后端零改动　**技术路径**：方案一——继续用 Element Plus，自研少量移动端组件

## 0. 背景与现状证据

frpc-mesh 的 Web 面板按桌面设计，手机访问基本不可用。核实到的具体成因：

| 现状 | 位置 | 手机上的后果 |
|---|---|---|
| 侧栏固定 220px（折叠 64px），无移动端形态 | `web/src/layouts/MainLayout.vue:3` | 375px 视口里内容区只剩 155px |
| 全站只有 2 条 `@media`（1024 / 992） | `global.css:139`、`DashboardView.vue:230` | 窄屏无任何排版适配 |
| 三张 `el-table` 列合计最小宽 850–1040px，操作列 `fixed="right"` 130–240px | `ProxiesView.vue:85-198`、`InstancesView.vue:18-85`、`DashboardView.vue:39-98` | 必须横向滚动，操作列浮在中间挡住内容 |
| 弹窗 720/780/640/440px，抽屉 560px | `ProxyFormDialog.vue:5`、`ProxiesView.vue:218/228`、`SettingsView.vue:305`、`InstanceFormDrawer.vue:5` | 超出视口宽度，按钮和输入框被裁掉 |
| 表单大量 `el-col :span="12"` 双列、`label-width="110px"` | `ProxyFormDialog.vue`、`InstanceFormDrawer.vue` | 单个输入框被压到 ~150px，无法看清输入内容 |
| 说明文字普遍放在 hover tooltip 里 | `DashboardView.vue:12/86`、`InstancesView.vue:73`、`SettingsView.vue:35/58`、`InstanceFormDrawer.vue:94` | 触屏没有 hover，信息直接丢失 |
| 外层 `height: 100vh`，viewport 无 `viewport-fit` | `MainLayout.vue:168`、`index.html:5` | iOS 上底部被浏览器工具栏盖住；刘海区不适配 |

访问环境（用户确认）：手机走 **http**——家里 Wi-Fi 直连内网 IP，或外出时通过自家 frp 隧道访问；浏览器需覆盖 **iOS Safari、微信内置浏览器、Android Chrome**。因此没有安全上下文，不能依赖 service worker / PWA 安装 / `navigator.clipboard`。

## 1. 目标与非目标

**目标**

1. 手机上五个页面全部可用且**全功能**：包括新建/编辑 FRPS 服务器与代理（表单真的能填、能提交）。
2. 具备原生 App 的操作感：卡片列表替代宽表格、表单与行操作走底部弹层、左侧覆盖抽屉导航、安全区与键盘适配。
3. **桌面端零回归**：桌面形态的模板逻辑、信息密度、视觉一律不改；移动形态是新增分支而不是改写。

**非目标（本轮明确不做）**

- 底部 Tab 导航（用户已否决，导航用顶栏汉堡 + 左抽屉）
- 手势下拉刷新（自研手势易与原生滚动打架，收益可由「回前台自动刷新」替代）
- 列表项左滑行操作、二级页侧滑返回
- PWA / service worker / 离线缓存（http 环境装不了，iOS 也要求 https）
- 虚拟滚动（个人 NAS 量级：2 台服务器 / 7 个代理）
- Element Plus 按需引入等构建层体积优化（单独一轮）
- 平板专属布局（768–1023 只做「保留桌面表格 + 侧栏默认折叠」）

## 2. 断点与形态判定

单一来源 composable `useIsMobile()`：`window.matchMedia('(max-width: 767px)')`，模块级单例 + `change` 监听，返回响应式 `isMobile`。

- **只看视口宽度**，不看 UA、不看 `pointer: coarse`：窄窗口里的桌面浏览器应保持桌面形态（表格横向滚动本来就能用）。
- **≤767px**：移动形态（卡片列表 + 底部弹层 + 汉堡左抽屉 + 表单单列）。
- **768–1023px**：桌面形态不变，但 `collapsed` 初值由 `matchMedia('(max-width: 1023px)')` 给出 → 侧栏默认折叠成 64px。
- **≥1024px**：完全现状。
- 横屏 iPhone（844/932px）自动落回桌面形态，可接受。

**形态切换不丢状态**（这是本设计最容易做砸的地方，列为硬约束）：

1. 表单的编辑态 model 一律放在表单组件自身，且移动/桌面**共用同一个组件**，只换外层容器（dialog ↔ bottom sheet），不做两套表单。
2. 弹层关闭不销毁内容：`el-drawer` 不设 `destroy-on-close`。
3. 列表数据与状态计算全在 Pinia store 与 `utils/status.ts`（已有 `statusText` / `statusDotColor` / `proxyTypeLabel` / `proxyTypeColor`），卡片组件只接 props、只发事件，**不复制一份业务逻辑**。
4. 轮询与下载任务状态本就在 store 里，组件卸载不影响；不在页面级 `onUnmounted` 停全局轮询。

## 3. 外壳改造（`MainLayout`）

| 项 | 做法 |
|---|---|
| 侧栏 | `el-aside` 在移动端 `v-if="!isMobile"` 不渲染（不是隐藏，省 DOM） |
| 汉堡 | 顶栏左侧 44×44 按钮（仅移动端），点开左侧覆盖抽屉 |
| 抽屉菜单 | `el-drawer direction="ltr" :size="260"`，内部渲染 `SideMenu.vue` |
| 菜单复用 | 把现有 5 个 `el-menu-item` 抽成 `components/layout/SideMenu.vue`，侧栏与抽屉共用同一份（菜单项定义只有一处） |
| 顶栏右侧 | 移动端只留「发现新版本」标签（点击跳设置）+ 头像下拉；版本号标签与仓库图标收进头像下拉菜单项，避免顶栏挤成一团 |
| 视口高度 | `height: 100vh` → `height: 100vh; height: 100dvh`（先旧后新，老浏览器自动忽略第二条） |
| 安全区 | 顶栏 `padding-top: env(safe-area-inset-top)`；`index.html` 的 viewport 补 `viewport-fit=cover`（不补则 `env()` 恒为 0） |
| 滚动容器 | 全站唯一滚动容器仍是 `el-main.main-content`，移动端加 `-webkit-overflow-scrolling: touch`；弹层内滚动只允许一个内层容器 |

## 4. 逐页移动形态

通用卡片约定：**状态点必须配文字**（不只靠颜色）；**空字段整行不渲染**（不放占位横线）；备注类文本只在表单里出现，不外显；一行内的直达操作不超过 2 个，其余收进「⋯」。

### 4.1 登录页 `LoginView`

已是居中卡片，仅需：卡片宽度改为 `min(92vw, 420px)`、输入框高度 44px、`autocomplete` 保持、页脚链接换行不重叠。

### 4.2 仪表盘 `DashboardView`

- 统计卡：现有 `:xs="12"` 两列保持（数值 + 标签在 170px 宽内不截断）。
- 服务器表格 → 卡片三行：① 名称 + 状态点 + 状态文字 ② 服务器地址（`addr:port`）③「N 个代理 · M 在线」+ frpc 版本口径文字。
- 操作：卡片只留一个入口「详情」→ 跳 `/proxies?instance=<id>`，与桌面 `goDetail(row.id)` 同一跳转、同一文案，不另起「管理代理」这种第二叫法。启停/编辑等回到服务器页做，仪表盘不重复放。

### 4.3 FRPS 服务器 `InstancesView`

- 卡片四行：① 名称（点击 = 跳 `/proxies?instance=<id>`，与桌面名称链接同一 `goDetail`）+ 状态点 + 状态文字 ② `addr:port`（`mono`）③「代理 M/N 在线」+ frpc 版本（口径沿用桌面：`默认 v0.61.1` / `v0.71.1` / `v9.9.9 · 暂用 v0.61.1`，走 `frpcVersionLabel` / `frpcVersionFallback`；版本回落时橙色 + `FieldHint` 说明原因）④ 操作行：启用开关（旁带「已启用/已停用」文字，沿用 `onToggleEnabled`）+「⋯」。
- **实测修正**：开关与「⋯」原本排在第①行，390px 下名称只剩 5 个字符宽（`demo-ssh` 被省略号截断），故把它们下移到独立操作行，第①行只留标识与状态。
- 桌面靠 hover tooltip 才看得见的两件事在触屏必须有地方读：失败原因（error 态在卡片补一行 `runtime.last_error`，无错误不占行；`/api/dashboard` 的 `DashboardInstance` 没有该字段，仪表盘卡片因此不显示）、版本回落原因（`FieldHint`）。
- 「⋯」面板项 = 桌面已有的四个操作，不新增：**重启、管理面板（`manage_port` 未配置时禁用并说明原因）、编辑、删除**。
- 「新建 FRPS 服务器」按钮保留在页头，移动端页头改为上下排（标题一行、按钮整宽一行）。

### 4.4 穿透代理 `ProxiesView`（核心页）

- **服务器切换**：`el-tabs` → 吸顶横向滚动 chip 条，每个 chip = 状态点 + 服务器名 + `frpc 版本口径`；选中态用主色渐变底 + 白字 + 加粗（不只靠颜色）。chip 条 `overflow-x: auto` 且隐藏滚动条，**绑定现有的 `activeTab`**（`ProxiesView.vue:295`，已负责与 `?instance=` 查询参数同步，`ProxiesView.vue:396-400/413-423`），不新增第二份激活态。
- **快速添加**：桌面的 5 张 `TYPE_CARDS`（按类型快速添加，`ProxiesView.vue:29-50`）在移动端改成第二排横向滚动 chip（类型色点 + 类型名 + 副标题），保持「点一下就按该类型新建」的原语义，**不替换成单个通用「+ 创建」按钮**。
- **实测补充**：两排 chip 同款同形（服务器切换 / 快速添加）在 390px 下极易混为一谈，误点类型排会直接开一张新代理表单。故快速添加那排上方加一行 11px 灰字「快速添加（点类型直接新建代理）」，验收脚本断言 `.fm-chips-caption` 存在。
- **工具条**：沿用桌面三个入口——配置文件、运行日志、全部 Ping（前两个是次要文字按钮，后者是主按钮）。跨服务器拷贝从卡片「⋯」进入。
- **代理卡片四行**（实测后把开关与「⋯」从第 1 行下移，否则名称被截成 5 个字符）：
  1. 类型徽标（`proxyTypeLabel` + `proxyTypeColor`）+ 名称 + 状态（已禁用 / 在线 / 离线，文字必带）
  2. 远程端口 / 域名（主行）——**与桌面表格同一渲染逻辑**：tcp/udp 有 `remote_port` 给可点链接、无则「随机」；http/https 优先 `custom_domains`、再 `subdomain`；抽成共享组件 `ProxyRemoteCell.vue`，表格与卡片都用它
  3. 本地地址 / 访问摘要 + 延迟（同样抽成 `ProxyLocalCell.vue` 与共享延迟渲染；任一为空则该字段不渲染，两个都空则整行不渲染）
  4. 操作行：启用开关 +「⋯」（44px 热区）
- **「⋯」操作面板**：编辑、拷贝到其他服务器…、删除（红色，需二次确认）——就是桌面 `ProxiesView.vue:178-198` 的三个操作，**不新增**「重启代理 / 查看该代理日志」这类桌面没有的入口（代理级重启与单代理日志后端没有对应操作）。
- **配置文件 / 运行日志**：桌面是 `infoVisible` 弹窗（`ProxiesView.vue:215-225`），移动端复用同一组件 `ConfigTab` / `LogsTab`，只把外层容器换成底部弹层。日志区 `terminal` 已有 `word-break`，移动端字号 12px、内层独立滚动。

### 4.5 frpc 版本管理 `VersionsView`

- 版本卡是 `el-row`/`el-col`（`VersionsView.vue:54-63`，现为 `:xs="24" :sm="12" :md="8" :lg="6"`），移动端把 `:xs` 改成 12 走两列；窄到 359px 以下用媒体查询退回单列。卡片内边距由 `:body-style` 内联给出，按形态取值（CSS 覆盖不掉内联样式）。
- 「使用中」「已下载」用文字徽标，不只靠颜色；下载中任务的进度条放在卡片下方整行。
- 顶部数据源选择器与「检查更新」按钮保持一行，`flex-wrap` 已具备。

### 4.6 设置 `SettingsView`

- 现有 `el-col :xs="24"` 两列自动折成一列，卡片顺序不变（运行配置 → 检查新版本 → 备份与恢复 → 修改密码 → 关于）。
- 容器升级分步引导**仍默认收起**（与桌面一致，避免卡片高度暴涨）；引导内每条命令行 `word-break` 已有，复制按钮触摸目标放大到 44×44。
- 表单类（修改密码、导入备份）字段 `:xs="24"`、输入框 44px。

## 5. 新增组件与 composable

| 名称 | 位置 | 职责 | 依赖 |
|---|---|---|---|
| `useIsMobile()` / `useIsNarrowDesktop()` | `web/src/composables/useMediaQuery.ts` | 全站唯一形态判定源：`matchMedia` 模块级缓存，多个组件共用一个监听 | 无 |
| `useSwipeToClose(target, onClose)` | `web/src/composables/useSwipeToClose.ts` | 把手区 touch 事件：位移 >80px 或速度 >0.5px/ms 关闭，否则回弹；忽略内层滚动容器冒泡 | 无 |
| `ActionSheet.vue` | `web/src/components/mobile/ActionSheet.vue` | 「⋯」操作面板：`el-drawer direction="btt"` + 标题 + 操作项（44px 行高、可标红、可禁用带原因）+ 取消 | `useSwipeToClose` |
| `FieldHint.vue` | `web/src/components/mobile/FieldHint.vue` | 说明文字双形态：`(hover: hover)` 渲染现有 tooltip，触屏渲染可点「?」展开同一段文案 | 无 |
| `ProxyCardList.vue` | `web/src/components/mobile/ProxyCardList.vue` | 代理卡片列表：props `rows`（`ProxyCardRow`）+ `remoteHost`，事件 `toggle` / `more`（编辑·拷贝·删除走 `ActionSheet`，与桌面表格操作列同权） | `utils/status.ts`、四个共享 cell |
| `ProxyRemoteCell.vue` | `web/src/components/mobile/ProxyRemoteCell.vue` | 「远程端口 / 域名」渲染（tcp/udp 端口、随机、custom_domains、subdomain 四分支），**桌面表格与移动卡片共用** | 无 |
| `ProxyLocalCell.vue` | `web/src/components/mobile/ProxyLocalCell.vue` | 「本地地址 / 访问摘要」渲染（visitor 与本地端口分支），**桌面表格与移动卡片共用** | 无 |
| `ProxyStatusCell.vue` | `web/src/components/mobile/ProxyStatusCell.vue` | 「已禁用 / 在线 / 离线」标签，**桌面表格与移动卡片共用** | 无 |
| `ProxyLatencyCell.vue` | `web/src/components/mobile/ProxyLatencyCell.vue` | 延迟四态（`--` 未测 / `不通` / `可通` / `N ms`），**桌面表格与移动卡片共用** | 无 |
| `InstanceCardList.vue` | `web/src/components/mobile/InstanceCardList.vue` | 服务器卡片列表（服务器页与仪表盘共用）：props `rows`（`InstanceCardRow`）+ 事件 `detail` / `toggle` / `more`；error 态附一行失败原因、版本回落附 `FieldHint` | `utils/status.ts`、`utils/frpcVersion.ts`、`FieldHint` |
| `SideMenu.vue` | `web/src/components/layout/SideMenu.vue` | 5 项菜单定义唯一来源，侧栏与抽屉共用 | 无 |
| `mobile.css` | `web/src/styles/mobile.css` | 只放媒体查询与移动形态工具类：`.fm-sheet`（把 `el-dialog` 变成底部弹层）、chip 条、卡片列表、安全区、触摸热区 | 无 |

**容器不新建包装组件**（降低桌面回归面）：

- `ProxyFormDialog`（`el-dialog`）保持**同一份模板**，只在 `isMobile` 时把 `width` 换成 `100%` 并挂 `.fm-sheet` 类，用 CSS 把对话框改成贴底、圆角、内层滚动的底部弹层；头部加把手并接 `useSwipeToClose`。（实测坑：EP 有 `.el-drawer__header > :first-child { flex: 1 }`，把手作为 header 第一个子元素会被拉成一整条灰带、把标题挤到右边；用 `.fm-sheet .el-drawer__header .fm-handle` 以更高特异度压成 `flex: 0 0 100%` 独占一行，小灰条改由 `::after` 居中画，整行 18px 都是下滑抓取区。）
- `InstanceFormDrawer`（`el-drawer`）EP 原生支持 `direction`，`isMobile` 时改 `direction="btt"` + `size="92%"`，其余不动。
- 配置文件 / 运行日志弹窗（`ProxiesView` 的 `infoVisible`）与拷贝弹窗同理走 `.fm-sheet`。

约束：卡片组件**不含任何 API 调用**，只 props/events；视图里的 handler 只写一份，表格与卡片调同一批 store action。

## 6. 表单与底部弹层规则

1. `ProxyFormDialog`（720px `el-dialog`）与 `InstanceFormDrawer`（560px `el-drawer`）在移动端分别变成贴底弹层与 `direction="btt"` 抽屉（做法见 §5，**模板只有一份**，只改容器参数与类名）。
2. 所有 `el-col :span="12"` 补 `:xs="24"`（用 EP 栅格自带响应式，不用 `v-if` 写两套）。
3. `label-position` 移动端改 `top`（放弃 110px 左标），标签字号 12px、与输入框间距 5px。
4. 端口/数值字段加 `inputmode="numeric"`；密码字段 `autocomplete="new-password"`；数字键盘不弹全键盘。
5. 「保存」吸底（footer 区），`padding-bottom: calc(10px + env(safe-area-inset-bottom))`，不被键盘与浏览器工具栏盖住。
6. 校验失败：`scrollIntoView({ block: 'center' })` 定位到第一个错误字段。
7. iOS 键盘顶起：监听 `visualViewport.resize`，focus 后延迟约 300ms 再滚动定位；不支持时退化为「不自动滚动」，不阻塞输入。
8. **弹层内不再嵌套弹层**：需要二次确认（删除、覆盖导入）时先关当前 sheet，再开确认面板——避免 z-index、焦点锁与滚动锁互相打架。
9. 滚动锁：统一用 `html.is-sheet-open` 一个类管理，关掉 EP 自带 `lock-scroll`；锁定时用 `position: fixed` + 记录/恢复 `scrollY`，**不用 `body{overflow:hidden}`**（微信 webview 上会锁死页面）。

## 7. 刷新与轮询

- **不加顶栏全局刷新按钮**（仪表盘/版本页已有各自「刷新」，加一个会成重复入口）。
- `stores/instances.ts` 的 `refreshSilently()` / `refresh()` 加 **in-flight 去重**：同一时刻只允许一个在途请求，后到的调用复用同一个 Promise。
- 新增「回到前台立即刷新一次」：`visibilitychange` 变为 `visible` 时调用 `refreshSilently()`（现有轮询已只在 visible 时跑，这里补的是回前台的第一时间刷新，省电与实时兼顾）。
- 移动形态下所有「操作后回刷」沿用现有逻辑（`withActing` 里已 `refreshSilently()`），不新增定时器。

## 8. 破坏性操作与防误触

- 触摸目标：移动形态下的**主要可点元素**（卡片操作、「⋯」、启用开关、按钮、输入框、chip）≥44×44px；**次要文字按钮**（如「收起升级步骤」「版本说明」这类）保持既有的轻量观感，但用 padding 把热区扩到 ≥32px 高，且与相邻可点元素间距 ≥8px——不为了凑 44px 把次要控件撑大。相邻可点元素间距 ≥8px；`touch-action: manipulation` 去掉 300ms 点击延迟。
- 删除、覆盖导入：底部确认面板，红色按钮 + 文案写清后果（「删除后该代理配置立即从 frpc 移除，隧道中断」）。**不做「等 2 秒才能点」的延时按钮**（会让人以为界面卡住）。
- 重启、停用：轻确认（沿用桌面已有文案）+ 结果 toast 回显。
- 启用开关整块热区扩到 44px 高，但只在开关本身响应，卡片其余区域不绑 toggle（防误触开关）。

## 9. 兼容与降级

| 环境 | 问题 | 处理 |
|---|---|---|
| iOS < 15.4 无 `dvh` | 高度算错 | `100vh` 兜底在前，`100dvh` 覆盖在后 |
| 无 `viewport-fit` | `env(safe-area-inset-*)` 恒为 0 | `index.html` 补 `viewport-fit=cover` |
| http 非安全上下文 | `navigator.clipboard` 被拒 | 复用 `utils/clipboard.ts` 三级兜底（已实测 http 下 `execCommand` 可用） |
| 微信内置浏览器 | `body{overflow:hidden}` 锁死、内层滚动失效、`el-select` 弹层定位偏 | 统一 `position:fixed` 滚动锁；下拉选择优先用原生 `select` 风格或 EP 原生 popper 并实测 |
| 老 Android WebView 无 `visualViewport` | 键盘补偿失效 | 特性检测后跳过补偿，不报错 |

## 10. 桌面零回归策略

1. 移动形态全部收敛在：`isMobile` 的 `v-if` 分支 + `@media (max-width: 767px)` 样式 + EP 栅格 `:xs`，**不改写桌面模板结构**。
2. 新增样式尽量放 `web/src/styles/mobile.css`（只含媒体查询与触摸工具类），由 `main.ts` 引入一次，避免散落到各 .vue 里难以核对。
3. 合并前必须跑桌面回归基线（见 §11），桌面 1440×844 下断言：`el-table` 存在、`el-aside` 宽 220px、无移动 drawer 节点、无卡片列表节点。

## 11. 测试与验收

**自动化（无头 Chrome + CDP，临时容器 + 临时数据目录，绝不碰生产 18080）**

视口 375×812 / 390×844 / 430×932，iPhone UA，`deviceScaleFactor: 3`，断言清单：

1. 五个页面 `document.documentElement.scrollWidth <= innerWidth`（无横向滚动）。**实测补充**：EP 表格列总宽超出视口时是在 `.el-table__body-wrapper` 内部横滚，文档级 `scrollWidth` 不报警（脚手架第一版就是这么漏掉的），所以再加两条结构性断言：三个表格页（`/`、`/instances`、`/proxies`）在移动形态下不得渲染 `.el-table`，且必须渲染 `.fm-cards`。
2. 移动形态下主要可点元素（卡片操作、「⋯」、启用开关、按钮、输入框、chip）尺寸 ≥44×44，相邻可点间距 ≥8px；次要文字按钮按 §8 的放宽口径断言（热区 ≥32px 高）。
3. 汉堡 → 左抽屉可打开，菜单项与桌面完全一致（同一份 `SideMenu`），点击能跳转且抽屉自动关闭。
4. 代理页 chip 条可横向滚动，切换后卡片数据随之变化且条数 == store 里该实例代理数。
5. 卡片「⋯」→ ActionSheet 可打开、可下滑关闭、删除项为红色且需要二次确认。
6. 「+ 创建」→ 底部弹层表单：字段单列、label 在上、保存吸底；填完提交后新代理出现在列表（真实打后端）。
7. 触屏形态下 `FieldHint` 可点开看到文案（断言文案文本存在且可见）。
8. 配置文件底部弹层里的「复制」按钮在 http 下给出「已复制」或「已选中」提示（复用 `utils/clipboard.ts` 的三档兜底，不许谎报成功）。
9. `visibilitychange` 回前台确实触发一次列表请求（Network 计数）。
10. 安全区：无头 Chrome 里 `env(safe-area-inset-*)` 恒为 0，**不能断言像素值**。改为两条可测断言：`index.html` 的 viewport 含 `viewport-fit=cover`；弹层 footer 与顶栏的 computed style 里 `padding-bottom` / `padding-top` 声明了 `env(...)`（用 `getPropertyValue` 检查声明串，而非解析后的 0px）。真机再人工确认刘海与底部工具栏不遮挡。
11. 桌面回归：1440×844 下断言 `el-table` 存在、`el-aside` 220px、无移动 drawer、无 `ProxyCardList` 节点。

**跑通这套路子时踩到的七条，记下来免得下次重新踩**

1. 无头 Chrome 的 `mobile: true` 仿真**不会**把 `(hover: hover)` 改成 `none`（`Emulation.setEmulatedMedia` 也压不住）。凡是靠 hover 判触屏的分支（`FieldHint` 走 tooltip 还是可点问号、复制兜底文案的措辞）在验收里永远只会走桌面分支，必须在 `Page.addScriptToEvaluateOnNewDocument` 里替页面重写 `matchMedia`，桌面段落跑完再 `removeScriptToEvaluateOnNewDocument` 摘掉。
2. `el-drawer` / `el-dialog` 有约 300ms 开合动画，动画里元素的 rect 还在位移：不等落定就量「保存按钮是否在屏内」、就按坐标点按钮，会得到假的 FAIL。凡是「刚打开就要量/就要点」的步骤，中间补 600–700ms。
3. 断言用的选择器要按**容器语义**而不是样式类：`.fm-sheet` 会先命中已关闭但仍挂在 DOM 里的 ActionSheet，`.el-drawer.btt, .el-dialog` 会先命中隐藏的信息弹窗，量到的是不可见容器的读数。表单弹层因此专门打一个 `.fm-form` 标记（`fm-sheet fm-form`）。
4. 输入控件的高度量 `.el-input__wrapper`（外框），字号量 `.el-input__inner`：`.el-input__inner` 只是里面那行文字，恒在 24 上下，量它会误判不达 44。
5. `el-drawer` 关闭后节点仍在 DOM（只是隐藏），「关掉了」要按 `getBoundingClientRect().height === 0` 判，不能按「查不到节点」。
6. 验收会真的打后端建/删数据，所以灌数据必须幂等：实例和代理都按名字补齐，测试专用的 `cdp-mobile-prox` 每轮开头先清掉；删除断言要点**自己新建的那张卡**，否则每跑一轮就吃掉一条演示数据，跑到后面页面直接空列表。
7. 填表单按 `.el-form-item__label` 的文字定位字段，别按 placeholder —— placeholder 里写的是「例如: web-server」，不含「名称」，按 placeholder 找会找不到，而页面里的 JS 异常被 CDP 吞掉后只会表现成「保存没生效」。

**组件级冒烟（可选，不阻塞本轮）**：`useSwipeToClose` 阈值、`useIsMobile` 切换、卡片列表渲染与事件冒泡。引入 vitest 成本约 1–2 天，本轮以 CDP 端到端为准，vitest 留下一轮再定。

**人工真机清单**（交付给用户自测）：iPhone Safari 直连内网 IP、微信里点开链接、外出走 frp 隧道三种路径各跑一遍登录 → 看代理状态 → 启停一个代理 → 新建一个代理 → 看运行日志。

## 12. 发布顺序与版本号

1. GitHub 先建 issue（用户指定「先自己提一个 issue」）。
2. 分支 `feat/mobile-adaptation` → 实现 → 互审 + 自动化验收 → 本地打包。
3. 用户验收放行后：push → PR（`Closes #<issue>`）→ 合并 → tag → Release 挂 4 包 + `sha256sums.txt`。
4. 再单独确认是否重建生产容器（隧道中断数秒）。
5. **版本号待定**：本轮是纯前端的功能新增，按既有规则默认走 patch（1.1.2）；但移动端适配属于「增加了较大的功能」，也可能该走 1.2.0。打 tag 前需与用户确认一次。

## 13. 明确不做

底部 Tab、手势下拉刷新、左滑行操作、侧滑返回、PWA/service worker、虚拟滚动、按需引入体积优化、后端接口改动、平板专属布局。
