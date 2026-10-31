<template>
  <div class="page-container">
    <h2 class="page-title">仪表盘</h2>
    <p class="page-subtitle">frpc-mesh 运行概览 · 数据每 10 秒自动刷新</p>

    <el-row :gutter="16" class="stat-row">
      <el-col v-for="c in statCards" :key="c.key" :xs="12" :sm="8" class="stat-col">
        <div class="stat-card" :class="c.cls">
          <div class="stat-label">
            {{ c.label }}
            <el-tooltip :content="c.tip" placement="top">
              <el-icon class="stat-help"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <div class="stat-value">{{ dash[c.key] }}</div>
          <el-icon class="stat-icon"><component :is="c.icon" /></el-icon>
        </div>
      </el-col>
    </el-row>

    <el-card class="card" shadow="never">
      <template #header>
        <div class="card-header">
          <div class="card-header-text">
            <span class="card-title">FRPS 服务器连接状态</span>
            <span class="card-subtitle">每一行代表一台远程 frps 服务器及其 frpc 连接</span>
          </div>
          <el-button text :icon="Refresh" :loading="store.loading" @click="store.refresh()">
            刷新
          </el-button>
        </div>
      </template>
      <el-table
        :data="dash.instances"
        v-loading="store.loading"
        stripe
        empty-text="暂无 FRPS 服务器，请前往「FRPS 服务器」页面创建"
      >
        <el-table-column prop="name" label="服务器名称" min-width="150">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click="goDetail(row.id)">
              {{ row.name }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" effect="dark" size="small" round>
              {{ statusText(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="frpc 版本" width="170">
          <template #default="{ row }">
            <el-tooltip
              :disabled="!rowVersionFallback(row)"
              :content="FRPC_VERSION_FALLBACK_TIP"
              placement="top"
            >
              <span :class="rowVersionFallback(row) ? 'ver-fallback' : 'mono'">
                {{ rowVersionLabel(row) }}
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="服务器地址" min-width="170">
          <template #default="{ row }">
            <span class="mono">{{ row.server_addr }}:{{ row.server_port }}</span>
          </template>
        </el-table-column>
        <el-table-column label="代理（在线/总数）" width="150">
          <template #default="{ row }">
            <span :class="proxyOnlineClass(row)">{{ row.proxies_online }}</span>
            <span class="proxies-sep">/</span>
            <span>{{ row.proxies_total }}</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #header>
            <span class="th-enabled">
              启用
              <el-tooltip
                content="启用：立即启动并随服务自动拉起；停用：立即停止且不自动启动"
                placement="top"
              >
                <el-icon class="th-help"><QuestionFilled /></el-icon>
              </el-tooltip>
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
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text type="primary" @click="goDetail(row.id)">
              详情
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import type { Component } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  CircleCheck,
  Connection,
  Menu,
  QuestionFilled,
  Refresh,
  Share,
  View
} from '@element-plus/icons-vue'
import { apiGetInstance, apiUpdateInstance } from '../api'
import { useInstancesStore } from '../stores/instances'
import { statusTagType, statusText } from '../utils/status'
import { FRPC_VERSION_FALLBACK_TIP, frpcVersionFallback, frpcVersionLabel } from '../utils/frpcVersion'
import type { Dashboard, DashboardInstance } from '../api/types'

const router = useRouter()
const store = useInstancesStore()
const dash = computed(() => store.dashboard)
const toggling = reactive<Record<number, boolean>>({})

interface StatCard {
  key: keyof Omit<Dashboard, 'instances'>
  label: string
  cls: string
  icon: Component
  tip: string
}

const versionState = computed(() => store.frpcVersions)

/** 仪表盘行来自 /api/dashboard（精简视图），版本信息按 id 回查完整实例 */
function rowVersionLabel(row: DashboardInstance): string {
  const inst = store.getInstance(row.id)
  return inst ? frpcVersionLabel(inst, versionState.value) : ''
}

function rowVersionFallback(row: DashboardInstance): boolean {
  const inst = store.getInstance(row.id)
  return inst ? frpcVersionFallback(inst, versionState.value) : false
}

const statCards: StatCard[] = [
  {
    key: 'instances_total',
    label: '服务器总数',
    cls: 'stat-gradient-blue',
    icon: Menu,
    tip: '已配置的 FRPS 服务器数量，每台对应一个独立 frpc 进程'
  },
  {
    key: 'instances_online',
    label: '在线服务器',
    cls: 'stat-gradient-green',
    icon: CircleCheck,
    tip: 'frpc 已成功登录 frps 并保持连接的服务器数量'
  },
  {
    key: 'proxies_total',
    label: '代理总数',
    cls: 'stat-gradient-purple',
    icon: Share,
    tip: '所有服务器下已启用的代理（穿透隧道）总数，禁用中的与访问器不计'
  },
  {
    key: 'proxies_online',
    label: '在线代理',
    cls: 'stat-gradient-cyan',
    icon: Connection,
    tip: '隧道已建立且本地目标服务可连通的代理数量，即当前真正可用的连接'
  },
  {
    key: 'visitors_total',
    label: '访问器',
    cls: 'stat-gradient-orange',
    icon: View,
    tip: '访问端（visitor）配置数量，用于访问他人的 stcp/xtcp/sudp 加密隧道'
  }
]

/** 在线代理数着色：>0 绿色；为 0 且总数 >0 灰色 */
function proxyOnlineClass(row: DashboardInstance): string {
  if (row.proxies_online > 0) return 'proxies-online-on'
  if (row.proxies_total > 0) return 'proxies-online-off'
  return ''
}

function goDetail(id: number) {
  router.push(`/proxies?instance=${id}`)
}

/** 启用开关：取完整实例翻转 enabled 后 PUT，后端联动启停；失败时开关自动回滚（受控于 row.enabled） */
async function onToggleEnabled(row: DashboardInstance, val: boolean) {
  toggling[row.id] = true
  try {
    const full = await apiGetInstance(row.id, true)
    await apiUpdateInstance(row.id, { ...full, enabled: val })
    ElMessage.success(val ? '已启用，正在启动服务器连接' : '已停用，服务器连接已停止')
    await store.refreshSilently()
  } catch {
    /* 拦截器已提示 */
  } finally {
    toggling[row.id] = false
  }
}
</script>

<style scoped>
.stat-row {
  margin-bottom: 18px;
}

.stat-row .el-col {
  margin-bottom: 12px;
}

@media (min-width: 992px) {
  .stat-row .stat-col {
    flex: 0 0 20%;
    max-width: 20%;
  }
}

.stat-help {
  font-size: 12px;
  margin-left: 4px;
  vertical-align: -1px;
  opacity: 0.75;
  cursor: help;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-header-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.card-title {
  font-weight: 600;
  font-size: 15px;
}

.card-subtitle {
  font-size: 12px;
  color: #909399;
}

.ver-fallback {
  font-size: 12px;
  color: var(--el-color-warning);
}

.proxies-sep {
  color: #c0c4cc;
  margin: 0 4px;
}

.proxies-online-on {
  color: #16a34a;
  font-weight: 600;
}

.proxies-online-off {
  color: #909399;
}

.th-enabled {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.th-help {
  color: #909399;
  cursor: help;
}
</style>
