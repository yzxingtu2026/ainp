<script setup lang="ts">
import { ref } from 'vue';
import { $t } from '@/locales';
import { fetchSetupInitialize } from '@/service/api';
import { resetSetupCache } from '@/router/guard/route';

interface Props {
  dsn: string;
  bgThemeColor: string;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (e: 'next'): void;
  (e: 'initialized', result: Api.Setup.InitResult): void;
}>();

const form = ref({
  tenantName: '',
  username: 'admin',
  password: '',
  confirmPassword: ''
});
const submitting = ref(false);
const error = ref('');

async function handleSubmit() {
  if (form.value.password !== form.value.confirmPassword) {
    error.value = $t('page.setup.passwordMismatch');
    return;
  }
  if (form.value.password.length < 6) {
    error.value = $t('page.setup.passwordTooShort');
    return;
  }

  submitting.value = true;
  error.value = '';
  try {
    const res = await fetchSetupInitialize({
      dsn: props.dsn || undefined,
      tenant_name: form.value.tenantName,
      username: form.value.username,
      password: form.value.password
    });
    if (res.error || !res.data) {
      error.value = res.error?.message || 'Initialization failed';
      return;
    }
    emit('initialized', res.data);
    resetSetupCache();
    emit('next');
  } catch (e: any) {
    error.value = e.message;
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div>
    <NAlert v-if="error" type="error" class="mb-16px">{{ error }}</NAlert>

    <NForm label-placement="left" label-width="120px">
      <NFormItem :label="$t('page.setup.tenantName')" required>
        <NInput v-model:value="form.tenantName" :placeholder="$t('page.setup.tenantNamePlaceholder')" />
      </NFormItem>
      <NFormItem :label="$t('page.setup.adminUsername')" required>
        <NInput v-model:value="form.username" :placeholder="$t('page.setup.adminUsernamePlaceholder')" />
      </NFormItem>
      <NFormItem :label="$t('page.setup.adminPassword')" required>
        <NInput
          v-model:value="form.password"
          type="password"
          show-password-on="click"
          :placeholder="$t('page.setup.adminPasswordPlaceholder')"
        />
      </NFormItem>
      <NFormItem :label="$t('page.setup.confirmPassword')" required>
        <NInput
          v-model:value="form.confirmPassword"
          type="password"
          show-password-on="click"
          :placeholder="$t('page.setup.confirmPasswordPlaceholder')"
        />
      </NFormItem>
    </NForm>

    <div class="flex justify-end">
      <NButton type="primary" :loading="submitting" @click="handleSubmit">
        {{ $t('page.setup.initialize') }}
      </NButton>
    </div>
  </div>
</template>
