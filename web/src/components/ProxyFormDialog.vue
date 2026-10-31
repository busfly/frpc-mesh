<template>
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    width="720px"
    destroy-on-close
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
    @open="onOpen"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" hide-required-asterisk>
      <!-- stcp / xtcp / sudp：选择提供方 / 访问方 -->
      <el-segmented
        v-if="isSecretType"
        v-model="role"
        :options="ROLE_OPTIONS"
        class="role-segmented"
      />

      <el-form-item :label="role === 'visitor' ? '访问器名称（不支持中文）' : '代理名称（不支持中文）'" prop="name">
        <el-input v-model="form.name" placeholder="例如: web-server" maxlength="60" />
      </el-form-item>
      <el-form-item label="启用（禁用后不写入配置，运行中的 frpc 自动重启生效）">
        <el-switch v-model="form.enabled" />
      </el-form-item>
      <el-form-item label="备注说明">
        <el-input
          v-model="form.remark"
          type="textarea"
          :rows="2"
          maxlength="200"
          show-word-limit
          placeholder="选填。记录用途、负责人、特殊说明等，仅保存不在列表显示"
        />
      </el-form-item>

      <!-- visitor：类型可选 -->
      <el-form-item v-if="role === 'visitor' && !isEdit" label="类型">
        <el-select v-model="form.type" class="full-width">
          <el-option v-for="t in VISITOR_TYPES" :key="t" :label="proxyTypeLabel(t)" :value="t" />
        </el-select>
      </el-form-item>

      <!-- proxy：本地服务 -->
      <template v-if="role === 'proxy'">
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="本地地址">
              <el-input
                v-model="form.local_ip"
                placeholder="如 127.0.0.1；容器部署穿透宿主机服务请用宿主机地址"
              />
              <div class="hint-chips">
                <el-tooltip
                  :disabled="!!hostHint"
                  content="未探测到宿主机地址（非容器环境？）"
                  placement="top"
                >
                  <span class="chip-wrap">
                    <el-tag
                      size="small"
                      effect="plain"
                      :type="hostHint ? 'primary' : 'info'"
                      class="chip"
                      :class="{ 'chip-disabled': !hostHint }"
                      @click="fillLocalIp(hostHint)"
                    >
                      宿主机{{ hostHint ? ` ${hostHint}` : '' }}
                    </el-tag>
                  </span>
                </el-tooltip>
                <el-tag
                  v-if="netHints?.host_gateway && netHints.host_docker_internal"
                  size="small"
                  effect="plain"
                  type="info"
                  class="chip"
                  @click="fillLocalIp(netHints.host_gateway)"
                >
                  网关 {{ netHints.host_gateway }}
                </el-tag>
                <el-tag size="small" type="primary" effect="plain" class="chip" @click="fillLocalIp('127.0.0.1')">
                  127.0.0.1
                </el-tag>
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="本地端口" prop="local_port">
              <el-input-number
                v-model="form.local_port"
                :min="1"
                :max="65535"
                :controls="false"
                class="full-width"
                placeholder="例如: 8080"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- tcp / udp：远程端口 -->
        <el-form-item v-if="form.type === 'tcp' || form.type === 'udp'" label="远程端口" prop="remote_port">
          <el-input-number
            v-model="form.remote_port"
            :min="0"
            :max="65535"
            :controls="false"
            class="full-width"
            placeholder="服务端暴露的端口"
          />
          <div class="field-tip">frps 侧对外暴露的端口，0 表示随机分配</div>
        </el-form-item>

        <!-- http / https：域名 / 子域名 / 路径 / Basic Auth -->
        <template v-if="form.type === 'http' || form.type === 'https'">
          <el-segmented
            v-model="domainMode"
            :options="DOMAIN_MODES"
            class="domain-segmented"
          />
          <el-form-item
            v-if="domainMode === '自定义域名'"
            label="自定义域名（支持多个，逗号分隔）"
            prop="custom_domains"
            class="segmented-field"
          >
            <el-input
              v-model="form.custom_domains"
              placeholder="例如: app.example.com, www.example.com"
            />
          </el-form-item>
          <el-form-item v-else label="子域名" prop="subdomain" class="segmented-field">
            <el-input v-model="form.subdomain" placeholder="例如: nas" />
          </el-form-item>
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="路径（可选，逗号分隔）">
                <el-input v-model="form.locations" placeholder="例如: /, /api" />
              </el-form-item>
            </el-col>
            <template v-if="form.type === 'http'">
              <el-col :span="6">
                <el-form-item label="用户名">
                  <el-input v-model="form.http_user" placeholder="HTTP Basic Auth" />
                </el-form-item>
              </el-col>
              <el-col :span="6">
                <el-form-item label="密码">
                  <el-input
                    v-model="form.http_password"
                    type="password"
                    show-password
                    placeholder="HTTP Basic Auth"
                  />
                </el-form-item>
              </el-col>
            </template>
          </el-row>
        </template>

        <!-- stcp / xtcp / sudp：密钥 -->
        <el-form-item v-if="isSecretType" label="密钥 SECRET KEY" prop="secret_key">
          <el-input
            v-model="form.secret_key"
            type="password"
            show-password
            placeholder="安全密钥，双方需要一致"
          />
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="8">
            <el-form-item class="check-item">
              <el-checkbox v-model="form.use_encryption">加密传输</el-checkbox>
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item class="check-item">
              <el-checkbox v-model="form.use_compression">压缩传输</el-checkbox>
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="带宽限制（单位必须大写）(如: 1MB, 500KB，留空不限制)" prop="bandwidth_limit">
          <el-input v-model="form.bandwidth_limit" placeholder="留空不限制" />
        </el-form-item>
      </template>

      <!-- visitor 字段 -->
      <template v-else>
        <el-form-item label="对端代理名 (server_name)" prop="server_name">
          <el-input v-model="form.server_name" placeholder="要访问的对端 proxy 名称" />
        </el-form-item>
        <el-form-item label="密钥 SECRET KEY" prop="secret_key">
          <el-input
            v-model="form.secret_key"
            type="password"
            show-password
            placeholder="安全密钥，双方需要一致"
          />
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="绑定地址">
              <el-input v-model="form.bind_addr" placeholder="127.0.0.1" />
              <div class="hint-chips">
                <el-tag size="small" type="primary" effect="plain" class="chip" @click="form.bind_addr = '0.0.0.0'">
                  所有网卡 0.0.0.0
                </el-tag>
                <el-tag size="small" type="primary" effect="plain" class="chip" @click="form.bind_addr = '127.0.0.1'">
                  本机 127.0.0.1
                </el-tag>
              </div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="绑定端口" prop="bind_port">
              <el-input-number
                v-model="form.bind_port"
                :min="1"
                :max="65535"
                :controls="false"
                class="full-width"
                placeholder="例如: 9000"
              />
            </el-form-item>
          </el-col>
        </el-row>
      </template>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:modelValue', false)">取消</el-button>
        <el-button type="primary" :loading="saving" @click="onSave">
          {{ isEdit ? '保存并应用' : '添加并应用' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { apiCreateProxy, apiUpdateProxy, getNetHints } from '../api'
import type { NetHints, Proxy } from '../api/types'
import { proxyTypeLabel } from '../utils/status'

const props = defineProps<{
  modelValue: boolean
  instanceId: number
  /** 新建时的代理类型（由类型卡片决定） */
  createType?: string
  /** 新建时的类别，默认 proxy；visitor 表示打开时直接进入访问方模式 */
  createKind?: 'proxy' | 'visitor'
  /** 编辑时传入已有记录 */
  proxy?: Proxy | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'saved'): void
}>()

const VISITOR_TYPES = ['stcp', 'xtcp', 'sudp']
const DOMAIN_MODES = ['自定义域名', '子域名']
const ROLE_OPTIONS = [
  { label: '提供方（分享服务）', value: 'proxy' },
  { label: '访问方（访问他人服务）', value: 'visitor' }
]

const formRef = ref<FormInstance>()
const saving = ref(false)
const domainMode = ref<'自定义域名' | '子域名'>('自定义域名')
/** 角色：提供方 proxy / 访问方 visitor（stcp/xtcp/sudp 时可在弹窗内切换） */
const role = ref<'proxy' | 'visitor'>('proxy')

const isEdit = computed(() => !!props.proxy?.id)
const isSecretType = computed(() => VISITOR_TYPES.includes(form.type))

/** 后端探测的本地地址快捷提示（宿主机网关等），仅拉取一次 */
const netHints = ref<NetHints | null>(null)

/** 宿主机首选地址：host.docker.internal 域名优先，其次网桥网关 */
const hostHint = computed(() => netHints.value?.host_docker_internal || netHints.value?.host_gateway || '')

async function loadNetHints() {
  if (netHints.value) return
  try {
    netHints.value = await getNetHints()
  } catch {
    /* 探测失败静默，芯片保持禁用 */
  }
}

function fillLocalIp(ip?: string) {
  if (ip) form.local_ip = ip
}

const dialogTitle = computed(() => {
  const label = proxyTypeLabel(form.type)
  if (role.value === 'visitor') return isEdit.value ? `编辑 ${label} 访问器` : `添加 ${label} 访问器`
  return isEdit.value ? `编辑 ${label} 代理` : `添加 ${label} 代理`
})

const form = reactive({
  name: '',
  type: 'tcp',
  enabled: true,
  local_ip: '127.0.0.1',
  local_port: undefined as number | undefined,
  remote_port: undefined as number | undefined,
  custom_domains: '',
  subdomain: '',
  locations: '',
  http_user: '',
  http_password: '',
  use_encryption: false,
  use_compression: false,
  bandwidth_limit: '',
  secret_key: '',
  server_name: '',
  bind_addr: '127.0.0.1',
  bind_port: undefined as number | undefined,
  remark: ''
})

function portValidator(allowZero: boolean) {
  return (_r: unknown, v: number, cb: (e?: Error) => void) => {
    if (v === undefined || v === null || Number.isNaN(v)) {
      return cb(new Error(allowZero ? '请输入端口（0 表示随机）' : '请输入端口'))
    }
    const min = allowZero ? 0 : 1
    if (!Number.isInteger(v) || v < min || v > 65535) {
      return cb(new Error(`端口范围 ${min}-65535`))
    }
    cb()
  }
}

const rules = computed<FormRules>(() => ({
  name: [
    { required: true, message: '请输入名称', trigger: 'blur' },
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (v && !/^[A-Za-z0-9_-]+$/.test(v.trim())) {
          return cb(new Error('仅支持字母、数字、下划线和连字符（不支持中文）'))
        }
        cb()
      },
      trigger: 'blur'
    }
  ],
  local_port: [{ validator: portValidator(false), trigger: 'blur' }],
  remote_port: [{ validator: portValidator(true), trigger: 'blur' }],
  bind_port: [{ validator: portValidator(false), trigger: 'blur' }],
  custom_domains: [
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (role.value === 'proxy' && (form.type === 'http' || form.type === 'https')) {
          if (domainMode.value === '自定义域名' && !v.trim() && !form.subdomain.trim()) {
            return cb(new Error('请填写自定义域名，或切换到子域名填写'))
          }
        }
        cb()
      },
      trigger: 'blur'
    }
  ],
  subdomain: [
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (role.value === 'proxy' && (form.type === 'http' || form.type === 'https')) {
          if (domainMode.value === '子域名' && !v.trim() && !form.custom_domains.trim()) {
            return cb(new Error('请填写子域名，或切换到自定义域名填写'))
          }
        }
        cb()
      },
      trigger: 'blur'
    }
  ],
  server_name: [
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (role.value === 'visitor' && !v.trim()) {
          return cb(new Error('访问器必须指定对端代理名'))
        }
        cb()
      },
      trigger: 'blur'
    }
  ],
  secret_key: [
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (isSecretType.value && !v) return cb(new Error('stcp/xtcp/sudp 必须设置密钥'))
        cb()
      },
      trigger: 'blur'
    }
  ],
  bandwidth_limit: [
    {
      validator: (_r: unknown, v: string, cb: (e?: Error) => void) => {
        if (v && v.trim() && !/^\d+(\.\d+)?(KB|MB|GB)$/.test(v.trim())) {
          return cb(new Error('格式如 1MB、500KB，单位必须大写'))
        }
        cb()
      },
      trigger: 'blur'
    }
  ]
}))

