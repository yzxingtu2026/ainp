<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { loginModuleRecord } from '@/constants/app';
import { useAuthStore } from '@/store/modules/auth';
import { useRouterPush } from '@/hooks/common/router';
import { useFormRules, useNaiveForm } from '@/hooks/common/form';
import { $t } from '@/locales';
import { createAuthChallenge, fetchAuthMethods } from '@/service/api';
import {
  normalizeRedirectMethods,
  safeAuthorizeURL,
  safeInternalRedirect,
  saveRedirectAuthContext
} from '@/utils/auth-redirect';
import { useRoute } from 'vue-router';

defineOptions({
  name: 'PwdLogin'
});

const authStore = useAuthStore();
const route = useRoute();
const { toggleLoginModule } = useRouterPush();
const { formRef, validate } = useNaiveForm();
const enabledMethods = ref(new Set<string>(['password']));
const redirectMethods = ref<Api.Auth.AuthMethod[]>([]);
const redirectLoading = ref('');
const showDemoAccount = import.meta.env.DEV;

onMounted(async () => {
  const { data, error } = await fetchAuthMethods();
  if (!error && data) {
    enabledMethods.value = new Set(data.methods.map(item => item.method));
    redirectMethods.value = normalizeRedirectMethods(data.methods);
  }
});

function methodLabel(item: Api.Auth.AuthMethod) {
  const { label, label_key: labelKey } = item.capabilities;
  if (labelKey) {
    const translated = $t(labelKey as App.I18n.I18nKey);
    if (translated !== labelKey) return translated;
  }
  return label?.trim() || item.method;
}

async function handleRedirectLogin(item: Api.Auth.AuthMethod) {
  redirectLoading.value = item.method;
  const { data, error } = await createAuthChallenge(item.method);
  redirectLoading.value = '';
  if (error || !data) return;
  const authorizeURL = safeAuthorizeURL(data.data?.authorize_url);
  if (!authorizeURL || !data.challenge_id) {
    window.$message?.error($t('page.login.redirect.invalidAuthorizeUrl'));
    return;
  }
  saveRedirectAuthContext({
    method: item.method,
    challengeId: data.challenge_id,
    redirect: safeInternalRedirect(route.query.redirect)
  });
  window.location.assign(authorizeURL);
}

interface FormModel {
  userName: string;
  password: string;
}

const model: FormModel = reactive({
  userName: showDemoAccount ? 'admin' : '',
  password: showDemoAccount ? 'Admin@123' : ''
});

const rules = computed<Record<keyof FormModel, App.Global.FormRule[]>>(() => {
  // AINP 密码可含特殊字符；登录页仅校验必填，不套模板默认 REG_PWD
  const { formRules, createRequiredRule } = useFormRules();

  return {
    userName: formRules.userName,
    password: [createRequiredRule($t('form.pwd.required'))]
  };
});

async function handleSubmit() {
  await validate();
  await authStore.login(model.userName, model.password);
}

type AccountKey = 'admin';

interface Account {
  key: AccountKey;
  label: string;
  userName: string;
  password: string;
}

const accounts = computed<Account[]>(() => [
  {
    key: 'admin',
    label: '演示管理员',
    userName: 'admin',
    password: 'Admin@123'
  }
]);

async function handleAccountLogin(account: Account) {
  await authStore.login(account.userName, account.password);
}
</script>

<template>
  <NForm ref="formRef" :model="model" :rules="rules" size="large" :show-label="false" @keyup.enter="handleSubmit">
    <NFormItem path="userName">
      <NInput v-model:value="model.userName" :placeholder="$t('page.login.common.userNamePlaceholder')" />
    </NFormItem>
    <NFormItem path="password">
      <NInput
        v-model:value="model.password"
        type="password"
        show-password-on="click"
        :placeholder="$t('page.login.common.passwordPlaceholder')"
      />
    </NFormItem>
    <NSpace vertical :size="24">
      <div class="flex-y-center justify-between">
        <NCheckbox>{{ $t('page.login.pwdLogin.rememberMe') }}</NCheckbox>
        <NButton quaternary @click="toggleLoginModule('reset-pwd')">
          {{ $t('page.login.pwdLogin.forgetPassword') }}
        </NButton>
      </div>
      <NButton type="primary" size="large" block :loading="authStore.loginLoading" @click="handleSubmit">
        {{ $t('common.confirm') }}
      </NButton>
      <div class="flex-y-center justify-between gap-12px">
        <NButton
          v-if="enabledMethods.has('sms_otp') || enabledMethods.has('email_otp')"
          class="flex-1"
          block
          @click="toggleLoginModule('code-login')"
        >
          {{ $t(loginModuleRecord['code-login']) }}
        </NButton>
      </div>
      <template v-if="redirectMethods.length">
        <NDivider class="text-14px text-#666 !m-0">{{ $t('page.login.redirect.divider') }}</NDivider>
        <div class="grid grid-cols-1 gap-12px sm:grid-cols-2">
          <NButton
            v-for="item in redirectMethods"
            :key="item.method"
            block
            :loading="redirectLoading === item.method"
            :disabled="Boolean(redirectLoading)"
            @click="handleRedirectLogin(item)"
          >
            <template v-if="item.capabilities.icon" #icon>
              <SvgIcon :icon="item.capabilities.icon" />
            </template>
            {{ methodLabel(item) }}
          </NButton>
        </div>
      </template>
      <NDivider v-if="showDemoAccount" class="text-14px text-#666 !m-0">
        {{ $t('page.login.pwdLogin.otherAccountLogin') }}
      </NDivider>
      <div v-if="showDemoAccount" class="flex-center gap-12px">
        <NButton v-for="item in accounts" :key="item.key" type="primary" @click="handleAccountLogin(item)">
          {{ item.label }}
        </NButton>
      </div>
    </NSpace>
  </NForm>
</template>

<style scoped></style>
