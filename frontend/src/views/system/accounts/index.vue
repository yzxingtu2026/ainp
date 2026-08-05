<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns, FormInst, SelectOption } from 'naive-ui';
import { NButton, NPopconfirm, NSpace, NTag } from 'naive-ui';
import { REG_PWD, REG_SYSTEM_USERNAME } from '@/constants/reg';
import { ACTIVATION_STATUS } from '@/constants/codeset';
import { SYSTEM_ACTIONS } from '@/constants/system';
import { $t } from '@/locales';
import { formatDateTime } from '@/utils/datetime';
import { usePageContextProvider } from '@/composables/page-context';
import {
  createAccount,
  fetchRoleList,
  fetchAccountList,
  resetAccountPassword,
  updateAccount,
  updateAccountRoles,
  updateAccountStatus
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'SystemAccounts' });

const loading = ref(false);
const saving = ref(false);
const authStore = useAuthStore();
const records = ref<Api.Account.Record[]>([]);
const total = ref(0);
const roleOptions = ref<SelectOption[]>([]);
const query = reactive<Api.Account.ListParams>({ page: 1, page_size: 20, keyword: '', status: '', role_id: undefined });

const editorVisible = ref(false);
const rolesVisible = ref(false);
const passwordVisible = ref(false);
const editing = ref<Api.Account.Record | null>(null);
const formRef = ref<FormInst | null>(null);
const rolesFormRef = ref<FormInst | null>(null);
const passwordFormRef = ref<FormInst | null>(null);

const form = reactive({
  username: '',
  display_name: '',
  password: '',
  confirm_password: '',
  status: ACTIVATION_STATUS.enabled as Api.Account.ActivationStatus,
  role_ids: [] as string[]
});
const rolesForm = reactive({ role_ids: [] as string[] });
const passwordForm = reactive({ password: '', confirm_password: '' });

usePageContextProvider(() => {
  const editorMode = editorVisible.value ? (editing.value ? 'edit' : 'create') : null;
  const selected = editorMode === 'edit' ? editing.value : null;
  return {
    schema_version: '1.0',
    page_type: 'system.accounts',
    route_name: 'system_accounts',
    mode: editorMode || 'list',
    resource_refs: selected
      ? [{ resource_type: 'system.account', resource_id: selected.id, revision: selected.updated_at }]
      : undefined,
    ui_state:
      editorMode === null
        ? { keyword: query.keyword || '', status: query.status || '', role_id: query.role_id || '' }
        : undefined
  };
});

const statusOptions = computed(() => [
  { label: $t('page.account.enabled'), value: ACTIVATION_STATUS.enabled },
  { label: $t('page.account.disabled'), value: ACTIVATION_STATUS.disabled }
]);

function can(permission: string) {
  return authStore.userInfo.buttons.includes(permission) || authStore.userInfo.buttons.includes('*');
}

function isCurrentAccount(row: Api.Account.Record) {
  return row.id === authStore.userInfo.userId;
}

function roleNames(row: Api.Account.Record) {
  return row.roles.map(role => role.name || role.code).join(', ') || '-';
}

const columns = computed<DataTableColumns<Api.Account.Record>>(() => [
  { title: $t('page.account.username'), key: 'username', minWidth: 140 },
  { title: $t('page.account.displayName'), key: 'display_name', minWidth: 140 },
  {
    title: $t('page.account.status'),
    key: 'status',
    width: 100,
    render: row =>
      h(
        NTag,
        { type: row.status === ACTIVATION_STATUS.enabled ? 'success' : 'default' },
        {
          default: () => $t(`page.account.${row.status}`)
        }
      )
  },
  {
    title: $t('page.account.roles'),
    key: 'roles',
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: row => roleNames(row)
  },
  {
    title: $t('page.account.updatedAt'),
    key: 'updated_at',
    width: 160,
    render: row => formatDateTime(row.updated_at)
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 360,
    fixed: 'right',
    render: row =>
      h(NSpace, null, {
        default: () =>
          [
            can(SYSTEM_ACTIONS.accountUpdate)
              ? h(NButton, { size: 'small', onClick: () => openEditor(row) }, { default: () => $t('common.edit') })
              : null,
            can(SYSTEM_ACTIONS.accountRolesUpdate)
              ? h(
                  NButton,
                  { size: 'small', type: 'primary', ghost: true, onClick: () => openRoles(row) },
                  { default: () => $t('page.account.assignRoles') }
                )
              : null,
            can(SYSTEM_ACTIONS.accountPasswordReset)
              ? h(
                  NButton,
                  { size: 'small', onClick: () => openPassword(row) },
                  { default: () => $t('page.account.resetPassword') }
                )
              : null,
            can(SYSTEM_ACTIONS.accountStatusUpdate)
              ? h(
                  NPopconfirm,
                  {
                    onPositiveClick: () =>
                      handleStatus(
                        row,
                        row.status === ACTIVATION_STATUS.enabled
                          ? ACTIVATION_STATUS.disabled
                          : ACTIVATION_STATUS.enabled
                      )
                  },
                  {
                    trigger: () =>
                      h(
                        NButton,
                        {
                          size: 'small',
                          type: row.status === ACTIVATION_STATUS.enabled ? 'warning' : 'success',
                          disabled: isCurrentAccount(row) && row.status === ACTIVATION_STATUS.enabled
                        },
                        {
                          default: () =>
                            row.status === ACTIVATION_STATUS.enabled
                              ? $t('page.account.disable')
                              : $t('page.account.enable')
                        }
                      ),
                    default: () =>
                      row.status === ACTIVATION_STATUS.enabled
                        ? $t('page.account.confirmDisable')
                        : $t('page.account.confirmEnable')
                  }
                )
              : null
          ].filter(Boolean)
      })
  }
]);

