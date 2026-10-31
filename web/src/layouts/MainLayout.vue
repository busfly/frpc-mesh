<template>
  <el-container class="main-layout">
    <el-aside :width="collapsed ? '64px' : '220px'" class="sidebar">
      <div class="logo" @click="router.push('/')">
        <div class="logo-icon">
          <img src="/logo.svg" alt="frpc-mesh" class="logo-img" />
        </div>
        <transition name="fade">
          <div v-show="!collapsed" class="logo-text">
            <div class="logo-name">frpc-mesh</div>
            <div class="logo-slogan">多服务器 frp 管理</div>
          </div>
        </transition>
      </div>
      <el-menu
        :default-active="activeMenu"
        :collapse="collapsed"
        :collapse-transition="false"
        background-color="transparent"
        text-color="#a3adc7"
        active-text-color="#ffffff"
        router
        class="side-menu"
      >
        <el-menu-item index="/">
          <el-icon><Odometer /></el-icon>
          <template #title>仪表盘</template>
        </el-menu-item>
        <el-menu-item index="/instances">
          <el-icon><Menu /></el-icon>
          <template #title>FRPS 服务器</template>
        </el-menu-item>
        <el-menu-item index="/proxies">
          <el-icon><Connection /></el-icon>
          <template #title>穿透代理</template>
        </el-menu-item>
        <el-menu-item index="/versions">
          <el-icon><Download /></el-icon>
          <template #title>frpc 版本管理</template>
        </el-menu-item>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <template #title>设置</template>
        </el-menu-item>
      </el-menu>
      <div class="sidebar-footer">
        <el-tooltip :content="collapsed ? '展开侧边栏' : '折叠侧边栏'" placement="top">
          <div class="collapse-btn" @click="collapsed = !collapsed">
            <el-icon :size="16">
              <Expand v-if="collapsed" />
              <Fold v-else />
            </el-icon>
          </div>
        </el-tooltip>
      </div>
    </el-aside>

    <el-container class="right-container">
      <el-header class="topbar" height="56px">
        <div class="topbar-left">
          <span class="topbar-title">{{ currentTitle }}</span>
        </div>
        <div class="topbar-right">
          <el-tag v-if="auth.version" size="small" type="info" effect="plain" round>
            v{{ auth.version }}
          </el-tag>
          <el-dropdown trigger="click" @command="onCommand">
            <span class="user-chip">
              <el-avatar :size="26" class="user-avatar">
                {{ (auth.username || 'A').charAt(0).toUpperCase() }}
              </el-avatar>
              <span class="user-name">{{ auth.username || '管理员' }}</span>
              <el-icon :size="12"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="settings">
                  <el-icon><Setting /></el-icon> 设置
                </el-dropdown-item>
                <el-dropdown-item command="logout" divided>
                  <el-icon><SwitchButton /></el-icon> 退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main-content">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { useInstancesStore } from '../stores/instances'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const instancesStore = useInstancesStore()
const collapsed = ref(false)

const activeMenu = computed(() => (route.meta.activeMenu as string) || route.path)
const currentTitle = computed(() => (route.meta.title as string) || 'frpc-mesh')

onMounted(() => {
  auth.fetchMe()
  auth.fetchVersion()
  instancesStore.startPolling()
})

onUnmounted(() => {
  instancesStore.stopPolling()
})

function onCommand(cmd: string) {
  if (cmd === 'settings') {
    router.push('/settings')
  } else if (cmd === 'logout') {
    ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '退出',
      cancelButtonText: '取消',
      type: 'warning'
    })
      .then(() => {
        instancesStore.stopPolling()
        auth.logout()
      })
      .catch(() => undefined)
  }
}
</script>

<style scoped>
.main-layout {
  height: 100vh;
}

.sidebar {
  background: linear-gradient(180deg, #171c2e 0%, #1a2038 100%);
  display: flex;
  flex-direction: column;
  transition: width 0.25s ease;
  overflow: hidden;
}

.logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 14px;
  cursor: pointer;
  height: 56px;
  flex-shrink: 0;
}

.logo-icon {
  width: 34px;
  height: 34px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.logo-img {
  width: 26px;
  height: 26px;
  display: block;
}

.logo-text {
  white-space: nowrap;
}

.logo-name {
  color: #fff;
  font-weight: 700;
  font-size: 16px;
  line-height: 1.2;
}

.logo-slogan {
  color: #7c86a3;
  font-size: 11px;
  line-height: 1.4;
}

.side-menu {
  border-right: none;
  flex: 1;
  padding: 6px 8px;
}

.side-menu :deep(.el-menu-item) {
  border-radius: 8px;
  margin-bottom: 4px;
  height: 44px;
}

.side-menu :deep(.el-menu-item:hover) {
  background-color: rgba(255, 255, 255, 0.06);
}

.side-menu :deep(.el-menu-item.is-active) {
  background: var(--fm-primary-grad);
  color: #fff;
}

.sidebar-footer {
  padding: 10px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  flex-shrink: 0;
}

.collapse-btn {
  color: #a3adc7;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 34px;
  border-radius: 8px;
  cursor: pointer;
}

.collapse-btn:hover {
  background: rgba(255, 255, 255, 0.08);
  color: #fff;
}

.right-container {
  min-width: 0;
}

.topbar {
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #e8ebf3;
  padding: 0 20px;
}

.topbar-title {
  font-size: 15px;
  font-weight: 600;
  color: #1f2433;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: 14px;
}

.user-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  color: #4b5563;
  outline: none;
}

.user-avatar {
  background: var(--fm-primary-grad);
  color: #fff;
  font-weight: 600;
}

.user-name {
  font-size: 13px;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main-content {
  background: var(--fm-page-bg);
  padding: 0;
  overflow-y: auto;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
