<template>
  <div class="page-container">
    <h2 class="page-title">frpc 版本管理</h2>
    <p class="page-subtitle">在线下载与切换 frpc 二进制版本，切换后运行中的服务器将自动重启生效</p>

    <el-card class="card status-card" shadow="never">
      <div class="status-row">
        <div class="status-items">
          <div class="status-item">
            <div class="status-label">生效版本</div>
            <div class="status-value mono">
              {{ data.active ? `v${data.active}` : '自定义路径/系统 PATH' }}
            </div>
          </div>
          <div class="status-item">
            <div class="status-label">当前二进制实际版本</div>
            <div class="status-value mono">{{ data.current ? `v${data.current}` : '-' }}</div>
          </div>
          <div class="status-item">
            <div class="status-label">已下载版本数</div>
            <div class="status-value">{{ downloadedCount }}</div>
          </div>
        </div>
        <div class="status-source">
          <span v-if="syncing" class="sync-hint">
            <el-icon class="is-loading"><Loading /></el-icon>
            <span class="text-muted">同步远程版本中…</span>
          </span>
          <span class="text-muted source-label">数据源</span>
          <el-select v-model="source" :disabled="busy" style="width: 150px" @change="syncRemote()">
            <el-option label="GitHub 官方" value="github" />
            <el-option label="ghfast 镜像" value="ghfast" />
          </el-select>
          <el-button
            text
            :icon="Refresh"
            :disabled="busy || syncing"
            title="同步远程版本"
            @click="syncRemote()"
          />
        </div>
      </div>
    </el-card>

    <el-alert
      v-if="listError"
      class="list-alert"
      type="error"
      show-icon
      :closable="false"
      :title="`远程版本同步失败：${listError}，当前展示本地已下载版本，可切换数据源后重试`"
    />

    <el-row :gutter="12" class="version-grid">
      <el-col
        v-for="v in displayVersions"
        :key="v.version"
        :xs="24"
        :sm="12"
        :md="8"
        :lg="6"
        class="version-col"
      >
        <el-card
          class="card version-card"
          shadow="never"
          :class="{ 'is-busy': busy }"
          :body-style="{ padding: '12px 14px' }"
        >
          <div class="version-head">
            <span class="version-name mono">v{{ v.version }}</span>
            <el-tag v-if="v.active" type="success" effect="dark" size="small" round>
              使用中
            </el-tag>
            <el-tag v-else-if="v.downloaded" type="primary" effect="plain" size="small" round>
              已下载
            </el-tag>
          </div>
          <div v-if="hasMeta(v)" class="version-meta">
            <span v-if="v.size_bytes && v.size_bytes > 0" class="mono">
              {{ formatSize(v.size_bytes) }}
            </span>
            <span v-if="v.download_count && v.download_count > 0" class="mono">
              {{ v.download_count }} 次下载
            </span>
            <span v-if="v.published_at" class="mono">{{ formatDate(v.published_at) }}</span>
          </div>
          <div class="version-actions">
            <template v-if="v.active">
              <span class="text-muted current-text">当前版本</span>
            </template>
            <template v-else-if="v.downloaded">
              <el-button size="small" type="primary" :disabled="busy" @click="onActivate(v)">
                使用此版本
              </el-button>
              <el-button size="small" type="danger" text :disabled="busy" @click="onDelete(v)">
                删除
              </el-button>
            </template>
            <template v-else>
              <el-button
                size="small"
                type="primary"
                :icon="Download"
                :loading="downloadingVersion === v.version"
                :disabled="busy"
                @click="onDownload(v)"
              >
                {{ downloadingVersion === v.version ? '下载中…' : '下载' }}
              </el-button>
            </template>
            <el-link
              type="primary"
              :underline="false"
              class="release-link"
              :href="releaseUrl(v.version)"
              target="_blank"
            >
              下载链接
            </el-link>
          </div>
        </el-card>
      </el-col>
      <el-col v-if="!syncing && !displayVersions.length" :span="24">
        <el-empty description="暂无版本数据" />
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Download, Loading, Refresh } from '@element-plus/icons-vue'
import {
  activateVersion,
  deleteVersion,
  downloadVersion,
  getDownloadStatus,
  getVersions,
  getVersionsLocal
} from '../api'
import type { VersionInfo } from '../api/types'

const source = ref<'github' | 'ghfast'>('github')
const syncing = ref(false)
const downloadingVersion = ref('')
const listError = ref('')

// 主列表：首屏为本地已下载版本，远程同步成功后被合并列表（本地+远程，降序）覆盖
const data = reactive({
  active: '',
  current: '',
  versions: [] as VersionInfo[]
})

const busy = computed(() => !!downloadingVersion.value)

const displayVersions = computed<VersionInfo[]>(() => data.versions)

