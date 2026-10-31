<template>
  <div class="page-container">
    <div class="page-head">
      <div>
        <h2 class="page-title">穿透代理</h2>
        <p class="page-subtitle">按 FRPS 服务器分 Tab 管理所有穿透隧道，可跨服务器拷贝代理</p>
      </div>
    </div>

    <el-card class="card" shadow="never" v-loading="store.loading && !store.instances.length">
      <!-- 无任何服务器 -->
      <el-empty v-if="!store.instances.length && !store.loading" description="暂无 FRPS 服务器，请先创建">
        <el-button type="primary" @click="router.push('/instances')">前往 FRPS 服务器管理</el-button>
      </el-empty>

      <el-tabs v-else v-model="activeTab">
        <el-tab-pane v-for="inst in store.instances" :key="inst.id" :name="String(inst.id)">
          <template #label>
            <span class="tab-label">
              <span
                class="status-dot"
                :style="{ background: statusDotColor(inst.runtime?.status) }"
              ></span>
              {{ inst.name }}
              <span class="tab-ver">{{ frpcVersionLabel(inst, versionState) }}</span>
            </span>
          </template>

          <div class="type-cards">
            <button
              v-for="t in TYPE_CARDS"
              :key="t.type"
              class="type-card"
              @click="openCreate(inst.id, t.type + ':proxy')"
            >
              <el-icon class="type-card-icon" :style="{ color: t.color }">
                <component :is="t.icon" />
              </el-icon>
              <span class="type-card-text">
                <span class="type-card-name">{{ t.name }}</span>
                <span class="type-card-sub">{{ t.sub }}</span>
              </span>
              <el-tooltip :content="`点击添加「${t.name} · ${t.sub}」代理`" placement="top">
                <span class="type-card-add">
                  <el-icon :size="10"><Plus /></el-icon>
                  创建
                </span>
              </el-tooltip>
            </button>
          </div>

          <div class="tab-toolbar">
            <div class="section-title">
              已配置代理
              <span class="text-muted section-count">
                （在线 {{ onlineCount(inst.id) }} / 共 {{ (proxiesMap[inst.id] || []).length }} 条）
              </span>
            </div>
            <div class="tab-toolbar-actions">
              <el-button size="small" text type="primary" class="toolbar-link" @click="openInfo(inst, 'config')">
                配置文件
              </el-button>
              <el-button size="small" text type="primary" class="toolbar-link" @click="openInfo(inst, 'logs')">
                运行日志
              </el-button>
              <el-button
                type="success"
                round
                :loading="!!pingingMap[inst.id]"
                @click="onPing(inst.id)"
              >
                全部 Ping
              </el-button>
            </div>
          </div>

          <el-table
            :data="proxiesMap[inst.id] || []"
            :row-class-name="rowClassName"
            v-loading="!!loadingMap[inst.id]"
            size="small"
            stripe
            empty-text="暂无代理，点击上方类型卡片快速添加"
          >
            <el-table-column label="类型" width="70">
              <template #default="{ row }">
                <span class="type-badge" :style="{ background: proxyTypeColor(row.type) }">
                  {{ proxyTypeLabel(row.type) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column prop="name" label="名称" min-width="120" show-overflow-tooltip />
            <el-table-column label="状态" width="80">
              <template #default="{ row }">
                <el-tag v-if="!row.enabled" type="warning" effect="plain" size="small" round>
                  已禁用
                </el-tag>
                <el-tag v-else-if="isOnline(inst.id, row)" type="success" effect="dark" size="small" round>
                  在线
                </el-tag>
                <el-tag v-else type="info" effect="plain" size="small" round>离线</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="启用" width="70">
              <template #default="{ row }">
                <el-switch
                  :model-value="row.enabled"
                  :loading="!!togglingProxy[row.id]"
                  @change="onToggleProxy(inst.id, row, $event as boolean)"
                />
              </template>
            </el-table-column>
            <el-table-column label="本地地址 / 访问摘要" min-width="170" show-overflow-tooltip>
              <template #default="{ row }">
                <el-link
                  v-if="row.kind === 'visitor' && row.bind_port"
                  type="primary"
                  :underline="false"
                  class="mono"
                  :href="`http://${row.bind_addr || '127.0.0.1'}:${row.bind_port}`"
                  target="_blank"
                >
                  {{ row.bind_addr || '127.0.0.1' }}:{{ row.bind_port }}
                </el-link>
                <span v-else-if="row.kind === 'visitor'" class="mono">
                  访问 → {{ row.server_name || '-' }}
                </span>
                <el-link
                  v-else-if="row.local_port"
                  type="primary"
                  :underline="false"
                  class="mono"
                  :href="`http://${row.local_ip || '127.0.0.1'}:${row.local_port}`"
                  target="_blank"
                >
                  {{ row.local_ip || '127.0.0.1' }}:{{ row.local_port }}
                </el-link>
                <span v-else class="mono">{{ row.local_ip || '127.0.0.1' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="远程端口 / 域名" min-width="130" show-overflow-tooltip>
              <template #default="{ row }">
                <el-link
                  v-if="(row.type === 'tcp' || row.type === 'udp') && row.remote_port"
                  type="primary"
                  :underline="false"
                  class="mono"
                  :href="`http://${remoteHost(inst)}:${row.remote_port}`"
                  target="_blank"
                >
                  {{ row.remote_port }}
                </el-link>
                <span v-else-if="row.type === 'tcp' || row.type === 'udp'" class="mono">随机</span>
                <el-link
                  v-else-if="row.custom_domains"
                  type="primary"
                  :underline="false"
                  class="mono"
                  :href="`${row.type === 'https' ? 'https' : 'http'}://${row.custom_domains.split(',')[0].trim()}/`"
                  target="_blank"
                >
                  {{ row.custom_domains.split(',')[0].trim() }}
                </el-link>
                <span v-else-if="row.subdomain" class="mono">子域名: {{ row.subdomain }}</span>
                <span v-else class="text-muted">-</span>
              </template>
            </el-table-column>
            <el-table-column label="延迟" width="80">
              <template #default="{ row }">
                <span v-if="!row.enabled || latencyOf(inst.id, row) === null" class="text-muted">--</span>
                <span v-else-if="latencyOf(inst.id, row) === -1" class="lat-na">不通</span>
                <span v-else-if="latencyOf(inst.id, row) === 0" class="lat-ok">可通</span>
                <span v-else :class="latencyOf(inst.id, row)! > 200 ? 'lat-slow' : 'lat-ok'">
                  {{ latencyOf(inst.id, row) }} ms
                </span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="130" fixed="right">
              <template #default="{ row }">
                <div class="op-cell">
                  <el-button size="small" text type="primary" @click="openEdit(inst.id, row)">
                    编辑
                  </el-button>
                  <el-button size="small" text type="primary" @click="openCopy(inst.id, row)">
                    拷贝
                  </el-button>
                  <el-button
                    size="small"
                    text
                    type="danger"
                    :loading="!!deletingMap[row.id]"
                    @click="onDelete(inst.id, row)"
                  >
                    删除
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 添加 / 编辑代理 -->
    <ProxyFormDialog
      v-model="dialogVisible"
      :instance-id="dialogInstanceId"
      :create-type="createType"
      :create-kind="createKind"
      :proxy="editing"
      @saved="onProxySaved"
    />

    <!-- 配置文件 / 运行日志 查看弹窗 -->
    <el-dialog
      v-model="infoVisible"
      :title="infoTitle"
      width="780px"
      top="6vh"
      append-to-body
      destroy-on-close
    >
      <ConfigTab v-if="infoKind === 'config'" :instance-id="infoInstanceId" />
      <LogsTab v-else :instance-id="infoInstanceId" :running="infoRunning" />
    </el-dialog>

    <!-- 拷贝代理到其他服务器 -->
    <el-dialog v-model="copyVisible" title="拷贝代理到服务器" width="440px" append-to-body>
      <el-form label-position="top">
        <el-form-item :label="`将「${copySource?.name || ''}」拷贝到`">
          <el-select v-model="copyTargetId" placeholder="选择目标服务器" class="full-width">
            <el-option
              v-for="inst in copyTargets"
              :key="inst.id"
              :label="inst.name"
              :value="inst.id"
            >
              <span class="select-inst">
                <span
                  class="status-dot"
                  :style="{ background: statusDotColor(inst.runtime?.status) }"
                ></span>
                {{ inst.name }}
              </span>
            </el-option>
          </el-select>
        </el-form-item>
        <div class="copy-tip text-muted">支持拷贝到同一服务器快速复制配置；目标存在同名代理时名称自动追加「-copy」</div>
      </el-form>
      <template #footer>
        <el-button @click="copyVisible = false">取消</el-button>
        <el-button type="primary" :loading="copying" :disabled="!copyTargetId" @click="onCopyConfirm">
          拷贝
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Key, Link, Lock, Pouring, Share, Sort } from '@element-plus/icons-vue'
import ProxyFormDialog from '../components/ProxyFormDialog.vue'
import ConfigTab from '../components/ConfigTab.vue'
import LogsTab from '../components/LogsTab.vue'
import {
  apiCreateProxy,
  apiDeleteProxy,
  apiListProxies,
  apiPingProxies,
  apiProxiesOnline,
  apiSetProxyEnabled
} from '../api'
import type { Proxy } from '../api/types'
import { useInstancesStore } from '../stores/instances'
import { proxyTypeColor, proxyTypeLabel, statusDotColor } from '../utils/status'
import { frpcVersionLabel, loadVersionState, type VersionState } from '../utils/frpcVersion'

const route = useRoute()
const router = useRouter()
const store = useInstancesStore()

/** 快速添加类型卡片 */
const TYPE_CARDS = [
  { type: 'tcp', name: 'TCP', sub: '端口转发', color: '#409eff', icon: Sort },
  { type: 'udp', name: 'UDP', sub: '端口转发', color: '#9254de', icon: Pouring },
  { type: 'http', name: 'HTTP', sub: '网页服务', color: '#67c23a', icon: Link },
  { type: 'https', name: 'HTTPS', sub: '加密网页', color: '#faad14', icon: Lock },
  { type: 'stcp', name: 'STCP', sub: '安全隧道', color: '#eb2f96', icon: Key },
  { type: 'xtcp', name: 'P2P', sub: '点对点', color: '#13c2c2', icon: Share }
]

const activeTab = ref('')
const proxiesMap = reactive<Record<number, Proxy[]>>({})
const loadingMap = reactive<Record<number, boolean>>({})
const pingedMap = reactive<Record<number, boolean>>({})
const pingingMap = reactive<Record<number, boolean>>({})
const deletingMap = reactive<Record<number, boolean>>({})
const loadedMap = reactive<Record<number, boolean>>({})
/** 每实例的延迟结果：proxyId → latency_ms */
const latencyMaps = reactive<Record<number, Record<number, number>>>({})
/** 每实例的在线代理 ID 集合 */
const onlineMaps = reactive<Record<number, Record<number, boolean>>>({})

const dialogVisible = ref(false)
const dialogInstanceId = ref(0)
const createType = ref('tcp')
const createKind = ref<'proxy' | 'visitor'>('proxy')
const editing = ref<Proxy | null>(null)
/** 编辑弹窗打开时所属的实例，保存后刷新对应 tab */
const editingInstanceId = ref(0)

const copyVisible = ref(false)
const copySource = ref<Proxy | null>(null)
const copySourceInstance = ref(0)
const copyTargetId = ref<number | null>(null)
const copying = ref(false)

const copyTargets = computed(() => store.instances)

function currentInstanceId(): number {
  return Number(activeTab.value) || 0
}

/** 远程链接主机：服务器配置了域名优先用域名，否则 IP 兜底 */
function remoteHost(inst: { domain?: string; server_addr: string }): string {
  return inst.domain?.trim() || inst.server_addr
}

function latencyOf(instanceId: number, row: Proxy): number | null {
  if (!pingedMap[instanceId]) return null
  const m = latencyMaps[instanceId]
  return m && row.id in m ? m[row.id] : null
}

async function loadProxies(instanceId: number, silent = false) {
  if (!instanceId) return
  if (!silent) loadingMap[instanceId] = true
  try {
    proxiesMap[instanceId] = (await apiListProxies(instanceId)) || []
    loadedMap[instanceId] = true
  } catch {
    proxiesMap[instanceId] = proxiesMap[instanceId] || []
    loadedMap[instanceId] = true
  } finally {
    loadingMap[instanceId] = false
  }
  loadOnline(instanceId)
}

/** 拉取在线（隧道建立且本地目标可连通）代理 ID 集合，失败静默 */
async function loadOnline(instanceId: number) {
  try {
    const r = await apiProxiesOnline(instanceId)
    const m: Record<number, boolean> = {}
    for (const id of r?.online_ids || []) m[id] = true
    onlineMaps[instanceId] = m
  } catch {
    /* 静默 */
  }
}

function isOnline(instanceId: number, row: Proxy): boolean {
  return !!onlineMaps[instanceId]?.[row.id]
}

/** 禁用行整体置灰 */
function rowClassName({ row }: { row: Proxy }): string {
  return row.enabled ? '' : 'row-disabled'
}

const togglingProxy = reactive<Record<number, boolean>>({})

async function onToggleProxy(instanceId: number, row: Proxy, val: boolean) {
  togglingProxy[row.id] = true
  try {
    await apiSetProxyEnabled(row.id, val)
    ElMessage.success(val ? '已启用，运行中的 frpc 将自动重启生效' : '已禁用，运行中的 frpc 将自动重启生效')
    await loadProxies(instanceId, true)
  } catch {
    /* 拦截器已提示 */
  } finally {
    togglingProxy[row.id] = false
  }
}

function onlineCount(instanceId: number): number {
  const m = onlineMaps[instanceId]
  if (!m) return 0
  return (proxiesMap[instanceId] || []).filter((p) => m[p.id]).length
}

/** 选中 tab 后按需加载其代理列表 */
watch(activeTab, (id) => {
  const n = Number(id)
  if (n && !loadedMap[n]) loadProxies(n)
  if (n && route.query.instance !== String(n)) {
    router.replace({ query: { ...route.query, instance: String(n) } })
  }
})

/** 全局 store 每 10 秒轮询服务器状态，跟随刷新当前 tab 的代理在线状态 */
watch(
  () => store.instances.map((i) => `${i.id}:${i.runtime?.status}`).join(','),
  () => {
    const n = currentInstanceId()
    if (n && loadedMap[n]) loadOnline(n)
  }
)

/** 实例列表到达/变化后确定激活的 tab（优先 ?instance= 查询参数） */
watch(
  () => store.instances.map((i) => i.id).join(','),
  () => {
    const list = store.instances
    if (!list.length) return
    const wanted = Number(route.query.instance)
    const ids = list.map((i) => i.id)
    if (activeTab.value && ids.includes(Number(activeTab.value))) return
    if (wanted && ids.includes(wanted)) {
      activeTab.value = String(wanted)
    } else {
      activeTab.value = String(ids[0])
    }
  },
  { immediate: true }
)

let onlineTimer: number | undefined

const versionState = ref<VersionState>({ active: '', downloaded: [] })

onMounted(() => {
  loadVersionState().then((s) => (versionState.value = s))
  // 实例列表由全局 store 每 10 秒静默轮询刷新；此处确保已有数据并加载当前 tab 代理
  if (!store.instances.length) store.refresh()
  const n = currentInstanceId()
  if (n) loadProxies(n)
  // 在线状态定时自愈：frpc 重启后数秒内日志才确认隧道，避免停留在过期状态
  onlineTimer = window.setInterval(() => {
    const cur = currentInstanceId()
    if (cur && loadedMap[cur]) loadOnline(cur)
  }, 10000)
})

onUnmounted(() => {
  if (onlineTimer) window.clearInterval(onlineTimer)
})

// ---------- 添加 / 编辑 ----------

function openCreate(instanceId: number, cmd: string) {
  const [type, kind] = cmd.split(':')
  editing.value = null
  createType.value = type
  createKind.value = (kind as 'proxy' | 'visitor') || 'proxy'
  dialogInstanceId.value = instanceId
  editingInstanceId.value = instanceId
  dialogVisible.value = true
}

function openEdit(instanceId: number, row: Proxy) {
  editing.value = row
  dialogInstanceId.value = instanceId
  editingInstanceId.value = instanceId
  dialogVisible.value = true
}

function onProxySaved() {
  loadProxies(editingInstanceId.value, true)
  // frpc 重启约需数秒，延迟回刷一次在线状态，避免保存后短暂全显离线
  window.setTimeout(() => loadOnline(editingInstanceId.value), 4000)
}

// ---------- 配置文件 / 日志 弹窗 ----------

const infoVisible = ref(false)
const infoKind = ref<'config' | 'logs'>('config')
const infoInstanceId = ref(0)
const infoRunning = ref(false)

const infoTitle = computed(() => {
  const inst = store.instances.find((i) => i.id === infoInstanceId.value)
  const name = inst?.name || ''
  return infoKind.value === 'config' ? `配置文件 - ${name}` : `运行日志 - ${name}`
})

function openInfo(inst: { id: number; runtime?: { status?: string } | null }, kind: 'config' | 'logs') {
  infoKind.value = kind
  infoInstanceId.value = inst.id
  const st = inst.runtime?.status || ''
  infoRunning.value = st === 'online' || st === 'running' || st === 'starting'
  infoVisible.value = true
}

// ---------- Ping ----------

async function onPing(instanceId: number) {
  pingingMap[instanceId] = true
  try {
    const results = await apiPingProxies(instanceId)
    const m: Record<number, number> = {}
    for (const r of results || []) m[r.id] = r.latency_ms
    latencyMaps[instanceId] = m
    pingedMap[instanceId] = true
    await loadOnline(instanceId)
    ElMessage.success('Ping 测量完成')
  } catch {
    /* 拦截器已提示 */
  } finally {
    pingingMap[instanceId] = false
  }
}

// ---------- 拷贝 ----------

function openCopy(instanceId: number, row: Proxy) {
  copySource.value = row
  copySourceInstance.value = instanceId
  copyTargetId.value = null
  copyVisible.value = true
}

async function onCopyConfirm() {
  const src = copySource.value
  const targetId = copyTargetId.value
  if (!src || !targetId) return
  const target = store.instances.find((i) => i.id === targetId)
  copying.value = true
  try {
    // 目标存在同名代理时名称追加「-copy」直至不冲突
    const targetProxies = (await apiListProxies(targetId)) || []
    const names = new Set(targetProxies.map((p) => p.name))
    let name = src.name
    while (names.has(name)) name = `${name}-copy`

    const payload: Partial<Proxy> = {
      instance_id: targetId,
      kind: src.kind,
      name,
      type: src.type,
      local_ip: src.local_ip,
      local_port: src.local_port,
      remote_port: src.remote_port,
      custom_domains: src.custom_domains,
      subdomain: src.subdomain,
      locations: src.locations,
      http_user: src.http_user,
      http_password: src.http_password,
      use_encryption: src.use_encryption,
      use_compression: src.use_compression,
      bandwidth_limit: src.bandwidth_limit,
      secret_key: src.secret_key,
      server_name: src.server_name,
      bind_addr: src.bind_addr,
      bind_port: src.bind_port,
      enabled: src.enabled,
      remark: src.remark
    }
    await apiCreateProxy(targetId, payload)
    ElMessage.success(`已拷贝到 ${target?.name || '目标服务器'}`)
    copyVisible.value = false
    // 目标 tab 已加载过则刷新，切过去即可看到
    if (loadedMap[targetId]) loadProxies(targetId, true)
  } catch {
    /* 拦截器已提示 */
  } finally {
    copying.value = false
  }
}

// ---------- 删除 ----------

function onDelete(instanceId: number, row: Proxy) {
  ElMessageBox.confirm(
    `确定删除「${row.name}」吗？运行中的 frpc 将自动重启生效。`,
    '删除确认',
    {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    }
  )
    .then(async () => {
      deletingMap[row.id] = true
      try {
        await apiDeleteProxy(row.id)
        ElMessage.success('已删除')
        await loadProxies(instanceId, true)
      } catch {
        /* 拦截器已提示 */
      } finally {
        deletingMap[row.id] = false
      }
    })
    .catch(() => undefined)
}
</script>

<style scoped>
.page-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 4px;
}

.tab-label {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}

.tab-ver {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}

.tab-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 18px 0 12px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
}

.section-count {
  font-weight: 400;
  font-size: 12px;
}

.toolbar-link {
  font-size: 13px;
  padding: 4px 6px;
}

.type-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 10px;
  margin: 2px 0 4px;
}

