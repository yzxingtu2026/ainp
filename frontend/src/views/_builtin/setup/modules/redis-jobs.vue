<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { fetchSetupTestRedis } from '@/service/api';

const props = defineProps<{ defaultAddr: string }>();
const emit = defineEmits<{
  (e: 'ready', options: Api.Setup.RedisOptions): void;
  (e: 'next'): void;
}>();

const jobsEnabled = ref(false);
const embeddedWorker = ref(false);
const addr = ref(props.defaultAddr);
const db = ref(0);
const testing = ref(false);
const testResult = ref<Api.Setup.TestDBResult | null>(null);

const canProceed = computed(() => !jobsEnabled.value || testResult.value?.ok === true);

async function testRedis() {
  testing.value = true;
  testResult.value = null;
  const res = await fetchSetupTestRedis(addr.value, db.value);
  testResult.value = res.data || { ok: false, error: res.error?.message || $t('page.setup.redisConnectFail') };
  testing.value = false;
}

function handleNext() {
  emit('ready', {
    addr: addr.value,
    db: db.value,
    jobs_enabled: jobsEnabled.value,
    embedded_worker: jobsEnabled.value && embeddedWorker.value
  });
  emit('next');
}
</script>

<template>
  <div>
    <NForm label-placement="left" label-width="140px">
      <NFormItem :label="$t('page.setup.enableJobs')">
        <NSwitch v-model:value="jobsEnabled" @update:value="testResult = null" />
      </NFormItem>
      <template v-if="jobsEnabled">
        <NFormItem :label="$t('page.setup.redisAddr')">
          <NInput v-model:value="addr" placeholder="127.0.0.1:6379" @update:value="testResult = null" />
        </NFormItem>
        <NFormItem :label="$t('page.setup.redisDb')">
          <NInputNumber v-model:value="db" :min="0" :max="15" @update:value="testResult = null" />
        </NFormItem>
        <NFormItem :label="$t('page.setup.embeddedWorker')">
          <NSwitch v-model:value="embeddedWorker" />
        </NFormItem>
        <NAlert type="info" class="mb-16px">{{ $t('page.setup.redisPasswordEnv') }}</NAlert>
        <NAlert v-if="testResult" :type="testResult.ok ? 'success' : 'error'" class="mb-16px">
          {{ testResult.ok ? $t('page.setup.redisConnectOk') : `${$t('page.setup.redisConnectFail')}: ${testResult.error}` }}
        </NAlert>
      </template>
      <NAlert v-else type="warning" class="mb-16px">{{ $t('page.setup.jobsDisabledHint') }}</NAlert>
    </NForm>
    <div class="flex justify-end gap-12px">
      <NButton v-if="jobsEnabled" :loading="testing" @click="testRedis">{{ $t('page.setup.testRedis') }}</NButton>
      <NButton type="primary" :disabled="!canProceed" @click="handleNext">{{ $t('common.next') }}</NButton>
    </div>
  </div>
</template>
