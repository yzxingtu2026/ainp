<script lang="ts" setup>
import { computed, ref, shallowRef } from 'vue';
import { useRouter } from 'vue-router';
import { onKeyStroke, useDebounceFn, useOnline } from '@vueuse/core';
import { useRouteStore } from '@/store/modules/route';
import { useAppStore } from '@/store/modules/app';
import { $t } from '@/locales';
import { fetchNavigationSearch } from '@/service/api';
import SearchResult from './search-result.vue';
import SearchFooter from './search-footer.vue';

defineOptions({ name: 'SearchModal' });

const router = useRouter();
const appStore = useAppStore();
const routeStore = useRouteStore();

const isMobile = computed(() => appStore.isMobile);
const online = useOnline();

const keyword = ref('');
const activePath = ref('');
const resultOptions = shallowRef<Api.Route.NavigationSearchResult[]>([]);
const loading = ref(false);
const error = ref(false);
const degraded = ref(false);
let requestSequence = 0;

const handleSearch = useDebounceFn(search, 300);

const visible = defineModel<boolean>('show', { required: true });

async function search() {
  const query = keyword.value.trim();
  const sequence = ++requestSequence;
  error.value = false;
  degraded.value = false;
  if (!query) {
    loading.value = false;
    resultOptions.value = [];
    activePath.value = '';
    return;
  }
  loading.value = true;
  const { data, error: requestError } = await fetchNavigationSearch(query);
  if (sequence !== requestSequence) return;
  loading.value = false;
  error.value = Boolean(requestError) || !data;
  degraded.value = Boolean(data?.degraded);
  resultOptions.value = data?.items || [];
  activePath.value = resultOptions.value[0]?.path ?? '';
}

function handleClose() {
  // handle with setTimeout to prevent user from seeing some operations
  setTimeout(() => {
    visible.value = false;
    resultOptions.value = [];
    keyword.value = '';
    loading.value = false;
    error.value = false;
    degraded.value = false;
  }, 200);
}

/** key up */
function handleUp() {
  const { length } = resultOptions.value;
  if (length === 0) return;

  const index = getActivePathIndex();
  if (index === -1) return;

  const activeIndex = index === 0 ? length - 1 : index - 1;

  activePath.value = resultOptions.value[activeIndex].path;
}

/** key down */
function handleDown() {
  const { length } = resultOptions.value;
  if (length === 0) return;

  const index = getActivePathIndex();
  if (index === -1) return;

  const activeIndex = index === length - 1 ? 0 : index + 1;

  activePath.value = resultOptions.value[activeIndex].path;
}

function getActivePathIndex() {
  return resultOptions.value.findIndex(item => item.path === activePath.value);
}

/** key enter */
function handleEnter() {
  if (resultOptions.value?.length === 0 || activePath.value === '') return;
  const target = resultOptions.value.find(item => item.path === activePath.value);
  if (!target || !routeStore.isNavigationTargetAvailable(target.menu_id, target.path)) {
    window.$message?.warning($t('chat.navigationUnavailable'));
    return;
  }
  handleClose();
  router.push(target.path);
}

function registerShortcut() {
  onKeyStroke('Escape', handleClose);
  onKeyStroke('Enter', handleEnter);
  onKeyStroke('ArrowUp', handleUp);
  onKeyStroke('ArrowDown', handleDown);
}

registerShortcut();
</script>

<template>
  <NModal
    v-model:show="visible"
    :segmented="{ footer: 'soft' }"
    :closable="false"
    preset="card"
    auto-focus
    footer-style="padding: 0; margin: 0"
    class="fixed left-0 right-0"
    :class="[isMobile ? 'size-full top-0px rounded-0' : 'w-630px top-50px']"
    @after-leave="handleClose"
  >
    <NInputGroup>
      <NInput v-model:value="keyword" clearable :placeholder="$t('common.keywordSearch')" @input="handleSearch">
        <template #prefix>
          <icon-uil-search class="text-15px text-#c2c2c2" />
        </template>
      </NInput>
      <NButton v-if="isMobile" type="primary" ghost @click="handleClose">{{ $t('common.cancel') }}</NButton>
    </NInputGroup>

    <div class="mt-20px">
      <div v-if="loading" class="h-120px flex-center"><NSpin /></div>
      <NResult
        v-else-if="error"
        status="error"
        :title="$t('common.error')"
        :description="online ? $t('chat.navigationSearchError') : $t('chat.navigationOffline')"
      />
      <template v-else>
        <NAlert v-if="degraded" type="warning" class="mb-10px">{{ $t('chat.navigationDegraded') }}</NAlert>
        <NEmpty v-if="resultOptions.length === 0" :description="$t('common.noData')" />
        <SearchResult v-else v-model:path="activePath" :options="resultOptions" @enter="handleEnter" />
      </template>
    </div>
    <template #footer>
      <SearchFooter v-if="!isMobile" />
    </template>
  </NModal>
</template>

<style lang="scss" scoped></style>
