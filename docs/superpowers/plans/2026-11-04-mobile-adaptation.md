# 移动端（手机）适配 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 frpc-mesh 面板在手机浏览器（iOS Safari / 微信内置 / Android Chrome，http 访问）上全功能可用——卡片列表替代宽表格、表单与行操作走底部弹层、顶栏汉堡 + 左侧覆盖抽屉导航，同时桌面端零回归。

**Architecture:** 单一形态判定源 `useIsMobile()`（`matchMedia('(max-width: 767px)')`）驱动结构切换：CSS 媒体查询管排版，`v-if` 管「表格 ↔ 卡片列表」这类结构差异；表单与弹窗**只有一份模板**，靠容器参数（`el-drawer direction="btt"` / `component :is`）变成底部弹层；卡片组件是纯展示组件（props + emits），所有数据与状态计算仍留在 Pinia store 与 `utils/`，表格与卡片共用同一批 cell 组件与 handler。

**Tech Stack:** Vue 3.5 Composition API + `<script setup>` + TypeScript、Element Plus 2.9、Pinia、Vite 6；验证用无头 Chrome + CDP（项目无前端单测框架，本计划的「测试」= CDP 端到端断言 + `vue-tsc` 类型检查 + `vite build`）。

**规格**：`docs/superpowers/specs/2026-11-04-mobile-adaptation-design.md`
**分支**：`feat/mobile-adaptation`（已创建）
**Go 后端**：零改动。每个 Task 结束时 `go test ./...` 必须仍是全绿（防止前端改动误碰后端）。

---

## 约定：验收脚本怎么跑

所有前端断言都跑在**临时容器**上，绝不碰生产 18080：

```bash
cd /Users/busfly/Documents/code_workspace/Frpc-Mesh && export PATH="$HOME/.local/go/bin:$PATH" && \
docker build -t frpc-mesh:mobile-test . && \
rm -rf /tmp/fm-mobile-data && \
docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test && \
sleep 8 && curl -s http://127.0.0.1:18096/api/health
```

期望：`{"status":"ok","version":"1.1.1"}`（版本号本轮先不动，见 Task 13）。

跑验收：

```bash
cd /Users/busfly/Documents/code_workspace/Frpc-Mesh && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

收尾：`docker rm -f fm-mobile-test && docker rmi frpc-mesh:mobile-test && rm -rf /tmp/fm-mobile-data`

---

## Task 1: 形态判定与移动样式基座

**Files:**
- Create: `web/src/composables/useMediaQuery.ts`
- Create: `web/src/styles/mobile.css`
- Modify: `web/src/main.ts`（加一行 import）
- Modify: `web/index.html:5`（viewport 补 `viewport-fit=cover`）

- [ ] **Step 1: 写形态判定 composable**

创建 `web/src/composables/useMediaQuery.ts`：

```ts
import { computed, ref, type ComputedRef, type Ref } from 'vue'

const MOBILE = '(max-width: 767px)'
const NARROW_DESKTOP = '(min-width: 768px) and (max-width: 1023px)'

// 同一个查询全站只挂一个监听：模块级缓存，组件卸载不回收（生命周期与页面一致）
const cache = new Map<string, Ref<boolean>>()

function stateOf(query: string): Ref<boolean> {
  let s = cache.get(query)
  if (s) return s
  s = ref(false)
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    const mq = window.matchMedia(query)
    s.value = mq.matches
    mq.addEventListener('change', (e) => {
      s!.value = e.matches
    })
  }
  cache.set(query, s)
  return s
}

/** 移动形态：只看视口宽度，不看 UA、不看 pointer —— 窄窗口的桌面浏览器应保持桌面形态 */
export function useIsMobile(): ComputedRef<boolean> {
  return computed(() => stateOf(MOBILE).value)
}

/** 768–1023px：仍是桌面形态，但侧栏默认折叠 */
export function useIsNarrowDesktop(): ComputedRef<boolean> {
  return computed(() => stateOf(NARROW_DESKTOP).value)
}
```

- [ ] **Step 2: 写移动样式表**

创建 `web/src/styles/mobile.css`（只放移动形态工具类与媒体查询，桌面端一条都不生效）：

```css
/* 移动形态专用样式。类名统一 fm- 前缀，只在 isMobile 分支挂上。 */

/* —— 底部弹层（el-drawer direction=btt）—— */
.fm-sheet.el-drawer {
  --el-drawer-padding-primary: 14px;
}
.fm-sheet .el-drawer__header {
  margin-bottom: 8px;
  padding: 4px 14px 10px;
}
.fm-sheet .el-drawer__body {
  padding: 0 14px;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}
.fm-sheet .el-drawer__footer {
  padding: 10px 14px calc(10px + env(safe-area-inset-bottom));
  border-top: 1px solid var(--el-border-color-lighter);
}
.fm-sheet.el-drawer--bottom {
  max-height: 92vh;
  border-radius: 16px 16px 0 0;
}
.fm-handle {
  width: 40px;
  height: 4px;
  border-radius: 2px;
  background: #d8dce8;
  margin: 6px auto 2px;
  touch-action: none;
}
/* 抽屉头部没有 el-dialog__title 样式，代理表单换容器时用这条 */
.fm-sheet-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

/* —— 吸顶 chip 条（服务器切换 / 类型快速添加）—— */
.fm-chips {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  gap: 8px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  padding: 4px 0 10px;
  background: var(--fm-page-bg);
  scrollbar-width: none;
}
.fm-chips::-webkit-scrollbar {
  display: none;
}
.fm-chip {
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 44px;
  padding: 9px 14px;
  border-radius: 999px;
  border: 1px solid var(--el-border-color-lighter);
  background: #fff;
  color: var(--el-text-color-regular);
  font-size: 12.5px;
  white-space: nowrap;
  cursor: pointer;
  touch-action: manipulation;
}
.fm-chip.on {
  background: var(--fm-primary-grad);
  border-color: transparent;
  color: #fff;
  font-weight: 600;
}
.fm-chip .chip-sub {
  font-size: 11px;
  opacity: 0.8;
}

/* —— 卡片列表 —— */
.fm-cards {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.fm-card {
  background: #fff;
  border-radius: 12px;
  padding: 11px 12px;
  box-shadow: 0 1px 4px rgba(31, 36, 51, 0.06);
}
.fm-card-row1 {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.fm-card-name {
  font-size: 14.5px;
  font-weight: 600;
  color: var(--fm-ink, #1f2433);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.fm-card-row {
  margin-top: 9px;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.fm-card-k {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  flex: none;
}
.fm-card-sub {
  margin-top: 7px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.fm-more {
  margin-left: auto;
  flex: none;
  width: 44px;
  height: 44px;
  border-radius: 10px;
  border: 1px solid var(--el-border-color-lighter);
  background: #fff;
  color: var(--el-text-color-secondary);
  font-size: 17px;
  line-height: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  touch-action: manipulation;
}

/* —— 操作面板 —— */
.fm-actionsheet .as-title {
  margin: 2px 0 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.fm-actionsheet .as-item {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  min-height: 44px;
  padding: 0 2px;
  border: none;
  border-top: 1px solid var(--el-border-color-lighter);
  background: transparent;
  color: var(--fm-ink, #1f2433);
  font-size: 14.5px;
  text-align: left;
  cursor: pointer;
  touch-action: manipulation;
}
.fm-actionsheet .as-item[disabled] {
  color: #c0c4cc;
  cursor: not-allowed;
}
.fm-actionsheet .as-item.danger {
  color: var(--el-color-danger);
}
.fm-actionsheet .as-hint {
  margin-left: auto;
  font-size: 11.5px;
  color: var(--el-text-color-secondary);
}
.fm-actionsheet .as-cancel {
  margin-top: 8px;
  justify-content: center;
  color: var(--el-text-color-regular);
}

/* —— 说明文字（触屏）—— */
.fm-hint-btn {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 1px solid #c4cad8;
  background: #fff;
  color: #8a93ad;
  font-size: 11px;
  line-height: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  touch-action: manipulation;
}

/* —— 全局移动形态微调 —— */
@media (max-width: 767px) {
  .page-container {
    padding: 12px;
  }
  /* 主要按钮：实心按钮一律 ≥44 高（文字按钮另算，见下） */
  .page-container .el-button--primary:not(.is-text):not(.is-link),
  .page-container .el-button--success:not(.is-text):not(.is-link),
  .page-container .el-button--danger:not(.is-text):not(.is-link) {
    min-height: 44px;
  }
  /* 次要文字按钮：保持轻量观感，只用 padding 把热区抬到 32 */
  .page-container .el-button.is-text,
  .page-container .el-button.is-link {
    min-height: 32px;
  }
  /* 卡片里的启用开关：热区扩大，视觉不变 */
  .fm-card .el-switch {
    padding-block: 10px;
  }
  /* 弹层里的输入控件：整档提到 44（用 EP 的 component-size 变量，wrapper 会跟着长高）
     字号 16px 是硬要求：iOS Safari 对 <16px 的输入框聚焦时会自动放大页面 */
  .fm-sheet .el-input,
  .fm-sheet .el-select,
  .fm-sheet .el-input-number {
    --el-component-size: 44px;
  }
  .fm-sheet .el-input__inner,
  .fm-sheet .el-select__wrapper {
    font-size: 16px;
  }
  .el-message-box {
    width: 88vw !important;
    max-width: 420px;
  }
  .terminal {
    font-size: 12px;
    padding: 10px 12px;
  }
}
```

- [ ] **Step 3: 引入样式表**

`web/src/main.ts` 里，在 `import './styles/global.css'` 之后加一行：

```ts
import './styles/mobile.css'
```

- [ ] **Step 4: viewport 补 viewport-fit**

`web/index.html:5` 从：

```html
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
```

改为：

```html
    <meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover" />
```

> 不补 `viewport-fit=cover`，`env(safe-area-inset-*)` 在 iOS 上恒为 0，后面所有安全区样式都是空写。

- [ ] **Step 5: 类型检查与构建**

Run: `cd web && pnpm type-check && pnpm build 2>&1 | tail -3`
Expected: type-check 退出 0；`✓ built in …`

- [ ] **Step 6: 提交**

```bash
git add web/src/composables/useMediaQuery.ts web/src/styles/mobile.css web/src/main.ts web/index.html
git commit -m "feat(web): 移动形态判定源与样式基座"
```

---

## Task 2: 验收脚手架（先让它红）

**Files:**
- Create: `scripts/verify-mobile.mjs`

本项目的「测试」是 CDP 端到端断言。这个 Task 先把脚手架与两条断言建起来，此时移动断言必然失败（还没实现），这就是后续 Task 的红灯。

- [ ] **Step 1: 写脚手架（连接、登录、造演示数据、断言收集器）**

创建 `scripts/verify-mobile.mjs`：

```js
// 移动端适配验收：无头 Chrome + CDP，多视口断言 + 桌面回归基线
// 用法：BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
import { spawn } from 'node:child_process'
import fs from 'node:fs/promises'

const BASE = process.env.BASE || 'http://127.0.0.1:18096'
const PORT = Number(process.env.CDP_PORT || 9340)
// 移动视口宽度，Task 12 的三档回归靠它切换：MOBILE_WIDTH=375|390|430
const WIDTH = Number(process.env.MOBILE_WIDTH || 390)
const HEIGHT = Number(process.env.MOBILE_HEIGHT || 844)
const UA = process.env.MOBILE_UA || ''
const OUT = process.env.SHOT_DIR || '/tmp/shots-mobile'
const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const USER = process.env.FM_USER || 'admin'
const PASS = process.env.FM_PASS || 'admin123'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok: !!ok, detail: String(detail).slice(0, 220) })
}

// 演示数据：全部假地址，不含用户真实配置
const SEED = [
  { name: 'demo-a', addr: '127.0.0.1', port: 7600, domain: 'frps-a.example.com', proxies: [
      { name: 'demo-ssh', type: 'tcp', local_ip: '127.0.0.1', local_port: 22, remote_port: 22001 },
      { name: 'demo-web', type: 'http', local_ip: '127.0.0.1', local_port: 8080, custom_domains: 'web.example.com' },
      { name: 'demo-off', type: 'udp', local_ip: '127.0.0.1', local_port: 53, remote_port: 5353, enabled: false }
    ] },
  { name: 'demo-b', addr: '127.0.0.1', port: 7601, domain: '', proxies: [
      { name: 'demo-p2p', type: 'xtcp', local_ip: '127.0.0.1', local_port: 0, remote_port: 0 }
    ] }
]

async function loginOnce() {
  return fetch(`${BASE}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS })
  })
}

async function seed() {
  // 容器刚起时端口可能还没就绪（OrbStack 挂载新目录也偶发慢几秒），先探通再灌数据
  let login
  for (let i = 0; ; i++) {
    try {
      login = await loginOnce()
      if (login.ok) break
    } catch {}
    if (i > 40) throw new Error('后端未就绪：' + BASE)
    await new Promise((r) => setTimeout(r, 500))
  }
  if (!login.ok) throw new Error(`登录失败 ${login.status}`)
  const { token } = await login.json()
  const auth = { headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' } }
  const list = (await (await fetch(`${BASE}/api/instances`, auth)).json()).map((i) => i.name)
  for (const inst of SEED) {
    if (list.includes(inst.name)) continue
    const created = await (
      await fetch(`${BASE}/api/instances`, {
        ...auth,
        method: 'POST',
        body: JSON.stringify({
          name: inst.name,
          server_addr: inst.addr,
          server_port: inst.port,
          auth_method: 'none',
          domain: inst.domain,
          manage_port: 0,
          frpc_version: '',
          enabled: false // 不真连，只为渲染布局
        })
      })
    ).json()
    for (const p of inst.proxies) {
      await fetch(`${BASE}/api/instances/${created.id}/proxies`, {
        ...auth,
        method: 'POST',
        body: JSON.stringify({
          kind: 'proxy',
          name: p.name,
          type: p.type,
          enabled: p.enabled !== false,
          local_ip: p.local_ip,
          local_port: p.local_port,
          remote_port: p.remote_port,
          custom_domains: p.custom_domains || '',
          subdomain: ''
        })
      })
    }
  }
}

async function connect() {
  for (let i = 0; i < 80; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${PORT}/json`)).json()
      const page = list.find((t) => t.type === 'page')
      if (page) {
        const ws = new WebSocket(page.webSocketDebuggerUrl)
        await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej })
        return ws
      }
    } catch {}
    await new Promise((r) => setTimeout(r, 300))
  }
  throw new Error('Chrome CDP 未就绪')
}

