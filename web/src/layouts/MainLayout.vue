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
          <el-tag
            v-if="update.hasUpdate"
            class="update-tag"
            size="small"
            type="warning"
            effect="plain"
            round
            title="前往设置页查看并升级"
            @click="router.push('/settings')"
          >
            发现新版本 v{{ update.info?.latest }}
          </el-tag>
          <el-tag v-if="auth.version" size="small" type="info" effect="plain" round>
            v{{ auth.version }}
          </el-tag>
          <el-tooltip content="开源仓库：看更新、提 Issue 反馈" placement="bottom">
            <a class="repo-link" :href="REPO_URL" target="_blank" rel="noopener noreferrer" aria-label="GitHub 仓库">
              <svg viewBox="0 0 16 16" width="18" height="18" fill="currentColor" aria-hidden="true">
                <path
                  d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.05-.13-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.13 1.99.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"
                />
              </svg>
            </a>
          </el-tooltip>
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
import { useUpdateStore } from '../stores/update'
import { REPO_URL } from '../api/repo'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const instancesStore = useInstancesStore()
const update = useUpdateStore()
const collapsed = ref(false)

const activeMenu = computed(() => (route.meta.activeMenu as string) || route.path)
const currentTitle = computed(() => (route.meta.title as string) || 'frpc-mesh')

onMounted(() => {
  auth.fetchMe()
  auth.fetchVersion()
  // 只读本地已知的检查结果，不主动联网
  update.load()
  instancesStore.startPolling()
})

onUnmounted(() => {
  instancesStore.stopPolling()
  update.stopPolling()
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

.repo-link {
  display: inline-flex;
  align-items: center;
  color: var(--el-text-color-secondary);
}

.repo-link:hover {
  color: var(--el-color-primary);
}

.update-tag {
  cursor: pointer;
  font-size: 12px;
}

.update-tag:hover {
  background: #fdf6ec;
  border-color: #f3d19e;
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