.type-card {
  position: relative;
  background: #fff;
  border: 1px solid var(--el-border-color-light);
  border-radius: 10px;
  padding: 10px 12px;
  display: flex;
  align-items: center;
  gap: 10px;
  text-align: left;
  cursor: pointer;
  transition: all 0.15s ease;
  font-family: inherit;
}

.type-card:hover {
  border-color: #ff9a2e;
  background: #fff8f0;
  box-shadow: 0 4px 14px rgba(255, 154, 46, 0.18);
}

.type-card-add {
  position: absolute;
  right: 8px;
  bottom: 5px;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10px;
  line-height: 1;
  color: #c0c4cc;
  transition: color 0.15s ease;
}

.type-card:hover .type-card-add {
  color: #ff9a2e;
}

.type-card-icon {
  font-size: 20px;
  flex-shrink: 0;
}

.type-card-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.type-card-name {
  font-size: 14px;
  font-weight: 600;
  line-height: 1.3;
}

.type-card-sub {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.3;
}

.tab-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.type-badge {
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  border-radius: 6px;
  padding: 3px 8px;
  min-width: 42px;
  text-align: center;
  display: inline-block;
}

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

:deep(.row-disabled) {
  opacity: 0.55;
}

.op-cell {
  display: flex;
  align-items: center;
  white-space: nowrap;
  gap: 0;
}

.op-cell .el-button {
  padding: 2px 4px;
}

.full-width {
  width: 100%;
}

.select-inst {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}

.copy-tip {
  font-size: 12px;
  line-height: 1.6;
}
</style>
