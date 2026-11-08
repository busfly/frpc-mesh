<template>
  <el-drawer
    :model-value="modelValue"
    :title="isEdit ? '编辑 FRPS 服务器' : '新建 FRPS 服务器'"
    :direction="isMobile ? 'btt' : 'rtl'"
    :size="isMobile ? '92%' : '560px'"
    :class="isMobile ? 'fm-sheet fm-form' : ''"
    destroy-on-close
    @update:model-value="emit('update:modelValue', $event)"
    @open="onOpen"
  >
    <template #header="{ titleId, titleClass }">
      <!-- 把手放标题上方，和代理表单弹层一致（iOS 底部弹层的惯例） -->
      <div v-if="isMobile" ref="handleEl" class="fm-handle"></div>
      <span :id="titleId" :class="isMobile ? 'fm-sheet-title' : titleClass || 'el-drawer__title'">{{
        isEdit ? '编辑 FRPS 服务器' : '新建 FRPS 服务器'
      }}</span>
    </template>
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="110px"
      label-position="top"
      :scroll-to-error="isMobile"
    >
      <el-divider content-position="left">基本信息</el-divider>
      <el-form-item label="服务器名称（别名）" prop="name">
        <el-input v-model="form.name" placeholder="例如：阿里云 frps" maxlength="50" />
      </el-form-item>
      <el-row :gutter="12">
        <el-col :xs="24" :span="14">
          <el-form-item label="服务器地址 (server_addr)" prop="server_addr">
            <el-input v-model="form.server_addr" placeholder="IP 或域名，例如 1.2.3.4" />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :span="10">
          <el-form-item label="服务器端口 (server_port)" prop="server_port">
            <el-input-number inputmode="numeric"
              v-model="form.server_port"
              :min="1"
              :max="65535"
              :controls="false"
              class="full-width"
              placeholder="7000"
            />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :xs="24" :span="12">
          <el-form-item label="认证方式 (auth_method)">
            <el-select v-model="form.auth_method" class="full-width">
              <el-option label="token" value="token" />
              <el-option label="none（无认证）" value="none" />
            </el-select>
          </el-form-item>
        </el-col>
        <el-col :xs="24" :span="12">
          <el-form-item v-if="form.auth_method === 'token'" label="认证 Token" prop="auth_token">
            <el-input
              v-model="form.auth_token"
              type="password"
              show-password
              placeholder="与 frps 一致的 token"
            />
          </el-form-item>
        </el-col>
      </el-row>
      <el-row :gutter="12">
        <el-col :xs="24" :span="12">
          <el-form-item label="FRPS 管理端口 (dashboard_port)" prop="manage_port">
            <el-input-number inputmode="numeric"
              v-model="form.manage_port"
              :min="0"
              :max="65535"
              :controls="false"
              class="full-width"
              placeholder="7500，选填"
            />
          </el-form-item>
        </el-col>
        <el-col :xs="24" :span="12">
          <div class="field-hint">
            frps 管理面板端口，填写后列表「管理」按钮可直达
            <span class="mono">http://服务器地址:管理端口</span>
            ；再填下面的凭据，代理表单的远程端口「检测」就能拿 frps 的权威端口表判定占用
          </div>
        </el-col>
      </el-row>
      <el-collapse v-if="(form.manage_port || 0) > 0" class="dash-collapse">
        <el-collapse-item name="dash">
          <template #title>
            <span class="label-with-hint">
              远端 frps 管理面板凭据（端口检测用）
              <FieldHint
                content="对应 frps.toml 里的 webServer.user / webServer.password（旧版 ini 是 dashboard_user / dashboard_pwd），frps 的管理接口只认这一种用户名密码认证；frps 那边没设用户名密码时这里留空即可。配好后，代理表单点「检测」会直接问这台 frps 已登记了哪些远程端口，能报出真实占用者（包括别的客户端建的隧道）；不配则只比对本机其它代理，远端一律「无法确定」。检测结果只作提醒，任何情况都不拦你保存。"
              />
            </span>
          </template>
          <el-row :gutter="12">
            <el-col :xs="24" :span="12">
              <el-form-item label="用户名">
                <el-input v-model="dash.user" placeholder="webServer.user" autocomplete="off" />
              </el-form-item>
            </el-col>
            <el-col :xs="24" :span="12">
              <el-form-item label="密码">
                <el-input
                  v-model="dash.password"
                  type="password"
                  show-password
                  placeholder="webServer.password"
                  autocomplete="new-password"
                />
              </el-form-item>
            </el-col>
          </el-row>
        </el-collapse-item>
      </el-collapse>
      <el-form-item label="服务器域名 (domain)">
        <el-input
          v-model="form.domain"
          placeholder="选填，例如 frps.example.com；配置后代理列表的远程链接优先用域名打开"
        />
      </el-form-item>
      <el-form-item>
        <template #label>
          <span class="label-with-hint">
            frpc 版本
