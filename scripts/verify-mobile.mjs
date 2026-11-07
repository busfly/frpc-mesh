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
      { name: 'demo-off', type: 'udp', local_ip: '127.0.0.1', local_port: 53, remote_port: 5353, enabled: false },
      // 列表要够长才谈得上「吸顶」与滚动，凑够 10 条
      ...Array.from({ length: 7 }, (_, k) => ({
        name: `demo-http-${k + 1}`,
        type: 'http',
        local_ip: '127.0.0.1',
        local_port: 9000 + k,
        custom_domains: `app${k + 1}.example.com`
      }))
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
  // 上一轮中断会留下 cdp-mobile-prox，卡片数就对不上；按名字清干净，保证每轮起点一致
  const instances = await (await fetch(`${BASE}/api/instances`, auth)).json()
  for (const inst of instances) {
    const proxies = await (await fetch(`${BASE}/api/instances/${inst.id}/proxies`, auth)).json()
    for (const proxy of (proxies || []).filter((x) => x.name === 'cdp-mobile-prox')) {
      await fetch(`${BASE}/api/proxies/${proxy.id}`, { ...auth, method: 'DELETE' })
    }
  }
  // 逐个实例补齐：缺实例建实例，缺代理建代理（删除断言会真的删掉一条，不能只灌一次）
  for (const inst of SEED) {
    let target = instances.find((i) => i.name === inst.name)
    if (!target) {
      target = await (
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
      instances.push(target)
    }
    const have = (await (await fetch(`${BASE}/api/instances/${target.id}/proxies`, auth)).json()).map((x) => x.name)
    for (const proxy of inst.proxies) {
      if (have.includes(proxy.name)) continue
      await fetch(`${BASE}/api/instances/${target.id}/proxies`, {
        ...auth,
        method: 'POST',
        body: JSON.stringify({
          kind: 'proxy',
          name: proxy.name,
          type: proxy.type,
          enabled: proxy.enabled !== false,
          local_ip: proxy.local_ip,
          local_port: proxy.local_port,
          remote_port: proxy.remote_port,
          custom_domains: proxy.custom_domains || '',
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

/** 点「含指定文字的那张卡片」里的元素：删除断言要删自己建的那条，不能顺手删掉演示数据 */
async function tapInCard(S, text, sel) {
  const box = await S.js(
    `(() => { const card = [...document.querySelectorAll('.fm-card')].find((c) => c.innerText.includes(${JSON.stringify(text)}));` +
    ` if (!card) return null; const el = card.querySelector(${JSON.stringify(sel)}); if (!el) return null;` +
    ` el.scrollIntoView({ block: 'center' }); const r = el.getBoundingClientRect();` +
    ` return { x: Math.round(r.x + r.width / 2), y: Math.round(r.y + r.height / 2) } })()`
  )
  if (!box) throw new Error(`找不到含「${text}」的卡片或其中的 ${sel}`)
  await new Promise((r) => setTimeout(r, 250))
  await clickXY(S, box.x, box.y)
}

async function clickXY(S, x, y) {
  for (const type of ['mousePressed', 'mouseReleased']) {
    await S.cmd('Input.dispatchMouseEvent', { type, x, y, button: 'left', clickCount: 1 })
  }
}

/** 按可见文字点按钮（工具栏/消息框这类没有稳定 class 的入口） */
async function tapText(S, label, rootSel = 'document', itemSel = 'button, .el-button, .el-message-box__btns button') {
  const box = await S.js(
    `(() => { const els = [...${rootSel}.querySelectorAll(${JSON.stringify(itemSel)})];` +
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
 * 三档一起判：文档级、主内容区、以及「容器自身能不能横滚」——
 * 只判前两档会漏掉被 overflow-x:auto 裁掉的内容（弹层 body 只要设了 overflow-y:auto，
 * x 会连带算成 auto，超宽芯片被滚出可视区却永远不触发文档级溢出；独立验收就是这么漏了一条）。
 * 刻意可横滚的容器（chip 条、终端）在 LEGIT 里白名单放行。
 */
async function noHorizontalOverflow(S) {
  return S.js(`(() => {
    const de = document.documentElement;
    const main = document.querySelector('.main-content') || de;
    const box = (el) => el.getBoundingClientRect();
    const visible = (el) => { const r = box(el); return r.width > 0 && r.height > 0 };
    const isScrollBox = (el) => { const cs = getComputedStyle(el); return cs.overflowX === 'auto' || cs.overflowX === 'scroll' };
    // 刻意可横滚的容器（chip 条、日志/配置终端）与 EP 自己的滚动盒子不算问题
    const LEGIT = '.fm-chips, .terminal, .el-textarea__inner, .el-scrollbar__wrap, textarea, pre';
    const inScrollBox = (el) => {
      for (let p = el.parentElement; p; p = p.parentElement) if (p.matches(LEGIT) || isScrollBox(p)) return true;
      return false;
    };
    // 溢出屏幕且不在合法滚动容器里：真的会把内容裁掉
    const wide = [...document.querySelectorAll('body *')].filter((el) =>
      visible(el) && box(el).right > window.innerWidth + 1 && !inScrollBox(el));
    // 溢出屏幕但在合法滚动容器里，可那个容器本身不该能横滚（如弹层 body 被 overflow-y:auto 连带变成 x:auto）
    const scrollers = [...document.querySelectorAll('body *')].filter((el) =>
      visible(el) && isScrollBox(el) && !el.matches(LEGIT) && !el.closest(LEGIT) && el.scrollWidth > el.clientWidth + 1);
    const nm = (e) => String(e.className || e.tagName).split(' ').filter(Boolean).slice(0, 2).join('.');
    return {
      doc: de.scrollWidth <= window.innerWidth + 1,
      main: main.scrollWidth <= main.clientWidth + 1,
      offenders: wide.slice(0, 3).map((e) => nm(e) + '@' + Math.round(box(e).right)),
      scrollerNames: scrollers.slice(0, 3).map((e) => nm(e) + ':' + e.scrollWidth + '/' + e.clientWidth)
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
      check(
        `[${path}] 无横向溢出（含弹层内部）`,
        r.doc && r.main && r.offenders.length === 0 && r.scrollerNames.length === 0,
        JSON.stringify(r)
      )
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

    // —— 触摸热区：主要可点元素与页内输入框 ≥44、次要（文字按钮、说明问号）≥32 ——
    // 输入框这一族早先漏了：设置页「修改密码」不在弹层里，直接铺在页面上，
    // 只审弹层就永远看不到它停在 32 高 + 14 字号（iOS 聚焦会强制放大整页）
    const tapBad = []
    for (const path of PAGES) {
      await S.cmd('Page.navigate', { url: BASE + path })
      await S.wait('document.querySelector(".page-container")', `tap audit ${path}`)
      await new Promise((r) => setTimeout(r, 500))
      const t = await S.js(`(() => {
        const PRIM = '.fm-more, .fm-chip, .hamburger, .el-button--primary:not(.is-text):not(.is-link), .el-button--success:not(.is-text):not(.is-link), .fm-card .el-switch, .page-container .el-input__wrapper, .page-container .el-select__wrapper';
        const SECOND = '.el-button.is-text, .el-button.is-link, .fm-hint-btn, .tab-toolbar .el-button, .user-chip';
        const bad = (sel, min) => [...document.querySelectorAll(sel)]
          .filter((el) => { const r = el.getBoundingClientRect(); return r.width > 0 && Math.round(r.height) < min; })
          .map((el) => (el.innerText || '').trim().slice(0, 6) + '|' + String(el.getAttribute('class') || '').split(' ').filter(Boolean).slice(0, 2).join('.') + ' h=' + Math.round(el.getBoundingClientRect().height));
        const smallFont = [...document.querySelectorAll('.page-container .el-input__inner, .page-container .el-select__wrapper')]
          .filter((el) => parseFloat(getComputedStyle(el).fontSize) < 16)
          .map((el) => (el.placeholder || el.tagName) + ' fs=' + getComputedStyle(el).fontSize);
        return { primary: bad(PRIM, 44), secondary: bad(SECOND, 32), smallFont };
      })()`)
      for (const x of t.primary) tapBad.push(`${path} 主:${x}`)
      for (const x of t.secondary) tapBad.push(`${path} 次:${x}`)
      for (const x of t.smallFont) tapBad.push(`${path} 字号:${x}`)
    }
    check('[移动] 触摸热区与页内输入框达标（主44/次32/输入16号）', tapBad.length === 0, tapBad.slice(0, 6).join(' | '))

    // 规格 §4.2：仪表盘只看状态，启停回服务器页做，卡片上不重复放开关
    await S.cmd('Page.navigate', { url: `${BASE}/` })
    await S.wait('document.querySelector(".fm-cards")', 'dashboard cards')
    check('[移动] 仪表盘卡片不重复放启停开关', await S.js(
      `!document.querySelector('.fm-card .el-switch')`
    ), '仪表盘卡片里出现了开关')

    // 规格 §4.3：窄屏页头改上下排、新建按钮整宽
    await S.cmd('Page.navigate', { url: `${BASE}/instances` })
    await S.wait('document.querySelector(".page-head .el-button")', 'instances head')
    const head = await S.js(
      `(() => { const t = document.querySelector('.page-title').getBoundingClientRect();` +
      ` const b = document.querySelector('.page-head .el-button').getBoundingClientRect();` +
      ` return { below: b.top >= t.bottom, ratio: Math.round((b.width / window.innerWidth) * 100) / 100 } })()`
    )
    check('[移动] 服务器页页头上下排且按钮整宽', head.below && head.ratio > 0.8, JSON.stringify(head))

    // 规格 §4.4：两排同款 chip 语义不同，快速添加那排要有说明文字，别把「加代理」当成「切服务器」
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-type-chips")', 'type chips for caption')
    check('[移动] 快速添加排有说明标题', await S.js(`!!document.querySelector('.fm-chips-caption')`), '缺少说明标题')

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
    check('[移动] 操作面板有编辑与删除', asOK.items.includes('编辑') && asOK.items.includes('删除'), JSON.stringify(asOK.items))
    // 与桌面操作列的一致性在下面 1440 基线里对着真实表格比（早先这里是硬编码清单，桌面改了也不会报警）
    const mobileActions = asOK.items
    check('[移动] 操作面板行高 ≥44', asOK.tall, JSON.stringify(asOK.hs))
    await tapAt(S, '.fm-actionsheet .as-cancel')
    await new Promise((r) => setTimeout(r, 700))
    // el-drawer 关闭后节点仍留在 DOM（只是隐藏），按高度为 0 判关闭
    check('[移动] 操作面板可取消关闭', await S.js(
      `(() => { const d = document.querySelector('.fm-actionsheet');` +
      ` return !d || d.getBoundingClientRect().height === 0 })()`
    ))

    // 卡片数 == 当前激活那台服务器的代理数（不能拿 list[0] 比：默认激活的那台未必是第一台，
    // URL 里带 ?instance= 时更是直接错位）
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-more")', 'proxy cards')
    const counts = await S.js(`document.querySelectorAll('.fm-cards .fm-card').length`)
    const api = await S.js(
      `(async () => { const t = localStorage.getItem('frpc_mesh_token');` +
      ` const active = document.querySelector('.fm-chips:not(.fm-type-chips) .fm-chip[aria-selected="true"]');` +
      ` const name = active ? active.innerText.trim() : '';` +
      ` const list = await (await fetch('/api/instances', { headers: { Authorization: 'Bearer ' + t } })).json();` +
      ` const hit = list.find((i) => i.name === name) || list[0];` +
      ` const r2 = await fetch('/api/instances/' + hit.id + '/proxies', { headers: { Authorization: 'Bearer ' + t } });` +
      ` return (await r2.json()).length })()`
    )
    check('[移动] 卡片条数与当前服务器代理数一致', counts === api, `卡片 ${counts} / 接口 ${api}`)

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
      // 弹层内部横向溢出：overflow-y:auto 会连带把 overflow-x 算成 auto，超宽内容被滚出可视区，
      // 文档级 scrollWidth 看不出来 —— 320/360 屏上「本地地址」的快捷芯片就是这么被裁掉的
      const body = sheet.querySelector('.el-drawer__body, .el-dialog__body');
      const spill = [...sheet.querySelectorAll('*')].filter((el) => {
        const r = el.getBoundingClientRect();
        return r.width > 0 && r.right > window.innerWidth + 1 && !(el.closest('.fm-chips') || el.matches('.terminal'));
      }).slice(0, 2).map((el) => String(el.className || el.tagName).split(' ').filter(Boolean).slice(0, 2).join('.') + '@' + Math.round(el.getBoundingClientRect().right));
      return {
        sheetH: Math.round(sheet.getBoundingClientRect().height),
        cols: cols.length, narrow, wraps: wraps.length, short, tiny,
        bodyW: body ? body.scrollWidth : 0, bodyCw: body ? body.clientWidth : 0, spill,
        footVisible: !!foot && foot.getBoundingClientRect().bottom <= window.innerHeight + 1
      };
    })()`)
    check('[移动] 表单贴底、字段单列', formOK.sheetH > 500 && formOK.cols > 0 && formOK.narrow === 0, JSON.stringify(formOK))
    check('[移动] 输入框 44 高且字号≥16（防 iOS 聚焦放大）', formOK.wraps > 0 && formOK.short === 0 && formOK.tiny === 0, JSON.stringify(formOK))
    check('[移动] 保存按钮吸底可见', formOK.footVisible, JSON.stringify(formOK))
    check('[移动] 表单弹层内部不横向溢出', formOK.bodyW <= formOK.bodyCw + 1 && formOK.spill.length === 0, JSON.stringify(formOK))

    // 分段选择器只在「安全隧道」这类表单里出现（提供方 / 访问方），单独开一次来量它
    await tapText(S, '取消', `document.querySelector('.fm-form')`)
    await new Promise((r) => setTimeout(r, 700))
    await tapText(S, 'STCP', 'document.querySelector(".fm-type-chips")')
    await S.wait('document.querySelector(".fm-form .el-segmented")', 'role segmented sheet')
    await new Promise((r) => setTimeout(r, 700))
    const segOK = await S.js(
      `(() => { const seg = document.querySelector('.fm-form .el-segmented');` +
      ` if (!seg) return { shell: 0, items: [], bodySW: 0, bodyCW: 0, spill: [] };` +
      ` const items = [...seg.querySelectorAll('.el-segmented__item')].map((e) => Math.round(e.getBoundingClientRect().height));` +
      ` const body = document.querySelector('.fm-form .el-drawer__body');` +
      ` const spill = [...document.querySelectorAll('.fm-form *')].filter((el) => { const r = el.getBoundingClientRect();` +
      ` return r.width > 0 && r.right > window.innerWidth + 1 }).slice(0, 2)` +
      ` .map((el) => String(el.className || el.tagName).split(' ').filter(Boolean).slice(0, 2).join('.') + '@' + Math.round(el.getBoundingClientRect().right));` +
      ` return { shell: Math.round(seg.getBoundingClientRect().height), items, bodySW: body.scrollWidth, bodyCW: body.clientWidth, spill } })()`
    )
    check(
      '[移动] 弹层内分段选择器够点且不溢出（壳≥44 / 项≥40 / 不顶出屏）',
      segOK.items.length > 0 && segOK.shell >= 44 && segOK.items.every((h) => h >= 40) && segOK.bodySW <= segOK.bodyCW + 1 && segOK.spill.length === 0,
      JSON.stringify(segOK)
    )

    // 底部弹层的圆角与限高：EP 的抽屉方向类是裸 `btt`，写成 el-drawer--bottom 会永不命中。
    // 必须在表单弹层真的开着的时候量，且只量 height>0 的那个：
    // 按类名直取会先量到一个还留在 DOM 里的隐藏 ActionSheet（早先那条断言就是这么一直「绿」的）
    const sheetShape = await S.js(
      `(() => { const el = [...document.querySelectorAll('.fm-sheet.btt')].find((n) => n.getBoundingClientRect().height > 0);` +
      ` if (!el) return { found: false }; const cs = getComputedStyle(el);` +
      ` return { found: true, cls: String(el.className).slice(0, 60), radius: cs.borderTopLeftRadius, maxH: cs.maxHeight } })()`
    )
    check('[移动] 底部弹层圆角与限高真的生效', sheetShape.found && parseFloat(sheetShape.radius) >= 12, JSON.stringify(sheetShape))
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

    // chip 条「吸顶」：EP 的 .el-card 带 overflow:hidden，sticky 的参照会变成那个永不滚动的盒子，
    // 宣称的吸顶其实从未生效 —— 滚一段再量，必须还贴在可视区顶部
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-chips:not(.fm-type-chips)")', 'chip row')
    const sticky = await S.js(
      `(() => { const row = document.querySelector('.fm-chips:not(.fm-type-chips)');` +
      ` const sc = document.querySelector('.main-content'); sc.scrollTop = 420;` +
      ` const r = row.getBoundingClientRect();` +
      ` return { scrolled: sc.scrollTop > 100, top: Math.round(r.top), h: Math.round(r.height),` +
      ` stuck: r.height > 0 && r.top >= 0 && r.top < 120 } })()`
    )
    check('[移动] 服务器 chip 条真的吸顶', sticky.scrolled && sticky.stuck, JSON.stringify(sticky))

    // 下滑关闭手势：在把手上造 touch 序列，位移 >80px 应当关掉弹层（规格 §6.2 承诺过，之前只测了取消按钮）
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-type-chips")', 'chips for swipe')
    await tapAt(S, '.fm-type-chips .fm-chip')
    await S.wait('document.querySelector(".fm-form")', 'form for swipe')
    await new Promise((r) => setTimeout(r, 700))
    const grab = await S.js(
      `(() => { const h = document.querySelector('.fm-form .fm-handle');` +
      ` const r = h.getBoundingClientRect(); return { x: Math.round(r.x + r.width / 2), y: Math.round(r.y + r.height / 2) } })()`
    )
    for (const [type, dy] of [['touchStart', 0], ['touchMove', 30], ['touchMove', 60], ['touchMove', 120], ['touchEnd', 120]]) {
      await S.cmd('Input.dispatchTouchEvent', {
        type,
        touchPoints: type === 'touchEnd' ? [] : [{ x: grab.x, y: grab.y + dy }]
      })
      await new Promise((r) => setTimeout(r, 60))
    }
    await new Promise((r) => setTimeout(r, 700))
    check('[移动] 把手下滑能关闭弹层', await S.js(
      `(() => { const el = document.querySelector('.fm-form');` +
      ` return !el || el.getBoundingClientRect().height === 0 })()`
    ), '下滑 120px 后表单弹层仍在')

    // 对话框侧的弹层（拷贝代理）也要 44 高输入 + 16px 字号 + 44 主按钮：它 teleport 到 body，拿不到页面内规则
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".fm-more")', 'cards for copy dialog')
    await tapAt(S, '.fm-more')
    await S.wait('document.querySelector(".fm-actionsheet")', 'sheet for copy')
    await tapText(S, '拷贝', `document.querySelector('.fm-actionsheet')`)
    await S.wait('document.querySelector(".fm-sheet-dialog .el-select__wrapper")', 'copy dialog')
    await new Promise((r) => setTimeout(r, 600))
    const copySheet = await S.js(
      `(() => { const w = document.querySelector('.fm-sheet-dialog .el-select__wrapper');` +
      ` const b = document.querySelector('.fm-sheet-dialog .el-button--primary');` +
      ` return { h: Math.round(w.getBoundingClientRect().height), fs: parseFloat(getComputedStyle(w).fontSize),` +
      ` btnH: b ? Math.round(b.getBoundingClientRect().height) : 0 } })()`
    )
    check('[移动] 对话框弹层输入 44 高且字号≥16', copySheet.h >= 44 && copySheet.fs >= 16, JSON.stringify(copySheet))
    check('[移动] 对话框弹层主按钮 ≥44', copySheet.btnH >= 44, JSON.stringify(copySheet))

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
    // 安全区：只判「底部动作区那几条规则」真的带 env()，不做全站扫（全站扫是恒真式，
    // 任何一处无关规则里有 env() 都会误判成通过）
    const safeArea = await S.js(
      `(() => { const hit = (want) => [...document.styleSheets].some((sh) => { try {` +
      ` return [...sh.cssRules].some((r) => r.selectorText && r.selectorText.includes(want) &&` +
      ` r.cssText.includes('env(safe-area-inset-bottom)'))` +
      ` } catch (e) { return false } });` +
      ` return { drawer: hit('.fm-sheet .el-drawer__footer'), dialog: hit('.fm-sheet-dialog .el-dialog__footer') } })()`
    )
    check('[移动] 弹层底部动作区声明了 safe-area', safeArea.drawer && safeArea.dialog, JSON.stringify(safeArea))

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
    // 页面刚 navigate，10 秒轮询计时器是全新的；窗口内出现请求只可能来自「回前台」这一路。
    // 判 ≥1 而不是恰好 2：dashboard 与 instances 谁先落地取决于挂载顺序，硬数会偶发误红
    await new Promise((r) => setTimeout(r, 700))
    check('[移动] 回前台补刷一轮', reqs.length >= 1 && reqs.every((u) => /\/api\/(instances|dashboard)/.test(u)), `捕获 ${reqs.length} 次: ${reqs.join(',')}`)

    // —— 桌面回归基线 1440×844 ——
    await S.cmd('Page.removeScriptToEvaluateOnNewDocument', { identifier: hoverPatch.identifier })
    await S.setViewport(1440, 844)
    // 视口改动要等一帧再导航：MainLayout 的折叠初值读 window.innerWidth，抢跑会拿到旧宽度
    await new Promise((r) => setTimeout(r, 400))
    await S.cmd('Page.navigate', { url: `${BASE}/proxies` })
    await S.wait('document.querySelector(".el-table") || document.querySelector(".el-empty")', 'desktop proxies table')
    check('[桌面] 代理页仍是表格形态', await S.js('!!document.querySelector(".el-table")'))
    check('[桌面] 侧栏存在且 220px', await S.js(`(() => { const a = document.querySelector('.el-aside'); return a && a.getBoundingClientRect().width === 220 })()`))
    check('[桌面] 无移动卡片列表', await S.js('!document.querySelector(".fm-cards")'))
    await S.shot('desktop-proxies.png')

    // 操作项口径与桌面真实对齐：读桌面第一行操作列的按钮文案，和移动端面板逐项比
    await S.cmd('Page.navigate', { url: `${BASE}/instances` })
    await S.wait('document.querySelector(".el-table__row")', 'desktop instances table')
    const desktopActions = await S.js(
      `(() => { const row = document.querySelector('.el-table__row');` +
      ` const cells = row.querySelectorAll('td'); const last = cells[cells.length - 1];` +
      ` return [...last.querySelectorAll('button, .el-button')].map((b) => b.innerText.trim()).filter(Boolean) })()`
    )
    // 桌面操作列受列宽限制用短词（管理），手机面板整行用全称（管理面板），
    // 所以按「互相包含」配对，并把手机面板特有的「取消」行剔出比较
    const dAct = desktopActions.filter((x) => x !== '取消').sort()
    const mAct = mobileActions.filter((x) => x !== '取消').sort()
    check(
      '[桌面↔移动] 操作面板项与桌面操作列一致',
      dAct.length > 0 && dAct.length === mAct.length && dAct.every((d) => mAct.some((m) => m === d || m.includes(d) || d.includes(m))),
      `桌面 ${JSON.stringify(dAct)} / 移动 ${JSON.stringify(mAct)}`
    )

  // —— 登录页（清掉登录态单独看一档：规格 §4.1 要求 44 高输入 + 16px 字号，之前脚本没看过登录页）——
  await S.setViewport(WIDTH, HEIGHT)
  await S.js('localStorage.clear()')
  await S.cmd('Page.navigate', { url: `${BASE}/login` })
  await S.wait('document.querySelector(".login-btn")', 'login page')
  await new Promise((r) => setTimeout(r, 500))
  const login = await S.js(
    `(() => { const w = document.querySelector('.login-card .el-input__wrapper');` +
    ` const i = document.querySelector('.login-card .el-input__inner');` +
    ` const b = document.querySelector('.login-btn');` +
    ` return { h: Math.round(w.getBoundingClientRect().height),` +
    ` fs: parseFloat(getComputedStyle(i).fontSize), btn: Math.round(b.getBoundingClientRect().height) } })()`
  )
  check('[移动] 登录页输入 44 高、字号≥16、按钮 44', login.h >= 44 && login.fs >= 16 && login.btn >= 44, JSON.stringify(login))
  await S.shot('mobile-login.png')
  } finally {
    ws.close()
    chrome.kill('SIGKILL')
  }

  for (const r of results) console.log(`${r.ok ? 'PASS' : 'FAIL'} ${r.name}${r.detail ? ' :: ' + r.detail : ''}`)
  const failed = results.filter((r) => !r.ok).length
  console.log(`\n共 ${results.length} 项，失败 ${failed}`)
  process.exitCode = failed ? 1 : 0
}

run().catch((e) => {
  console.error('脚本异常:', e.message)
  for (const r of results) console.log(`${r.ok ? 'PASS' : 'FAIL'} ${r.name}${r.detail ? ' :: ' + r.detail : ''}`)
  process.exitCode = 2
})
