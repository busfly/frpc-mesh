<template>
  <div class="page-container">
    <h2 class="page-title">设置</h2>
    <p class="page-subtitle">运行配置与账号安全</p>

    <el-row :gutter="16">
      <el-col :xs="24" :md="14">
        <el-card class="card setting-card" shadow="never" v-loading="loadingSettings">
          <template #header>
            <span class="card-title">运行配置</span>
          </template>
          <el-form :model="settingsForm" label-position="top">
            <el-form-item label="当前生效的 frpc">
              <div class="frpc-current">
                <span v-if="frpcCurrent" class="mono">v{{ frpcCurrent }}</span>
                <span v-else class="text-muted">未检测到</span>
                <el-link type="primary" :underline="false" @click="goVersions">
                  前往 frpc 版本管理
                </el-link>
              </div>
            </el-form-item>

            <el-form-item label="服务启动时自动拉起所有已启用服务器 (auto_start)">
              <el-switch v-model="settingsForm.auto_start" />
            </el-form-item>

            <el-form-item>
              <template #label>
                <span class="label-with-hint">
                  定时检查 frpc-mesh 新版本 (auto_update_check)
                  <el-tooltip
                    placement="top"
                    content="每 6 小时联网检查一次 GitHub Releases，仅在此页与顶栏提醒，不会自动下载或升级"
                  >
                    <el-icon class="hint-icon"><QuestionFilled /></el-icon>
                  </el-tooltip>
                </span>
              </template>
              <el-switch v-model="settingsForm.auto_update_check" />
            </el-form-item>

            <el-form-item>
              <el-button type="primary" :loading="savingSettings" @click="onSaveSettings">
                保存设置
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
        <el-card class="card setting-card" shadow="never" v-loading="update.loading">
          <template #header>
            <div class="card-head">
              <span class="card-title">
                检查新版本
                <el-tooltip
                  placement="right"
                  content="检查 GitHub Releases 上 frpc-mesh 的最新版本；二进制运行可就地升级，容器运行请换镜像重建"
                >
                  <el-icon class="hint-icon"><QuestionFilled /></el-icon>
                </el-tooltip>
              </span>
              <el-select
                v-model="checkSource"
                size="small"
                style="width: 140px"
                :disabled="updateBusy"
              >
                <el-option label="GitHub 官方" value="github" />
                <el-option label="ghfast 镜像" value="ghfast" />
              </el-select>
            </div>
          </template>

          <div class="update-cols">
            <div class="update-item">
              <div class="update-label">当前版本</div>
              <div class="update-value mono">v{{ info?.current || '-' }}</div>
            </div>
            <div class="update-item">
              <div class="update-label">最新版本</div>
              <div class="update-value mono">
                <template v-if="info?.latest">v{{ info.latest }}</template>
                <span v-else class="text-muted">-</span>
                <el-tag v-if="update.hasUpdate" type="warning" size="small" effect="dark" round>
                  可升级
                </el-tag>
                <el-tag
                  v-else-if="info?.latest"
                  type="success"
                  size="small"
                  effect="plain"
                  round
                >
                  已是最新
                </el-tag>
              </div>
            </div>
          </div>

          <div class="update-meta">
            <template v-if="updateMeta.length">
              <span v-for="m in updateMeta" :key="m">{{ m }}</span>
            </template>
            <span v-else>尚未检查过，点击「检查更新」获取最新版本</span>
            <span v-if="info && !info.auto_check">自动检查已关闭</span>
            <el-link
              v-if="info?.release_url"
              type="primary"
              :underline="false"
              class="notes-link"
              @click="notesVisible = true"
            >
              版本说明
            </el-link>
          </div>

          <el-alert
            v-if="info?.error"
            class="update-alert"
            type="error"
            show-icon
            :closable="false"
            :title="`检查失败：${info.error}`"
            description="可切换数据源后重试，或直接前往 GitHub Releases 手动下载"
          />

          <div v-if="capability && info?.latest" class="update-env">
            <el-tag :type="envTagType" size="small" effect="plain">{{ envLabel }}</el-tag>
            <span class="env-reason">{{ capability.reason }}</span>
            <el-button
              v-if="capability.guide"
              class="guide-toggle"
              text
              type="primary"
              size="small"
              @click="guideOpen = !guideOpen"
            >
              {{ guideOpen ? '收起升级步骤' : '怎么升级？看步骤' }}
            </el-button>
          </div>
          <UpgradeGuide v-if="guideOpen && capability?.guide" :guide="capability.guide" />

          <div v-if="update.running" class="upgrade-progress">
            <el-icon class="is-loading"><Loading /></el-icon>
            <span>{{ jobLabel }}</span>
          </div>
          <el-alert
            v-else-if="update.job?.state === 'error'"
            class="update-alert"
            type="error"
            show-icon
            :closable="false"
            :title="`升级失败：${update.job.message}`"
            description="原二进制已保留，可修正网络后重试或手动下载替换"
          />

          <div class="update-actions">
            <el-button
              :icon="Refresh"
              :loading="update.checking"
              :disabled="updateBusy"
              @click="onCheck"
            >
              检查更新
            </el-button>
            <!-- 容器环境有分步引导时不再放这个点不动的主按钮：入口只留「怎么升级？看步骤」 -->
            <el-button
              v-if="!hasGuide"
              type="primary"
              :loading="updateBusy"
              :disabled="!canUpgradeNow"
              @click="onUpgrade"
            >
              {{ upgradeLabel }}
            </el-button>
          </div>
        </el-card>
        <el-card id="backup" class="card setting-card" shadow="never">
          <template #header>
            <span class="card-title">备份与恢复</span>
          </template>
          <p class="backup-desc text-muted">
            将全部 FRPS 服务器与代理配置导出为一个 JSON 文件；导入时以备份数据为准，覆盖当前全部服务器与代理配置。
          </p>
          <div class="backup-actions">
            <el-button :icon="Download" :loading="exporting" @click="onExport">导出备份</el-button>
            <el-button type="warning" plain :icon="Upload" :loading="restoring" @click="fileInput?.click()">
              导入并恢复
            </el-button>
            <input
              ref="fileInput"
              type="file"
              accept=".json,application/json"
              class="hidden-file"
              @change="onFileChange"
            />
          </div>
        </el-card>
      </el-col>

      <el-col :xs="24" :md="10">
        <el-card class="card setting-card" shadow="never">
          <template #header>
            <span class="card-title">修改密码</span>
          </template>
          <el-form
            ref="pwdFormRef"
            :model="pwdForm"
            :rules="pwdRules"
            label-position="top"
          >
            <el-form-item label="旧密码" prop="old_password">
              <el-input
                v-model="pwdForm.old_password"
                type="password"
                show-password
                placeholder="请输入当前密码"
                autocomplete="current-password"
              />
            </el-form-item>
            <el-form-item label="新密码" prop="new_password">
              <el-input
                v-model="pwdForm.new_password"
                type="password"
                show-password
                placeholder="至少 6 位"
                autocomplete="new-password"
              />
            </el-form-item>
            <el-form-item label="确认新密码" prop="confirm_password">
              <el-input
                v-model="pwdForm.confirm_password"
                type="password"
                show-password
                placeholder="再次输入新密码"
                autocomplete="new-password"
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="changingPwd" @click="onChangePassword">
                修改密码
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card class="card setting-card about-card" shadow="never">
          <template #header>
            <span class="card-title">关于</span>
          </template>
          <div class="about-row">
            <span class="text-muted">产品</span>
            <span class="brand-text about-brand">frpc-mesh</span>
          </div>
          <div class="about-row">
            <span class="text-muted">版本</span>
            <span class="mono">v{{ auth.version || '-' }}</span>
          </div>
          <div class="about-row about-block">
            <span class="text-muted">定位</span>
            <span>多服务器 frp 内网穿透 Web 管理工具</span>
          </div>
          <div class="about-desc">
            可同时连接多台远程 FRPS 服务器，每台服务器运行一个独立的 frpc 进程，
            所有穿透隧道在本界面统一管理；支持在线下载与切换 frpc 版本。
            适合部署于 NAS、软路由与任何 Docker 主机，配置与隧道数据保存于数据目录的
            SQLite 数据库，备份数据目录即完成备份。
          </div>
          <div class="about-links">
            <el-link
              type="primary"
              :underline="false"
              :href="REPO_URL"
              target="_blank"
              rel="noopener noreferrer"
            >
              开源仓库
            </el-link>
            <el-link
              type="primary"
              :underline="false"
              :href="REPO_ISSUES_URL"
              target="_blank"
              rel="noopener noreferrer"
            >
              提 Issue 反馈
            </el-link>
            <el-link
              type="primary"
              :underline="false"
              :href="REPO_RELEASES_URL"
              target="_blank"
              rel="noopener noreferrer"
            >
              Release 更新日志
            </el-link>
          </div>
          <div class="about-star">觉得有用？去 GitHub 给个 Star，帮助更多人发现它。</div>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog
      v-model="notesVisible"
      :title="notesTitle"
      width="640px"
    >
      <div v-if="info" class="notes-head">
        <span v-if="info.published_at" class="text-muted">
          发布于 {{ info.published_at.slice(0, 10) }}
        </span>
        <el-link
          type="primary"
          :underline="false"
          :href="info.release_url"
          target="_blank"
          rel="noopener noreferrer"
        >
          在 GitHub 查看
        </el-link>
      </div>
      <pre v-if="info?.notes" class="notes-body">{{ info.notes }}</pre>
      <p v-else class="text-muted">该版本没有填写说明</p>
      <div v-if="info?.assets.length" class="assets">
        <div class="assets-title">安装包</div>
        <div v-for="a in info.assets" :key="a.name" class="asset-row">
          <span class="mono asset-name">{{ a.name }}</span>
          <span class="text-muted">{{ assetSize(a.size_bytes) }}</span>
          <el-link
            type="primary"
            :underline="false"
            :href="a.url"
            target="_blank"
            rel="noopener noreferrer"
          >
            下载
          </el-link>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Download, Loading, QuestionFilled, Refresh, Upload } from '@element-plus/icons-vue'
