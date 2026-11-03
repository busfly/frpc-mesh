<template>
  <div class="page-container">
    <div class="page-head">
      <div>
        <h2 class="page-title">FRPS 服务器管理</h2>
        <p class="page-subtitle">每台服务器对应一个远程 frps 与一个独立 frpc 进程，可同时在线</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建 FRPS 服务器</el-button>
    </div>

    <el-card class="card" shadow="never">
      <el-table
        v-if="!isMobile"
        :data="store.instances"
        v-loading="store.loading"
        stripe
        empty-text="暂无服务器，点击右上角「新建 FRPS 服务器」开始"
      >
        <el-table-column prop="name" label="服务器名称" min-width="150">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click="goDetail(row.id)">
              {{ row.name }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="服务器" min-width="170">
          <template #default="{ row }">
            <span class="mono">{{ row.server_addr }}:{{ row.server_port }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tooltip
              :disabled="!row.runtime?.last_error"
              :content="row.runtime?.last_error"
              placement="top"
            >
              <el-tag :type="statusTagType(row.runtime?.status)" effect="dark" size="small" round>
                {{ statusText(row.runtime?.status) }}
              </el-tag>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="frpc 版本" width="170">
          <template #default="{ row }">
            <el-tooltip
              :disabled="!frpcVersionFallback(row, versionState)"
              :content="FRPC_VERSION_FALLBACK_TIP"
              placement="top"
            >
              <span :class="frpcVersionFallback(row, versionState) ? 'ver-fallback' : 'mono'">
                {{ frpcVersionLabel(row, versionState) }}
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="代理（在线/总数）" width="130">
          <template #default="{ row }">
            <span :class="proxyStats[row.id]?.online ? 'cnt-on' : 'cnt-off'">
              {{ proxyStats[row.id]?.online ?? '-' }}
            </span>
            <span class="proxies-sep">/</span>
            <span>{{ proxyStats[row.id]?.total ?? '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #header>
            <span class="th-enabled">
              启用
<FieldHint content="启用：立即启动并随服务自动拉起；停用：立即停止且不自动启动" />
            </span>
          </template>
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :loading="!!toggling[row.id]"
              @change="onToggleEnabled(row, $event as boolean)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <div class="op-cell">
              <el-button
                size="small"
                text
                type="primary"
                :loading="!!store.acting[row.id]"
                @click="onRestart(row)"
              >
                重启
              </el-button>
              <el-divider direction="vertical" />
              <el-tooltip
                :disabled="row.manage_port > 0"
                content="未配置管理端口，请在编辑中填写"
                placement="top"
              >
                <span>
                  <el-button
                    size="small"
                    text
                    type="primary"
                    :disabled="!(row.manage_port > 0)"
                    @click="openManage(row)"
                  >
                    管理
                  </el-button>
                </span>
              </el-tooltip>
              <el-divider direction="vertical" />
              <el-button size="small" text type="primary" @click="openEdit(row)">编辑</el-button>
              <el-divider direction="vertical" />
              <el-button size="small" text type="danger" @click="onDelete(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <InstanceCardList
        v-else
        :rows="cardRows"
        empty-text="暂无服务器，点击右上角「新建 FRPS 服务器」开始"
        @detail="goDetail"
        @toggle="onToggleEnabledById"
        @more="openActions"
      />
    </el-card>

    <InstanceFormDrawer v-model="drawerVisible" :instance="editing" />

    <ActionSheet
      v-model="sheetVisible"
      :title="sheetTitle"
      :actions="sheetActions"
      @action="onAction"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import InstanceFormDrawer from '../components/InstanceFormDrawer.vue'
import { apiListProxies, apiProxiesOnline, apiUpdateInstance } from '../api'
import type { Instance } from '../api/types'
import { useInstancesStore } from '../stores/instances'
import { statusTagType, statusText } from '../utils/status'
import InstanceCardList, { type InstanceCardRow } from '../components/mobile/InstanceCardList.vue'
import ActionSheet, { type ActionItem } from '../components/mobile/ActionSheet.vue'
import FieldHint from '../components/mobile/FieldHint.vue'
import { useIsMobile } from '../composables/useMediaQuery'
import { FRPC_VERSION_FALLBACK_TIP, frpcVersionFallback, frpcVersionLabel } from '../utils/frpcVersion'

const router = useRouter()
const store = useInstancesStore()

const drawerVisible = ref(false)
const editing = ref<Instance | null>(null)
const toggling = reactive<Record<number, boolean>>({})
const proxyStats = reactive<Record<number, { total: number; online: number }>>({})

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

/** 操作项与桌面表格操作列同权；「管理」在没配端口时禁用并写明原因 */
const sheetActions = computed<ActionItem[]>(() => {
  const i = store.getInstance(sheetId.value)
  const manageable = !!i && i.manage_port > 0
  return [
    { key: 'edit', label: '编辑' },
    { key: 'restart', label: '重启', hint: '隧道中断数秒后自动恢复' },
    { key: 'manage', label: '管理面板', disabled: !manageable, hint: manageable ? '' : '未配置管理端口' },
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

/** 卡片开关只回传 id，复用既有的 onToggleEnabled */
async function onToggleEnabledById(id: number, val: boolean) {
  const row = store.getInstance(id)
  if (row) await onToggleEnabled(row, val)
}

/** 尽力获取每台服务器的代理总数与在线数，失败静默（显示 '-'） */
async function refreshProxyCounts() {
  await Promise.all(
    store.instances.map(async (inst) => {
      try {
        const [list, on] = await Promise.all([
          apiListProxies(inst.id),
          apiProxiesOnline(inst.id)
        ])
        proxyStats[inst.id] = {
          total: list?.length ?? 0,
          online: (on?.online_ids || []).length
        }
      } catch {
        /* 静默 */
      }
    })
  )
}

const versionState = computed(() => store.frpcVersions)

onMounted(refreshProxyCounts)
watch(() => store.instances.map((i) => `${i.id}:${i.updated_at}`).join(','), refreshProxyCounts)

/** 名称点击 → 穿透代理页对应实例 Tab */
function goDetail(id: number) {
  router.push(`/proxies?instance=${id}`)
}

function openCreate() {
  editing.value = null
  drawerVisible.value = true
}

function openEdit(row: Instance) {
  editing.value = row
  drawerVisible.value = true
}

/** 打开 frps 管理面板（server_addr + manage_port 组装） */
function openManage(row: Instance) {
  if (!(row.manage_port > 0)) return
  window.open(`http://${row.server_addr}:${row.manage_port}`, '_blank')
}

/** 启用开关：翻转 enabled 后 PUT 完整实例，后端联动启停；失败时开关自动回滚（受控于 row.enabled） */
async function onToggleEnabled(row: Instance, val: boolean) {
  toggling[row.id] = true
  try {
    await apiUpdateInstance(row.id, { ...row, enabled: val } as Partial<Instance>)
    ElMessage.success(val ? '已启用，正在启动服务器连接' : '已停用，服务器连接已停止')
    await store.refreshSilently()
  } catch {
    /* 拦截器已提示 */
  } finally {
    toggling[row.id] = false
  }
}

async function onRestart(row: Instance) {
  try {
    await store.restart(row.id)
    ElMessage.success(`服务器「${row.name}」重启指令已下发`)
  } catch {
    /* 拦截器已提示 */
  }
}

function onDelete(row: Instance) {
  ElMessageBox.confirm(
    `确定删除服务器「${row.name}」吗？其下所有代理/访问器配置也将被删除，运行中的进程会先被停止。此操作不可恢复。`,
    '删除确认',
    {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    }
  )
    .then(async () => {
      const { apiDeleteInstance } = await import('../api')
      await apiDeleteInstance(row.id)
      ElMessage.success('服务器已删除')
      await store.refreshSilently()
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

.th-enabled {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}


.op-cell {
  display: flex;
  align-items: center;
  white-space: nowrap;
}

.ver-fallback {
  font-size: 12px;
  color: var(--el-color-warning);
}

.proxies-sep {
  margin: 0 3px;
  color: #c0c4cc;
}

.cnt-on {
  color: #67c23a;
  font-weight: 600;
}

.cnt-off {
  color: #c0c4cc;
}
</style>
