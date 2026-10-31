import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { TOKEN_KEY } from '../api/http'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/LoginView.vue'),
    meta: { public: true, title: '登录' }
  },
  {
    path: '/',
    component: () => import('../layouts/MainLayout.vue'),
    children: [
      {
        path: '',
        name: 'dashboard',
        component: () => import('../views/DashboardView.vue'),
        meta: { title: '仪表盘', icon: 'Odometer' }
      },
      {
        path: 'instances',
        name: 'instances',
        component: () => import('../views/InstancesView.vue'),
        meta: { title: 'FRPS 服务器', icon: 'Menu' }
      },
      {
        path: 'proxies',
        name: 'proxies',
        component: () => import('../views/ProxiesView.vue'),
        meta: { title: '穿透代理', icon: 'Connection' }
      },
      {
        path: 'versions',
        name: 'versions',
        component: () => import('../views/VersionsView.vue'),
        meta: { title: 'frpc 版本管理', icon: 'Download' }
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('../views/SettingsView.vue'),
        meta: { title: '设置', icon: 'Setting' }
      }
    ]
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (!to.meta.public && !token) {
    return { name: 'login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
  }
  if (to.name === 'login' && token) {
    return { name: 'dashboard' }
  }
  document.title = to.meta.title ? `${to.meta.title} - frpc-mesh` : 'frpc-mesh'
  return true
})

export default router