function session(ws) {
  let id = 0
  const pend = new Map()
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data)
    if (m.id && pend.has(m.id)) { pend.get(m.id)(m); pend.delete(m.id) }
  }
  const raw = (method, params = {}) =>
    new Promise((res) => { const n = ++id; pend.set(n, res); ws.send(JSON.stringify({ id: n, method, params })) })
  const cmd = async (method, params = {}) => {
    const r = await raw(method, params)
    if (r.error) throw new Error(`${method}: ${JSON.stringify(r.error)}`)
    return r.result
  }
  const js = async (expression) =>
    (await cmd('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result?.value
  const wait = async (expr, label, timeout = 20000) => {
    const t0 = Date.now()
    for (;;) {
      if (await js(`!!(${expr})`)) return
      if (Date.now() - t0 > timeout) throw new Error(`超时: ${label}`)
      await new Promise((r) => setTimeout(r, 200))
    }
  }
  const setViewport = async (w, h) =>
    cmd('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 3, mobile: w < 768 })
  const shot = async (name) => {
    const p = await cmd('Page.captureScreenshot', { format: 'png' })
    await fs.writeFile(`${OUT}/${name}`, Buffer.from(p.data, 'base64'))
  }
  return { cmd, js, wait, setViewport, shot }
}

/** 真实鼠标/触摸点击（CDP 输入事件才有用户激活） */
async function tapAt(S, selector) {
  const box = await S.js(
    `(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return null;` +
    ` el.scrollIntoView({ block: 'center' }); const r = el.getBoundingClientRect();` +
    ` return { x: Math.round(r.x + r.width / 2), y: Math.round(r.y + r.height / 2) } })()`
  )
  if (!box) throw new Error(`找不到元素 ${selector}`)
  await new Promise((r) => setTimeout(r, 250))
  await clickXY(S, box.x, box.y)
}

async function clickXY(S, x, y) {
  for (const type of ['mousePressed', 'mouseReleased']) {
    await S.cmd('Input.dispatchMouseEvent', { type, x, y, button: 'left', clickCount: 1 })
  }
}

/** 按可见文字点按钮（工具栏/消息框这类没有稳定 class 的入口） */
async function tapText(S, label, rootSel = 'document') {
  const box = await S.js(
    `(() => { const els = [...${rootSel}.querySelectorAll('button, .el-button, .el-message-box__btns button')];` +
    ` const el = els.find((e) => e.innerText.trim().includes(${JSON.stringify(label)}));` +
    ` if (!el) return null; el.scrollIntoView({ block: 'center' });` +
    ` const r = el.getBoundingClientRect();` +
    ` return { x: Math.round(r.x + r.width / 2), y: Math.round(r.y + r.height / 2) } })()`
  )
  if (!box) throw new Error(`找不到文字为「${label}」的按钮`)
  await new Promise((r) => setTimeout(r, 250))
  await clickXY(S, box.x, box.y)
  return box
}

const PAGES = ['/', '/instances', '/proxies', '/versions', '/settings']
/** 桌面用表格、移动形态必须换成卡片的三个页面 */
const TABLE_PAGES = ['/', '/instances', '/proxies']

async function loginIfNeeded(S) {
  await S.cmd('Page.navigate', { url: BASE })
  await S.wait('document.querySelector(".login-btn") || document.querySelector(".main-layout")', 'ready')
  if (await S.js('!!document.querySelector(".login-btn")')) {
    await S.js(`(() => { const set=(el,v)=>{const d=Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el),'value');` +
      `d.set.call(el,v);el.dispatchEvent(new Event('input',{bubbles:true}))};` +
      ` const i=document.querySelectorAll('.login-card input'); set(i[0],${JSON.stringify(USER)}); set(i[1],${JSON.stringify(PASS)}); })()`)
    await tapAt(S, '.login-btn')
  }
  await S.wait('document.querySelector(".main-layout")', 'logged in')
}

/**
 * 页面在移动形态下不该出现横向溢出。
 * 注意：EP 表格撑宽时是在自己的 .el-table__body-wrapper 里横滚，文档级 scrollWidth 看不出来，
 * 所以「有没有表格」必须用下面的结构性断言单独判；这里只兜住真正的文档级溢出。
 */
async function noHorizontalOverflow(S) {
  return S.js(`(() => {
    const de = document.documentElement;
    const main = document.querySelector('.main-content') || de;
    const wide = [...document.querySelectorAll('body *')].filter((el) => {
      const cs = getComputedStyle(el);
      if (cs.overflowX === 'auto' || cs.overflowX === 'scroll') return false;
      const r = el.getBoundingClientRect();
      return r.right > window.innerWidth + 1 && r.width > 0;
    });
    const scrollers = [...document.querySelectorAll('body *')].filter((el) => {
      const cs = getComputedStyle(el);
      if (cs.overflowX !== 'auto' && cs.overflowX !== 'scroll') return false;
      if (el.matches('.fm-chips, .fm-chips *')) return false;   // chip 条刻意可横滚
      const r = el.getBoundingClientRect();
      return r.width > 0 && el.scrollWidth > el.clientWidth + 1;
    });
    return {
      doc: de.scrollWidth <= window.innerWidth + 1,
      main: main.scrollWidth <= main.clientWidth + 1,
      offenders: wide.slice(0, 3).map((e) => String(e.className || e.tagName).slice(0, 60)),
      innerScrollers: scrollers.slice(0, 3).map((e) => String(e.className || e.tagName).slice(0, 60))
    };
  })()`)
}

async function run() {
  await fs.mkdir(OUT, { recursive: true })
  await fs.rm('/tmp/fm-chrome-mobile', { recursive: true, force: true })
  await seed()
  const chrome = spawn(CHROME, [
    '--headless=new', `--remote-debugging-port=${PORT}`, '--user-data-dir=/tmp/fm-chrome-mobile',
    '--no-first-run', '--disable-gpu', `--window-size=${WIDTH},${HEIGHT}`, '--hide-scrollbars', 'about:blank'
  ], { stdio: 'ignore' })
  const ws = await connect()
  const S = session(ws)
  await S.cmd('Page.enable')
  // MOBILE_UA 用于模拟微信内置浏览器等 webview，行为差异（剪贴板、滚动锁）要在这一档验
  if (UA) await S.cmd('Network.setUserAgentOverride', { userAgent: UA })
  try {
    // —— 移动视口 ——
    await S.setViewport(WIDTH, HEIGHT)
    await loginIfNeeded(S)
    for (const path of PAGES) {
      await S.cmd('Page.navigate', { url: BASE + path })
      await S.wait('document.querySelector(".page-container") || document.querySelector(".page-title")', `page ${path}`)
      await new Promise((r) => setTimeout(r, 600))
      const r = await noHorizontalOverflow(S)
      check(`[${path}] 无横向溢出`, r.doc && r.main, JSON.stringify(r))
      if (TABLE_PAGES.includes(path)) {
        const shape = await S.js(`(() => ({
          table: !!document.querySelector('.el-table'),
          cards: !!document.querySelector('.fm-cards')
        }))()`)
        check(`[${path}] 移动形态不渲染表格`, !shape.table, JSON.stringify(shape))
        check(`[${path}] 移动形态渲染卡片列表`, shape.cards, JSON.stringify(shape))
      }
      await S.shot('mobile' + (path === '/' ? '-dashboard' : path.replace(/^\//, '-')) + '.png')
    }
    // —— 桌面回归基线 1440×844 ——
    await S.setViewport(1440, 844)
    // 视口改动要等一帧再导航：MainLayout 的折叠初值读 window.innerWidth，抢跑会拿到旧宽度
    await new Promise((r) => setTimeout(r, 400))
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".el-table") || document.querySelector(".el-empty")', 'desktop proxies table')
    check('[桌面] 代理页仍是表格形态', await S.js('!!document.querySelector(".el-table")'))
    check('[桌面] 侧栏存在且 220px', await S.js(`(() => { const a = document.querySelector('.el-aside'); return a && a.getBoundingClientRect().width === 220 })()`))
    check('[桌面] 无移动卡片列表', await S.js('!document.querySelector(".fm-cards")'))
    await S.shot('desktop-proxies.png')
  } finally {
    ws.close()
    chrome.kill('SIGKILL')
  }
  for (const r of results) console.log(`${r.ok ? 'PASS' : 'FAIL'} ${r.name}${r.detail ? ' :: ' + r.detail : ''}`)
  const failed = results.filter((r) => !r.ok).length
  console.log(`\n共 ${results.length} 项，失败 ${failed}`)
  process.exit(failed ? 1 : 0)
}

run().catch((e) => {
  console.error('脚本异常:', e.message)
  for (const r of results) console.log(`${r.ok ? 'PASS' : 'FAIL'} ${r.name}`)
  process.exit(2)
})
```

- [ ] **Step 2: 起临时容器并跑一次，确认脚手架本身可用**

```bash
cd /Users/busfly/Documents/code_workspace/Frpc-Mesh && export PATH="$HOME/.local/go/bin:$PATH" && \
docker build -t frpc-mesh:mobile-test . && rm -rf /tmp/fm-mobile-data && \
docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

Expected: 脚本能跑完并打印结果。此时**三个表格页的「移动形态不渲染表格」「移动形态渲染卡片列表」共 6 条应当 FAIL**（现在只有表格），「无横向溢出」大多会 PASS——EP 表格撑宽时是在自己 body-wrapper 里横滚，文档级 `scrollWidth` 不报警，所以结构性断言才是真红灯。桌面三条应当 PASS；若桌面三条 FAIL，先修脚手架（选择器/登录/seed），别往下走。

- [ ] **Step 3: 提交**

```bash
git add scripts/verify-mobile.mjs
git commit -m "test(web): 移动端 CDP 验收脚手架与桌面回归基线"
```

---

## Task 3: 导航——SideMenu 抽取 + MainLayout 移动外壳

**Files:**
- Create: `web/src/components/layout/SideMenu.vue`
- Modify: `web/src/layouts/MainLayout.vue`

- [ ] **Step 1: 抽 SideMenu（菜单项定义只留一份）**

创建 `web/src/components/layout/SideMenu.vue`：

```vue
<template>
  <el-menu
    :default-active="activeMenu"
    :collapse="collapsed"
    :collapse-transition="false"
    background-color="transparent"
    text-color="#a3adc7"
    active-text-color="#ffffff"
    router
    class="side-menu"
    @select="emit('navigate')"
  >
    <el-menu-item index="/">
      <el-icon><Odometer /></el-icon>
      <template #title>仪表盘</template>
    </el-menu-item>
    <el-menu-item index="/instances">
      <el-icon><Menu /></el-icon>
      <template #title>FRPS 服务器</template>
    </el-menu-item>
    <el-menu-item index="/proxies">
      <el-icon><Connection /></el-icon>
      <template #title>穿透代理</template>
    </el-menu-item>
    <el-menu-item index="/versions">
      <el-icon><Download /></el-icon>
      <template #title>frpc 版本管理</template>
    </el-menu-item>
    <el-menu-item index="/settings">
      <el-icon><Setting /></el-icon>
      <template #title>设置</template>
    </el-menu-item>
  </el-menu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

defineProps<{ collapsed?: boolean }>()
const emit = defineEmits<{ navigate: [] }>()

const route = useRoute()
const activeMenu = computed(() => (route.meta.activeMenu as string) || route.path)
</script>

<style scoped>
.side-menu {
  border-right: none;
  padding: 6px 8px;
}

.side-menu :deep(.el-menu-item) {
  border-radius: 8px;
  margin-bottom: 4px;
  height: 44px;
}

.side-menu :deep(.el-menu-item:hover) {
  background-color: rgba(255, 255, 255, 0.06);
}

.side-menu :deep(.el-menu-item.is-active) {
  background: var(--fm-primary-grad);
  color: #fff;
}
</style>
```

- [ ] **Step 2: MainLayout 换成 SideMenu，并加移动外壳**

`web/src/layouts/MainLayout.vue` 的 `<template>` 整体替换为：

```vue
<template>
  <el-container class="main-layout">
    <el-aside v-if="!isMobile" :width="collapsed ? '64px' : '220px'" class="sidebar">
      <div class="logo" @click="router.push('/')">
        <div class="logo-icon">
          <img src="/logo.svg" alt="frpc-mesh" class="logo-img" />
        </div>
        <transition name="fade">
          <div v-show="!collapsed" class="logo-text">
            <div class="logo-name">frpc-mesh</div>
            <div class="logo-slogan">多服务器 frp 管理</div>
          </div>
        </transition>
      </div>
      <SideMenu :collapsed="collapsed" class="menu-fill" />
      <div class="sidebar-footer">
        <el-tooltip :content="collapsed ? '展开侧边栏' : '折叠侧边栏'" placement="top">
          <div class="collapse-btn" @click="collapsed = !collapsed">
            <el-icon :size="16">
              <Expand v-if="collapsed" />
              <Fold v-else />
            </el-icon>
          </div>
        </el-tooltip>
      </div>
    </el-aside>

    <el-container class="right-container">
      <el-header class="topbar" height="56px">
        <div class="topbar-left">
          <button v-if="isMobile" class="hamburger" aria-label="打开菜单" @click="navOpen = true">
            <span class="bars"></span>
          </button>
          <span class="topbar-title">{{ currentTitle }}</span>
        </div>
        <div class="topbar-right">
          <el-tag
            v-if="update.hasUpdate"
            class="update-tag"
            size="small"
            type="warning"
            effect="plain"
            round
            title="前往设置页查看并升级"
            @click="router.push('/settings')"
          >
            发现新版本 v{{ update.info?.latest }}
          </el-tag>
          <template v-if="!isMobile">
            <el-tag v-if="auth.version" size="small" type="info" effect="plain" round>
              v{{ auth.version }}
            </el-tag>
            <el-tooltip content="开源仓库：看更新、提 Issue 反馈" placement="bottom">
              <a class="repo-link" :href="REPO_URL" target="_blank" rel="noopener noreferrer" aria-label="GitHub 仓库">
                <svg viewBox="0 0 16 16" width="18" height="18" fill="currentColor" aria-hidden="true">
                  <path
                    d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.05-.13-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.13 1.99.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"
                  />
                </svg>
              </a>
            </el-tooltip>
          </template>
          <el-dropdown trigger="click" @command="onCommand">
            <span class="user-chip">
              <el-avatar :size="26" class="user-avatar">
                {{ (auth.username || 'A').charAt(0).toUpperCase() }}
              </el-avatar>
              <span class="user-name">{{ auth.username || '管理员' }}</span>
              <el-icon :size="12"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-if="isMobile && auth.version" command="version" disabled>
                  <el-icon><InfoFilled /></el-icon> 当前版本 v{{ auth.version }}
                </el-dropdown-item>
                <el-dropdown-item v-if="isMobile" command="repo">
                  <el-icon><Link /></el-icon> 开源仓库
                </el-dropdown-item>
                <el-dropdown-item command="settings">
                  <el-icon><Setting /></el-icon> 设置
                </el-dropdown-item>
                <el-dropdown-item command="logout" divided>
                  <el-icon><SwitchButton /></el-icon> 退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main-content">
        <router-view />
      </el-main>
    </el-container>

    <!-- 移动形态导航：左侧覆盖抽屉，菜单项与侧栏同一份 SideMenu -->
    <el-drawer
      v-model="navOpen"
      direction="ltr"
      size="260px"
      :with-header="false"
      class="fm-nav-drawer"
    >
      <div class="drawer-logo">
        <img src="/logo.svg" alt="frpc-mesh" class="logo-img" />
        <div>
          <div class="logo-name">frpc-mesh</div>
          <div class="logo-slogan">多服务器 frp 管理</div>
        </div>
      </div>
      <SideMenu @navigate="navOpen = false" />
    </el-drawer>
  </el-container>
</template>
```

- [ ] **Step 3: 改 script**

`<script setup>` 里：把 `import { computed, onMounted, onUnmounted, ref } from 'vue'` 保持不变；在 `import { REPO_URL } from '../api/repo'` 之后加两行 import，并替换 `collapsed` 声明、加 `isMobile` / `navOpen` / `onCommand` 新分支：

```ts
import SideMenu from '../components/layout/SideMenu.vue'
import { useIsMobile, useIsNarrowDesktop } from '../composables/useMediaQuery'
```

```ts
const isMobile = useIsMobile()
const navOpen = ref(false)
// 768–1023px 窄桌面默认折叠侧栏；手机不渲染侧栏
const collapsed = ref(useIsNarrowDesktop().value)
```

`onCommand` 的 `if (cmd === 'settings') {` 之前插入两个新分支（其余原样保留）：

```ts
function onCommand(cmd: string) {
  if (cmd === 'repo') {
    window.open(REPO_URL, '_blank', 'noopener,noreferrer')
    return
  }
  if (cmd === 'version') return
  if (cmd === 'settings') {
```

- [ ] **Step 4: 加移动外壳样式**

`<style scoped>` 末尾追加（原有 `.side-menu` 相关规则已在 Task 3 Step 1 移进 SideMenu，此处删掉 MainLayout 里的 `.side-menu` 与 `.side-menu :deep(...)` 四条规则）：

```css
.menu-fill {
  flex: 1;
  overflow-y: auto;
}

.main-layout {
  height: 100vh;
  height: 100dvh;
}

.topbar {
  padding-top: env(safe-area-inset-top);
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.hamburger {
  width: 44px;
  height: 44px;
  border: none;
  border-radius: 10px;
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  touch-action: manipulation;
}

.hamburger .bars {
  position: relative;
  display: block;
  width: 18px;
  height: 2px;
  background: #1f2433;
}

.hamburger .bars::before,
.hamburger .bars::after {
  content: '';
  position: absolute;
  left: 0;
  width: 18px;
  height: 2px;
  background: #1f2433;
}

.hamburger .bars::before {
  top: -6px;
}

.hamburger .bars::after {
  top: 6px;
}

.main-content {
  -webkit-overflow-scrolling: touch;
}

.fm-nav-drawer :deep(.el-drawer__body) {
  background: linear-gradient(180deg, #171c2e 0%, #1a2038 100%);
  padding: 0;
  display: flex;
  flex-direction: column;
}

.drawer-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.drawer-logo .logo-img {
  width: 26px;
  height: 26px;
}
```

同时把原 `.main-layout { height: 100vh; }` 那一条删掉（已在上面用 dvh 双写替代）。

- [ ] **Step 5: 类型检查 + 构建 + 跑验收**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -3 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

Expected: 桌面三条仍 PASS（侧栏 220px、表格形态、无 `.fm-cards`）。移动溢出仍 FAIL 属正常（表格还没换卡片）。

- [ ] **Step 6: 手工确认汉堡抽屉可用**

浏览器开 `http://127.0.0.1:18096`，Chrome DevTools 设备模式 390×844：点汉堡 → 左侧抽屉滑出，5 项菜单齐全 → 点「设置」→ 抽屉自动关闭且地址变 `/settings`；顶栏不再出现版本号标签与仓库图标（它们进了头像下拉）。

- [ ] **Step 7: 提交**

```bash
git add web/src/components/layout/SideMenu.vue web/src/layouts/MainLayout.vue
git commit -m "feat(web): 汉堡抽屉导航，菜单项抽成 SideMenu 单一来源"
```

---

## Task 4: 下滑关闭与操作面板

**Files:**
- Create: `web/src/composables/useSwipeToClose.ts`
- Create: `web/src/components/mobile/ActionSheet.vue`

- [ ] **Step 1: 写 useSwipeToClose**

创建 `web/src/composables/useSwipeToClose.ts`：

```ts
import { onBeforeUnmount, watch, type Ref } from 'vue'

const THRESHOLD_PX = 80
const THRESHOLD_SPEED = 0.5

/**
 * 把手下滑关闭：位移 >80px 或速度 >0.5px/ms 才关闭，否则回弹。
 * 只跟手把手区（不绑整个弹层），避免与内层列表滚动抢手势。
 * 用 watch 而不是 onMounted：el-drawer 的内容是懒渲染的，挂载那一刻把手还不存在。
 */
export function useSwipeToClose(handle: Ref<HTMLElement | null>, onClose: () => void) {
  let panel: HTMLElement | null = null
  let startY = 0
  let startT = 0
  let dy = 0
  let tracking = false

  const onDown = (e: TouchEvent) => {
    if (e.touches.length !== 1) return
    startY = e.touches[0].clientY
    startT = Date.now()
    dy = 0
    tracking = true
  }
  const onMove = (e: TouchEvent) => {
    if (!tracking || e.touches.length !== 1) return
    dy = Math.max(0, e.touches[0].clientY - startY)
    // 拖的是整块面板，只动把手会出现「把手跑了、内容没动」的错觉
    if (panel) panel.style.transform = `translateY(${dy}px)`
    if (dy > 0 && e.cancelable) e.preventDefault()
  }
  const onEnd = () => {
    if (!tracking) return
    tracking = false
    const speed = dy / Math.max(1, Date.now() - startT)
    if (panel) panel.style.transform = ''
    if (dy > THRESHOLD_PX || speed > THRESHOLD_SPEED) onClose()
  }

  let bound: HTMLElement | null = null
  const unbind = () => {
    if (!bound) return
    bound.removeEventListener('touchstart', onDown)
    bound.removeEventListener('touchmove', onMove)
    bound.removeEventListener('touchend', onEnd)
    bound.removeEventListener('touchcancel', onEnd)
    bound = null
    panel = null
  }

  watch(
    handle,
    (el) => {
      unbind()
      if (!el) return
      bound = el
      panel = el.closest('.el-drawer') ?? el
      el.addEventListener('touchstart', onDown, { passive: true })
      // touchmove 必须非 passive：不 preventDefault 时页面会跟着滚，手势被判给滚动容器
      el.addEventListener('touchmove', onMove, { passive: false })
      el.addEventListener('touchend', onEnd)
      el.addEventListener('touchcancel', onEnd)
    },
    { immediate: true }
  )

  onBeforeUnmount(unbind)
}
```

- [ ] **Step 2: 写 ActionSheet**

创建 `web/src/components/mobile/ActionSheet.vue`：

```vue
<template>
  <el-drawer
    :model-value="modelValue"
    direction="btt"
    size="auto"
    :with-header="false"
    class="fm-sheet fm-actionsheet"
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div ref="handleEl" class="fm-handle"></div>
    <p v-if="title" class="as-title">{{ title }}</p>
    <button
      v-for="a in actions"
      :key="a.key"
      class="as-item"
      :class="{ danger: a.danger }"
      :disabled="a.disabled"
      @click="pick(a)"
    >
      <span>{{ a.label }}</span>
      <span v-if="a.hint" class="as-hint">{{ a.hint }}</span>
    </button>
    <button class="as-item as-cancel" @click="emit('update:modelValue', false)">取消</button>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useSwipeToClose } from '../../composables/useSwipeToClose'

export interface ActionItem {
  key: string
  label: string
  hint?: string
  danger?: boolean
  disabled?: boolean
}

const props = defineProps<{ modelValue: boolean; title?: string; actions: ActionItem[] }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; action: [string] }>()

const handleEl = ref<HTMLElement | null>(null)
useSwipeToClose(handleEl, () => emit('update:modelValue', false))

function pick(a: ActionItem) {
  if (a.disabled) return
  emit('action', a.key)
  emit('update:modelValue', false)
}
</script>
```

- [ ] **Step 3: 类型检查 + 构建**

Run: `cd web && pnpm type-check && pnpm build 2>&1 | tail -3`
Expected: 退出 0。

- [ ] **Step 4: 提交**

```bash
git add web/src/composables/useSwipeToClose.ts web/src/components/mobile/ActionSheet.vue
git commit -m "feat(web): 下滑关闭把手与移动端操作面板"
```

---

## Task 5: 触屏说明文字——FieldHint

**Files:**
- Create: `web/src/components/mobile/FieldHint.vue`

- [ ] **Step 1: 写组件**

创建 `web/src/components/mobile/FieldHint.vue`：

```vue
<template>
  <!-- 有 hover 的设备保持原样（桌面零回归）；触屏改成点击展开同一段文案 -->
  <el-tooltip v-if="hoverCapable" :content="content" :placement="placement">
    <el-icon class="fm-hint-icon"><QuestionFilled /></el-icon>
  </el-tooltip>
  <el-popover v-else :content="content" trigger="click" placement="top" :width="230">
    <template #reference>
      <button type="button" class="fm-hint-btn" aria-label="查看说明">?</button>
    </template>
  </el-popover>
</template>

<script setup lang="ts">
import { computed } from 'vue'

withDefaults(defineProps<{ content: string; placement?: 'top' | 'bottom' | 'left' | 'right' }>(), {
  placement: 'top'
})

const hoverCapable = computed(
  () => typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia('(hover: hover)').matches
)
</script>

<style scoped>
.fm-hint-icon {
  color: #909399;
  cursor: help;
}
</style>
```

- [ ] **Step 2: 类型检查 + 构建**

Run: `cd web && pnpm type-check && pnpm build 2>&1 | tail -3`
Expected: 退出 0。

- [ ] **Step 3: 提交**

```bash
git add web/src/components/mobile/FieldHint.vue
git commit -m "feat(web): FieldHint 让说明文字在触屏可点开"
```

---

## Task 6: 抽出四个共享 cell（桌面表格先改用，视觉零变化）

**Files:**
- Create: `web/src/components/mobile/ProxyRemoteCell.vue`
- Create: `web/src/components/mobile/ProxyLocalCell.vue`
- Create: `web/src/components/mobile/ProxyStatusCell.vue`
- Create: `web/src/components/mobile/ProxyLatencyCell.vue`
- Modify: `web/src/views/ProxiesView.vue:93-177`（四个列的 template 换成组件调用）

> 先做这一步，Task 8 的卡片列表就能直接复用同一批渲染，不出现「卡片一套逻辑、表格另一套」。

- [ ] **Step 1: ProxyRemoteCell**

创建 `web/src/components/mobile/ProxyRemoteCell.vue`（内容与 `ProxiesView.vue:141-167` 的列模板一致，只是数据来自 props）：

```vue
<template>
  <el-link
    v-if="(proxy.type === 'tcp' || proxy.type === 'udp') && proxy.remote_port"
    type="primary"
    :underline="false"
    class="mono"
    :href="`http://${remoteHost}:${proxy.remote_port}`"
    target="_blank"
  >
    {{ proxy.remote_port }}
  </el-link>
  <span v-else-if="proxy.type === 'tcp' || proxy.type === 'udp'" class="mono">随机</span>
  <el-link
    v-else-if="proxy.custom_domains"
    type="primary"
    :underline="false"
    class="mono"
    :href="`${proxy.type === 'https' ? 'https' : 'http'}://${proxy.custom_domains.split(',')[0].trim()}/`"
    target="_blank"
  >
    {{ proxy.custom_domains.split(',')[0].trim() }}
  </el-link>
  <span v-else-if="proxy.subdomain" class="mono">子域名: {{ proxy.subdomain }}</span>
  <span v-else class="text-muted">-</span>
</template>

<script setup lang="ts">
import type { Proxy } from '../../api/types'

defineProps<{ proxy: Proxy; remoteHost: string }>()
</script>
```

- [ ] **Step 2: ProxyLocalCell**

创建 `web/src/components/mobile/ProxyLocalCell.vue`（对应 `ProxiesView.vue:113-140`）：

```vue
<template>
  <el-link
    v-if="proxy.kind === 'visitor' && proxy.bind_port"
    type="primary"
    :underline="false"
    class="mono"
    :href="`http://${proxy.bind_addr || '127.0.0.1'}:${proxy.bind_port}`"
    target="_blank"
  >
    {{ proxy.bind_addr || '127.0.0.1' }}:{{ proxy.bind_port }}
  </el-link>
  <span v-else-if="proxy.kind === 'visitor'" class="mono">访问 → {{ proxy.server_name || '-' }}</span>
  <el-link
    v-else-if="proxy.local_port"
    type="primary"
    :underline="false"
    class="mono"
    :href="`http://${proxy.local_ip || '127.0.0.1'}:${proxy.local_port}`"
    target="_blank"
  >
    {{ proxy.local_ip || '127.0.0.1' }}:{{ proxy.local_port }}
  </el-link>
  <span v-else class="mono">{{ proxy.local_ip || '127.0.0.1' }}</span>
</template>

<script setup lang="ts">
import type { Proxy } from '../../api/types'

defineProps<{ proxy: Proxy }>()
</script>
```

- [ ] **Step 3: ProxyStatusCell**

创建 `web/src/components/mobile/ProxyStatusCell.vue`（对应 `ProxiesView.vue:93-103`）：

```vue
<template>
  <el-tag v-if="!proxy.enabled" type="warning" effect="plain" size="small" round>已禁用</el-tag>
  <el-tag v-else-if="online" type="success" effect="dark" size="small" round>在线</el-tag>
  <el-tag v-else type="info" effect="plain" size="small" round>离线</el-tag>
</template>

<script setup lang="ts">
import type { Proxy } from '../../api/types'

defineProps<{ proxy: Proxy; online: boolean }>()
</script>
```

- [ ] **Step 4: ProxyLatencyCell**

创建 `web/src/components/mobile/ProxyLatencyCell.vue`（对应 `ProxiesView.vue:168-177`；`latency` 语义沿用现有 `latencyOf`：`null` 未测、`-1` 不通、`0` 可通、其余为毫秒）：

```vue
<template>
  <span v-if="!enabled || latency === null" class="text-muted">--</span>
  <span v-else-if="latency === -1" class="lat-na">不通</span>
  <span v-else-if="latency === 0" class="lat-ok">可通</span>
  <span v-else :class="latency > 200 ? 'lat-slow' : 'lat-ok'">{{ latency }} ms</span>
</template>

<script setup lang="ts">
defineProps<{ enabled: boolean; latency: number | null }>()
</script>
```

> `.lat-na` / `.lat-ok` / `.lat-slow` 三个类现在定义在 `ProxiesView.vue` 的 scoped style 里。子组件根元素会带上父作用域 id，但这里类挂在子组件内部元素上，父级 scoped 规则不再生效——把这三条规则从 `ProxiesView.vue` 移到 `web/src/styles/global.css` 末尾：

```css
/* 延迟列语义（桌面表格与移动卡片共用，见 components/mobile/ProxyLatencyCell.vue） */
.lat-ok {
  color: #67c23a;
  font-weight: 600;
}

.lat-slow {
  color: #e6a23c;
  font-weight: 600;
}

.lat-na {
  color: #c0c4cc;
}
```

并从 `ProxiesView.vue` 的 `<style scoped>` 里删掉同名三条。

- [ ] **Step 5: ProxiesView 四个列改用组件**

`web/src/views/ProxiesView.vue`：

`ProxiesView.vue:93-103` 的状态列整段替换为：

```vue
            <el-table-column label="状态" width="80">
              <template #default="{ row }">
                <ProxyStatusCell :proxy="row" :online="isOnline(inst.id, row)" />
              </template>
            </el-table-column>
```

`ProxiesView.vue:113-140` 的本地地址列替换为：

```vue
            <el-table-column label="本地地址 / 访问摘要" min-width="170" show-overflow-tooltip>
              <template #default="{ row }">
                <ProxyLocalCell :proxy="row" />
              </template>
            </el-table-column>
```

`ProxiesView.vue:141-167` 的远程列替换为：

```vue
            <el-table-column label="远程端口 / 域名" min-width="130" show-overflow-tooltip>
              <template #default="{ row }">
                <ProxyRemoteCell :proxy="row" :remote-host="remoteHost(inst)" />
              </template>
            </el-table-column>
```

`ProxiesView.vue:168-177` 的延迟列替换为：

```vue
            <el-table-column label="延迟" width="80">
              <template #default="{ row }">
                <ProxyLatencyCell :enabled="row.enabled" :latency="latencyOf(inst.id, row)" />
              </template>
            </el-table-column>
```

import 区（`ProxiesView.vue:265-267` 附近，紧接 `import LogsTab` 之后）加：

```ts
import ProxyRemoteCell from '../components/mobile/ProxyRemoteCell.vue'
import ProxyLocalCell from '../components/mobile/ProxyLocalCell.vue'
import ProxyStatusCell from '../components/mobile/ProxyStatusCell.vue'
import ProxyLatencyCell from '../components/mobile/ProxyLatencyCell.vue'
```

- [ ] **Step 6: 桌面视觉确认 + 验收**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -3 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs 2>&1 | grep 桌面
```

Expected: 三条桌面断言 PASS；再肉眼对比 `/tmp/shots-mobile/desktop-proxies.png` 与改前截图——状态标签、远程端口链接、延迟文案（含 `--` / `不通` / `可通` / `N ms` 四态）位置与颜色都不变。

- [ ] **Step 7: 提交**

```bash
git add web/src/components/mobile/ProxyRemoteCell.vue web/src/components/mobile/ProxyLocalCell.vue web/src/components/mobile/ProxyStatusCell.vue web/src/components/mobile/ProxyLatencyCell.vue web/src/views/ProxiesView.vue web/src/styles/global.css
git commit -m "refactor(web): 代理状态/本地/远程/延迟四段渲染抽成共享 cell"
```

---

## Task 7: 服务器卡片列表 + 服务器页与仪表盘接入

**Files:**
- Create: `web/src/components/mobile/InstanceCardList.vue`
- Modify: `web/src/views/InstancesView.vue`
- Modify: `web/src/views/DashboardView.vue`

- [ ] **Step 1: 写卡片列表（纯展示，props + emits）**

创建 `web/src/components/mobile/InstanceCardList.vue`：

```vue
<template>
  <div class="fm-cards">
    <div v-for="row in rows" :key="row.id" class="fm-card">
      <div class="fm-card-row1">
        <span class="status-dot" :style="{ background: statusDotColor(row.status) }"></span>
        <span class="fm-card-name" @click="emit('detail', row.id)">{{ row.name }}</span>
        <span class="fm-card-state">{{ statusText(row.status) }}</span>
      </div>
      <div class="fm-card-row">
        <span class="fm-card-k">服务器</span>
        <span class="mono">{{ row.addr }}</span>
      </div>
      <div class="fm-card-sub">
        <span v-if="row.proxiesTotal != null">
          代理 <b>{{ row.proxiesOnline ?? '-' }}</b>/{{ row.proxiesTotal }} 在线
        </span>
        <span v-if="row.versionLabel" :class="{ 'fm-card-warn': row.versionFallback }">
          {{ row.versionLabel }}
          <!-- 桌面表格这段靠 hover tooltip 解释「为什么暂用别的版本」，触屏要能点开 -->
          <FieldHint v-if="row.versionFallback" :content="FRPC_VERSION_FALLBACK_TIP" />
        </span>
      </div>
      <!-- 桌面靠 hover tooltip 才看得见的失败原因，触屏必须有地方读；没有错误就不占一行 -->
      <div v-if="row.error" class="fm-card-err">{{ row.error }}</div>
      <div class="fm-card-foot">
        <span class="fm-card-k">{{ row.enabled ? '已启用' : '已停用' }}</span>
        <el-switch
          :model-value="row.enabled"
          :loading="row.toggling"
          @change="emit('toggle', row.id, $event as boolean)"
        />
        <button
          class="fm-more"
          :class="{ 'fm-more-text': moreLabel }"
          :aria-label="moreLabel ? undefined : '更多操作'"
          @click="emit('more', row.id)"
        >
          {{ moreLabel || '⋯' }}
        </button>
      </div>
    </div>
    <el-empty v-if="!rows.length" :description="emptyText" />
  </div>
</template>

<script setup lang="ts">
import { statusDotColor, statusText } from '../../utils/status'
import { FRPC_VERSION_FALLBACK_TIP } from '../../utils/frpcVersion'
import FieldHint from './FieldHint.vue'

export interface InstanceCardRow {
  id: number
  name: string
  status: string
  addr: string
  enabled: boolean
  toggling?: boolean
  proxiesTotal?: number | null
  proxiesOnline?: number | null
  versionLabel?: string
  /** 指定版本未生效（还在后台补下载），卡片用橙色 + FieldHint 说明原因 */
  versionFallback?: boolean
  /** 上次启动/运行失败原因，仅 error 态有值 */
  error?: string
}

withDefaults(defineProps<{ rows: InstanceCardRow[]; emptyText?: string; moreLabel?: string }>(), {
  moreLabel: ''
})
const emit = defineEmits<{
  detail: [number]
  toggle: [number, boolean]
  more: [number]
}>()
</script>

<style scoped>
.fm-card-state {
  font-size: 12px;
  color: var(--el-text-color-regular);
  flex: none;
}
.fm-card-sub b {
  color: #67c23a;
  font-weight: 600;
}
.fm-card-warn {
  color: var(--el-color-warning);
}
.fm-card-err {
  margin-top: 7px;
  padding: 6px 8px;
  border-radius: 7px;
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger);
  font-size: 11.5px;
  line-height: 1.45;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
```

> `.status-dot` 已存在于 `ProxiesView.vue` 的 scoped style。把它的规则也放进 `global.css`（同 Task 6 Step 4 的做法），并从 ProxiesView 的 scoped 里删掉，避免卡片里点不中：

```css
.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex: none;
}
```

- [ ] **Step 2: InstancesView 接入卡片 + 操作面板**

`web/src/views/InstancesView.vue`：

模板里 `<el-card class="card" shadow="never">` 到 `</el-card>`（`InstancesView.vue:11-123`）整块替换为：

```vue
    <el-card class="card" shadow="never">
      <template v-if="!isMobile">
        <el-table
          :data="store.instances"
          v-loading="store.loading"
          stripe
          empty-text="暂无服务器，点击右上角「新建 FRPS 服务器」开始"
        >
          <!-- 原 7 个 el-table-column 原样保留，一行都不改 -->
        </el-table>
      </template>

      <InstanceCardList
        v-else
        :rows="cardRows"
        empty-text="暂无服务器，点击下方「新建 FRPS 服务器」开始"
        @detail="goDetail"
        @toggle="onToggleEnabledById"
        @more="openActions"
      />
    </el-card>

    <ActionSheet
      v-model="sheetVisible"
      :title="sheetTitle"
      :actions="sheetActions"
      @action="onAction"
    />
```

> 注释那行是提醒：**桌面表格的 7 个列定义原封不动保留**，只是被包进 `<template v-if="!isMobile">`。

- [ ] **Step 3: InstancesView 的 script 增补**

import 区加：

```ts
import InstanceCardList, { type InstanceCardRow } from '../components/mobile/InstanceCardList.vue'
import ActionSheet, { type ActionItem } from '../components/mobile/ActionSheet.vue'
import { useIsMobile } from '../composables/useMediaQuery'
```

在 `const proxyStats = ...` 之后加：

```ts
const isMobile = useIsMobile()
const sheetVisible = ref(false)
const sheetId = ref(0)

const cardRows = computed<InstanceCardRow[]>(() =>
  store.instances.map((i) => ({
    id: i.id,
    name: i.name,
    status: i.runtime?.status || 'stopped',
    addr: `${i.server_addr}:${i.server_port}`,
    enabled: i.enabled,
    toggling: !!toggling[i.id],
    proxiesTotal: proxyStats[i.id]?.total ?? null,
    proxiesOnline: proxyStats[i.id]?.online ?? null,
    versionLabel: frpcVersionLabel(i, versionState.value),
    versionFallback: frpcVersionFallback(i, versionState.value),
    error: i.runtime?.status === 'error' ? i.runtime.last_error || '' : ''
  }))
)

const sheetTitle = computed(() => {
  const i = store.getInstance(sheetId.value)
  return i ? `${i.name} · ${statusText(i.runtime?.status)}` : ''
})

const sheetActions = computed<ActionItem[]>(() => {
  const i = store.getInstance(sheetId.value)
  return [
    { key: 'edit', label: '编辑' },
    { key: 'restart', label: '重启', hint: '隧道中断数秒后自动恢复' },
    {
      key: 'manage',
      label: '管理面板',
      disabled: !(i && i.manage_port > 0),
      hint: i && i.manage_port > 0 ? '' : '未配置管理端口'
    },
    { key: 'delete', label: '删除', danger: true }
  ]
})

function openActions(id: number) {
  sheetId.value = id
  sheetVisible.value = true
}

function onAction(key: string) {
  const row = store.getInstance(sheetId.value)
  if (!row) return
  if (key === 'edit') openEdit(row)
  else if (key === 'restart') onRestart(row)
  else if (key === 'manage') openManage(row)
  else if (key === 'delete') onDelete(row)
}

/** 卡片开关只给 id，复用既有 onToggleEnabled */
async function onToggleEnabledById(id: number, val: boolean) {
  const row = store.getInstance(id)
  if (row) await onToggleEnabled(row, val)
}
```

`statusText` 已在该文件的 import 里（`InstancesView.vue:138`），无需重复引入。

- [ ] **Step 4: DashboardView 接入卡片**

`web/src/views/DashboardView.vue`：把 `<el-card class="card" shadow="never">` 内的 `el-table`（`DashboardView.vue:33-105`）包进 `v-if="!isMobile"`，并加移动分支：

```vue
      <el-table v-if="!isMobile" :data="dash.instances" v-loading="store.loading" stripe empty-text="暂无 FRPS 服务器，请前往「FRPS 服务器」页面创建">
        <!-- 原列定义原样保留 -->
      </el-table>

      <InstanceCardList
        v-else
        :rows="dashRows"
        empty-text="暂无 FRPS 服务器，请前往「FRPS 服务器」页面创建"
        more-label="详情"
        @detail="goDetail"
        @toggle="onToggleEnabledById"
        @more="goDetail"
      />
```

script 加：

```ts
import InstanceCardList, { type InstanceCardRow } from '../components/mobile/InstanceCardList.vue'
import { useIsMobile } from '../composables/useMediaQuery'

const isMobile = useIsMobile()

const dashRows = computed<InstanceCardRow[]>(() =>
  dash.value.instances.map((i) => ({
    id: i.id,
    name: i.name,
    status: i.status,
    addr: `${i.server_addr}:${i.server_port}`,
    enabled: i.enabled,
    toggling: !!toggling[i.id],
    proxiesTotal: i.proxies_total,
    proxiesOnline: i.proxies_online,
    versionLabel: rowVersionLabel(i)
  }))
)

function goInstances() {
  router.push('/instances')
}

async function onToggleEnabledById(id: number, val: boolean) {
  const row = dash.value.instances.find((i) => i.id === id)
  if (row) await onToggleEnabled(row, val)
}
```

> 仪表盘卡片不配操作面板：`more-label="详情"` 把「⋯」换成文字按钮，与桌面表格那一列同一文案、同一 `goDetail` 跳转（规格 4.1）。`.fm-more-text` 必须写在 `.fm-more` 之后——同特异度靠书写顺序覆盖定宽。
>
> `dashRows` 不给 `error`：`/api/dashboard` 返回的 `DashboardInstance`（`web/src/api/types.ts:95-104`）没有 `last_error` 字段，仪表盘卡片显示空错误行不如不显示；要看失败原因走「详情」或服务器页卡片。

- [ ] **Step 5: 类型检查 + 构建 + 验收**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -3 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

Expected: `[/]`、`[/instances]` 的「移动形态不渲染表格」「移动形态渲染卡片列表」4 条转 PASS（`/proxies` 的 2 条仍 FAIL，Task 8 才修）；桌面三条仍 PASS。

- [ ] **Step 6: 提交**

```bash
git add web/src/components/mobile/InstanceCardList.vue web/src/views/InstancesView.vue web/src/views/DashboardView.vue web/src/styles/global.css
git commit -m "feat(web): 服务器卡片列表接入服务器页与仪表盘"
```

---

## Task 8: 代理页移动形态（chip 导航 + 卡片列表 + 操作面板）

**Files:**
- Create: `web/src/components/mobile/ProxyCardList.vue`
- Modify: `web/src/views/ProxiesView.vue`

- [ ] **Step 1: 写 ProxyCardList**

创建 `web/src/components/mobile/ProxyCardList.vue`：

```vue
<template>
  <div class="fm-cards">
    <div v-for="row in rows" :key="row.proxy.id" class="fm-card">
      <div class="fm-card-row1">
        <span class="type-badge" :style="{ background: proxyTypeColor(row.proxy.type) }">
          {{ proxyTypeLabel(row.proxy.type) }}
        </span>
        <span class="fm-card-name">{{ row.proxy.name }}</span>
        <ProxyStatusCell :proxy="row.proxy" :online="row.online" />
      </div>
      <div v-if="hasRemote(row.proxy)" class="fm-card-row">
        <span class="fm-card-k">远程</span>
        <ProxyRemoteCell :proxy="row.proxy" :remote-host="remoteHost" />
      </div>
      <div v-if="hasLocal(row.proxy) || row.latency !== null" class="fm-card-sub">
        <span v-if="hasLocal(row.proxy)">
          本地 <ProxyLocalCell :proxy="row.proxy" :linkable="false" />
        </span>
        <span v-if="row.latency !== null">
          延迟 <ProxyLatencyCell :enabled="row.proxy.enabled" :latency="row.latency" />
        </span>
      </div>
      <div class="fm-card-foot">
        <el-switch
          :model-value="row.proxy.enabled"
          :loading="row.toggling"
          @change="emit('toggle', row.proxy, $event as boolean)"
        />
        <button class="fm-more" aria-label="更多操作" @click="emit('more', row.proxy)">⋯</button>
      </div>
    </div>
    <el-empty v-if="!rows.length" :description="emptyText" />
  </div>
</template>

<script setup lang="ts">
import type { Proxy } from '../../api/types'
import { proxyTypeColor, proxyTypeLabel } from '../../utils/status'
import ProxyRemoteCell from './ProxyRemoteCell.vue'
import ProxyLocalCell from './ProxyLocalCell.vue'
import ProxyStatusCell from './ProxyStatusCell.vue'
import ProxyLatencyCell from './ProxyLatencyCell.vue'

export interface ProxyCardRow {
  proxy: Proxy
  online: boolean
  latency: number | null
  toggling?: boolean
}

/** 空字段不渲染：远程/本地两栏各自判断，避免出现「-」占位 */
function hasRemote(p: Proxy): boolean {
  return Boolean(p.remote_port || p.custom_domains || p.subdomain)
}
function hasLocal(p: Proxy): boolean {
  if (p.kind === 'visitor') return Boolean(p.bind_port || p.server_name)
  return Boolean(p.local_port || p.local_ip)
}

defineProps<{ rows: ProxyCardRow[]; remoteHost: string; emptyText?: string }>()
const emit = defineEmits<{
  toggle: [Proxy, boolean]
  more: [Proxy]
}>()
</script>

<style scoped>
.type-badge {
  color: #fff;
  font-size: 10.5px;
  font-weight: 700;
  letter-spacing: 0.4px;
  padding: 2px 6px;
  border-radius: 5px;
  flex: none;
}
</style>
```

> `.type-badge` 在 `ProxiesView.vue` 的 scoped style 里已有同名规则（桌面表格用）。这里子组件内的同名类不会被父 scoped 命中，所以卡片自带一份；两处规则内容一致，不产生视觉差异。

- [ ] **Step 2: ProxiesView 模板改成「导航 + 单一内容区」**

把 `ProxiesView.vue:16-201` 的 `<el-tabs v-else …>…</el-tabs>` 整块替换为下面两段（桌面用空壳 tabs 只做导航，内容区两形态共用一份）：

```vue
      <!-- 导航：桌面 = 空壳 el-tabs（内容不再塞进 pane，避免两套模板）；移动 = 吸顶 chip 条 -->
      <el-tabs v-if="!isMobile" v-model="activeTab" class="nav-tabs">
        <el-tab-pane v-for="inst in store.instances" :key="inst.id" :name="String(inst.id)">
          <template #label>
            <span class="tab-label">
              <span class="status-dot" :style="{ background: statusDotColor(inst.runtime?.status) }"></span>
              {{ inst.name }}
              <span class="tab-ver">{{ frpcVersionLabel(inst, versionState) }}</span>
            </span>
          </template>
        </el-tab-pane>
      </el-tabs>

      <div v-else class="fm-chips" role="tablist">
        <button
          v-for="inst in store.instances"
          :key="inst.id"
          class="fm-chip"
          :class="{ on: String(inst.id) === activeTab }"
          role="tab"
          @click="activeTab = String(inst.id)"
        >
          <span class="status-dot" :style="{ background: statusDotColor(inst.runtime?.status) }"></span>
          {{ inst.name }}
          <span class="chip-sub">{{ frpcVersionLabel(inst, versionState) }}</span>
        </button>
      </div>

      <!-- 内容：两形态共用 -->
      <template v-if="activeInstance">
        <div v-if="!isMobile" class="type-cards">
          <button
            v-for="t in TYPE_CARDS"
            :key="t.type"
            class="type-card"
            @click="openCreate(cur.id, t.type + ':proxy')"
          >
            <el-icon class="type-card-icon" :style="{ color: t.color }">
              <component :is="t.icon" />
            </el-icon>
            <span class="type-card-text">
              <span class="type-card-name">{{ t.name }}</span>
              <span class="type-card-sub">{{ t.sub }}</span>
            </span>
            <el-tooltip :content="`点击添加「${t.name} · ${t.sub}」代理`" placement="top">
              <span class="type-card-add"><el-icon :size="10"><Plus /></el-icon>创建</span>
            </el-tooltip>
          </button>
        </div>

        <div v-else class="fm-chips fm-type-chips">
          <button
            v-for="t in TYPE_CARDS"
            :key="t.type"
            class="fm-chip"
            @click="openCreate(cur.id, t.type + ':proxy')"
          >
            <span class="type-dot" :style="{ background: t.color }"></span>
            {{ t.name }}
            <span class="chip-sub">{{ t.sub }}</span>
          </button>
        </div>

        <div class="tab-toolbar">
          <div class="section-title">
            已配置代理
            <span class="text-muted section-count">
              （在线 {{ onlineCount(cur.id) }} / 共 {{ (proxiesMap[cur.id] || []).length }} 条）
            </span>
          </div>
          <div class="tab-toolbar-actions">
            <el-button size="small" text type="primary" class="toolbar-link" @click="openInfo(cur, 'config')">
              配置文件
            </el-button>
            <el-button size="small" text type="primary" class="toolbar-link" @click="openInfo(cur, 'logs')">
              运行日志
            </el-button>
            <el-button type="success" round :loading="!!pingingMap[cur.id]" @click="onPing(cur.id)">
              全部 Ping
            </el-button>
          </div>
        </div>

        <el-table
          v-if="!isMobile"
          :data="proxiesMap[cur.id] || []"
          :row-class-name="rowClassName"
          v-loading="!!loadingMap[cur.id]"
          size="small"
          stripe
          empty-text="暂无代理，点击上方类型卡片快速添加"
        >
          <!-- 原 8 个列定义原样保留（Task 6 已把四段渲染换成共享 cell） -->
        </el-table>

        <ProxyCardList
          v-else
          :rows="proxyCardRows"
          :remote-host="remoteHost(cur)"
          empty-text="暂无代理，点上方类型 chip 快速添加"
          @toggle="(p, v) => onToggleProxy(cur.id, p, v)"
          @more="openProxyActions"
        />
      </template>
```

> 注释那行同样是提醒：**桌面表格的列定义一行都不改**，只是从 tab pane 里挪到内容区，并被 `v-if="!isMobile"` 包住。

模板末尾（`</el-card>` 之后、`<ProxyFormDialog` 之前）加操作面板：

```vue
    <ActionSheet
      v-model="proxySheetVisible"
      :title="proxySheetTitle"
      :actions="proxySheetActions"
      @action="onProxyAction"
    />
```

- [ ] **Step 3: ProxiesView 的 script 增补**

import 区加：

```ts
import ProxyCardList, { type ProxyCardRow } from '../components/mobile/ProxyCardList.vue'
import ActionSheet, { type ActionItem } from '../components/mobile/ActionSheet.vue'
import { useIsMobile } from '../composables/useMediaQuery'
```

`Instance` 类型也要进来（`cur` 的断言用得到）：把 `ProxiesView.vue:276` 的 `import type { Proxy } from '../api/types'` 改成 `import type { Instance, Proxy } from '../api/types'`。

> **搬桌子时必须跟着改的一处**：原来 8 个列定义在 `v-for="inst in store.instances"` 的作用域里，现在挪到共用区后那个 `inst` 不存在了。逐列把模板里的 `inst.id` 换成 `cur.id`、`remoteHost(inst)` 换成 `remoteHost(cur)`：
>
> ```bash
> python3 - <<'PY'
> p = 'web/src/views/ProxiesView.vue'
> s = open(p).read()
> lo, hi = s.index('<el-table\n          v-if="!isMobile"'), s.index('</el-table>')
> seg = s[lo:hi].replace('inst.id', 'cur.id').replace('remoteHost(inst)', 'remoteHost(cur)')
> open(p, 'w').write(s[:lo] + seg + s[hi:])
> PY
> grep -n "inst\.id\|remoteHost(inst)" web/src/views/ProxiesView.vue
> ```
>
> Expected: grep 只剩 Tab 标签与 chip 条里的 `inst`（那两处仍在 `v-for` 作用域内，必须保留），表格列定义里一条不剩。

在 `const activeTab = ref('')` 之后加：

```ts
const isMobile = useIsMobile()
const proxySheetVisible = ref(false)
const proxySheetRow = ref<Proxy | null>(null)

/** 当前激活实例（chip 条与内容区都用它，替代原来 per-pane 的 inst） */
const activeInstance = computed(() => {
  const id = Number(activeTab.value) || 0
  return store.instances.find((i) => i.id === id) || null
})

/**
 * 内容区模板里用它取实例：`cur` 类型上非空。
 * 列定义在 `#default="{ row }"` 闭包里，`v-if="activeInstance"` 的空值收窄传不进去，
 * 直接写 `activeInstance.id` 会被 vue-tsc 判成可能为 null。渲染仍由那层 v-if 兜住。
 */
const cur = computed(() => activeInstance.value as Instance)

const proxyCardRows = computed<ProxyCardRow[]>(() => {
  const inst = activeInstance.value
  if (!inst) return []
  return (proxiesMap[inst.id] || []).map((p) => ({
    proxy: p,
    online: isOnline(inst.id, p),
    latency: latencyOf(inst.id, p),
    toggling: !!togglingProxy[p.id]
  }))
})

const proxySheetTitle = computed(() => {
  const p = proxySheetRow.value
  return p ? `${p.name} · ${proxyTypeLabel(p.type)}` : ''
})

const proxySheetActions = computed<ActionItem[]>(() => [
  { key: 'edit', label: '编辑' },
  { key: 'copy', label: '拷贝到其他服务器…' },
  { key: 'delete', label: '删除', danger: true, hint: '需再确认一次' }
])

function openProxyActions(p: Proxy) {
  proxySheetRow.value = p
  proxySheetVisible.value = true
}

function onProxyAction(key: string) {
  const inst = activeInstance.value
  const p = proxySheetRow.value
  if (!inst || !p) return
  if (key === 'edit') openEdit(inst.id, p)
  else if (key === 'copy') openCopy(inst.id, p)
  else if (key === 'delete') onDelete(inst.id, p)
}
```

- [ ] **Step 4: 补两条样式，让空壳 tabs 与 chip 条不留空隙**

`ProxiesView.vue` 的 `<style scoped>` 末尾加：

```css
.nav-tabs :deep(.el-tabs__header) {
  margin-bottom: 16px;
}

/* 内容已挪到两形态共用区，pane 是空的；EP 的 .el-tabs__content 带 overflow:hidden，留着会多一块空白 */
.nav-tabs :deep(.el-tabs__content) {
  display: none;
}

.type-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}

.fm-type-chips {
  padding-top: 0;
}
```

- [ ] **Step 5: 类型检查 + 构建 + 验收**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -3 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

Expected: `[/proxies]` 溢出 PASS；桌面三条 PASS（尤其「代理页仍是表格形态」——空壳 tabs 不影响 `el-table` 存在）。

- [ ] **Step 6: 桌面肉眼回归（这一步不能跳）**

打开 `http://127.0.0.1:18096/proxies`（桌面宽度），确认：Tab 标签、Tab 下方依次是类型卡片、工具条、表格；Tab 切换仍会同步 `?instance=`；点 Tab 不再出现空白内容区。与生产 `http://127.0.0.1:18080/proxies` 并排对比一次截图。

- [ ] **Step 7: 提交**

```bash
git add web/src/components/mobile/ProxyCardList.vue web/src/views/ProxiesView.vue
git commit -m "feat(web): 代理页移动形态（chip 导航 + 卡片列表 + 操作面板）"
```

---

## Task 9: 表单与弹窗改底部弹层（模板仍只有一份）

**Files:**
- Modify: `web/src/components/ProxyFormDialog.vue:2-10`（容器参数化）
- Modify: `web/src/components/InstanceFormDrawer.vue:2-9`（方向/尺寸/把手）
- Modify: `web/src/views/ProxiesView.vue:215-225`（配置文件/运行日志弹窗）、`ProxiesView.vue:228`（拷贝弹窗）、`web/src/views/SettingsView.vue:302`（版本说明弹窗）
- Modify: `web/src/components/ConfigTab.vue:42-60`、`web/src/components/LogsTab.vue`、`web/src/components/GuideSteps.vue:54-57`（终端高度与复制反馈）
- Modify: `web/src/styles/mobile.css`（`.fm-sheet-dialog` / `.fm-dialog-overlay`）

- [ ] **Step 1: ProxyFormDialog 容器参数化**

`web/src/components/ProxyFormDialog.vue` 根元素（`ProxyFormDialog.vue:2-10`）从：

```vue
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="720px"
    destroy-on-close
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
    @open="onOpen"
  >
```

改为：

```vue
  <component
    :is="container"
    :model-value="modelValue"
    v-bind="containerProps"
    @update:model-value="emit('update:modelValue', $event)"
    @open="onOpen"
  >
    <template #header>
      <div ref="handleEl" v-if="isMobile" class="fm-handle"></div>
      <span :class="isMobile ? 'fm-sheet-title' : 'el-dialog__title'">{{ dialogTitle }}</span>
    </template>
```

对应地，把文件里 `</el-dialog>`（`ProxyFormDialog.vue:252`）改成 `</component>`。

> 抽屉的头部没有 `el-dialog__title` 这条样式，所以按形态换类名：桌面沿用 EP 原类名保证零回归，手机用 `.fm-sheet-title`（样式在 Step 6 的 mobile.css 里）。抽屉与对话框都支持 `#header` 插槽（EP 2.6 起对话框有 `header`，抽屉有 `__header` 类名与插槽，本仓库锁 2.9.1）。

- [ ] **Step 2: ProxyFormDialog script**

import 区（`ProxyFormDialog.vue:255-258` 附近）加：

```ts
import { ElDialog, ElDrawer } from 'element-plus'
import { useIsMobile } from '../composables/useMediaQuery'
import { useSwipeToClose } from '../composables/useSwipeToClose'
```

在 `const saving = ref(false)` 之类状态附近加：

```ts
const isMobile = useIsMobile()
const handleEl = ref<HTMLElement | null>(null)
useSwipeToClose(handleEl, () => emit('update:modelValue', false))

// 桌面保持原 el-dialog 与 720px；手机换成贴底抽屉。模板只有一份，容器靠参数切。
const container = computed(() => (isMobile.value ? ElDrawer : ElDialog))
// 显式标注成 Record：两个分支属性名不同，union 传给 v-bind 会被 vue-tsc 挑刺
const containerProps = computed<Record<string, unknown>>(() =>
  isMobile.value
    ? { direction: 'btt', size: '92%', class: 'fm-sheet fm-form', destroyOnClose: true, appendToBody: true }
    : { width: '720px', destroyOnClose: true, appendToBody: true }
)
```

> `title` 不再传：两形态的标题都由 `#header` 插槽渲染，避免 EP 内部再渲染一份重复标题。

- [ ] **Step 3: InstanceFormDrawer 同理**

`web/src/components/InstanceFormDrawer.vue:2-9` 的 `<el-drawer … size="560px" …>` 改为：

```vue
  <el-drawer
    :model-value="modelValue"
    :title="isEdit ? '编辑 FRPS 服务器' : '新建 FRPS 服务器'"
    :direction="isMobile ? 'btt' : 'rtl'"
    :size="isMobile ? '92%' : '560px'"
    :class="isMobile ? 'fm-sheet fm-form' : ''"
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
    @open="onOpen"
  >
    <div v-if="isMobile" ref="handleEl" class="fm-handle"></div>
```

script 加：

```ts
import { useIsMobile } from '../composables/useMediaQuery'
import { useSwipeToClose } from '../composables/useSwipeToClose'

const isMobile = useIsMobile()
const handleEl = ref<HTMLElement | null>(null)
useSwipeToClose(handleEl, () => emit('update:modelValue', false))
```

并把 `label-width="110px"` 改为 `:label-width="isMobile ? '0px' : '110px'"`（两者都已是 `label-position="top"`，移动端不必留左标宽度）。

- [ ] **Step 4: 表单栅格在移动端变单列**

两个表单里所有 `<el-col :span="12">` 改成 `<el-col :xs="24" :span="12">`，`:span="8"` 改成 `:xs="24" :span="8"`，`:span="6"` 改成 `:xs="12" :span="6"`（端口/数值这类小字段在手机上两两并排仍可接受，宽度不足 12 时自动单列）。

用一条命令批量改并核对计数：

```bash
cd web/src/components && \
python3 - <<'PY'
import re
for f in ['ProxyFormDialog.vue', 'InstanceFormDrawer.vue']:
    s = open(f).read()
    s = s.replace('<el-col :span="12">', '<el-col :xs="24" :span="12">')
    s = s.replace('<el-col :span="8">', '<el-col :xs="24" :span="8">')
    s = s.replace('<el-col :span="6">', '<el-col :xs="12" :span="6">')
    s = s.replace('<el-col :span="14">', '<el-col :xs="24" :span="14">')
    s = s.replace('<el-col :span="10">', '<el-col :xs="24" :span="10">')
    open(f, 'w').write(s)
PY
grep -c ':xs=' ProxyFormDialog.vue InstanceFormDrawer.vue
```

Expected: 两个文件都有非零计数（ProxyFormDialog ≥5、InstanceFormDrawer ≥12）。

- [ ] **Step 5: 数值字段用数字键盘**

两个表单里的端口/池大小/心跳等字段全是 `el-input-number`（`ProxyFormDialog.vue:90/105/230`，`InstanceFormDrawer.vue:29/63/139/149/161/178/188/217`），EP 2.9 的 `el-input` 与 `el-input-number` 都有 `inputmode` 属性（已在 `node_modules/element-plus/es/components/input-number/src/input-number.d.ts:83` 核对）。iOS 上不带 `inputmode` 的 `type=number` 只会弹「数字与符号」键盘，点端口要切一次键盘。逐个手写容易漏，用一条命令给全部 `el-input-number` 补上：

```bash
cd web/src/components && python3 - <<'PY'
for f in ['ProxyFormDialog.vue', 'InstanceFormDrawer.vue']:
    s = open(f).read()
    n = s.count('<el-input-number')
    s = s.replace('<el-input-number', '<el-input-number inputmode="numeric"')
    open(f, 'w').write(s)
    print(f, '补了', n, '处')
PY
grep -c 'inputmode="numeric"' ProxyFormDialog.vue InstanceFormDrawer.vue
```

Expected: `ProxyFormDialog.vue 补了 3 处`、`InstanceFormDrawer.vue 补了 8 处`，grep 计数与之一致。

- [ ] **Step 6: 弹窗类（配置/日志/拷贝/版本说明）也贴底**

`ProxiesView.vue:215-225` 的 `infoVisible` 弹窗与 `ProxiesView.vue:228` 的 `copyVisible` 弹窗、`SettingsView.vue:302-305` 的版本说明弹窗，统一加两个属性：

```vue
      :width="isMobile ? '100%' : '780px'"
      :class="isMobile ? 'fm-sheet-dialog' : ''"
      modal-class="fm-dialog-overlay"
```

（`copyVisible` 桌面宽度是 `440px`、版本说明是 `640px`，各自保留原值，只在 `isMobile` 时换成 `100%`。`SettingsView.vue` 里 `isMobile` 需按 Step 3 的方式引入。）

`web/src/styles/mobile.css` 末尾加：

```css
/* 桌面 el-dialog 居中；移动端把 overlay 变成贴底容器，对话框变底部弹层 */
.fm-dialog-overlay {
  display: flex;
  align-items: flex-end;
  justify-content: center;
}
.fm-sheet-dialog {
  margin: 0 !important;
  width: 100% !important;
  max-width: none;
  border-radius: 16px 16px 0 0;
  max-height: 88vh;
  display: flex;
  flex-direction: column;
}
.fm-sheet-dialog .el-dialog__body {
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}
.fm-sheet-dialog .el-dialog__footer {
  padding-bottom: calc(10px + env(safe-area-inset-bottom));
}
```

- [ ] **Step 7: 配置文件 / 运行日志弹层内的终端与复制反馈**

`ConfigTab.vue:78-81` 与 `LogsTab.vue:166` 的终端区现在是 `height: calc(100vh - 420px); min-height: 320px`。桌面这么写没问题，手机上 `100vh` 比可视区高，88vh 的弹层里再塞一个 424px 高的终端会把「关闭」按钮顶出屏幕。两条 `<style scoped>` 末尾各加：

```css
@media (max-width: 767px) {
  .config-terminal {
    height: 46vh;
    min-height: 180px;
  }
}
```

（`LogsTab.vue` 里类名是 `.log-terminal`，同样写法。）

`ConfigTab.vue:42-60` 的 `copy()` 自己写了一份 clipboard + `execCommand`，且 `execCommand` 返回 `false` 时照样弹「已复制到剪贴板」——手机上会谎报成功。改回复用 `web/src/utils/clipboard.ts` 的三档兜底（与 `GuideSteps.vue:54-57` 同一套），顺便删掉重复代码：

```ts
import { copyText } from '../utils/clipboard'

async function copy() {
  if (!toml.value) {
    ElMessage.warning('没有可复制的内容')
    return
  }
  const r = await copyText(toml.value, document.querySelector('.config-terminal') as HTMLElement | undefined)
  if (r === 'copied') ElMessage.success('已复制到剪贴板')
  else if (r === 'selected') ElMessage.info('已选中配置内容，长按可复制')
  else ElMessage.error('复制失败，请长按配置文本手动选择复制')
}
```

`GuideSteps.vue:56` 那句「按 Ctrl/Cmd+C 复制」在手机上同样不成立（触屏没有这两个键）。`copyText` 已经返回了档位，措辞按是否有 hover 能力分开：

```ts
    else if (r === 'selected')
      ElMessage.info(
        matchMedia('(hover: hover)').matches
          ? '已选中该命令，按 Ctrl/Cmd+C 复制'
          : '已选中该命令，长按可复制'
      )
```

- [ ] **Step 8: 弹层里不套弹层**

`ProxiesView.vue` 的 `onDelete(instanceId, row)`（删除代理）与 `InstancesView.vue` 的 `onDelete(row)` 目前用 `ElMessageBox.confirm`。移动形态下它们是从 ActionSheet 关闭之后才弹出的，**不存在嵌套**（ActionSheet 在 `pick()` 里已先 `emit('update:modelValue', false)`）——保持现状即可，但要在浏览器里实测一次确认：ActionSheet 关闭动画与确认框弹出不能互相遮挡。若发现确认框被抽屉残影压住，把 ActionSheet 的关闭改为 `nextTick` 后再触发 `onDelete`：

```ts
function onProxyAction(key: string) {
  // …
  else if (key === 'delete') nextTick(() => onDelete(inst.id, p))
}
```

- [ ] **Step 9: 类型检查 + 构建 + 验收**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -3 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

Expected: 移动结构性断言 6 条全 PASS（三个表格页都换成卡片了）；溢出断言与桌面三条 PASS。

- [ ] **Step 10: 真机手感自查（390×844 设备模式）**

逐个确认：表单从底部升起、把手可见、下滑 100px 能关闭；字段单列、label 在上；点端口字段弹数字键盘（Chrome 设备模式会模拟）；「保存并应用」吸底且不被键盘盖住；填完提交后新代理出现在卡片列表里。

- [ ] **Step 11: 提交**

```bash
git add web/src/components/ProxyFormDialog.vue web/src/components/InstanceFormDrawer.vue \
  web/src/components/ConfigTab.vue web/src/components/LogsTab.vue web/src/components/GuideSteps.vue \
  web/src/views/ProxiesView.vue web/src/views/SettingsView.vue web/src/styles/mobile.css
git commit -m "feat(web): 表单与弹窗在移动端变成底部弹层"
```

---

## Task 10: 版本页 / 设置页 / 登录页适配 + 说明文字接 FieldHint

**Files:**
- Modify: `web/src/views/VersionsView.vue`
- Modify: `web/src/views/SettingsView.vue`
- Modify: `web/src/views/LoginView.vue`
- Modify: `web/src/views/DashboardView.vue:11-13`+`82-87`、`InstancesView.vue:69-74`、`InstanceFormDrawer.vue:90-95`、`SettingsView.vue:31-36`+`54-59`（六处 tooltip → FieldHint）

- [ ] **Step 1: 版本卡在移动端变两列**

`VersionsView.vue` 的版本卡不是 CSS grid，而是 `el-row`/`el-col`（`VersionsView.vue:54-63`，当前 `:xs="24" :sm="12" :md="8" :lg="6"`）。移动端单列会把十几张版本卡拉成一长串，改 `:xs` 为 12 让它两列排：

```vue
      <el-col
        v-for="v in displayVersions"
        :key="v.version"
        :xs="12"
        :sm="12"
        :md="8"
        :lg="6"
        class="version-col"
      >
```

卡片内边距靠 `:body-style` 内联样式给（`VersionsView.vue:68`），CSS 覆盖不掉，所以按形态取值（本 Task 需要在 `VersionsView.vue` 引入 `useIsMobile`，见 Step 4 的 import 块）：

```vue
        <el-card
          class="card version-card"
          shadow="never"
          :class="{ 'is-busy': busy }"
          :body-style="{ padding: isMobile ? '10px' : '12px 14px' }"
        >
```

> 这段里只有 `:body-style` 一行是新增改动，`class`/`shadow`/`:class` 都是文件里原有的，照抄别动。

极窄屏（≤359px，如 iPhone SE1 横向缩放）两列会把「使用此版本」挤到换行，这时退回单列。在 `<style scoped>` 末尾一并加：

```css
@media (max-width: 767px) {
  /* 顶部三项状态在窄屏靠换行排，间距收紧 */
  .status-items {
    gap: 16px;
  }
  /* 44 高的主按钮 + 「下载链接」在两列窄卡里需要换行余地 */
  .version-actions {
    flex-wrap: wrap;
    gap: 6px 8px;
  }
  .version-name {
    font-size: 15px;
  }
  .version-meta {
    gap: 6px;
    font-size: 11px;
  }
}

@media (max-width: 359px) {
  .version-col {
    flex: 0 0 100%;
    max-width: 100%;
  }
}
```

- [ ] **Step 2: 设置页窄屏收紧间距**

`SettingsView.vue` 已是 `:xs="24"`（两列自动折一列），实心主按钮与文字按钮的热区由 Task 1 的全局规则兜住（`.page-container .el-button--primary:not(.is-text)` ≥44、`.is-text` ≥32），这里只处理「当前版本/最新版本」并排的 40px 间距与升级引导折叠入口：

```css
@media (max-width: 767px) {
  .update-cols {
    gap: 16px;
  }
  .update-item {
    min-width: 108px;
  }
}
```

- [ ] **Step 3: 登录页窄屏**

`LoginView.vue` 现在用 `height: 100vh`（`LoginView.vue:110-112`）和固定 36px 内边距（`LoginView.vue:170-172`）：iOS Safari 的 `100vh` 比可视区高，底部「v1.1.1 / GitHub」会被地址栏压住；窄屏再留 36px 边距，输入区只剩 260px。`<style scoped>` 末尾加：

```css
@media (max-width: 767px) {
  .login-page {
    height: 100dvh;
    padding: 0 12px;
  }
  .login-card {
    width: 100%;
    max-width: 420px;
  }
  .login-card :deep(.el-card__body) {
    padding: 24px 18px 18px;
  }
  .login-header {
    margin-bottom: 18px;
  }
  .login-btn {
    min-height: 44px;
  }
}
```

- [ ] **Step 4: 把现有 hover tooltip 换成 FieldHint**

全站 `el-tooltip` 型的「问号说明」共 6 处（`DashboardView.vue:11-13` 统计卡、`DashboardView.vue:82-87` 表头「启用」、`InstancesView.vue:69-74` 表头「启用」、`InstanceFormDrawer.vue:90-95` 高级参数、`SettingsView.vue:31-36` auto_update_check、`SettingsView.vue:54-59` 检查新版本）。桌面靠 hover，触屏没有 hover——统一换成 `FieldHint`（`(hover: hover)` 仍是 tooltip，否则渲染成可点的 `?` + popover）。逐处把外层 `el-tooltip` 整块换成：

```vue
              <FieldHint content="启用：立即启动并随服务自动拉起；停用：立即停止且不自动启动" />
```

`DashboardView.vue:11-13` 的统计卡那处传的是变量，写法为 `<FieldHint :content="c.tip" />`。各文件 import 区加 `import FieldHint from '../components/mobile/FieldHint.vue'`（`InstanceFormDrawer.vue` 用 `./components/mobile/FieldHint.vue`）。`QuestionFilled` 若因此不再被使用，一并删掉 import（vue-tsc 会报未使用）。

`VersionsView.vue` 的 `useIsMobile`（Step 1 用到）单独加：

```ts
import { useIsMobile } from '../composables/useMediaQuery'

const isMobile = useIsMobile()
```

- [ ] **Step 5: 类型检查 + 构建 + 验收**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -3 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs
```

Expected: 移动段落全 PASS（含 6 条结构性断言）+ 桌面三条 PASS。

- [ ] **Step 6: 提交**

```bash
git add web/src/views web/src/components/InstanceFormDrawer.vue
git commit -m "feat(web): 版本/设置/登录页移动适配，说明文字改用 FieldHint"
```

---

## Task 11: 刷新去重与回前台立即刷新

**Files:**
- Modify: `web/src/stores/instances.ts:38-48`、`web/src/stores/instances.ts:73-82`

- [ ] **Step 1: 写失败断言**

`scripts/verify-mobile.mjs` 的 `run()` 里，桌面基线之前插入一段（断言「回前台只发一次列表请求」）：

```js
    // —— 回前台刷新去重 ——
    await S.setViewport(WIDTH, HEIGHT)
    await S.cmd('Page.navigate', { url: `${BASE}/instances` })
    await S.wait('document.querySelector(".fm-cards")', 'mobile instances')
    await S.cmd('Network.enable')
    const reqs = []
    ws.addEventListener('message', (e) => {
      const m = JSON.parse(e.data)
      if (m.method === 'Network.requestWillBeSent') {
        const u = m.params.request.url
        if (/\/api\/(instances|dashboard)/.test(u)) reqs.push(u.replace(BASE, ''))
      }
    })
    await S.js('Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true }); document.dispatchEvent(new Event("visibilitychange"))')
    await new Promise((r) => setTimeout(r, 400))
    await S.js('Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true }); document.dispatchEvent(new Event("visibilitychange"))')
    // 页面刚 navigate，10 秒轮询计时器是全新的；700ms 窗口内只有「回前台」这一轮刷新
    await new Promise((r) => setTimeout(r, 700))
    check('[移动] 回前台补刷一轮（instances+dashboard）', reqs.length === 2, `捕获 ${reqs.length} 次: ${reqs.join(',')}`)
```

- [ ] **Step 2: 跑一次确认 FAIL**

Run: `BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs 2>&1 | grep 回前台`
Expected: FAIL（现在还没有 visibilitychange 处理器，`reqs.length === 0`）。

- [ ] **Step 3: 实现去重与回前台刷新**

`web/src/stores/instances.ts`：把 `refreshSilently`（`instances.ts:38-48`）替换为带在途复用的版本：

```ts
  // 同一时刻只允许一个在途刷新：轮询、手动刷新、回前台刷新共用，避免互相覆盖
  let pending: Promise<void> | null = null

  async function doRefreshSilently() {
    try {
      const [list, dash] = await Promise.all([apiListInstances(true), apiDashboard(true)])
      instances.value = list || []
      dashboard.value = dash
    } catch {
      // 静默处理
    }
    loadFrpcVersions()
  }

  /** 静默刷新（轮询/回前台/操作后回刷），后端不可用时不打扰用户 */
  function refreshSilently() {
    if (!pending) pending = doRefreshSilently().finally(() => { pending = null })
    return pending
  }
```

把 `startPolling`（`instances.ts:73-82`）替换为：

```ts
  function startPolling() {
    if (started) return
    started = true
    refresh()
    timer = setInterval(() => {
      if (document.visibilityState === 'visible') refreshSilently()
    }, POLL_INTERVAL)
    // 手机切走再切回时立刻补一次，避免停在旧状态等下一个 10 秒
    document.addEventListener('visibilitychange', onVisible)
  }

  function onVisible() {
    if (document.visibilityState === 'visible') refreshSilently()
  }
```

`stopPolling` 里加一行摘掉监听：

```ts
  function stopPolling() {
    started = false
    document.removeEventListener('visibilitychange', onVisible)
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }
```

- [ ] **Step 4: 跑验收确认 PASS**

```bash
cd web && pnpm type-check && pnpm build 2>&1 | tail -2 && cd .. && \
docker build -q -t frpc-mesh:mobile-test . && docker rm -f fm-mobile-test >/dev/null 2>&1; \
rm -rf /tmp/fm-mobile-data && docker run -d --name fm-mobile-test -p 127.0.0.1:18096:18080 -v /tmp/fm-mobile-data:/data frpc-mesh:mobile-test >/dev/null && \
sleep 8 && BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs 2>&1 | grep 回前台
```

Expected: PASS。

- [ ] **Step 5: 后端测试不回归 + 提交**

```bash
export PATH="$HOME/.local/go/bin:$PATH" && go test ./... 2>&1 | grep -v "no test files" | tail -8 && \
git add web/src/stores/instances.ts scripts/verify-mobile.mjs && \
git commit -m "perf(web): 刷新在途去重，切回前台立即补一次"
```

---

## Task 12: 全量验收与修复循环

**Files:**
- Modify: `scripts/verify-mobile.mjs`（补齐剩余断言）

- [ ] **Step 1: 补齐断言清单**

下面是 `run()` 里移动段落的**现状全文**（`scripts/verify-mobile.mjs` 就是这段，跑通后 37 项全绿）。相对最初的草稿，实测中改了七处，都已在代码注释里落到位：

1. 触屏分支要靠 `Page.addScriptToEvaluateOnNewDocument` 重写 `matchMedia` 才测得到（无头 Chrome 的 `mobile: true` 不改 `(hover: hover)`），桌面段落前再摘掉。
2. 刚打开弹层就量位置/点坐标会撞上 300ms 开合动画，统一补 600–700ms 落定。
3. 表单弹层用专用标记 `.fm-form` 选，`.fm-sheet` / `.el-dialog` 会先命中已关闭但仍在 DOM 里的容器。
4. 输入框高度量 `.el-input__wrapper`，字号量 `.el-input__inner`。
5. 移动形态下「类型卡」本身就是新建入口（点一下即 `openCreate`），没有单独的「新增代理」按钮，别再去找它。
6. 填字段按 `.el-form-item__label` 的文字定位，placeholder 里没有「名称」两个字。
7. 删除断言点自己新建的那张卡（`tapInCard`），`seed()` 按名字补齐实例**和代理**，否则每轮吃掉一条演示数据、后面页面直接空列表。

```js
    // 无头 Chrome 的 Emulation.setEmulatedMedia 实测改不动 hover/pointer（恒为 hover:true），
    // 而真手机是 hover:none；FieldHint（问号 tooltip 还是可点浮层）与复制兜底文案都靠它判定，
    // 所以在文档加载前把这个判定替成手机的值。
    const hoverPatch = await S.cmd('Page.addScriptToEvaluateOnNewDocument', {
      source: `(() => {
        const orig = window.matchMedia.bind(window)
        const fake = (q, val) => ({
          media: q, matches: val, onchange: null,
          addEventListener: () => {}, removeEventListener: () => {},
          addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false
        })
        window.matchMedia = (q) => {
          const key = String(q).replace(/\\s+/g, '')
          if (key === '(hover:hover)') return fake(q, false)
          if (key === '(pointer:coarse)') return fake(q, true)
          return orig(q)
        }
      })()`
    })

    // —— 触摸热区：主要可点元素 ≥44、次要文字按钮 ≥32（逐页审，含汉堡与卡片「⋯」）——
    const tapBad = []
    for (const path of PAGES) {
      await S.cmd('Page.navigate', { url: BASE + path })
      await S.wait('document.querySelector(".page-container")', `tap audit ${path}`)
      await new Promise((r) => setTimeout(r, 500))
      const t = await S.js(`(() => {
        const PRIM = '.fm-more, .fm-chip, .hamburger, .el-button--primary:not(.is-text):not(.is-link), .el-button--success:not(.is-text):not(.is-link), .fm-card .el-switch';
        const SECOND = '.el-button.is-text, .el-button.is-link';
        const bad = (sel, min) => [...document.querySelectorAll(sel)]
          .filter((el) => { const r = el.getBoundingClientRect(); return r.width > 0 && Math.round(r.height) < min; })
          .map((el) => (el.innerText || '').trim().slice(0, 6) + '|' + String(el.getAttribute('class') || '') + ' h=' + Math.round(el.getBoundingClientRect().height));
        return { primary: bad(PRIM, 44), secondary: bad(SECOND, 32) };
      })()`)
      for (const x of t.primary) tapBad.push(`${path} 主:${x}`)
      for (const x of t.secondary) tapBad.push(`${path} 次:${x}`)
    }
    check('[移动] 触摸热区达标（主44/次32）', tapBad.length === 0, tapBad.slice(0, 4).join(' | '))

    // 汉堡与侧栏互换
    await S.cmd('Page.navigate', { url: `${BASE}/` })
    await S.wait('document.querySelector(".hamburger")', 'hamburger')
    check('[移动] 汉堡入口出现且侧栏不占位', await S.js(
      `!!document.querySelector('.hamburger') && !document.querySelector('.el-aside')`
    ))

    // 操作面板：项与桌面一致、行高够点、可取消
    await S.cmd('Page.navigate', { url: `${BASE}/instances` })
    await S.wait('document.querySelector(".fm-more")', 'instance cards')
    await tapAt(S, '.fm-more')
    await S.wait('document.querySelector(".fm-actionsheet")', 'action sheet open')
    const asOK = await S.js(`(() => {
      const items = [...document.querySelectorAll('.fm-actionsheet .as-item')].map((e) => e.innerText.trim().split('\\n')[0]);
      const hs = [...document.querySelectorAll('.fm-actionsheet .as-item')].map((e) => Math.round(e.getBoundingClientRect().height));
      return { items, hs, tall: hs.every((h) => h >= 44) };
    })()`)
    check('[移动] 操作面板项与桌面一致', ['编辑', '重启', '管理面板', '删除'].every((x) => asOK.items.includes(x)), JSON.stringify(asOK.items))
    check('[移动] 操作面板行高 ≥44', asOK.tall, JSON.stringify(asOK.hs))
    await tapAt(S, '.fm-actionsheet .as-cancel')
    await new Promise((r) => setTimeout(r, 700))
    // el-drawer 关闭后节点仍留在 DOM（只是隐藏），按高度为 0 判关闭
    check('[移动] 操作面板可取消关闭', await S.js(
      `(() => { const d = document.querySelector('.fm-actionsheet');` +
      ` return !d || d.getBoundingClientRect().height === 0 })()`
    ))

    // 卡片数 == 后端代理数
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-more")', 'proxy cards')
    const counts = await S.js(`document.querySelectorAll('.fm-cards .fm-card').length`)
    const api = await S.js(
      `(async () => { const t = localStorage.getItem('frpc_mesh_token');` +
      ` const r = await fetch('/api/instances', { headers: { Authorization: 'Bearer ' + t } });` +
      ` const list = await r.json();` +
      ` const r2 = await fetch('/api/instances/' + list[0].id + '/proxies', { headers: { Authorization: 'Bearer ' + t } });` +
      ` return (await r2.json()).length })()`
    )
    check('[移动] 卡片条数与后端一致', counts === api, `卡片 ${counts} / 接口 ${api}`)

    // 表单弹层：贴底、字段单列、控件够大、保存可见
    await tapAt(S, '.fm-chips:not(.fm-type-chips) .fm-chip')
    await S.wait('document.querySelector(".fm-type-chips")', 'type chips')
    // 移动形态下「类型卡」本身就是新建入口（点一下即 openCreate），没有单独的「新增代理」按钮
    await tapAt(S, '.fm-type-chips .fm-chip')
    // fm-form 是表单弹层专用标记：ActionSheet 也带 .fm-sheet、隐藏的信息弹窗也带 .el-dialog，
    // 用那两个选择器会先量到一个不可见容器，读数没有意义
    await S.wait('document.querySelector(".fm-form")', 'form sheet')
    // el-drawer 有 300ms 开启动画，动画里 rect 还在位移，量早了会误判「按钮不在屏内」
    await new Promise((r) => setTimeout(r, 700))
    const formOK = await S.js(`(() => {
      const sheet = document.querySelector('.fm-form');
      const cols = [...sheet.querySelectorAll('.el-col')];
      const narrow = cols.filter((c) => c.getBoundingClientRect().width < window.innerWidth * 0.6).length;
      // 高度量 .el-input__wrapper（inner 只有文字行高，量它会误判成不达 44）
      const wraps = [...sheet.querySelectorAll('.el-input__wrapper')];
      const short = wraps.filter((w) => Math.round(w.getBoundingClientRect().height) < 44).length;
      const tiny = [...sheet.querySelectorAll('.el-input__inner')].filter((i) => parseFloat(getComputedStyle(i).fontSize) < 16).length;
      const foot = sheet.querySelector('.el-drawer__footer, .dialog-footer');
      return {
        sheetH: Math.round(sheet.getBoundingClientRect().height),
        cols: cols.length, narrow, wraps: wraps.length, short, tiny,
        footVisible: !!foot && foot.getBoundingClientRect().bottom <= window.innerHeight + 1
      };
    })()`)
    check('[移动] 表单贴底、字段单列', formOK.sheetH > 500 && formOK.cols > 0 && formOK.narrow === 0, JSON.stringify(formOK))
    check('[移动] 输入框 44 高且字号≥16（防 iOS 聚焦放大）', formOK.wraps > 0 && formOK.short === 0 && formOK.tiny === 0, JSON.stringify(formOK))
    check('[移动] 保存按钮吸底可见', formOK.footVisible, JSON.stringify(formOK))
    await S.shot('form-sheet.png')

    // 服务器表单里的「frpc 版本」说明：触屏必须是可点的问号，而不是只有 hover 才出现的 tooltip
    await S.cmd('Page.navigate', { url: `${BASE}/instances` })
    await S.wait('document.querySelector(".fm-more")', 'instance cards for form')
    await tapAt(S, '.fm-more')
    await S.wait('document.querySelector(".fm-actionsheet")', 'action sheet for edit')
    await tapText(S, '编辑', `document.querySelector('.fm-actionsheet')`)
    await S.wait('document.querySelector(".fm-form")', 'instance form sheet')
    await new Promise((r) => setTimeout(r, 500))
    const hintInForm = await S.js(`(() => {
      const sheet = document.querySelector('.fm-form');
      return { label: [...sheet.querySelectorAll('.el-form-item__label')].some((l) => /frpc 版本/.test(l.innerText)),
        qm: sheet.querySelectorAll('.fm-hint-btn').length,
        tip: sheet.querySelectorAll('.fm-hint-icon').length };
    })()`)
    check('[移动] 表单说明是触屏可点的问号', hintInForm.label && hintInForm.qm >= 1 && hintInForm.tip === 0, JSON.stringify(hintInForm))
    await tapAt(S, '.fm-form .fm-hint-btn')
    await new Promise((r) => setTimeout(r, 500))
    check('[移动] 表单问号点得开', await S.js(
      `!!([...document.querySelectorAll('.el-popover')].some((p) => p.innerText.includes('默认版本')))`
    ), '浮层里没有说明文字')

    // 设置页的说明同样靠问号读（服务器/仪表盘的表头说明只在桌面出现，故这里用设置页）
    await S.cmd('Page.navigate', { url: `${BASE}/settings` })
    await S.wait('document.querySelector(".fm-hint-btn")', 'hint button')
    await tapAt(S, '.fm-hint-btn')
    await new Promise((r) => setTimeout(r, 500))
    check('[移动] 说明文字点得开、看得到', await S.js(
      `!!([...document.querySelectorAll('.el-popover')].some((p) => p.innerText.includes('GitHub Releases')))`
    ), '浮层里找不到说明文字')

    // 安全区：viewport 声明 + 样式里真的有 env()（无头 Chrome 解析后恒为 0，只能查声明串）
    check('[移动] viewport 有 viewport-fit=cover', await S.js(
      `document.querySelector('meta[name=viewport]').content.includes('viewport-fit=cover')`
    ))
    check('[移动] 样式里真的写了 env(safe-area-inset)', await S.js(
      `[...document.styleSheets].some((sh) => { try {` +
      ` return [...sh.cssRules].some((r) => /env\\(safe-area-inset/.test(r.cssText))` +
      ` } catch (e) { return false } })`
    ), '没有任何规则引用 env(safe-area-inset)')

    // 卡片上真实新建一条代理（走后端，验证表单在手机上可用）
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-type-chips")', 'type chips for create')
    const before = await S.js(`document.querySelectorAll('.fm-cards .fm-card').length`)
    await tapAt(S, '.fm-type-chips .fm-chip')
    await S.wait('document.querySelector(".fm-form")', 'create form')
    await new Promise((r) => setTimeout(r, 600))
    // 按「表单项标签」定位而不是 placeholder：placeholder 里没有「名称」两个字
    const filled = await S.js(`(() => {
      const set = (el, v) => {
        const d = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(el), 'value')
        d.set.call(el, v)
        el.dispatchEvent(new Event('input', { bubbles: true }))
        el.dispatchEvent(new Event('change', { bubbles: true }))
      }
      const labelOf = (it) => { const l = it.querySelector('.el-form-item__label'); return l ? l.innerText : '' }
      const items = [...document.querySelectorAll('.fm-form .el-form-item')]
      const done = []
      for (const [re, val] of [[/代理名称/, 'cdp-mobile-prox'], [/本地端口/, '8088'], [/远程端口/, '22099']]) {
        const it = items.find((x) => re.test(labelOf(x)))
        const input = it && it.querySelector('.el-input__inner')
        if (input) { set(input, val); done.push(labelOf(it).slice(0, 4) + '=' + val) }
      }
      return done.join(',')
    })()`)
    check('[移动] 表单字段按标签填得进', /cdp-mobile-prox/.test(filled) && /本地端口/.test(filled), `已填: ${filled}`)
    // 新建是「添加并应用」、编辑是「保存并应用」，按共有的「并应用」匹配
    await tapText(S, '并应用', `document.querySelector('.fm-form')`)
    await S.wait(`!document.querySelector('.fm-form') || document.querySelector('.fm-form').getBoundingClientRect().height === 0`, 'form closed after save')
    await new Promise((r) => setTimeout(r, 900))
    const after = await S.js(`document.querySelectorAll('.fm-cards .fm-card').length`)
    const hasNew = await S.js(
      `[...document.querySelectorAll('.fm-cards .fm-card')].some((c) => c.innerText.includes('cdp-mobile-prox'))`
    )
    check('[移动] 卡片列表能真实新建代理', after === before + 1 && hasNew, `前 ${before} 后 ${after}，含新卡片=${hasNew}`)

    // 删除：ActionSheet → 删除 → 必须出现二次确认，确认后才减少
    await tapInCard(S, 'cdp-mobile-prox', '.fm-more')
    await S.wait('document.querySelector(".fm-actionsheet")', 'action sheet')
    await tapText(S, '删除', `document.querySelector('.fm-actionsheet')`)
    await S.wait('document.querySelector(".el-message-box")', 'second confirm')
    const dangerConfirm = await S.js(
      `(() => { const b = document.querySelector('.el-message-box .el-button--danger');` +
      ` return b && b.innerText.includes('删除') })()`
    )
    check('[移动] 删除保留二次确认', !!dangerConfirm, '未找到红色「删除」确认按钮')
    // ActionSheet 的关闭动画还在跑时，它的遮罩会吃掉确认框上的点击，等它落定
    await new Promise((r) => setTimeout(r, 800))
    await tapText(S, '删除', `document.querySelector('.el-message-box__btns')`)
    await new Promise((r) => setTimeout(r, 1200))
    check('[移动] 确认后卡片减少', await S.js(
      `document.querySelectorAll('.fm-cards .fm-card').length === ${before}`
    ), `回到 ${before} 条`)

    // 配置文件底部弹层 + http 剪贴板兜底
    // 重新进页面：删除留下的提示条浮在顶部会挡住工具栏的坐标点击
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".tab-toolbar-actions")', 'toolbar again')
    await new Promise((r) => setTimeout(r, 400))
    await tapText(S, '配置文件')
    await S.wait('document.querySelector(".fm-sheet-dialog .config-terminal")', 'config sheet')
    // 弹层开合有动画，动画没落定就点会打偏
    await new Promise((r) => setTimeout(r, 600))
    const termH = await S.js(
      `(() => { const t = document.querySelector('.fm-sheet-dialog .config-terminal');` +
      ` const r = t.getBoundingClientRect(); return { h: Math.round(r.height), inView: r.bottom <= window.innerHeight + 1 } })()`
    )
    check('[移动] 配置文件终端在弹层内不超出可视区', termH.inView && termH.h > 120, JSON.stringify(termH))
    await tapText(S, '复制', `document.querySelector('.fm-sheet-dialog')`)
    await new Promise((r) => setTimeout(r, 400))
    check('[移动] http 下复制给出已复制/已选中反馈', await S.js(
      `!![...document.querySelectorAll('.el-message')].find((m) => /已复制|已选中/.test(m.innerText))`
    ), '没有出现复制反馈消息')

    // 汉堡抽屉：打开后 5 项菜单与桌面一致，点一项能跳转且抽屉关闭
    await S.cmd('Page.navigate', { url: `${BASE}/versions` })
    await S.wait('document.querySelector(".hamburger")', 'hamburger')
    await tapAt(S, '.hamburger')
    await S.wait('document.querySelector(".fm-nav-drawer .el-menu-item")', 'drawer menu')
    await new Promise((r) => setTimeout(r, 600))
    const menuNames = await S.js(
      `[...document.querySelectorAll('.fm-nav-drawer .el-menu-item')].map((e) => e.innerText.trim().replace(/\\s+/g, ''))`
    )
    check(
      '[移动] 抽屉菜单与桌面同 5 项',
      JSON.stringify(menuNames) === JSON.stringify(['仪表盘', 'FRPS服务器', '穿透代理', 'frpc版本管理', '设置']),
      JSON.stringify(menuNames)
    )
    await tapText(S, '穿透代理', `document.querySelector('.fm-nav-drawer')`, '.el-menu-item')
    await new Promise((r) => setTimeout(r, 900))
    const navState = await S.js(
      `(() => { const d = document.querySelector('.fm-nav-drawer');` +
      ` return { path: location.pathname, h: d ? Math.round(d.getBoundingClientRect().height) : -1 } })()`
    )
    check('[移动] 点菜单跳转后抽屉自动关闭', navState.path === '/proxies' && navState.h <= 0, JSON.stringify(navState))

    // —— 回前台刷新去重 ——
    await S.setViewport(WIDTH, HEIGHT)
    await S.cmd('Page.navigate', { url: `${BASE}/instances` })
    await S.wait('document.querySelector(".fm-cards")', 'mobile instances')
    await S.cmd('Network.enable')
    const reqs = []
    ws.addEventListener('message', (e) => {
      const m = JSON.parse(e.data)
      if (m.method === 'Network.requestWillBeSent') {
        const u = m.params.request.url
        if (/\/api\/(instances|dashboard)/.test(u)) reqs.push(u.replace(BASE, ''))
      }
    })
    await S.js('Object.defineProperty(document, "visibilityState", { value: "hidden", configurable: true }); document.dispatchEvent(new Event("visibilitychange"))')
    await new Promise((r) => setTimeout(r, 400))
    await S.js('Object.defineProperty(document, "visibilityState", { value: "visible", configurable: true }); document.dispatchEvent(new Event("visibilitychange"))')
    // 页面刚 navigate，10 秒轮询计时器是全新的；700ms 窗口内只有「回前台」这一轮刷新
    await new Promise((r) => setTimeout(r, 700))
    check('[移动] 回前台补刷一轮（instances+dashboard）', reqs.length === 2, `捕获 ${reqs.length} 次: ${reqs.join(',')}`)

```

> `menuNames` 的期望值按 `SideMenu.vue` 里 5 项的真实标题填（Task 3 定的就是「仪表盘 / FRPS 服务器 / 穿透代理 / frpc 版本管理 / 设置」），文字里含空格所以先 `replace(/\s+/g,'')` 再比。抽屉关闭靠 `el-drawer` 卸载，`destroy-on-close` 后节点消失，故用「查不到节点」判定。

- [ ] **Step 2: 跑全量**

Run: `BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs`
Expected: 全绿。任何 FAIL 都回到对应 Task 修，修完重跑，不许跳过。

- [ ] **Step 3: 三档视口 + 微信 UA 各跑一遍**

```bash
for W in 375 390 430; do
  echo "== 视口 $W =="
  MOBILE_WIDTH=$W CDP_PORT=$((9300 + W)) SHOT_DIR=/tmp/shots-mobile-$W \
    BASE=http://127.0.0.1:18096 node scripts/verify-mobile.mjs 2>&1 | tail -3
done
```

Expected: 三档都 0 失败（脚本结尾会打印「共 N 项，失败 0」）。每档用独立 CDP 端口与截图目录，互不覆盖。

微信内置浏览器（iPhone 版 MicroMessenger UA）单独跑一遍，重点看 chip 条吸顶、底部弹层手势、剪贴板兜底是否与 Mobile Safari 一致：

```bash
MOBILE_UA='Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.49(0x18003129) NetType/WIFI Language/zh_CN' \
  CDP_PORT=9444 SHOT_DIR=/tmp/shots-mobile-wechat BASE=http://127.0.0.1:18096 \
  node scripts/verify-mobile.mjs 2>&1 | tail -3
```

Expected: 同样 0 失败。若这一档独有失败（例如滚动锁定不生效），按 `docs/superpowers/specs/2026-11-04-mobile-adaptation-design.md` 第 9 节的兼容表定位。

- [ ] **Step 4: 后端与桌面全量回归**

```bash
cd /Users/busfly/Documents/code_workspace/Frpc-Mesh && export PATH="$HOME/.local/go/bin:$PATH" && \
go vet ./... && go test ./... -count=1 2>&1 | grep -v "no test files" && \
cd web && pnpm type-check && pnpm build 2>&1 | tail -2
```

Expected: 全 ok，构建成功。

- [ ] **Step 5: 提交**

```bash
git add scripts/verify-mobile.mjs
git commit -m "test(web): 移动端全量断言（触摸目标/操作面板/表单/说明/安全区）"
```

---

## Task 13: 文档、版本号与本地打包

**Files:**
- Modify: `README.md`、`README-CN.md`、`docs/deployment.md`、`docs/operations.md`
- Modify: `Makefile:4`、`main.go:32`、`docker-compose.yml:7`、`Dockerfile:16`（版本号，取值待用户确认）

- [ ] **Step 1: 文档补一句移动端**

`README-CN.md` 特性列表里，在「自身版本更新」那条之后加：

```
- **手机可用**：窄屏自动切换卡片列表 + 底部弹层 + 汉堡抽屉，全功能（含新建/编辑服务器与代理），iOS Safari / 微信内置浏览器 / Android Chrome 实测
```

`README.md` 对应位置加：

```
- **Usable on phones** — narrow viewports switch to card lists, bottom sheets and a hamburger drawer, full featured including creating and editing servers and proxies; verified on iOS Safari, WeChat's webview and Android Chrome
```

`docs/deployment.md`「首次登录」小节末尾加：

```
- 手机访问：`http://<主机IP>:18080` 即可，界面会自动切到移动版（卡片 + 底部弹层）。用 http 时 iPhone 的「安全键盘」与剪贴板受限，复制命令会走兜底（选中后手动复制）
```

`docs/operations.md` 第 1 节之后加一小节：

```
## 2.x 手机访问说明

- 移动版是同一套页面的窄屏形态，不额外部署、没有独立地址
- 手机上「新建/编辑」表单从底部升起，可下滑关闭；保存按钮吸底，键盘弹起时先点空白处收起键盘再保存
- 状态不只靠颜色：色点旁一定有文字（在线/离线/启动中/已停用）
- 面板切到后台时停止轮询省电，切回前台会立刻刷新一次
```

- [ ] **Step 2: 版本号（本轮取 patch，即 1.1.2；要跳中间版本必须先问用户）**

四处版本号都写死着 `1.1.1`（`Makefile:4`、`main.go:32`、`docker-compose.yml:7`、`Dockerfile:16`），逐处替换为新值。按既有规则默认 `1.1.2`；**若用户在验收时认为「移动端算大功能」要走 `1.2.0`，改 `NEW_VER` 即可，其余命令不变**：

```bash
cd /Users/busfly/Documents/code_workspace/Frpc-Mesh && NEW_VER=1.1.2 && \
python3 - "$NEW_VER" <<'PY'
import sys
new = sys.argv[1]
sites = {
    'Makefile': [('VERSION   ?= 1.1.1', f'VERSION   ?= {new}')],
    'main.go': [
        ('var Version = "1.1.1"', f'var Version = "{new}"'),
        # 注释里的示例值也一起改，否则收尾的 grep 会把它当成残留
        ('-X main.Version=1.1.1', f'-X main.Version={new}'),
    ],
    'docker-compose.yml': [('VERSION: "${VERSION:-1.1.1}"', f'VERSION: "${{VERSION:-{new}}}"')],
    'Dockerfile': [('ARG VERSION=1.1.1', f'ARG VERSION={new}')],
}
for f, subs in sites.items():
    s = open(f).read()
    for old, rep in subs:
        assert old in s, f'{f} 里找不到 {old!r}'
        s = s.replace(old, rep)
    open(f, 'w').write(s)
    print('updated', f)
PY
grep -rn "1\.1\.1" Makefile main.go docker-compose.yml Dockerfile || echo "✓ 无残留旧版本号"
grep -n "$NEW_VER" Makefile main.go docker-compose.yml Dockerfile
```

`Makefile:7` 的 `-X main.Version=$(VERSION)` 与 `Dockerfile:21` 的 `${VERSION}` 都从变量取值，不用手改。`Dockerfile:25` 的 `ARG FRPC_VERSION=0.61.1` 是内置 frpc 版本，与本轮无关，**不要动**。

- [ ] **Step 3: 全量验证 + 本地打包**

```bash
cd /Users/busfly/Documents/code_workspace/Frpc-Mesh && export PATH="$HOME/.local/go/bin:$PATH" && \
go vet ./... && go test ./... -count=1 2>&1 | grep -v "no test files" && \
cd web && pnpm type-check && pnpm build 2>&1 | tail -2 && cd .. && \
make VERSION=$NEW_VER release && cd dist && shasum -a 256 -c sha256sums.txt
```

Expected: 4 包全 OK。

- [ ] **Step 4: 提交并汇报验收**

```bash
git add README.md README-CN.md docs/deployment.md docs/operations.md Makefile main.go docker-compose.yml Dockerfile
git commit -m "chore(release): 版本号升至 1.1.2，文档补移动端说明"
```

汇报内容：互审结论、CDP 断言全量输出（三档视口）、桌面回归截图对比、包与校验和。**未获用户许可前不 push、不开 PR、不发 Release、不打 tag、不动生产容器。**

---

## 发布后待办（本计划不执行）

1. 用户许可后：GitHub 先建 issue（本轮已由用户指定要提 issue）→ push `feat/mobile-adaptation` → PR `Closes #<issue>` → 合并 → tag → Release 挂 4 包 + `sha256sums.txt` → 校验 `releases/latest`。
2. `make release` 会把 `frpc-mesh:latest` 抢到新构建：**打包后立刻把 latest 退回原镜像**，新构建另挂版本 tag（本轮已两次踩过）。
3. 再单独确认是否重建生产容器（隧道中断数秒）。
4. 真机自测：iPhone Safari 内网直连、微信内置浏览器、外出走自家 frp 隧道三条路径各走一遍完整流程。
