<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns, FormInst, TreeOption } from 'naive-ui';
import { NButton, NPopconfirm, NSpace, NTag } from 'naive-ui';
import { $t } from '@/locales';
import { ACTIVATION_STATUS } from '@/constants/codeset';
import { SYSTEM_ACTIONS } from '@/constants/system';
import {
  createRole,
  deleteRole,
  fetchPermissionCatalog,
  fetchRole,
  fetchRoleList,
  updateRole,
  updateRolePermissions
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { formatDateTime } from '@/utils/datetime';
import { pageAfterDelete } from '@/hooks/common/table';
import { usePageContextProvider } from '@/composables/page-context';

defineOptions({ name: 'SystemRoles' });

const loading = ref(false);
const authStore = useAuthStore();
const saving = ref(false);
const records = ref<Api.Role.Record[]>([]);
const total = ref(0);
const query = reactive<Api.Role.ListParams>({ page: 1, page_size: 20, keyword: '', status: '' });
const editorVisible = ref(false);
const permissionsVisible = ref(false);
const editing = ref<Api.Role.Record | null>(null);
const formRef = ref<FormInst | null>(null);
const form = reactive({ code: '', name: '', description: '', status: ACTIVATION_STATUS.enabled as Api.Role.ActivationStatus });
const catalog = ref<Api.Role.PermissionGroup[]>([]);
const selectedPermissions = ref<string[]>([]);
const permissionKeyword = ref('');
const expandedPermissionNodes = ref<Array<string | number>>([]);
const isBuiltinAdmin = computed(() => editing.value?.is_builtin && editing.value.code === 'admin');

usePageContextProvider(() => {
  const editorMode = editorVisible.value ? (editing.value ? 'edit' : 'create') : null;
  const selected = editing.value && (editorMode === 'edit' || permissionsVisible.value) ? editing.value : null;
  return {
    schema_version: '1.0',
    page_type: 'system.roles',
    route_name: 'system_roles',
    mode: selected ? 'edit' : editorMode || 'list',
    resource_refs: selected
      ? [{ resource_type: 'system.role', resource_id: selected.id, revision: selected.updated_at }]
      : undefined,
    ui_state:
      !selected && editorMode === null ? { keyword: query.keyword || '', status: query.status || '' } : undefined
  };
});

const statusOptions = computed(() => [
  { label: $t('page.role.enabled'), value: ACTIVATION_STATUS.enabled },
  { label: $t('page.role.disabled'), value: ACTIVATION_STATUS.disabled }
]);

const permissionTreeData = computed<TreeOption[]>(() => {
  const keyword = permissionKeyword.value.trim().toLowerCase();
  return catalog.value.flatMap(capability => {
    const grouped = new Map<string, { name: string; items: Api.Role.PermissionItem[] }>();
    capability.items.forEach(item => {
      if (keyword && ![item.permission, item.name, item.description || '', item.group_name, ...item.action_ids].some(value => value.toLowerCase().includes(keyword))) {
        return;
      }
      const current = grouped.get(item.group) || { name: item.group_name || item.group, items: [] };
      current.items.push(item);
      grouped.set(item.group, current);
    });
    const children = Array.from(grouped.entries()).map(([groupKey, group]) => ({
      key: `group:${groupKey}`,
      label: group.name,
      nodeType: 'group',
      count: group.items.length,
      children: group.items.map(item => ({
        key: item.permission,
        label: item.name || item.permission,
        nodeType: 'permission',
        description: item.description,
        actionIds: item.action_ids,
        permission: item.permission
      }))
    }));
    if (children.length === 0) return [];
    return [{
      key: `capability:${capability.capability}`,
      label: capability.name || capability.capability,
      nodeType: 'capability',
      description: capability.description,
      count: children.reduce((sum, group) => sum + Number(group.count || 0), 0),
      children
    }];
  });
});

function can(permission: string) {
  return authStore.userInfo.buttons.includes(permission) || authStore.userInfo.buttons.includes('*');
}

function updatePermissionSelection(keys: Array<string | number>) {
  const assignable = new Set(catalog.value.flatMap(group => group.items.map(item => item.permission)));
  selectedPermissions.value = keys.map(String).filter(key => assignable.has(key)).sort();
}

function renderPermissionLabel({ option }: { option: TreeOption }) {
  if (option.nodeType !== 'permission') {
    return h('div', { class: option.nodeType === 'capability' ? 'font-700' : 'font-600' }, [
      String(option.label || option.key),
      h('span', { class: 'ml-8px text-12px font-normal text-gray-500' }, `(${String(option.count || 0)})`)
    ]);
  }
  const permission = String(option.permission || option.key);
  const actionIds = ((option.actionIds as string[] | undefined) || []).filter(actionId => actionId !== permission);
  return h('div', { class: 'py-4px' }, [
    h('div', { class: 'font-500' }, String(option.label || option.key)),
    h('div', { class: 'text-12px text-gray-500' }, [
      permission,
      actionIds.length ? ` · ${actionIds.join(', ')}` : '',
      option.description ? ` · ${String(option.description)}` : ''
    ])
  ]);
}

const columns = computed<DataTableColumns<Api.Role.Record>>(() => [
  { title: $t('page.role.code'), key: 'code', minWidth: 140 },
  { title: $t('page.role.name'), key: 'name', minWidth: 140 },
  {
    title: $t('page.role.status'),
    key: 'status',
    width: 100,
    render: row => h(NTag, { type: row.status === ACTIVATION_STATUS.enabled ? 'success' : 'default' }, { default: () => $t(`page.role.${row.status}`) })
  },
  {
    title: $t('page.role.type'),
    key: 'is_builtin',
    width: 110,
    render: row => row.is_builtin ? h(NTag, { type: 'warning' }, { default: () => $t('page.role.builtin') }) : $t('page.role.custom')
  },
  { title: $t('page.role.userCount'), key: 'user_count', width: 100 },
  { title: $t('page.role.description'), key: 'description', ellipsis: { tooltip: true }, minWidth: 180 },
  {
    title: $t('page.role.updatedAt'),
    key: 'updated_at',
    width: 160,
    render: row => formatDateTime(row.updated_at)
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 260,
    fixed: 'right',
    render: row => h(NSpace, null, {
      default: () => [
        can(SYSTEM_ACTIONS.roleUpdate) ? h(NButton, { size: 'small', onClick: () => openEditor(row) }, { default: () => $t('common.edit') }) : null,
        can(SYSTEM_ACTIONS.roleGet) && can(SYSTEM_ACTIONS.permissionCatalog) && can(SYSTEM_ACTIONS.rolePermissionsUpdate)
          ? h(NButton, { size: 'small', type: 'primary', ghost: true, onClick: () => openPermissions(row) }, { default: () => $t('page.role.permissions') })
          : null,
        can(SYSTEM_ACTIONS.roleDelete) ? h(NPopconfirm, { onPositiveClick: () => handleDelete(row) }, {
          trigger: () => h(NButton, { size: 'small', type: 'error', disabled: row.is_builtin || row.user_count > 0 }, { default: () => $t('common.delete') }),
          default: () => $t('common.confirmDelete')
        }) : null
      ].filter(Boolean)
    })
  }
]);

async function loadRoles() {
  loading.value = true;
  try {
    const { data, error } = await fetchRoleList(query);
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
  query.page = 1;
  loadRoles();
}

function openEditor(role?: Api.Role.Record) {
  editing.value = role || null;
  form.code = role?.code || '';
  form.name = role?.name || '';
  form.description = role?.description || '';
  form.status = role?.status || ACTIVATION_STATUS.enabled;
  editorVisible.value = true;
}

async function saveRole() {
  await formRef.value?.validate();
  saving.value = true;
  try {
    const result = editing.value
      ? await updateRole({ id: editing.value.id, name: form.name, description: form.description, status: form.status })
      : await createRole({ code: form.code, name: form.name, description: form.description, status: form.status, permissions: [] });
    if (!result.error) {
      window.$message?.success(editing.value ? $t('common.updateSuccess') : $t('common.addSuccess'));
      editorVisible.value = false;
      await loadRoles();
    }
  } finally {
    saving.value = false;
  }
}

async function openPermissions(role: Api.Role.Record) {
  editing.value = role;
  permissionsVisible.value = true;
  permissionKeyword.value = '';
  if (role.is_builtin && role.code === 'admin') {
    const roleResult = await fetchRole(role.id);
    if (!roleResult.error && roleResult.data) selectedPermissions.value = roleResult.data.permissions || [];
    return;
  }
  const [roleResult, catalogResult] = await Promise.all([fetchRole(role.id), fetchPermissionCatalog()]);
  if (!roleResult.error && roleResult.data) selectedPermissions.value = roleResult.data.permissions || [];
  if (!catalogResult.error && catalogResult.data) {
    catalog.value = catalogResult.data.groups;
    const firstCapability = catalog.value[0];
    const firstGroup = firstCapability?.items[0]?.group;
    expandedPermissionNodes.value = [
      ...(firstCapability ? [`capability:${firstCapability.capability}`] : []),
      ...(firstGroup ? [`group:${firstGroup}`] : [])
    ];
  }
}

async function savePermissions() {
  if (!editing.value) return;
  saving.value = true;
  try {
    const roleCode = editing.value.code;
    const affectsSelf = authStore.userInfo.roles.includes(roleCode);
    const { error } = await updateRolePermissions(editing.value.id, selectedPermissions.value);
    if (!error) {
      permissionsVisible.value = false;
      if (affectsSelf) {
        window.$dialog?.warning({
          title: $t('common.warning'),
          content: $t('page.role.reloginRequired'),
          positiveText: $t('page.role.reloginNow'),
          closable: false,
          maskClosable: false,
          closeOnEsc: false,
          onPositiveClick: async () => {
            await authStore.resetStore();
          }
        });
        return;
      }
      window.$message?.success($t('common.updateSuccess'));
      await loadRoles();
    }
  } finally {
    saving.value = false;
  }
}

async function handleDelete(role: Api.Role.Record) {
  const { error } = await deleteRole(role.id);
  if (!error) {
    window.$message?.success($t('common.deleteSuccess'));
    query.page = pageAfterDelete(query.page, records.value.length);
    await loadRoles();
  }
}

onMounted(loadRoles);
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('page.role.title')" :bordered="false" class="card-wrapper">
      <NSpace class="mb-16px" align="center">
        <NInput v-model:value="query.keyword" clearable :placeholder="$t('common.keywordSearch')" @keyup.enter="loadRoles" />
        <NSelect v-model:value="query.status" clearable class="w-160px" :options="statusOptions" :placeholder="$t('page.role.status')" />
        <NButton type="primary" @click="query.page = 1; loadRoles()">{{ $t('common.search') }}</NButton>
        <NButton @click="resetQuery">{{ $t('common.reset') }}</NButton>
        <NButton v-if="can(SYSTEM_ACTIONS.roleCreate)" type="primary" secondary @click="openEditor()">{{ $t('common.add') }}</NButton>
      </NSpace>
      <NDataTable :columns="columns" :data="records" :loading="loading" :row-key="row => row.id" :scroll-x="1100" />
      <div class="mt-16px flex justify-end">
        <NPagination v-model:page="query.page" v-model:page-size="query.page_size" :item-count="total" show-size-picker :page-sizes="[10, 20, 50]" @update:page="loadRoles" @update:page-size="query.page = 1; loadRoles()" />
      </div>
    </NCard>

    <NModal v-model:show="editorVisible" preset="card" class="w-560px max-w-90vw" :title="editing ? $t('page.role.editTitle') : $t('page.role.createTitle')">
      <NForm ref="formRef" :model="form" label-placement="left" label-width="90">
        <NFormItem :label="$t('page.role.code')" path="code" :rule="{ required: true, pattern: /^[a-z][a-z0-9_]{1,63}$/ }">
          <NInput v-model:value="form.code" :disabled="Boolean(editing)" />
        </NFormItem>
        <NFormItem :label="$t('page.role.name')" path="name" :rule="{ required: true }"><NInput v-model:value="form.name" /></NFormItem>
        <NFormItem :label="$t('page.role.description')"><NInput v-model:value="form.description" type="textarea" /></NFormItem>
        <NFormItem :label="$t('page.role.status')">
          <NSelect v-model:value="form.status" :options="statusOptions" :disabled="Boolean(editing?.is_builtin)" />
        </NFormItem>
      </NForm>
      <template #footer><NSpace justify="end"><NButton @click="editorVisible = false">{{ $t('common.cancel') }}</NButton><NButton type="primary" :loading="saving" @click="saveRole">{{ $t('common.confirm') }}</NButton></NSpace></template>
    </NModal>

    <NModal v-model:show="permissionsVisible" preset="card" class="w-760px max-w-95vw" :title="isBuiltinAdmin ? $t('page.role.superAdmin') : $t('page.role.permissionTitle', { name: editing?.name || '' })">
      <NAlert v-if="isBuiltinAdmin" type="info" :title="$t('page.role.superAdmin')">
        {{ $t('page.role.superAdminDescription') }}
      </NAlert>
      <template v-else>
        <NInput v-model:value="permissionKeyword" clearable class="mb-12px" :placeholder="$t('page.role.permissionSearch')" />
        <NScrollbar class="max-h-55vh">
          <NTree
            block-line
            checkable
            cascade
            check-strategy="child"
            :data="permissionTreeData"
            :checked-keys="selectedPermissions"
            :expanded-keys="expandedPermissionNodes"
            :render-label="renderPermissionLabel"
            @update:checked-keys="updatePermissionSelection"
            @update:expanded-keys="keys => expandedPermissionNodes = keys"
          />
        </NScrollbar>
      </template>
      <template #footer><NSpace justify="space-between"><span>{{ isBuiltinAdmin ? $t('page.role.allPermissions') : $t('page.role.selectedCount', { count: selectedPermissions.length }) }}</span><NSpace><NButton @click="permissionsVisible = false">{{ isBuiltinAdmin ? $t('common.close') : $t('common.cancel') }}</NButton><NButton v-if="!isBuiltinAdmin" type="primary" :loading="saving" @click="savePermissions">{{ $t('common.confirm') }}</NButton></NSpace></NSpace></template>
    </NModal>
  </div>
</template>
