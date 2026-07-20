<script setup lang="ts">
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAppStore } from '@/store/modules/app';
import { useRouteStore } from '@/store/modules/route';
import { useThemeStore } from '@/store/modules/theme';
import { $t } from '@/locales';

defineOptions({ name: 'AiWorkspaceLauncher' });

const props = defineProps<{ location: 'sider' | 'header' }>();
const appStore = useAppStore();
const routeStore = useRouteStore();
const themeStore = useThemeStore();
const route = useRoute();
const router = useRouter();

const launcherRoute = computed(() => routeStore.globalAiLauncherRoute);
const label = computed(() => {
  const meta = launcherRoute.value?.meta;
  if (meta?.i18nKey) return $t(meta.i18nKey);
  return meta?.title || $t('page.aiWorkspace.title');
});
const selected = computed(() => Boolean(launcherRoute.value && route.path.startsWith(launcherRoute.value.path)));
const collapsed = computed(() => props.location === 'sider' && appStore.siderCollapse);
const iconOnly = computed(
  () =>
    props.location === 'sider' &&
    (collapsed.value ||
      themeStore.layout.mode === 'vertical-mix' ||
      themeStore.layout.mode === 'vertical-hybrid-header-first' ||
      themeStore.layout.mode === 'top-hybrid-sidebar-first')
);

function openWorkspace() {
  if (launcherRoute.value) void router.push(launcherRoute.value.path);
}
</script>

<template>
  <div v-if="launcherRoute" class="ai-launcher-wrap" :class="`ai-launcher-wrap--${location}`">
    <NTooltip :disabled="!iconOnly" placement="right">
      <template #trigger>
        <button
          type="button"
          class="ai-launcher"
          :class="{ 'ai-launcher--selected': selected, 'ai-launcher--collapsed': iconOnly }"
          :aria-label="label"
          :aria-current="selected ? 'page' : undefined"
          @click="openWorkspace"
        >
          <AiMascot :size="location === 'header' ? 38 : 44" />
          <span v-if="!iconOnly" class="ai-launcher__label">{{ label }}</span>
        </button>
      </template>
      {{ label }}
    </NTooltip>
  </div>
</template>

<style scoped>
.ai-launcher-wrap--sider {
  flex: none;
  border-top: 1px solid rgba(127, 127, 127, 0.16);
  padding: 8px;
}

.ai-launcher-wrap--header {
  margin-right: 6px;
}

.ai-launcher {
  width: 100%;
  min-width: 44px;
  height: 48px;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  padding: 2px 10px 2px 4px;
  color: inherit;
  cursor: pointer;
  transition:
    background-color 160ms ease,
    color 160ms ease;
}

.ai-launcher:hover {
  background: rgba(var(--primary-color), 0.09);
}

.ai-launcher--selected {
  background: rgba(var(--primary-color), 0.14);
  color: rgb(var(--primary-color));
}

.ai-launcher--collapsed {
  justify-content: center;
  padding: 2px;
}

.ai-launcher__label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  font-weight: 600;
}

.ai-launcher-wrap--header .ai-launcher {
  width: auto;
  height: 42px;
  padding-right: 12px;
}

@media (prefers-reduced-motion: reduce) {
  .ai-launcher {
    transition: none;
  }
}
</style>