async function loadRoleOptions() {
  const { data, error } = await fetchRoleList({ page: 1, page_size: 100, status: ACTIVATION_STATUS.enabled });
  if (!error && data) {
    roleOptions.value = data.items.map(role => ({
      label: role.is_builtin ? `${role.name} (${$t('page.account.builtin')})` : role.name,
      value: role.id
    }));
  }
}

async function loadAccounts() {
  loading.value = true;
  try {
    const { data, error } = await fetchAccountList(query);
    if (!error && data) {
      records.value = data.items;
      total.value = data.total;
    }
  } finally {
    loading.value = false;
  }
}

function resetQuery() {
  query.keyword = '';
  query.status = '';
  query.role_id = undefined;
  query.page = 1;
  loadAccounts();
}

function openEditor(account?: Api.Account.Record) {
  editing.value = account || null;
  form.username = account?.username || '';
  form.display_name = account?.display_name || '';
  form.password = '';
  form.confirm_password = '';
  form.status = account?.status || ACTIVATION_STATUS.enabled;
  form.role_ids = account?.roles.map(role => role.id) || [];
  editorVisible.value = true;
}

function openRoles(account: Api.Account.Record) {
  editing.value = account;
  rolesForm.role_ids = account.roles.map(role => role.id);
  rolesVisible.value = true;
}

function openPassword(account: Api.Account.Record) {
  editing.value = account;
  passwordForm.password = '';
  passwordForm.confirm_password = '';
  passwordVisible.value = true;
}

async function saveAccount() {
  await formRef.value?.validate();
  saving.value = true;
  try {
    const result = editing.value
      ? await updateAccount({ id: editing.value.id, display_name: form.display_name })
      : await createAccount({
          username: form.username,
          display_name: form.display_name,
          password: form.password,
          status: form.status,
          role_ids: form.role_ids
        });
    if (!result.error) {
      window.$message?.success(editing.value ? $t('common.updateSuccess') : $t('common.addSuccess'));
      form.password = '';
      form.confirm_password = '';
      editorVisible.value = false;
      await loadAccounts();
    }
  } finally {
    saving.value = false;
  }
}

async function saveRoles() {
  if (!editing.value) return;
  await rolesFormRef.value?.validate();
  saving.value = true;
  try {
    const { error } = await updateAccountRoles(editing.value.id, rolesForm.role_ids);
    if (!error) {
      window.$message?.success($t('common.updateSuccess'));
      rolesVisible.value = false;
      await loadAccounts();
    }
  } finally {
    saving.value = false;
  }
}

async function savePassword() {
  if (!editing.value) return;
  await passwordFormRef.value?.validate();
  saving.value = true;
  try {
    const { error } = await resetAccountPassword(editing.value.id, passwordForm.password);
    if (!error) {
      window.$message?.success($t('common.updateSuccess'));
      passwordForm.password = '';
      passwordForm.confirm_password = '';
      passwordVisible.value = false;
    }
  } finally {
    saving.value = false;
  }
}

async function handleStatus(account: Api.Account.Record, status: Api.Account.ActivationStatus) {
  const { error } = await updateAccountStatus(account.id, status);
  if (!error) {
    window.$message?.success($t('common.updateSuccess'));
    await loadAccounts();
  }
}

