<template>
  <el-tag v-if="!proxy.enabled" type="warning" effect="plain" size="small" round>已禁用</el-tag>
  <el-tag v-else-if="online" type="success" effect="dark" size="small" round>在线</el-tag>
  <el-tag v-else type="info" effect="plain" size="small" round>离线</el-tag>
</template>

<script setup lang="ts">
import type { Proxy } from '../../api/types'

defineProps<{ proxy: Proxy; online: boolean }>()
</script>