import {
  apiChangePassword,
  apiExportBackup,
  apiGetSettings,
  apiRestoreBackup,
  apiSaveSettings,
  getVersionsLocal
} from '../api'
import type { BackupData } from '../api/types'
import { REPO_ISSUES_URL, REPO_RELEASES_URL, REPO_URL } from '../api/repo'
import UpgradeGuide from '../components/UpgradeGuide.vue'
import { useAuthStore } from '../stores/auth'
import { useInstancesStore } from '../stores/instances'
import { useUpdateStore } from '../stores/update'
import { formatTime } from '../utils/status'

const auth = useAuthStore()
const router = useRouter()
const instStore = useInstancesStore()
const update = useUpdateStore()

const loadingSettings = ref(false)
const savingSettings = ref(false)
const frpcCurrent = ref('')

const settingsForm = reactive({
  auto_start: true,
  auto_update_check: true
})

function goVersions() {
  router.push('/versions')
}

async function loadSettings() {
  loadingSettings.value = true
  try {
    const s = await apiGetSettings()
    settingsForm.auto_start = !!s.auto_start
    settingsForm.auto_update_check = !!s.auto_update_check
    if (s.version) auth.version = s.version
    const lv = await getVersionsLocal()
    frpcCurrent.value = lv.current || ''
  } catch {
    /* 拦截器已提示 */
  } finally {
    loadingSettings.value = false
  }
}