onMounted(async () => {
  await Promise.all([loadRoleOptions(), loadAccounts()]);
});
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('page.account.title')" :bordered="false" class="card-wrapper">
      <NSpace class="mb-16px" align="center">
        <NInput
          v-model:value="query.keyword"
          clearable
          :placeholder="$t('common.keywordSearch')"
          @keyup.enter="loadAccounts"
        />
        <AppSelect
          v-model:value="query.status"
          clearable
          class="w-160px"
          :options="statusOptions"
          :placeholder="$t('page.account.status')"
        />
        <AppSelect
          v-model:value="query.role_id"
          clearable
          class="w-200px"
          :options="roleOptions"
          :placeholder="$t('page.account.roles')"
        />
        <NButton
          type="primary"
          @click="
            query.page = 1;
            loadAccounts();
          "
        >
          {{ $t('common.search') }}
        </NButton>
        <NButton @click="resetQuery">{{ $t('common.reset') }}</NButton>
        <NButton v-if="can(SYSTEM_ACTIONS.accountCreate)" type="primary" secondary @click="openEditor()">
          {{ $t('common.add') }}
        </NButton>
      </NSpace>
      <NDataTable :columns="columns" :data="records" :loading="loading" :row-key="row => row.id" :scroll-x="1100" />
      <div class="mt-16px flex justify-end">
        <NPagination
          v-model:page="query.page"
          v-model:page-size="query.page_size"
          :item-count="total"
          show-size-picker
          :page-sizes="[10, 20, 50]"
          @update:page="loadAccounts"
          @update:page-size="
            query.page = 1;
            loadAccounts();
          "
        />
      </div>
    </NCard>

    <NModal
      v-model:show="editorVisible"
      preset="card"
      class="w-560px max-w-90vw"
      :title="editing ? $t('page.account.editTitle') : $t('page.account.createTitle')"
    >
      <NForm ref="formRef" :model="form" label-placement="left" label-width="100">
        <NFormItem
          :label="$t('page.account.username')"
          path="username"
          :rule="{ required: true, pattern: REG_SYSTEM_USERNAME, message: $t('page.account.usernameInvalid') }"
        >
          <NInput v-model:value="form.username" :disabled="Boolean(editing)" />
        </NFormItem>
        <NFormItem :label="$t('page.account.displayName')" path="display_name" :rule="{ required: true }">
          <NInput v-model:value="form.display_name" />
        </NFormItem>
        <template v-if="!editing">
          <NFormItem
            :label="$t('page.account.password')"
            path="password"
            :rule="{ required: true, pattern: REG_PWD, message: $t('page.account.passwordHint') }"
          >
            <NInput v-model:value="form.password" type="password" show-password-on="click" />
          </NFormItem>
          <NFormItem
            :label="$t('page.account.confirmPassword')"
            path="confirm_password"
            :rule="{
              required: true,
              validator: (_rule, value: string) => value === form.password,
              message: $t('page.account.passwordMismatch')
            }"
          >
            <NInput v-model:value="form.confirm_password" type="password" show-password-on="click" />
          </NFormItem>
          <NFormItem :label="$t('page.account.status')">
            <AppSelect v-model:value="form.status" :options="statusOptions" />
          </NFormItem>
          <NFormItem :label="$t('page.account.roles')" path="role_ids">
            <AppSelect v-model:value="form.role_ids" multiple filterable :options="roleOptions" />
          </NFormItem>
        </template>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="editorVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="saving" @click="saveAccount">{{ $t('common.confirm') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <NModal
      v-model:show="rolesVisible"
      preset="card"
      class="w-560px max-w-90vw"
      :title="$t('page.account.rolesTitle', { name: editing?.display_name || editing?.username || '' })"
    >
      <NForm ref="rolesFormRef" :model="rolesForm" label-placement="left" label-width="90">
        <NFormItem :label="$t('page.account.roles')" path="role_ids">
          <AppSelect v-model:value="rolesForm.role_ids" multiple filterable :options="roleOptions" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="rolesVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="saving" @click="saveRoles">{{ $t('common.confirm') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <NModal
      v-model:show="passwordVisible"
      preset="card"
      class="w-560px max-w-90vw"
      :title="$t('page.account.passwordTitle', { name: editing?.display_name || editing?.username || '' })"
    >
      <NForm ref="passwordFormRef" :model="passwordForm" label-placement="left" label-width="100">
        <NFormItem
          :label="$t('page.account.password')"
          path="password"
          :rule="{ required: true, pattern: REG_PWD, message: $t('page.account.passwordHint') }"
        >
          <NInput v-model:value="passwordForm.password" type="password" show-password-on="click" />
        </NFormItem>
        <NFormItem
          :label="$t('page.account.confirmPassword')"
          path="confirm_password"
          :rule="{
            required: true,
            validator: (_rule, value: string) => value === passwordForm.password,
            message: $t('page.account.passwordMismatch')
          }"
        >
          <NInput v-model:value="passwordForm.confirm_password" type="password" show-password-on="click" />
        </NFormItem>
        <div class="text-12px text-gray-500">{{ $t('page.account.passwordHint') }}</div>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="passwordVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="saving" @click="savePassword">{{ $t('common.confirm') }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