<FieldHint
              content="老服务端配新客户端（或反之）可能连不上，可只为这台服务器固定一个版本。不指定即跟随「frpc 版本管理」里的默认版本；指定的版本若还没下载，会先用默认版本启动，后台补下载完成后自动重启切过去。"
            />
          </span>
        </template>
        <el-select v-model="form.frpc_version" class="full-width">
          <el-option :value="''" :label="followLabel">
            <span>{{ followLabel }}</span>
          </el-option>
          <el-option v-for="v in versionOptions" :key="v" :value="v">
            <span class="mono">v{{ v }}</span>
            <span v-if="missingPin(v)" class="opt-note">未下载</span>
            <span v-else-if="v === activeVersion" class="opt-note">默认</span>
          </el-option>
        </el-select>
      </el-form-item>
      <el-form-item label="备注说明">
        <el-input
          v-model="form.remark"
          type="textarea"
          :rows="2"
          maxlength="200"
          show-word-limit
          placeholder="选填。记录服务器用途、到期时间、特殊说明等，方便日后查看"
        />
      </el-form-item>
      <el-form-item label="启用（保存后立即启动并随服务自动拉起）">
        <el-switch v-model="form.enabled" />
      </el-form-item>

      <el-collapse v-model="activeCollapse" class="form-collapse">
        <el-collapse-item name="transport">
          <template #title>
            <span class="collapse-title"><el-icon><Van /></el-icon> 传输 transport</span>
          </template>
          <el-form-item label="传输协议 (protocol)">
            <el-select v-model="transport.protocol" class="full-width">
              <el-option v-for="p in protocols" :key="p" :label="p" :value="p" />
            </el-select>
          </el-form-item>
          <el-form-item label="TCP 多路复用 (tcp_mux)">
            <el-switch v-model="transport.tcp_mux" />
          </el-form-item>
          <el-row :gutter="12">
            <el-col :xs="24" :span="12">
              <el-form-item label="连接池大小 (pool_count)">
                <el-input-number inputmode="numeric"
                  v-model="transport.pool_count"
                  :min="0"
                  :max="128"
                  class="full-width"
                />
              </el-form-item>
            </el-col>
            <el-col :xs="24" :span="12">
              <el-form-item label="心跳间隔秒 (heartbeat)">
                <el-input-number inputmode="numeric"
                  v-model="transport.heartbeat"
                  :min="-1"
                  :max="3600"
                  class="full-width"
                />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12">
            <el-col :xs="24" :span="12">
              <el-form-item label="拨号超时秒 (dial_timeout)">
                <el-input-number inputmode="numeric"
                  v-model="transport.dial_timeout"
                  :min="-1"
                  :max="3600"
                  class="full-width"
                />
              </el-form-item>
            </el-col>
            <el-col :xs="24" :span="12">
              <el-form-item label="TLS (tls_enable)">
                <el-switch v-model="transport.tls_enable" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12">
            <el-col :xs="24" :span="12">
              <el-form-item label="心跳超时秒 (heartbeat_timeout)">
                <el-input-number inputmode="numeric"
                  v-model="transport.heartbeat_timeout"
                  :min="-1"
                  :max="3600"
                  class="full-width"
                />
              </el-form-item>
            </el-col>
            <el-col :xs="24" :span="12">
              <el-form-item label="多路复用保活秒 (tcp_mux_keepalive)">
                <el-input-number inputmode="numeric"
                  v-model="transport.tcp_mux_keepalive"
                  :min="-1"
                  :max="3600"
                  class="full-width"
                />
              </el-form-item>
            </el-col>
          </el-row>
        </el-collapse-item>

        <el-collapse-item name="admin">
          <template #title>
            <span class="collapse-title"><el-icon><Monitor /></el-icon> frpc 本机管理接口（Admin API）</span>
          </template>
          <el-alert
            type="info"
            :closable="false"
            show-icon
            title="frpc 在本机开的管理端口，供本工具读隧道状态，需在本机唯一。远端 frps 面板的用户名密码不在这里填，见上面「FRPS 管理端口」下的凭据区"
            class="admin-tip"
          />
          <el-form-item label="启用本机管理接口">
            <el-switch v-model="admin.enabled" />
          </el-form-item>
          <template v-if="admin.enabled">
            <el-row :gutter="12">
              <el-col :xs="24" :span="12">
                <el-form-item label="本机端口 (port)" prop="admin_port">
                  <el-input-number inputmode="numeric"
                    v-model="admin.port"
                    :min="1"
                    :max="65535"
                    :controls="false"
                    class="full-width"
                  />
                </el-form-item>
              </el-col>
              <el-col :xs="24" :span="12">
                <el-form-item label="用户名 (user)">
                  <el-input v-model="admin.user" placeholder="admin" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="密码 (password)">
              <el-input v-model="admin.password" type="password" show-password />
            </el-form-item>
          </template>
        </el-collapse-item>
      </el-collapse>
    </el-form>

    <template #footer>
      <div class="drawer-footer">
        <el-button @click="emit('update:modelValue', false)">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">
          {{ isEdit ? '保存修改' : '创建服务器' }}
        </el-button>
      </div>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useIsMobile } from '../composables/useMediaQuery'
