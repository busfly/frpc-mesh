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
