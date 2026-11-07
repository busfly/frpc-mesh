<template>
  <el-drawer
    :model-value="modelValue"
    direction="btt"
    size="auto"
    :with-header="false"
    class="fm-sheet fm-actionsheet"
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div ref="handleEl" class="fm-handle"></div>
    <p v-if="title" class="as-title">{{ title }}</p>
    <button
      v-for="a in actions"
      :key="a.key"
      class="as-item"
      :class="{ danger: a.danger }"
      :disabled="a.disabled"
      @click="pick(a)"
    >
      <span>{{ a.label }}</span>
      <span v-if="a.hint" class="as-hint">{{ a.hint }}</span>
    </button>
    <button class="as-item as-cancel" @click="emit('update:modelValue', false)">取消</button>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useSwipeToClose } from '../../composables/useSwipeToClose'

export interface ActionItem {
  key: string
  label: string
  hint?: string
  danger?: boolean
  disabled?: boolean
}

defineProps<{ modelValue: boolean; title?: string; actions: ActionItem[] }>()
const emit = defineEmits<{ 'update:modelValue': [boolean]; action: [string] }>()

const handleEl = ref<HTMLElement | null>(null)
useSwipeToClose(handleEl, () => emit('update:modelValue', false))

function pick(a: ActionItem) {
  if (a.disabled) return
  emit('action', a.key)
  emit('update:modelValue', false)
}
</script>