async function onSaveSettings() {
  savingSettings.value = true
  try {
    await apiSaveSettings({
      auto_start: settingsForm.auto_start,
      auto_update_check: settingsForm.auto_update_check
    })
    ElMessage.success('设置已保存')
    await update.load()
  } catch {
    /* 拦截器已提示 */
  } finally {
    savingSettings.value = false
  }
}

// ---------- frpc-mesh 版本更新 ----------

const checkSource = ref<'github' | 'ghfast'>('github')
const notesVisible = ref(false)
// 升级步骤就地展开，不持久化：刷新后回到收起，避免卡片长期变高
const guideOpen = ref(false)

const info = computed(() => update.info)
const capability = computed(() => update.capability)
const updateBusy = computed(() => update.running || update.upgrading)
const canUpgradeNow = computed(() => update.canUpgrade && update.hasUpdate && !updateBusy.value)
// 后端只在容器环境且真有新版本时下发 guide；有分步引导时它就是唯一升级入口
const hasGuide = computed(() => !!capability.value?.guide)

const envLabel = computed(() => {
  const mode = capability.value?.mode
  if (mode === 'binary') return '二进制运行'
  if (mode === 'container') return '容器运行'
  return '需手动升级'
})

const envTagType = computed<'success' | 'warning' | 'info'>(() => {
  const mode = capability.value?.mode
  if (mode === 'binary') return 'success'
  if (mode === 'container') return 'warning'
  return 'info'
})

