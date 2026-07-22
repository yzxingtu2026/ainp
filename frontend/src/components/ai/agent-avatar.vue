<script setup lang="ts">
import { computed, toRef } from 'vue';
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
const initial = computed(() => Array.from(props.name.trim())[0]?.toUpperCase() || 'A');
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
    <span v-else class="agent-avatar__initial">{{ initial }}</span>
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

.agent-avatar__initial {
  font-size: 24px;
  font-weight: 700;
}
</style>
