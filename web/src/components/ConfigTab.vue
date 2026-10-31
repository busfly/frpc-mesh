<template>
  <div>
    <div class="tab-toolbar">
      <span class="text-muted count-tip">配置文件预览（高级配置页面）：后端生成的 frpc TOML，只读</span>
      <div>
        <el-button text :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button type="primary" plain size="small" :icon="CopyDocument" @click="copy">
          复制
        </el-button>
      </div>
    </div>
    <div v-loading="loading" class="terminal config-terminal">
      <template v-if="toml">{{ toml }}</template>
      <div v-else class="text-muted log-empty">暂无配置内容</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Refresh } from '@element-plus/icons-vue'
import { apiInstanceConfig } from '../api'

const props = defineProps<{ instanceId: number }>()

const toml = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const data = await apiInstanceConfig(props.instanceId)
    toml.value = data.toml || ''
  } catch {
    toml.value = ''
  } finally {
    loading.value = false
  }
}

async function copy() {
  if (!toml.value) {
    ElMessage.warning('没有可复制的内容')
    return
  }
  try {
    await navigator.clipboard.writeText(toml.value)
    ElMessage.success('已复制到剪贴板')
  } catch {
    // 降级方案
    const ta = document.createElement('textarea')
    ta.value = toml.value
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    ElMessage.success('已复制到剪贴板')
  }
}

onMounted(load)
defineExpose({ reload: load })
</script>

<style scoped>
.tab-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.count-tip {
  font-size: 13px;
}

.config-terminal {
  height: calc(100vh - 420px);
  min-height: 320px;
  color: #79c0ff;
}

.config-terminal :deep(.el-loading-mask) {
  background-color: rgba(13, 17, 23, 0.6);
}

.log-empty {
  padding: 20px 0;
}
</style>
