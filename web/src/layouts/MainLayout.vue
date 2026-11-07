<template>
  <el-container class="main-layout">
    <el-aside v-if="!isMobile" :width="collapsed ? '64px' : '220px'" class="sidebar">
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
      <SideMenu :collapsed="collapsed" class="menu-fill" />
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
          <button
            v-if="isMobile"
            class="hamburger"
            aria-label="打开菜单"
            :aria-expanded="navOpen"
            aria-controls="fm-nav-drawer"
            @click="navOpen = true"
          >
            <span class="bars"></span>
          </button>
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
          <!-- 窄屏顶栏只留「发现新版本」标签与头像下拉，版本号与仓库入口收进下拉菜单 -->
          <template v-if="!isMobile">
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
          </template>
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
                <el-dropdown-item v-if="isMobile && auth.version" command="version" disabled>
                  <el-icon><InfoFilled /></el-icon> 当前版本 v{{ auth.version }}
                </el-dropdown-item>
                <el-dropdown-item v-if="isMobile" command="repo">
                  <el-icon><Link /></el-icon> 开源仓库
                </el-dropdown-item>
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

    <!-- 移动形态导航：左侧覆盖抽屉，菜单项与侧栏共用同一份 SideMenu -->
    <el-drawer
      id="fm-nav-drawer"
      v-model="navOpen"
      direction="ltr"
      size="260px"
      :with-header="false"
      class="fm-nav-drawer"
    >
      <div class="drawer-logo">
        <img src="/logo.svg" alt="frpc-mesh" class="logo-img" />
        <div>
          <div class="logo-name">frpc-mesh</div>
          <div class="logo-slogan">多服务器 frp 管理</div>
        </div>
      </div>
      <SideMenu @navigate="navOpen = false" />
    </el-drawer>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import SideMenu from '../components/layout/SideMenu.vue'
import { useAuthStore } from '../stores/auth'
import { useInstancesStore } from '../stores/instances'
import { useUpdateStore } from '../stores/update'
import { useIsMobile, useIsNarrowDesktop } from '../composables/useMediaQuery'
import { REPO_URL } from '../api/repo'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const instancesStore = useInstancesStore()
const update = useUpdateStore()

const isMobile = useIsMobile()
/** 768–1023px 仍是桌面形态（表格照旧），只是侧栏默认折叠 */
const collapsed = ref(useIsNarrowDesktop().value)
const navOpen = ref(false)

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
  } else if (cmd === 'repo') {
    window.open(REPO_URL, '_blank', 'noopener,noreferrer')
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
  /* iOS Safari 的 100vh 含被地址栏遮住的部分，底部会点不到 */
  height: 100dvh;
}

.sidebar {
  background: linear-gradient(180deg, #171c2e 0%, #1a2038 100%);
  display: flex;
  flex-direction: column;
  transition: width 0.25s ease;
  overflow: hidden;
}

.sidebar .menu-fill {
  flex: 1;
  overflow-y: auto;
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
  /* viewport-fit=cover 之后，刘海与状态栏要靠自己让位 */
  padding-top: env(safe-area-inset-top);
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.hamburger {
  width: 44px;
  height: 44px;
  border: none;
  border-radius: 10px;
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  touch-action: manipulation;
}

.hamburger .bars {
  position: relative;
  display: block;
  width: 18px;
  height: 2px;
  background: #1f2433;
}

.hamburger .bars::before,
.hamburger .bars::after {
  content: '';
  position: absolute;
  left: 0;
  width: 18px;
  height: 2px;
  background: #1f2433;
}

.hamburger .bars::before {
  top: -6px;
}

.hamburger .bars::after {
  top: 6px;
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
  -webkit-overflow-scrolling: touch;
}

.fm-nav-drawer :deep(.el-drawer__body) {
  background: linear-gradient(180deg, #171c2e 0%, #1a2038 100%);
  padding: 0;
  display: flex;
  flex-direction: column;
}

.drawer-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

@media (max-width: 767px) {
  .topbar {
    /* 简写会顶掉上面的 env()，这里显式把顶部安全区带回来 */
    padding: env(safe-area-inset-top) 10px 0;
  }
  .topbar-right {
    gap: 6px;
  }
  .user-name {
    display: none;
  }
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
