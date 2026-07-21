<script lang="ts" setup>
import { useThemeStore } from '@/store/modules/theme';

defineOptions({ name: 'SearchResult' });

interface Props {
  options: Api.Route.NavigationSearchResult[];
}

defineProps<Props>();

interface Emits {
  (e: 'enter'): void;
}

const emit = defineEmits<Emits>();

const theme = useThemeStore();

const active = defineModel<string>('path', { required: true });

async function handleMouseEnter(item: Api.Route.NavigationSearchResult) {
  active.value = item.path;
}

function handleTo(item: Api.Route.NavigationSearchResult) {
  active.value = item.path;
  emit('enter');
}
</script>

<template>
  <NScrollbar>
    <div class="pb-12px">
      <template v-for="item in options" :key="item.menu_id">
        <div
          class="mt-8px h-56px flex-y-center cursor-pointer justify-between rounded-4px bg-#e5e7eb px-14px dark:bg-dark"
          :style="{
            background: item.path === active ? theme.themeColor : '',
            color: item.path === active ? '#fff' : ''
          }"
          @click="handleTo(item)"
          @mouseenter="handleMouseEnter(item)"
        >
          <SvgIcon :icon="item.icon || 'mdi:file-outline'" :local-icon="item.local_icon" class="text-20px" />
          <span class="ml-8px min-w-0 flex-1">
            <strong class="block truncate text-14px">{{ item.title }}</strong>
            <span class="block truncate text-12px opacity-75">{{ item.breadcrumb.join(' / ') }}</span>
            <span v-if="item.description" class="block truncate text-12px opacity-75">{{ item.description }}</span>
          </span>
          <icon-ant-design-enter-outlined class="icon mr-3px p-2px text-20px" />
        </div>
      </template>
    </div>
  </NScrollbar>
</template>

<style lang="scss" scoped></style>