import { useSwipeToClose } from '../composables/useSwipeToClose'
import FieldHint from './mobile/FieldHint.vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { apiCreateInstance, apiUpdateInstance } from '../api'
import type { Instance } from '../api/types'
import { useInstancesStore } from '../stores/instances'

const props = defineProps<{
  modelValue: boolean
  instance?: Instance | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const store = useInstancesStore()
const formRef = ref<FormInstance>()
const saving = ref(false)

const isMobile = useIsMobile()
const handleEl = ref<HTMLElement | null>(null)
useSwipeToClose(handleEl, () => emit('update:modelValue', false))
const activeCollapse = ref<string[]>([])
const protocols = ['tcp', 'kcp', 'quic', 'wss', 'websocket']

const isEdit = computed(() => !!props.instance?.id)

const form = reactive({
  name: '',
  server_addr: '',
  server_port: 7000 as number | undefined,
  auth_method: 'token' as 'token' | 'none',
  auth_token: '',
  manage_port: undefined as number | undefined,
  remark: '',
  domain: '',
  frpc_version: '',
  enabled: true
})

// 版本选择器只列本地已下载版本；编辑对象指定的版本若已被删，仍列出并标注
const versionOptions = computed(() => {
  const list = [...store.frpcVersions.downloaded]
  const pin = form.frpc_version
  if (pin && !list.includes(pin)) list.unshift(pin)
  return list
})

const activeVersion = computed(() => store.frpcVersions.active)

const followLabel = computed(() =>
  activeVersion.value ? `跟随默认（当前 v${activeVersion.value}）` : '跟随默认'
)

function missingPin(v: string) {
  return v === form.frpc_version && !store.frpcVersions.downloaded.includes(v)
}

const transport = reactive({
  protocol: 'tcp',
  tcp_mux: true,
  pool_count: 1,
  heartbeat: 30,
  heartbeat_timeout: 90,
  tcp_mux_keepalive: 30,
  dial_timeout: 10,
  tls_enable: true
})

const admin = reactive({
  enabled: false,
  port: 17400,
  user: 'admin',
  password: ''
})

// frps 管理接口凭据：端口用上面的 manage_port，这里只存认证信息
const dash = reactive({ user: '', password: '' })

const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: '请输入服务器名称', trigger: 'blur' }],
  server_addr: [{ required: true, message: '请输入服务器地址', trigger: 'blur' }],
  server_port: [
    { required: true, message: '请输入服务器端口', trigger: 'blur' },
    {
      validator: (_r: unknown, v: number, cb: (e?: Error) => void) => {
        if (v === undefined || v === null || Number.isNaN(v)) return cb(new Error('请输入端口'))
        if (!Number.isInteger(v) || v < 1 || v > 65535) return cb(new Error('端口范围 1-65535'))
        cb()
      },
      trigger: 'blur'
    }
  ],
  auth_token: [
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (form.auth_method === 'token' && !v) return cb(new Error('认证方式为 token 时必填'))
        cb()
      },
      trigger: 'blur'
    }
  ],
  manage_port: [
    {
      validator: (_r: unknown, v: number, cb: (e?: Error) => void) => {
        if (v === undefined || v === null || v === 0) return cb()
        if (!Number.isInteger(v) || v < 1 || v > 65535) return cb(new Error('端口范围 1-65535'))
        cb()
      },
      trigger: 'blur'
    }
  ],
  admin_port: [
    {
      // 值在独立的 admin reactive 里，form 上没有 admin_port 这个路径，
      // Element Plus 传进来的 v 永远是 undefined，必须直接读 admin.port
      validator: (_r: unknown, _v: unknown, cb: (e?: Error) => void) => {
        if (!admin.enabled) return cb()
        const p = admin.port
        if (p === undefined || p === null || Number.isNaN(p)) return cb(new Error('请输入端口'))
        if (!Number.isInteger(p) || p < 1 || p > 65535) return cb(new Error('端口范围 1-65535'))
        cb()
      },
      trigger: 'blur'
    }
  ]
}))

