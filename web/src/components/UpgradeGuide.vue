<template>
  <div class="upgrade-guide">
    <p class="guide-intro">{{ guide.intro }}</p>
    <GuideSteps :steps="guide.steps" />
    <p v-if="guide.note" class="guide-note">{{ guide.note }}</p>
    <el-collapse v-model="open" class="guide-fallback">
      <el-collapse-item name="fallback" :title="guide.fallback.title">
        <GuideSteps :steps="[guide.fallback]" hide-title />
      </el-collapse-item>
    </el-collapse>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { UpgradeGuide } from '../api/types'
import GuideSteps from './GuideSteps.vue'

defineProps<{ guide: UpgradeGuide }>()

// 回退指引默认收起：平时不占地方，出事时一定用得上
const open = ref<string[]>([])
</script>

<style scoped>
.upgrade-guide {
  margin-top: 10px;
  padding: 12px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  background: var(--el-fill-color-lighter);
  text-align: left;
}
.guide-intro {
  margin: 0 0 10px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--el-text-color-regular);
}
.guide-note {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.guide-fallback {
  margin-top: 8px;
  border-top: none;
}
.guide-fallback :deep(.el-collapse-item__header) {
  font-size: 13px;
  height: 34px;
  line-height: 34px;
}
.guide-fallback :deep(.guide-steps) {
  list-style: none;
  padding-left: 0;
}
</style>
