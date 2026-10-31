<template>
  <div>
    <div class="tab-toolbar">
      <div class="toolbar-left">
        <span class="text-muted count-tip">最近 500 行 frpc 输出</span>
        <div class="auto-refresh">
          <span class="text-muted auto-label">每 5 秒自动刷新</span>
          <el-switch
            v-model="autoRefresh"
            size="small"
            :disabled="!running"
          />
        </div>
        <el-tooltip v-if="!running" content="仅服务器运行时可自动刷新" placement="top">
          <el-icon color="#c0c4cc" :size="14"><InfoFilled /></el-icon>
        </el-tooltip>
      </div>
      <div class="toolbar-right">
        <el-button type="danger" plain size="small" :loading="clearing" @click="onClear">
          清空
        </el-button>
        <el-button text :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <div ref="termRef" v-loading="loading" class="terminal log-terminal">
      <template v-if="lines.length">
        <div v-for="(line, idx) in lines" :key="idx" :class="lineClass(line)">{{ line }}</div>
      </template>
      <div v-else class="text-muted log-empty">暂无日志输出（服务器可能未启动）</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { InfoFilled, Refresh } from '@element-plus/icons-vue'
import { apiClearInstanceLogs, apiInstanceLogs } from '../api'

const props = defineProps<{ instanceId: number; running: boolean }>()

const lines = ref<string[]>([])
const loading = ref(false)
const clearing = ref(false)
const autoRefresh = ref(false)
const termRef = ref<HTMLDivElement>()
let timer: ReturnType<typeof setInterval> | null = null

function lineClass(line: string): string {
  const lower = line.toLowerCase()
  if (lower.includes('[e]') || lower.includes('error') || lower.includes('fail')) return 'log-error'
  if (lower.includes('[w]') || lower.includes('warn')) return 'log-warn'
  if (lower.includes('login') || lower.includes('start') || lower.includes('success')) return 'log-ok'
  return ''
}

async function load(scroll = true) {
  loading.value = true
  try {
    const data = await apiInstanceLogs(props.instanceId, 500)
    lines.value = data.lines || []
    if (scroll) {
      await nextTick()
      if (termRef.value) {
        termRef.value.scrollTop = termRef.value.scrollHeight
      }
    }
  } catch {
    lines.value = []
  } finally {
    loading.value = false
  }
}

function onClear() {
  ElMessageBox.confirm('确定清空该服务器的日志缓冲吗？此操作不可恢复。', '清空确认', {
    confirmButtonText: '清空',
    cancelButtonText: '取消',
    type: 'warning'
  })
    .then(async () => {
      clearing.value = true
      try {
        await apiClearInstanceLogs(props.instanceId)
        ElMessage.success('日志已清空')
        await load()
      } catch {
        /* 拦截器已提示 */
      } finally {
        clearing.value = false
      }
    })
    .catch(() => undefined)
}

function startTimer() {
  stopTimer()
  timer = setInterval(() => {
    if (document.visibilityState === 'visible') {
      load(false)
    }
  }, 5000)
}

function stopTimer() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

watch(autoRefresh, (v) => {
  if (v) startTimer()
  else stopTimer()
})

watch(
  () => props.running,
  (v) => {
    if (!v && autoRefresh.value) {
      autoRefresh.value = false
    }
  }
)

onMounted(() => load())
onBeforeUnmount(stopTimer)
</script>

<style scoped>
.tab-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.count-tip {
  font-size: 13px;
}

.auto-refresh {
  display: flex;
  align-items: center;
  gap: 6px;
}

.auto-label {
  font-size: 12px;
}

.log-terminal {
  height: calc(100vh - 420px);
  min-height: 320px;
}

.log-terminal :deep(.el-loading-mask) {
  background-color: rgba(13, 17, 23, 0.6);
}

.log-error {
  color: #f85149;
}

.log-warn {
  color: #d29922;
}

.log-ok {
  color: #3fb950;
}

.log-empty {
  padding: 20px 0;
}
</style>
