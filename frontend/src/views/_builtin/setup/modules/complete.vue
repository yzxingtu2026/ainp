<script setup lang="ts">
import { $t } from '@/locales';

interface Props {
  result: Api.Setup.InitResult | null;
  bgThemeColor: string;
}

defineProps<Props>();

function goLogin() {
  window.location.href = '/login';
}
</script>

<template>
  <div class="text-center">
    <NResult status="success" :title="$t('page.setup.completeTitle')" :description="$t('page.setup.completeDesc')">
      <template #footer>
        <div class="flex flex-col items-center gap-12px">
          <NAlert type="warning" :bordered="false" class="max-w-360px">
            {{ $t('page.setup.restartRequired') }}
          </NAlert>
          <NAlert :type="result?.jobs_enabled ? 'info' : 'warning'" :bordered="false" class="max-w-420px text-left">
            {{
              result?.jobs_enabled
                ? result.embedded_worker
                  ? $t('page.setup.jobsEmbeddedHint')
                  : $t('page.setup.jobsWorkerHint')
                : $t('page.setup.jobsDisabledHint')
            }}
          </NAlert>
          <NButton type="primary" size="large" @click="goLogin">
            {{ $t('page.setup.goLogin') }}
          </NButton>
        </div>
      </template>
    </NResult>
  </div>
</template>
