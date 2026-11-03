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
    <div ref="termRef" v-loading="loading" class="terminal config-terminal">
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
import { copyHint, copyText } from '../utils/clipboard'

const props = defineProps<{ instanceId: number }>()

const toml = ref('')
const loading = ref(false)
const termRef = ref<HTMLElement | null>(null)

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
  // 复用同一套三档兜底：原来 execCommand 返回 false 时也报「已复制」，手机上会谎报成功
  const r = await copyText(toml.value, termRef.value)
  const hint = copyHint(r)
  ElMessage[hint.type](hint.text)
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

@media (max-width: 767px) {
  /* 弹层里 100vh 含被地址栏遮住的部分，终端按视口比例给高，关闭按钮才留在屏内 */
  .config-terminal {
    height: 46vh;
    min-height: 180px;
  }
  .tab-toolbar {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }
  .toolbar-right {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
