<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { fetchSetupTestDB } from '@/service/api';
import { getRequestErrorMessage } from '@/service/request';

interface Props {
  envData: Api.Setup.CheckEnv | null;
  bgThemeColor: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'next'): void;
  (e: 'dsn-ready', dsn: string): void;
}>();

const form = ref({
  host: '127.0.0.1',
  port: '5432',
  dbname: 'ainp',
  user: '',
  password: ''
});
const testing = ref(false);
const testResult = ref<{ ok: boolean; error?: string } | null>(null);

const dsnFromEnv = computed(() => props.envData?.dsn_present ?? false);

/** Build PostgreSQL DSN string from individual fields */
function buildDSN(): string {
  const parts: string[] = [];
  if (form.value.host) parts.push(`host=${form.value.host}`);
  if (form.value.user) parts.push(`user=${form.value.user}`);
  if (form.value.password) parts.push(`password=${form.value.password}`);
  if (form.value.dbname) parts.push(`dbname=${form.value.dbname}`);
  if (form.value.port) parts.push(`port=${form.value.port}`);
  parts.push('sslmode=disable');
  parts.push('TimeZone=UTC');
  return parts.join(' ');
}

async function testDB() {
  testing.value = true;
  testResult.value = null;
  try {
    const dsn = dsnFromEnv.value ? undefined : buildDSN();
    const res = await fetchSetupTestDB(dsn);
    if (res.error || !res.data) {
      testResult.value = { ok: false, error: getRequestErrorMessage(res.error, $t('request.failed')) };
      return;
    }
    testResult.value = res.data;
    if (res.data.ok) {
      emit('dsn-ready', dsn || '');
    }
  } catch {
    testResult.value = { ok: false, error: $t('request.failed') };
  } finally {
    testing.value = false;
  }
}

function handleNext() {
  const dsn = dsnFromEnv.value ? '' : buildDSN();
  emit('dsn-ready', dsn);
  emit('next');
}

const canProceed = computed(() => testResult.value?.ok === true);
</script>

<template>
  <div>
    <NAlert v-if="dsnFromEnv" type="success" class="mb-16px">
      {{ $t('page.setup.dsnFromEnv') }}
    </NAlert>

    <NForm label-placement="left" label-width="100px" class="mb-16px" :style="{ '--n-feedback-padding-bottom': '18px' }">
      <NFormItem :label="$t('page.setup.dbHost')" :show-feedback="false" class="!mb-16px">
        <NInput
          v-model:value="form.host"
          :placeholder="$t('page.setup.dbHostPlaceholder')"
          :disabled="dsnFromEnv"
        />
      </NFormItem>
      <NFormItem :label="$t('page.setup.dbPort')" :show-feedback="false" class="!mb-16px">
        <NInput
          v-model:value="form.port"
          :placeholder="$t('page.setup.dbPortPlaceholder')"
          :disabled="dsnFromEnv"
        />
      </NFormItem>
      <NFormItem :label="$t('page.setup.dbName')" :show-feedback="false" class="!mb-16px">
        <NInput
          v-model:value="form.dbname"
          :placeholder="$t('page.setup.dbNamePlaceholder')"
          :disabled="dsnFromEnv"
        />
      </NFormItem>
      <NFormItem :label="$t('page.setup.dbUser')" :show-feedback="false" class="!mb-16px">
        <NInput
          v-model:value="form.user"
          :placeholder="$t('page.setup.dbUserPlaceholder')"
          :disabled="dsnFromEnv"
        />
      </NFormItem>
      <NFormItem :label="$t('page.setup.dbPassword')" :show-feedback="false" class="!mb-16px">
        <NInput
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          :placeholder="$t('page.setup.dbPasswordPlaceholder')"
          :disabled="dsnFromEnv"
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