const downloadedCount = computed(() => data.versions.filter((v) => v.downloaded).length)

/** 元信息整块仅在至少一项有值时渲染，避免出现一排 dash */
function hasMeta(v: VersionInfo) {
  return (v.size_bytes ?? 0) > 0 || (v.download_count ?? 0) > 0 || !!v.published_at
}

function formatSize(bytes: number) {
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

function formatDate(s: string) {
  return s.slice(0, 10)
}

function releaseUrl(version: string) {
  return `https://github.com/fatedier/frp/releases/tag/v${version}`
}

/** 第一阶段：本地已下载版本，秒回，立即渲染 */
async function loadLocal() {
  try {
    const resp = await getVersionsLocal()
    data.active = resp.active || ''
    data.current = resp.current || ''
    data.versions = resp.versions || []
  } catch {
    /* 拦截器已提示 */
  }
}

/** 第二阶段：后台同步远程版本，不阻塞已渲染的本地列表 */
async function syncRemote() {
  syncing.value = true
  listError.value = ''
  try {
    const resp = await getVersions(source.value)
    data.active = resp.active || ''
    data.current = resp.current || ''
    // 后端已合并本地+远程并按版本降序，直接覆盖
    data.versions = resp.versions || []
    listError.value = resp.error || ''
  } catch {
    // 保留本地列表，仅提示
    listError.value = '请求失败'
  } finally {
    syncing.value = false
  }
}

/** 操作后的刷新：本地秒回更新状态，远程在后台跟进 */
async function refresh() {
  await loadLocal()
  syncRemote()
}

async function onDownload(v: VersionInfo) {
  downloadingVersion.value = v.version
  try {
    await downloadVersion(v.version, source.value)
    // 后端为异步任务，轮询直到 done/error
    for (;;) {
      await new Promise((r) => setTimeout(r, 2000))
      const st = await getDownloadStatus(v.version)
      if (st.status === 'done') {
        ElMessage.success(`版本 v${v.version} 下载完成`)
        break
      }
      if (st.status === 'error') {
        ElMessage.error(`下载失败：${st.message || '未知错误'}`)
        break
      }
      if (st.status === 'none') {
        // 任务记录丢失（如服务重启），以本地是否已有该版本为准
        await loadLocal()
        const has = data.versions.some((x) => x.version === v.version && x.downloaded)
        if (has) ElMessage.success(`版本 v${v.version} 下载完成`)
        else ElMessage.warning('下载任务已中断')
        break
      }
    }
    await refresh()
  } catch {
    /* 拦截器已提示 */
  } finally {
    downloadingVersion.value = ''
  }
}

async function onActivate(v: VersionInfo) {
  const ok = await ElMessageBox.confirm('切换后所有运行中的服务器将自动重启以生效', `使用 v${v.version}？`, {
    confirmButtonText: '切换',
    cancelButtonText: '取消',
    type: 'warning'
  }).catch(() => false)
  if (!ok) return
  try {
    await activateVersion(v.version)
    ElMessage.success(`已切换至 v${v.version}，运行中服务器已重启`)
    await refresh()
  } catch {
    /* 拦截器已提示 */
  }
}

async function onDelete(v: VersionInfo) {
  const ok = await ElMessageBox.confirm(`确定删除本地版本 v${v.version} 的二进制文件吗？`, '删除确认', {
    confirmButtonText: '删除',
    cancelButtonText: '取消',
    type: 'warning'
  }).catch(() => false)
  if (!ok) return
  try {
    await deleteVersion(v.version)
    ElMessage.success(`版本 v${v.version} 已删除`)
    await refresh()
  } catch {
    /* 拦截器已提示 */
  }
}

onMounted(async () => {
  await loadLocal()
  syncRemote()
})
</script>

<style scoped>
.status-card {
  margin-bottom: 16px;
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.status-items {
  display: flex;
  align-items: center;
  gap: 40px;
  flex-wrap: wrap;
}

.status-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 4px;
}

.status-value {
  font-size: 20px;
  font-weight: 700;
  color: #1f2433;
  line-height: 1.3;
}

.status-source {
  display: flex;
  align-items: center;
  gap: 10px;
}

.sync-hint {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.source-label {
  font-size: 13px;
}

.list-alert {
  margin-bottom: 16px;
}

.version-col {
  margin-bottom: 12px;
}

.version-card {
  height: 100%;
  border-left: 4px solid #4f6ef7;
  transition: opacity 0.2s;
}

.version-card.is-busy {
  opacity: 0.75;
}

.version-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.version-name {
  font-size: 16px;
  font-weight: 700;
  color: #1f2433;
}

.version-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
  color: #909399;
  margin-top: 6px;
}

.version-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-top: 10px;
}

.release-link {
  font-size: 13px;
}

.current-text {
  font-size: 13px;
}
</style>
