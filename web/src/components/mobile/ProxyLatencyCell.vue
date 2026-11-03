<template>
  <span v-if="!enabled || latency === null" class="text-muted">--</span>
  <span v-else-if="latency === -1" class="lat-na">不通</span>
  <span v-else-if="latency === 0" class="lat-ok">可通</span>
  <span v-else :class="latency > 200 ? 'lat-slow' : 'lat-ok'">{{ latency }} ms</span>
</template>

<script setup lang="ts">
// latency 语义沿用 ProxiesView 的 latencyOf：null 未测、-1 不通、0 可通、其余为毫秒
defineProps<{ enabled: boolean; latency: number | null }>()
</script>
