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
      <NSpin :size="15" />
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
            <span class="navigation-result__icon-badge" aria-hidden="true">
              <SvgIcon
                class="navigation-result__icon"
                :icon="item.icon || 'mdi:map-marker-path'"
                :local-icon="item.local_icon"
              />
            </span>
            <strong class="navigation-result__title">{{ item.title }}</strong>
            <span v-if="item.breadcrumb.length > 1" class="navigation-result__breadcrumb">
              {{ item.breadcrumb.slice(0, -1).join(' / ') }}
            </span>
            <SvgIcon class="navigation-result__arrow" icon="mdi:arrow-up-right" aria-hidden="true" />
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
  gap: 7px;
  margin-top: 8px;
}

.navigation-results__pending {
  display: flex;
  align-items: center;
  gap: 8px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 12.5px;
}

.navigation-result-trigger {
  display: inline-flex;
  max-width: 100%;
}

.navigation-result {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  max-width: 100%;
  height: 34px;
  border: 1px solid rgba(var(--primary-color), 0.22);
  border-radius: 10px;
  padding: 0 10px 0 7px;
  background: linear-gradient(135deg, rgba(var(--primary-color), 0.06), rgba(8, 145, 178, 0.04));
  color: inherit;
  font: inherit;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background 0.2s ease,
    color 0.2s ease,
    box-shadow 0.2s ease;
}

.navigation-result:hover:not(:disabled),
.navigation-result:focus-visible {
  border-color: rgba(var(--primary-color), 0.5);
  outline: none;
  background: linear-gradient(135deg, rgba(var(--primary-color), 0.12), rgba(8, 145, 178, 0.08));
  color: rgb(var(--primary-color));
  box-shadow: 0 3px 10px rgba(var(--primary-color), 0.12);
}

.navigation-result:disabled {
  cursor: not-allowed;
  opacity: 0.48;
  border-color: rgba(127, 127, 127, 0.2);
  background: rgba(127, 127, 127, 0.05);
}

.navigation-result__icon-badge {
  width: 22px;
  height: 22px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 6px;
  background: rgba(var(--primary-color), 0.12);
}

.navigation-result__icon {
  font-size: 14px;
  color: rgb(var(--primary-color));
}

.navigation-result__arrow {
  flex: none;
  font-size: 15px;
  color: rgba(var(--primary-color), 0.82);
  transition: transform 0.18s ease;
}

.navigation-result:hover:not(:disabled) .navigation-result__arrow,
.navigation-result:focus-visible .navigation-result__arrow {
  transform: translate(2px, -2px);
}

.navigation-result__title,
.navigation-result__breadcrumb {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.navigation-result__title {
  max-width: 168px;
  font-size: 12.5px;
  font-weight: 620;
}

.navigation-result__breadcrumb {
  max-width: 128px;
  padding-left: 7px;
  border-left: 1px solid rgba(127, 127, 127, 0.22);
  color: rgba(127, 127, 127, 0.92);
  font-size: 11px;
}

@media (max-width: 640px) {
  .navigation-result__breadcrumb {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .navigation-result,
  .navigation-result__arrow {
    transition: none;
  }

  .navigation-result:hover:not(:disabled) .navigation-result__arrow,
  .navigation-result:focus-visible .navigation-result__arrow {
    transform: none;
  }
}
</style>
