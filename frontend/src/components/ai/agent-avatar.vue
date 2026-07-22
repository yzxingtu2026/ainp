<script setup lang="ts">
import { toRef } from 'vue';
import defaultAgentAvatar from '@/assets/imgs/ai-workspace/mascot-blink.png';
import { useAgentAvatar } from '@/hooks/business/agent-avatar';

defineOptions({ name: 'AgentAvatar' });

const props = withDefaults(
  defineProps<{
    attachmentId?: string | null;
    name: string;
    size?: number;
  }>(),
  { attachmentId: null, size: 72 }
);

const attachmentId = toRef(props, 'attachmentId');
const { url, status } = useAgentAvatar(attachmentId);
</script>

<template>
  <div
    class="agent-avatar"
    :style="{ width: `${size}px`, height: `${size}px` }"
    role="img"
    :aria-label="$t('page.aiAgents.avatarLabel', { name })"
  >
    <img v-if="url" :src="url" :alt="name" />
    <NSpin v-else-if="status === 'loading'" :size="20" />
    <img v-else :src="defaultAgentAvatar" alt="" class="agent-avatar__default" />
  </div>
</template>

<style scoped>
.agent-avatar {
  display: grid;
  flex: none;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgb(var(--primary-color) / 18%);
  border-radius: 8px;
  color: rgb(var(--primary-color));
  background: rgb(var(--primary-color) / 10%);
}

.agent-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.agent-avatar__default {
  padding: 5px;
  object-fit: contain !important;
}
</style>