const upgradeLabel = computed(() => {
  const i = info.value
  if (!i?.latest) return '立即升级'
  return update.hasUpdate ? `升级到 v${i.latest}` : '已是最新'
})

const updateMeta = computed(() => {
  const i = info.value
  if (!i) return []
  const out: string[] = []
  if (i.published_at) out.push(`发布于 ${i.published_at.slice(0, 10)}`)
  if (i.checked_at) out.push(`上次检查 ${formatTime(i.checked_at)}`)
  if (i.checked_at) out.push(`数据源 ${i.source === 'ghfast' ? 'ghfast 镜像' : 'GitHub'}`)
  if (update.status?.last_upgrade) out.push(`最近升级 ${update.status.last_upgrade}`)
  return out
})

const jobLabel = computed(() => {
  const j = update.job
  if (!j) return ''
  const labels: Record<string, string> = {
    downloading: '下载安装包',
    verifying: '校验安装包',
    swapping: '替换二进制',
    restarting: '重启服务'
  }
  return `${labels[j.state] || j.state}：${j.message || '请稍候…'}`
})

function assetSize(bytes: number) {
  if (!bytes) return ''
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

const notesTitle = computed(() => {
  const i = info.value
  if (!i?.latest) return '版本说明'
  const name = i.release_name || ''
  return name.includes(i.latest) ? name : `v${i.latest} ${name}`.trim()
})

async function onCheck() {
  await update.check(checkSource.value)
  const i = update.info
  if (i?.error) ElMessage.error(i.error)
  else if (!i?.latest) ElMessage.error('未获取到最新版本信息')
  else if (update.hasUpdate) ElMessage.success(`发现新版本 v${i.latest}`)
  else ElMessage.success(`已是最新版本 v${i.current}`)
}

async function onUpgrade() {
  const latest = info.value?.latest
  if (!latest) return
  const ok = await ElMessageBox.confirm(
    `将下载 v${latest} 校验后替换当前二进制并重启服务，所有穿透隧道会中断数秒后自动恢复。确定开始升级？`,
    '升级确认',
    {
      confirmButtonText: '开始升级',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    }
  ).catch(() => false)
  if (!ok) return
  try {
    const job = await update.upgrade(latest)
    await auth.fetchVersion()
    if (job?.state === 'error') ElMessage.error(`升级失败：${job.message}`)
    else if (auth.version === latest) ElMessage.success(`已升级到 v${latest}，隧道已自动恢复`)
    else ElMessage.warning('升级任务已执行，服务可能仍在重启，请稍后刷新页面确认版本')
    await instStore.refreshSilently()
  } catch {
    /* 拦截器已提示 */
  }
}

// 修改密码
const pwdFormRef = ref<FormInstance>()
const changingPwd = ref(false)
const pwdForm = reactive({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

const pwdRules: FormRules = {
  old_password: [{ required: true, message: '请输入旧密码', trigger: 'blur' }],
  new_password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '新密码至少 6 位', trigger: 'blur' }
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    {
      validator: (_r, v: string, cb: (e?: Error) => void) => {
        if (v !== pwdForm.new_password) return cb(new Error('两次输入的新密码不一致'))
        cb()
      },
      trigger: 'blur'
    }
  ]
}

async function onChangePassword() {
  const valid = await pwdFormRef.value?.validate().catch(() => false)
  if (!valid) return
  changingPwd.value = true
  try {
    await apiChangePassword(pwdForm.old_password, pwdForm.new_password)
    ElMessage.success('密码修改成功，请牢记新密码')
    pwdForm.old_password = ''
    pwdForm.new_password = ''
    pwdForm.confirm_password = ''
    pwdFormRef.value?.clearValidate()
  } catch {
    /* 拦截器已提示 */
  } finally {
    changingPwd.value = false
  }
}

onMounted(() => {
  loadSettings()
  // 首屏用本地已知的检查结果渲染，联网检查由按钮或后端周期任务触发
  update.load()
  if (!auth.version) auth.fetchVersion()
})

onUnmounted(() => {
  update.stopPolling()
})

// ---------- 备份与恢复 ----------

const exporting = ref(false)
const restoring = ref(false)
const fileInput = ref<HTMLInputElement>()

async function onExport() {
  exporting.value = true
  try {
    const data = await apiExportBackup()
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    const d = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    a.href = url
    a.download = `frpc-mesh-backup-${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}.json`
    a.click()
    URL.revokeObjectURL(url)
    ElMessage.success(`已导出 ${data.instances.length} 台服务器的配置`)
  } catch {
    /* 拦截器已提示 */
  } finally {
    exporting.value = false
  }
}

async function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  let data: BackupData
  try {
    data = JSON.parse(await file.text())
  } catch {
    ElMessage.error('文件不是有效的 JSON')
    return
  }
  if (!data || data.app !== 'frpc-mesh' || !Array.isArray(data.instances)) {
    ElMessage.error('不是 frpc-mesh 的备份文件')
    return
  }
  const proxyCount = data.instances.reduce((n, i) => n + (i.proxies?.length || 0), 0)
  const ok = await ElMessageBox.confirm(
    `导入将先停止所有运行中的连接，清空当前全部服务器与代理配置，再以备份中的 ${data.instances.length} 台服务器、${proxyCount} 条代理覆盖。此操作不可恢复，确定继续？`,
    '导入确认',
    {
      confirmButtonText: '覆盖导入',
      cancelButtonText: '取消',
      type: 'warning',
      confirmButtonClass: 'el-button--danger'
    }
  ).catch(() => false)
  if (!ok) return
  restoring.value = true
  try {
    const res = await apiRestoreBackup(data)
    ElMessage.success(`恢复完成：${res.instances} 台服务器、${res.proxies} 条代理`)
    await instStore.refreshSilently()
  } catch {
    /* 拦截器已提示 */
  } finally {
    restoring.value = false
  }
}
</script>

