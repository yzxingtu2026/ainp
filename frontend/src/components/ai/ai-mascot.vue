<script setup lang="ts">
import { computed } from 'vue';
import idleUrl from '@/assets/imgs/ai-workspace/mascot-idle.png';
import breatheUrl from '@/assets/imgs/ai-workspace/mascot-breathe.png';
import blinkUrl from '@/assets/imgs/ai-workspace/mascot-blink.png';
import thinkingUrl from '@/assets/imgs/ai-workspace/mascot-thinking.png';
import workingUrl from '@/assets/imgs/ai-workspace/mascot-working.png';
import errorUrl from '@/assets/imgs/ai-workspace/mascot-error.png';

defineOptions({ name: 'AiMascot' });

type MascotState = 'idle' | 'thinking' | 'working' | 'error';

const props = withDefaults(
  defineProps<{
    state?: MascotState;
    size?: number;
    breathe?: boolean;
  }>(),
  { state: 'idle', size: 48, breathe: true }
);

const stateUrl = computed(() => {
  if (props.state === 'thinking') return thinkingUrl;
  if (props.state === 'working') return workingUrl;
  if (props.state === 'error') return errorUrl;
  return idleUrl;
});
</script>

<template>
  <span
    class="ai-mascot"
    :class="{ 'ai-mascot--breathe': breathe && state === 'idle' }"
    :style="{ width: `${size}px`, height: `${size}px` }"
    aria-hidden="true"
  >
    <img :src="stateUrl" alt="" class="ai-mascot__frame ai-mascot__frame--base" />
    <template v-if="breathe && state === 'idle'">
      <img :src="breatheUrl" alt="" class="ai-mascot__frame ai-mascot__frame--breathe" />
      <img :src="blinkUrl" alt="" class="ai-mascot__frame ai-mascot__frame--blink" />
    </template>
  </span>
</template>

<style scoped>
.ai-mascot {
  position: relative;
  display: inline-block;
  flex: none;
}

.ai-mascot__frame {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
  user-select: none;
}

.ai-mascot__frame--breathe,
.ai-mascot__frame--blink {
  opacity: 0;
}

.ai-mascot--breathe {
  animation: mascot-float 2.8s ease-in-out infinite;
}

.ai-mascot--breathe .ai-mascot__frame--base {
  animation: mascot-idle-frame 5.6s linear infinite;
}

.ai-mascot--breathe .ai-mascot__frame--breathe {
  animation: mascot-breathe-frame 2.8s ease-in-out infinite;
}

.ai-mascot--breathe .ai-mascot__frame--blink {
  animation: mascot-blink-frame 5.6s linear infinite;
}

@keyframes mascot-float {
  0%,
  100% {
    transform: translateY(0) scale(1);
  }
  50% {
    transform: translateY(-1px) scale(1.018);
  }
}

@keyframes mascot-idle-frame {
  0%,
  46%,
  51%,
  100% {
    opacity: 1;
  }
  48%,
  49% {
    opacity: 0;
  }
}

@keyframes mascot-breathe-frame {
  0%,
  100% {
    opacity: 0;
  }
  45%,
  55% {
    opacity: 1;
  }
}

@keyframes mascot-blink-frame {
  0%,
  47%,
  50%,
  100% {
    opacity: 0;
  }
  48%,
  49% {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ai-mascot--breathe,
  .ai-mascot--breathe .ai-mascot__frame {
    animation: none;
  }
}
</style>
