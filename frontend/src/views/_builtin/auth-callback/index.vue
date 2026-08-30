<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { authenticateRedirect } from '@/service/api';
import { getRequestErrorMessage } from '@/service/request';
import { useAuthStore } from '@/store/modules/auth';
import { isValidAuthMethod, safeInternalRedirect, takeRedirectAuthContext } from '@/utils/auth-redirect';
import { $t } from '@/locales';

defineOptions({ name: 'AuthCallback' });

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const status = ref<'loading' | 'error'>('loading');
const errorMessage = ref('');

function queryValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

async function clearSensitiveQuery() {
  await router.replace({ path: route.path, query: {} });
}

onMounted(async () => {
  const context = takeRedirectAuthContext();
  const callbackMethod = queryValue(route.query.method) || queryValue(route.query.provider);
  const method = callbackMethod || context?.method || '';
  const code = queryValue(route.query.code);
  const state = queryValue(route.query.state);
  const denied = queryValue(route.query.error);
  const redirect = safeInternalRedirect(context?.redirect);

  try {
    if (denied) throw new Error($t('page.login.redirect.providerRejected'));
    if (
      !isValidAuthMethod(method) ||
      !code ||
      !state ||
      (context && callbackMethod && callbackMethod !== context.method)
    )
      throw new Error($t('page.login.redirect.invalidCallback'));
    const { data, error } = await authenticateRedirect({ method, challengeId: context?.challengeId, code, state });
    if (error || !data) throw error || new Error($t('page.login.redirect.failed'));
    await clearSensitiveQuery();
    const loggedIn = await authStore.finishExternalLogin({
      token: data.access_token,
      refreshToken: data.refresh_token
    });
    if (!loggedIn) throw new Error($t('page.login.redirect.failed'));
    await router.replace(redirect || '/');
  } catch (error) {
    await clearSensitiveQuery();
    status.value = 'error';
    errorMessage.value =
      error instanceof Error && !('response' in error)
        ? error.message
        : getRequestErrorMessage(error, $t('page.login.redirect.failed'));
  }
});
</script>

<template>
  <div class="h-full min-h-400px flex-center p-24px">
    <NCard class="max-w-460px" :bordered="false">
      <NResult v-if="status === 'loading'" status="info" :title="$t('page.login.redirect.processing')">
        <template #icon><NSpin size="large" /></template>
      </NResult>
      <NResult v-else status="error" :title="$t('page.login.redirect.failed')" :description="errorMessage">
        <template #footer>
          <NButton type="primary" @click="router.replace('/login')">
            {{ $t('page.login.redirect.backToLogin') }}
          </NButton>
        </template>
      </NResult>
    </NCard>
  </div>
</template>
