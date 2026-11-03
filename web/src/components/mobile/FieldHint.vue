<template>
  <!-- 有 hover 的设备保持原样（桌面零回归）；触屏改成点击展开同一段文案 -->
  <el-tooltip v-if="hoverCapable" :content="content" :placement="placement">
    <el-icon class="fm-hint-icon"><QuestionFilled /></el-icon>
  </el-tooltip>
  <el-popover v-else :content="content" trigger="click" placement="top" :width="230">
    <template #reference>
      <button type="button" class="fm-hint-btn" aria-label="查看说明">?</button>
    </template>
  </el-popover>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { QuestionFilled } from '@element-plus/icons-vue'

withDefaults(defineProps<{ content: string; placement?: 'top' | 'bottom' | 'left' | 'right' }>(), {
  placement: 'top'
})

const hoverCapable = computed(
  () => typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia('(hover: hover)').matches
)
</script>

<style scoped>
.fm-hint-icon {
  color: #909399;
  cursor: help;
}
</style>
