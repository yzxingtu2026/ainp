<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { $t } from '@/locales';
import { fetchSetupCheckEnv } from '@/service/api';

interface Props {
  bgThemeColor: string;
}

defineProps<Props>();

const emit = defineEmits<{
  (e: 'checked', data: Api.Setup.CheckEnv): void;
  (e: 'next'): void;
}>();

const loading = ref(false);
const envData = ref<Api.Setup.CheckEnv | null>(null);
const error = ref('');

async function checkEnv() {
  loading.value = true;
  error.value = '';
  try {
    const res = await fetchSetupCheckEnv();
    if (res.error || !res.data) {
      error.value = res.error?.message || 'Failed to check environment';
      return;
    }
    envData.value = res.data;
    emit('checked', res.data);
  } catch (e: any) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

function handleNext() {
  emit('next');
}

onMounted(() => {
  checkEnv();
});
</script>

<template>
  <div>
    <NSpin :show="loading">
      <NAlert v-if="error" type="error" class="mb-16px">{{ error }}</NAlert>

      <NDescriptions v-if="envData" bordered :column="1" size="small" class="mb-24px">
        <NDescriptionsItem :label="$t('page.setup.goVersion')">{{ envData.go_version }}</NDescriptionsItem>
        <NDescriptionsItem :label="$t('page.setup.os')">{{ envData.os }} / {{ envData.arch }}</NDescriptionsItem>
        <NDescriptionsItem :label="$t('page.setup.dsnSource')">
          <NTag :type="envData.dsn_present ? 'success' : 'warning'" size="small">
            {{ envData.dsn_source }}
          </NTag>
        </NDescriptionsItem>
        <NDescriptionsItem :label="$t('page.setup.i18nLangs')">{{ envData.i18n_langs }}</NDescriptionsItem>
        <NDescriptionsItem :label="$t('page.setup.i18nKeys')">{{ envData.i18n_keys }}</NDescriptionsItem>
        <NDescriptionsItem :label="$t('page.setup.redisStatus')">
          <NTag :type="envData.redis_reachable ? 'success' : 'warning'" size="small">
            {{ envData.redis_reachable ? $t('page.setup.redisReachable') : $t('page.setup.redisUnavailable') }}
          </NTag>
          <span class="ml-8px text-12px text-gray">{{ envData.redis_addr }}</span>
        </NDescriptionsItem>
      </NDescriptions>
    </NSpin>

    <div class="flex justify-end gap-12px">
      <NButton :loading="loading" @click="checkEnv">{{ $t('common.refresh') }}</NButton>
      <NButton type="primary" :disabled="!envData || !!error" @click="handleNext">
        {{ $t('common.next') }}
      </NButton>
    </div>
  </div>
</template>
