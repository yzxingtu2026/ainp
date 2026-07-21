<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import sittingUrl from '@/assets/imgs/home-robot/robot-sitting.webp';
import thinkingUrl from '@/assets/imgs/home-robot/robot-thinking.webp';
import meditatingUrl from '@/assets/imgs/home-robot/robot-meditating.webp';
import jumpingUrl from '@/assets/imgs/home-robot/robot-jumping.webp';
import wavingUrl from '@/assets/imgs/home-robot/robot-waving.webp';
import staticUrl from '@/assets/imgs/home-robot/robot-sitting-static.png';

defineOptions({ name: 'HomeRobot' });

type RobotState = 'sitting' | 'thinking' | 'meditating' | 'jumping' | 'waving';

const props = defineProps<{ mode?: 'thinking' | 'jumping' | null }>();

const sources: Record<RobotState, string> = {
  sitting: sittingUrl,
  thinking: thinkingUrl,
  meditating: meditatingUrl,
  jumping: jumpingUrl,
  waving: wavingUrl
};

const state = ref<RobotState>('waving');
const reducedMotion = ref(false);
const documentVisible = ref(true);
const renderKey = ref(0);
let welcomeTimer: number | null = null;
let idleTimer: number | null = null;
let lastActivityAt = 0;
let mediaQuery: MediaQueryList | null = null;

const source = computed(() => (reducedMotion.value ? staticUrl : sources[state.value]));

function clearTimer(timer: number | null) {
  if (timer !== null) window.clearTimeout(timer);
}

function setState(next: RobotState) {
  if (state.value === next) return;
  state.value = next;
  renderKey.value += 1;
}

function scheduleMeditation() {
  clearTimer(idleTimer);
  idleTimer = null;
  if (reducedMotion.value || !documentVisible.value || props.mode) return;
  idleTimer = window.setTimeout(() => setState('meditating'), 20_000);
}

function returnToSitting() {
  if (!props.mode) setState('sitting');
  scheduleMeditation();
}

function registerActivity() {
  const now = Date.now();
  if (now - lastActivityAt < 800 || props.mode) return;
  lastActivityAt = now;
  if (state.value === 'meditating') setState('sitting');
  scheduleMeditation();
}

function handleVisibilityChange() {
  documentVisible.value = document.visibilityState === 'visible';
  if (documentVisible.value) returnToSitting();
  else {
    clearTimer(idleTimer);
    idleTimer = null;
  }
}

function handleReducedMotionChange(event: MediaQueryListEvent | MediaQueryList) {
  reducedMotion.value = event.matches;
  if (event.matches) {
    clearTimer(welcomeTimer);
    clearTimer(idleTimer);
    setState('sitting');
  } else {
    returnToSitting();
  }
}

watch(
  () => props.mode,
  mode => {
    clearTimer(idleTimer);
    idleTimer = null;
    if (mode) setState(mode);
    else returnToSitting();
  }
);

onMounted(() => {
  Object.values(sources).forEach(url => {
    const image = new Image();
    image.src = url;
  });

  mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
  handleReducedMotionChange(mediaQuery);
  mediaQuery.addEventListener('change', handleReducedMotionChange);
  document.addEventListener('visibilitychange', handleVisibilityChange);
  window.addEventListener('pointermove', registerActivity, { passive: true });
  window.addEventListener('keydown', registerActivity);
  window.addEventListener('focus', registerActivity);

  if (!reducedMotion.value) {
    setState('waving');
    welcomeTimer = window.setTimeout(returnToSitting, 1_000);
  }
});

onBeforeUnmount(() => {
  clearTimer(welcomeTimer);
  clearTimer(idleTimer);
  mediaQuery?.removeEventListener('change', handleReducedMotionChange);
  document.removeEventListener('visibilitychange', handleVisibilityChange);
  window.removeEventListener('pointermove', registerActivity);
  window.removeEventListener('keydown', registerActivity);
  window.removeEventListener('focus', registerActivity);
});
</script>

<template>
  <div class="home-robot" aria-hidden="true">
    <img :key="`${state}-${renderKey}`" :src="source" alt="" />
  </div>
</template>

<style scoped>
.home-robot {
  width: min(30vw, 260px);
  min-width: 190px;
  aspect-ratio: 1;
  display: grid;
  place-items: center;
  overflow: hidden;
}

.home-robot img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  user-select: none;
  pointer-events: none;
}

@media (max-width: 640px) {
  .home-robot {
    width: 154px;
    min-width: 154px;
  }
}
</style>
