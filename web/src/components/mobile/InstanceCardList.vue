<template>
  <div class="fm-cards">
    <div v-for="row in rows" :key="row.id" class="fm-card">
      <div class="fm-card-row1">
        <span class="status-dot" :style="{ background: statusDotColor(row.status) }"></span>
        <span class="fm-card-name" @click="emit('detail', row.id)">{{ row.name }}</span>
        <span class="fm-card-state">{{ statusText(row.status) }}</span>
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
