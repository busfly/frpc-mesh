<template>
  <ol class="guide-steps">
    <li v-for="(s, i) in steps" :key="i" class="guide-step">
      <div v-if="!hideTitle" class="step-head">
        <span class="step-title">{{ s.title }}</span>
        <el-tag v-if="s.optional" size="small" type="info" effect="plain">可选</el-tag>
      </div>
      <p v-if="s.desc" class="step-desc">{{ s.desc }}</p>
      <a v-if="s.link" class="step-link" :href="s.link.url" @click="onLink($event, s.link?.url)">
        {{ s.link.label }}
      </a>
      <template v-for="(c, ci) in s.commands || []" :key="ci">
        <!-- # 开头是后端的分组说明（如 compose / docker run 两套命令的分隔），不给复制按钮 -->
        <p v-if="isCaption(c)" class="cmd-caption">{{ c }}</p>
        <div v-else class="cmd-row">
          <code class="cmd mono">{{ c }}</code>
          <el-tooltip content="复制这条命令" placement="top">
            <el-button class="cmd-copy" text :icon="DocumentCopy" @click="onCopy($event, c)" />
          </el-tooltip>
        </div>
      </template>
    </li>
  </ol>
</template>
<script setup lang="ts">
import { DocumentCopy } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { GuideStep } from '../api/types'
import { copyText } from '../utils/clipboard'

defineProps<{ steps: GuideStep[]; hideTitle?: boolean }>()

function isCaption(cmd: string): boolean {
  return cmd.trimStart().startsWith('#')
}

/** # 开头的链接是页内动作：滚动到目标卡片，不跳转 */
function onLink(e: MouseEvent, url?: string) {
  if (!url || !url.startsWith('#')) return
  e.preventDefault()
  document.getElementById(url.slice(1))?.scrollIntoView({ block: 'center' })
}

function onCopy(e: MouseEvent, cmd: string) {
  const row = (e.currentTarget as HTMLElement | null)?.closest('.cmd-row')
  const code = row?.querySelector('code') ?? null
  void copyText(cmd, code).then((r) => {
    if (r === 'copied') ElMessage.success('已复制')
    else if (r === 'selected') ElMessage.info('已选中该命令，按 Ctrl/Cmd+C 复制')
    else ElMessage.error('复制失败，请用鼠标选中命令后手动复制')
  })
}
</script>

<style scoped>
.guide-steps {
  margin: 0;
  padding-left: 20px;
}
.guide-step + .guide-step {
  margin-top: 12px;
}
.step-head {
  display: flex;
  align-items: center;
  gap: 6px;
}
.step-title {
  font-weight: 600;
}
.step-desc {
  margin: 4px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
}
.step-link {
  display: inline-block;
  margin-top: 6px;
  font-size: 13px;
  color: var(--el-color-primary);
}
.cmd-caption {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}
.cmd-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
}
.cmd {
  flex: 1;
  min-width: 0;
  padding: 6px 8px;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 4px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  text-align: left;
}
.cmd-copy {
  flex: none;
}
</style>
