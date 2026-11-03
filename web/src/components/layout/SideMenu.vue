<template>
  <el-menu
    :default-active="activeMenu"
    :collapse="collapsed"
    :collapse-transition="false"
    background-color="transparent"
    text-color="#a3adc7"
    active-text-color="#ffffff"
    router
    class="side-menu"
    @select="emit('navigate')"
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
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

defineProps<{ collapsed?: boolean }>()
const emit = defineEmits<{ navigate: [] }>()

const route = useRoute()
const activeMenu = computed(() => (route.meta.activeMenu as string) || route.path)
</script>

<style scoped>
.side-menu {
  border-right: none;
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
</style>
