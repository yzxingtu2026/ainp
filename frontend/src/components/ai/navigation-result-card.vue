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
    <article v-for="item in items" :key="`${item.menu_id}:${item.path}`" class="navigation-result">
      <div class="navigation-result__icon" aria-hidden="true">
        <SvgIcon :icon="item.icon || 'mdi:map-marker-path'" :local-icon="item.local_icon" />
      </div>
      <div class="navigation-result__body">
        <strong class="navigation-result__title">{{ item.title }}</strong>
        <span class="navigation-result__breadcrumb">{{ item.breadcrumb.join(' / ') }}</span>
        <p v-if="item.description" class="navigation-result__description">{{ item.description }}</p>
      </div>
      <NTooltip :disabled="available(item)">
        <template #trigger>
          <NButton size="small" type="primary" secondary :disabled="!available(item)" @click="open(item)">
            <template #icon><SvgIcon icon="mdi:arrow-right" /></template>
            {{ $t('chat.navigationOpen') }}
          </NButton>
        </template>
        {{ $t('chat.navigationUnavailable') }}
      </NTooltip>
    </article>
  </div>
</template>

<style scoped>
.navigation-results {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
}

.navigation-results__pending {
  display: flex;
  align-items: center;
  gap: 8px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 13px;
}

.navigation-result {
  display: grid;
  grid-template-columns: 36px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-height: 76px;
  border: 1px solid rgba(127, 127, 127, 0.18);
  border-radius: 8px;
  padding: 10px 12px;
  background: rgba(127, 127, 127, 0.04);
}

.navigation-result__icon {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border-radius: 6px;
  background: rgba(var(--primary-color), 0.1);
  color: rgb(var(--primary-color));
  font-size: 20px;
}

.navigation-result__body {
  min-width: 0;
}
.navigation-result__title,
.navigation-result__breadcrumb {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.navigation-result__title {
  font-size: 14px;
}
.navigation-result__breadcrumb {
  margin-top: 2px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 12px;
}
.navigation-result__description {
  margin: 4px 0 0;
  color: rgba(127, 127, 127, 0.95);
  font-size: 12px;
  line-height: 1.45;
}

@media (max-width: 640px) {
  .navigation-result {
    grid-template-columns: 32px minmax(0, 1fr);
  }
  .navigation-result :deep(.n-button) {
    grid-column: 1 / -1;
    width: 100%;
  }
}
</style>
