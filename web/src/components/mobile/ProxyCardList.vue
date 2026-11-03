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
