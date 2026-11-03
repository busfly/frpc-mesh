<template>
  <el-link
    v-if="(proxy.type === 'tcp' || proxy.type === 'udp') && proxy.remote_port"
    type="primary"
    :underline="false"
    class="mono"
    :href="`http://${remoteHost}:${proxy.remote_port}`"
    target="_blank"
  >
    {{ proxy.remote_port }}
  </el-link>
  <span v-else-if="proxy.type === 'tcp' || proxy.type === 'udp'" class="mono">随机</span>
  <el-link
    v-else-if="proxy.custom_domains"
    type="primary"
    :underline="false"
    class="mono"
    :href="`${proxy.type === 'https' ? 'https' : 'http'}://${proxy.custom_domains.split(',')[0].trim()}/`"
    target="_blank"
  >
    {{ proxy.custom_domains.split(',')[0].trim() }}
  </el-link>
  <span v-else-if="proxy.subdomain" class="mono">子域名: {{ proxy.subdomain }}</span>
  <span v-else class="text-muted">-</span>
</template>

<script setup lang="ts">
import type { Proxy } from '../../api/types'

defineProps<{ proxy: Proxy; remoteHost: string }>()
</script>
