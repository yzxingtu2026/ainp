<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useRouteStore } from '@/store/modules/route';
import { $t } from '@/locales';

defineOptions({ name: 'NavigationResultCard' });

const props = defineProps<{
  result: unknown | unknown[];
  pending?: boolean;
}>();

const router = useRouter();
const routeStore = useRouteStore();

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function parseItem(value: unknown): Api.Route.NavigationSearchResult | null {
  if (!isRecord(value)) return null;
  if (typeof value.menu_id !== 'string' || typeof value.title !== 'string' || typeof value.path !== 'string')
    return null;
  if (!Array.isArray(value.breadcrumb) || !value.breadcrumb.every(item => typeof item === 'string')) return null;
  return value as unknown as Api.Route.NavigationSearchResult;
}

const items = computed(() => {
  const results = Array.isArray(props.result) ? props.result : [props.result];
  const unique = new Map<string, Api.Route.NavigationSearchResult>();
  for (const result of results) {
    if (!isRecord(result) || !isRecord(result.data) || !Array.isArray(result.data.items)) continue;
    for (const value of result.data.items) {
      const item = parseItem(value);
      if (item) unique.set(`${item.menu_id}:${item.path}`, item);
    }
  }
  return Array.from(unique.values()).slice(0, 3);
});

function available(item: Api.Route.NavigationSearchResult) {
  return routeStore.isNavigationTargetAvailable(item.menu_id, item.path);
}

function tooltip(item: Api.Route.NavigationSearchResult) {
  if (!available(item)) return $t('chat.navigationUnavailable');
  return [item.breadcrumb.join(' / '), item.description].filter(Boolean).join(' · ');
}

async function open(item: Api.Route.NavigationSearchResult) {
  if (!available(item)) {
    window.$message?.warning($t('chat.navigationUnavailable'));
    return;
  }
  await router.push(item.path);
}
</script>

<template>
  <div class="navigation-results">
    <div v-if="pending && items.length === 0" class="navigation-results__pending">
      <NSpin :size="16" />
      <span>{{ $t('chat.navigationSearching') }}</span>
    </div>
    <NEmpty v-else-if="items.length === 0" size="small" :description="$t('chat.navigationEmpty')" />
    <NTooltip v-for="item in items" :key="`${item.menu_id}:${item.path}`">
      <template #trigger>
        <span class="navigation-result-trigger">
          <button
            type="button"
            class="navigation-result"
            :disabled="!available(item)"
            :aria-label="`${$t('chat.navigationOpen')}：${item.title}`"
            @click="open(item)"
          >
            <SvgIcon
              class="navigation-result__icon"
              :icon="item.icon || 'mdi:map-marker-path'"
              :local-icon="item.local_icon"
            />
            <strong class="navigation-result__title">{{ item.title }}</strong>
            <span v-if="item.breadcrumb.length > 1" class="navigation-result__breadcrumb">
              {{ item.breadcrumb.slice(0, -1).join(' / ') }}
            </span>
            <SvgIcon class="navigation-result__arrow" icon="mdi:arrow-right" aria-hidden="true" />
          </button>
        </span>
      </template>
      {{ tooltip(item) }}
    </NTooltip>
  </div>
</template>

<style scoped>
.navigation-results {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-top: 8px;
}

.navigation-results__pending {
  display: flex;
  align-items: center;
  gap: 8px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 13px;
}

.navigation-result-trigger {
  display: inline-flex;
  max-width: 100%;
}

.navigation-result {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  height: 32px;
  border: 1px solid rgba(127, 127, 127, 0.18);
  border-radius: 999px;
  padding: 0 10px;
  background: rgba(127, 127, 127, 0.06);
  color: inherit;
  font: inherit;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background 0.2s ease,
    color 0.2s ease;
}

.navigation-result:hover:not(:disabled),
.navigation-result:focus-visible {
  border-color: rgba(var(--primary-color), 0.4);
  outline: none;
  background: rgba(var(--primary-color), 0.1);
  color: rgb(var(--primary-color));
}

.navigation-result:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.navigation-result__icon,
.navigation-result__arrow {
  flex: none;
  font-size: 16px;
  color: rgb(var(--primary-color));
}

.navigation-result__title,
.navigation-result__breadcrumb {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.navigation-result__title {
  max-width: 160px;
  font-size: 13px;
  font-weight: 600;
}
.navigation-result__breadcrumb {
  max-width: 120px;
  padding-left: 6px;
  border-left: 1px solid rgba(127, 127, 127, 0.22);
  color: rgba(127, 127, 127, 0.95);
  font-size: 11px;
}

@media (max-width: 640px) {
  .navigation-result__breadcrumb {
    display: none;
  }
}
</style>