function onOpen() {
  loadNetHints()
  formRef.value?.clearValidate()
  // 编辑已有记录时按其 kind 回填为提供方/访问方模式
  role.value = props.proxy?.kind || props.createKind || 'proxy'
  const p = props.proxy
  if (p) {
    Object.assign(form, {
      name: p.name || '',
      type: p.type || 'tcp',
      enabled: p.enabled ?? true,
      local_ip: p.local_ip || '127.0.0.1',
      local_port: p.local_port || undefined,
      remote_port: p.remote_port ?? undefined,
      custom_domains: p.custom_domains || '',
      subdomain: p.subdomain || '',
      locations: p.locations || '',
      http_user: p.http_user || '',
      http_password: p.http_password || '',
      use_encryption: !!p.use_encryption,
      use_compression: !!p.use_compression,
      bandwidth_limit: p.bandwidth_limit || '',
      secret_key: p.secret_key || '',
      server_name: p.server_name || '',
      bind_addr: p.bind_addr || '127.0.0.1',
      bind_port: p.bind_port || undefined,
      remark: p.remark || ''
    })
    domainMode.value = p.subdomain && !p.custom_domains ? '子域名' : '自定义域名'
  } else {
    Object.assign(form, {
      name: '',
      type: props.createType || 'tcp',
      enabled: true,
      local_ip: '127.0.0.1',
      local_port: undefined,
      remote_port: undefined,
      custom_domains: '',
      subdomain: '',
      locations: '',
      http_user: '',
      http_password: '',
      use_encryption: false,
      use_compression: false,
      bandwidth_limit: '',
      secret_key: '',
      server_name: '',
      bind_addr: '127.0.0.1',
      bind_port: undefined,
      remark: ''
    })
    domainMode.value = '自定义域名'
  }
}

