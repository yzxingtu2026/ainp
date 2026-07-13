<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { fetchSetupTestDB } from '@/service/api';

interface Props {
  envData: Api.Setup.CheckEnv | null;
  bgThemeColor: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'next'): void;
}>();

const dsn = ref('');
const testing = ref(false);
const testResult = ref<{ ok: boolean; error?: string } | null>(null);

const dsnFromEnv = computed(() => props.envData?.dsn_present ?? false);

async function testDB() {
  testing.value = true;
  testResult.value = null;
  try {
    const res = await fetchSetupTestDB(dsn.value || undefined);
    if (res.error || !res.data) {
      testResult.value = { ok: false, error: res.error?.message || 'Request failed' };
      return;
    }
    testResult.value = res.data;
  } catch (e: any) {
    testResult.value = { ok: false, error: e.message };
  } finally {
    testing.value = false;
  }
}

function handleNext() {
  emit('next');
}

const canProceed = computed(() => testResult.value?.ok === true);
</script>

<template>
  <div>
    <NAlert v-if="dsnFromEnv" type="success" class="mb-16px">
      {{ $t('page.setup.dsnFromEnv') }}
    </NAlert>

    <NForm label-placement="left" label-width="80px" class="mb-16px">
      <NFormItem :label="$t('page.setup.dsn')" :show-feedback="false">
        <NInput
          v-model:value="dsn"
          :placeholder="$t('page.setup.dsnPlaceholder')"
          :disabled="dsnFromEnv"
          type="textarea"
          :rows="2"
        />
      </NFormItem>
    </NForm>

    <NAlert v-if="testResult && !testResult.ok" type="error" class="mb-16px">
      {{ $t('page.setup.dbConnectFail') }}: {{ testResult.error }}
    </NAlert>
    <NAlert v-if="testResult && testResult.ok" type="success" class="mb-16px">
      {{ $t('page.setup.dbConnectOk') }}
    </NAlert>

    <div class="flex justify-end gap-12px">
      <NButton :loading="testing" @click="testDB">{{ $t('page.setup.testConnection') }}</NButton>
      <NButton type="primary" :disabled="!canProceed" @click="handleNext">
        {{ $t('common.next') }}
      </NButton>
    </div>
  </div>
</template>
