<template>
  <el-link
    v-if="linkable && href"
    type="primary"
    :underline="false"
    class="mono"
    :href="href"
    target="_blank"
  >
    {{ text }}
  </el-link>
  <span v-else class="mono">{{ text }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Proxy } from '../../api/types'

// linkable=false 给卡片用：手机上的 127.0.0.1 是手机自己，点了必然打不开
const props = withDefaults(defineProps<{ proxy: Proxy; linkable?: boolean }>(), { linkable: true })

const text = computed(() => {
  const p = props.proxy
  const bind = p.bind_addr || '127.0.0.1'
  const ip = p.local_ip || '127.0.0.1'
  if (p.kind === 'visitor' && p.bind_port) return `${bind}:${p.bind_port}`
  if (p.kind === 'visitor') return `访问 → ${p.server_name || '-'}`
  if (p.local_port) return `${ip}:${p.local_port}`
  return ip
})

const href = computed(() => {
  const p = props.proxy
  if (p.kind === 'visitor' && p.bind_port) return `http://${p.bind_addr || '127.0.0.1'}:${p.bind_port}`
  if (p.kind !== 'visitor' && p.local_port) return `http://${p.local_ip || '127.0.0.1'}:${p.local_port}`
  return ''
})
</script>
