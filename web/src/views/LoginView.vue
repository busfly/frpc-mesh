<template>
  <div class="login-page">
    <div class="login-bg-decoration deco-1"></div>
    <div class="login-bg-decoration deco-2"></div>
    <el-card class="login-card" shadow="always">
      <div class="login-header">
        <div class="login-logo">
          <img src="/favicon.svg" alt="frpc-mesh" class="login-logo-img" />
        </div>
        <h1 class="login-title brand-text">frpc-mesh</h1>
        <p class="login-slogan">多服务器 frp 内网穿透管理</p>
      </div>
      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        size="large"
        @keyup.enter="onSubmit"
      >
        <el-form-item prop="username">
          <el-input
            v-model="form.username"
            placeholder="用户名"
            :prefix-icon="User"
            autocomplete="username"
          />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            :prefix-icon="Lock"
            show-password
            autocomplete="current-password"
          />
        </el-form-item>
        <el-form-item>
          <el-button
            type="primary"
            class="login-btn"
            :loading="loading"
            @click="onSubmit"
          >
            登 录
          </el-button>
        </el-form-item>
      </el-form>
      <div class="login-footer">部署于 NAS · 默认账号 admin，首次登录后请及时修改密码</div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { Lock, User } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({
  username: '',
  password: ''
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

async function onSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    await auth.login(form.username, form.password)
    ElMessage.success('登录成功')
    const redirect = (route.query.redirect as string) || '/'
    router.replace(redirect)
  } catch (e: any) {
    if (e?.response?.status === 401) {
      ElMessage.error('用户名或密码错误')
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1a2038 0%, #2b3563 55%, #4f46e5 130%);
  position: relative;
  overflow: hidden;
}

.login-bg-decoration {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.5;
  pointer-events: none;
}

.deco-1 {
  width: 420px;
  height: 420px;
  background: #6d5df6;
  top: -120px;
  left: -100px;
}

.deco-2 {
  width: 360px;
  height: 360px;
  background: #3b82f6;
  bottom: -100px;
  right: -80px;
}

.login-card {
  width: 400px;
  max-width: calc(100vw - 40px);
  border-radius: 14px;
  border: none;
  z-index: 1;
}

.login-card :deep(.el-card__body) {
  padding: 36px 36px 24px;
}

.login-header {
  text-align: center;
  margin-bottom: 26px;
}

.login-logo {
  width: 58px;
  height: 58px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 14px;
  box-shadow: 0 6px 18px rgba(99, 102, 241, 0.4);
}

.login-logo-img {
  width: 58px;
  height: 58px;
  display: block;
  border-radius: 16px;
}

.login-title {
  font-size: 26px;
  margin: 0 0 6px;
  font-weight: 800;
  letter-spacing: 0.5px;
}

.login-slogan {
  margin: 0;
  color: #909399;
  font-size: 13px;
}

.login-btn {
  width: 100%;
  background: var(--fm-primary-grad);
  border: none;
  letter-spacing: 6px;
  font-weight: 600;
}

.login-footer {
  text-align: center;
  font-size: 12px;
  color: #b6bac2;
  margin-top: 6px;
}
</style>