<style scoped>
.setting-card {
  margin-bottom: 16px;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.card-title {
  font-weight: 600;
  font-size: 15px;
}

.label-with-hint {
  display: inline-flex;
  align-items: center;
}

.hint-icon {
  font-size: 13px;
  margin-left: 4px;
  color: var(--el-text-color-secondary);
  cursor: help;
}

.update-cols {
  display: flex;
  gap: 40px;
  flex-wrap: wrap;
}

.update-item {
  min-width: 130px;
}

.update-label {
  font-size: 12px;
  color: #909399;
  margin-bottom: 4px;
}

.update-value {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 18px;
  font-weight: 700;
  color: #1f2433;
  line-height: 1.3;
}

.update-meta {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-wrap: wrap;
  font-size: 12px;
  color: #909399;
  margin-top: 10px;
}

.notes-link {
  font-size: 12px;
}

.update-env {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 12px;
  font-size: 13px;
}

.guide-toggle {
  margin-left: auto;
  font-size: 12px;
}

.env-reason {
  color: var(--el-text-color-secondary);
}

.update-alert {
  margin-top: 12px;
}

.upgrade-progress {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  font-size: 13px;
  color: var(--el-color-primary);
}

.update-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 16px;
}

.notes-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  font-size: 13px;
}

.notes-body {
  margin: 0;
  padding: 12px;
  background: var(--fm-page-bg);
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 260px;
  overflow-y: auto;
}

.assets {
  margin-top: 14px;
}

.assets-title {
  font-size: 13px;
  font-weight: 600;
  color: #1f2433;
  margin-bottom: 6px;
}

.asset-row {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  padding: 3px 0;
}

.asset-name {
  flex: 1;
  min-width: 0;
  word-break: break-all;
}

.backup-desc {
  font-size: 13px;
  line-height: 1.7;
  margin: 0 0 12px;
}

.backup-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.hidden-file {
  display: none;
}

.frpc-current {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}

.about-row {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 13px;
  padding: 6px 0;
}

.about-row .text-muted {
  width: 40px;
  flex-shrink: 0;
}

.about-block {
  align-items: flex-start;
}

.about-brand {
  font-weight: 700;
  font-size: 15px;
}

.about-links {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 12px;
}

.about-star {
  margin-top: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.about-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  line-height: 1.7;
  margin-top: 4px;
}
</style>