function onOpen() {
  formRef.value?.clearValidate()
  const inst = props.instance
  if (inst) {
    form.name = inst.name || ''
    form.server_addr = inst.server_addr || ''
    form.server_port = inst.server_port || 7000
    form.auth_method = inst.auth_method || 'token'
    form.auth_token = inst.auth_token || ''
    form.manage_port = inst.manage_port || undefined
    form.remark = inst.remark || ''
    form.domain = inst.domain || ''
    form.frpc_version = inst.frpc_version || ''
    form.enabled = !!inst.enabled
    const t = inst.transport
    transport.protocol = t?.protocol || 'tcp'
    transport.tcp_mux = t?.tcp_mux ?? true
    transport.pool_count = t?.pool_count ?? 1
    transport.heartbeat = t?.heartbeat ?? 30
    transport.heartbeat_timeout = t?.heartbeat_timeout ?? 90
    transport.tcp_mux_keepalive = t?.tcp_mux_keepalive ?? 30
    transport.dial_timeout = t?.dial_timeout ?? 10
    transport.tls_enable = t?.tls_enable ?? true
    const a = inst.admin
    admin.enabled = a?.enabled ?? false
    admin.port = a?.port ?? 17400
    admin.user = a?.user ?? 'admin'
    admin.password = a?.password ?? ''
    const d = inst.dashboard
    dash.user = d?.user ?? ''
    dash.password = d?.password ?? ''
  } else {
    form.name = ''
    form.server_addr = ''
    form.server_port = 7000
    form.auth_method = 'token'
    form.auth_token = ''
    form.manage_port = undefined
    form.remark = ''
    form.domain = ''
    form.frpc_version = ''
    form.enabled = true
    transport.protocol = 'tcp'
    transport.tcp_mux = true
    transport.pool_count = 1
    transport.heartbeat = 30
    transport.heartbeat_timeout = 90
    transport.tcp_mux_keepalive = 30
    transport.dial_timeout = 10
    transport.tls_enable = true
    admin.enabled = false
    admin.port = 17400
    admin.user = 'admin'
    admin.password = ''
    dash.user = ''
    dash.password = ''
  }
  activeCollapse.value = []
  store.loadFrpcVersions()
}

async function onSave() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload: Partial<Instance> = {
      name: form.name.trim(),
      server_addr: form.server_addr.trim(),
      server_port: form.server_port as number,
      auth_method: form.auth_method,
      auth_token: form.auth_method === 'token' ? form.auth_token : '',
      manage_port: form.manage_port || 0,
      remark: form.remark.trim(),
      domain: form.domain.trim(),
      frpc_version: form.frpc_version,
      enabled: form.enabled,
      transport: { ...transport },
      admin: { ...admin },
      // 管理端口清掉后凭据区是隐藏的，别把看不见的旧密码继续留在库里
      dashboard: (form.manage_port || 0) > 0 ? { ...dash } : { user: '', password: '' }
    }
    if (isEdit.value && props.instance) {
      await apiUpdateInstance(props.instance.id, payload)
      ElMessage.success('服务器已更新，运行中的进程将自动重启生效')
    } else {
      await apiCreateInstance(payload)
      ElMessage.success('服务器创建成功')
    }
    emit('update:modelValue', false)
    await store.refreshSilently()
  } catch {
    /* 拦截器已提示 */
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.full-width {
  width: 100%;
}

.dash-collapse {
  margin: 0 0 12px;
  border-top: none;
}

.label-with-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}


.opt-note {
  margin-left: 8px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.field-hint {
  font-size: 12px;
  color: #909399;
  line-height: 1.7;
  padding-top: 4px;
}

.form-collapse {
  border-top: none;
  margin-top: 4px;
}

.collapse-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  font-size: 14px;
  color: #1f2433;
}

.admin-tip {
  margin-bottom: 14px;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
