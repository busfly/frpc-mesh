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
              <el-button type="primary" :loading="savingSettings" @click="onSaveSettings">
                保存设置
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
        <el-card class="card setting-card" shadow="never">
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
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Download, Upload } from '@element-plus/icons-vue'
import {
  apiChangePassword,
  apiExportBackup,
  apiGetSettings,
  apiRestoreBackup,
  apiSaveSettings,
  getVersionsLocal
} from '../api'
import type { BackupData } from '../api/types'
import { useAuthStore } from '../stores/auth'
import { useInstancesStore } from '../stores/instances'

const auth = useAuthStore()
const router = useRouter()
const instStore = useInstancesStore()

const loadingSettings = ref(false)
const savingSettings = ref(false)
const frpcCurrent = ref('')

const settingsForm = reactive({
  auto_start: true
})

function goVersions() {
  router.push('/versions')
}

async function loadSettings() {
  loadingSettings.value = true
  try {
    const s = await apiGetSettings()
    settingsForm.auto_start = !!s.auto_start
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
    await apiSaveSettings({ auto_start: settingsForm.auto_start })
    ElMessage.success('设置已保存')
  } catch {
    /* 拦截器已提示 */
  } finally {
    savingSettings.value = false
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
  if (!auth.version) auth.fetchVersion()
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

.card-title {
  font-weight: 600;
  font-size: 15px;
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

.about-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  line-height: 1.7;
  margin-top: 4px;
}
</style>