async function onSave() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload: Partial<Proxy> = {
      instance_id: props.instanceId,
      kind: role.value,
      name: form.name.trim(),
      type: form.type,
      enabled: form.enabled,
      remark: form.remark.trim(),
      use_encryption: form.use_encryption,
      use_compression: form.use_compression
    }
    if (role.value === 'visitor') {
      Object.assign(payload, {
        server_name: form.server_name.trim(),
        secret_key: form.secret_key,
        bind_addr: form.bind_addr || '127.0.0.1',
        bind_port: form.bind_port as number,
        local_ip: '',
        local_port: 0
      })
    } else {
      Object.assign(payload, {
        local_ip: form.local_ip || '127.0.0.1',
        local_port: form.local_port as number,
        bandwidth_limit: form.bandwidth_limit.trim()
      })
      if (form.type === 'tcp' || form.type === 'udp') {
        payload.remote_port = form.remote_port as number
      }
      if (form.type === 'http' || form.type === 'https') {
        Object.assign(payload, {
          custom_domains: form.custom_domains.trim(),
          subdomain: form.subdomain.trim(),
          locations: form.locations.trim(),
          http_user: form.http_user,
          http_password: form.http_password
        })
      }
      if (isSecretType.value) {
        payload.secret_key = form.secret_key
      }
    }
    if (isEdit.value && props.proxy) {
      await apiUpdateProxy(props.proxy.id, payload)
      ElMessage.success('已保存，运行中的 frpc 将自动重启生效')
    } else {
      await apiCreateProxy(props.instanceId, payload)
      ElMessage.success('添加成功，运行中的 frpc 将自动重启生效')
    }
    emit('update:modelValue', false)
    emit('saved')
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

.field-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.6;
  margin-top: 4px;
}

.hint-chips {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
}

.chip-wrap {
  display: inline-flex;
}

.chip {
  cursor: pointer;
  user-select: none;
}

.chip-disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.domain-segmented {
  width: 100%;
  margin-bottom: 18px;
}

.role-segmented {
  width: 100%;
  margin-bottom: 18px;
}

.segmented-field {
  margin-top: -4px;
}

.check-item {
  margin-bottom: 18px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
