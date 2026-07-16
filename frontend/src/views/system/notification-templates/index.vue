<script setup lang="ts">
import { h, onMounted, reactive, ref, computed } from 'vue';
import type { DataTableColumns, FormInst } from 'naive-ui';
import { NButton, NCard, NDataTable, NDivider, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NPopconfirm, NSpace, NSwitch, NTag } from 'naive-ui';
import { $t } from '@/locales';
import {
  fetchNotificationTemplates,
  fetchPreviewNotificationTemplate,
  fetchResetNotificationTemplate,
  fetchUpdateNotificationTemplate
} from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'SystemNotificationTemplates' });

const loading = ref(false);
const records = ref<Api.Notification.TemplateDefinitionItem[]>([]);

/** Flattened row: one per template variant (channel+locale) */
interface FlatRow {
  definition_key: string;
  category: string;
  name_key: string;
  priority: string;
  allowed_channels: string[];
  variables: Api.Notification.VariableField[];
  channel: string;
  locale: string;
  default_subject: string;
  default_body: string;
  has_override: boolean;
  override: Api.Notification.TemplateOverrideInfo | null;
}

const flatRows = computed<FlatRow[]>(() => {
  const rows: FlatRow[] = [];
  for (const def of records.value) {
    for (const tpl of def.templates) {
      rows.push({
        definition_key: def.definition_key,
        category: def.category,
        name_key: def.name_key,
        priority: def.priority,
        allowed_channels: def.allowed_channels,
        variables: def.variables,
        channel: tpl.channel,
        locale: tpl.locale,
        default_subject: tpl.default_subject,
        default_body: tpl.default_body,
        has_override: tpl.has_override,
        override: tpl.override
      });
    }
  }
  return rows;
});

// ── Editor ──
const editorVisible = ref(false);
const editorSaving = ref(false);
const formRef = ref<FormInst | null>(null);
const editForm = reactive({
  definition_key: '',
  channel: '',
  locale: '',
  subject: '',
  body: '',
  enabled: true
});

// ── Preview ──
const previewVisible = ref(false);
const previewLoading = ref(false);
const previewData = ref<Api.Notification.PreviewData | null>(null);
const previewForm = reactive({
  definition_key: '',
  channel: '',
  locale: '',
  variables: '{}'
});

const priorityTagType: Record<string, 'default' | 'info' | 'success' | 'warning' | 'error'> = {
  low: 'default',
  normal: 'info',
  high: 'warning',
  critical: 'error'
};

const columns: DataTableColumns<FlatRow> = [
  {
    title: $t('notificationAdmin.colDefinition'),
    key: 'definition_key',
    width: 220,
    ellipsis: { tooltip: true },
    render: row => [
      h('div', { class: 'font-mono text-13px' }, row.definition_key),
      h('div', { class: 'text-12px opacity-60' }, row.category)
    ]
  },
  {
    title: $t('notificationAdmin.colPriority'),
    key: 'priority',
    width: 80,
    render: row => h(NTag, { type: priorityTagType[row.priority] || 'default', size: 'small' }, { default: () => row.priority })
  },
  {
    title: $t('notificationAdmin.colChannel'),
    key: 'channel',
    width: 90,
    render: row => h(NTag, { size: 'small', bordered: false }, { default: () => row.channel })
  },
  { title: $t('notificationAdmin.colLocale'), key: 'locale', width: 80 },
  {
    title: $t('notificationAdmin.colDefaultSubject'),
    key: 'default_subject',
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: row => row.default_subject || '—'
  },
  {
    title: $t('notificationAdmin.colOverride'),
    key: 'has_override',
    width: 100,
    render: row => {
      if (!row.has_override) {
        return h(NTag, { size: 'small', type: 'default' }, { default: () => $t('notificationAdmin.noOverride') });
      }
      const enabled = row.override?.enabled;
      return h(
        NTag,
        { size: 'small', type: enabled ? 'success' : 'error' },
        { default: () => (enabled ? $t('notificationAdmin.overrideOn') : $t('notificationAdmin.overrideOff')) }
      );
    }
  },
  {
    title: $t('notificationAdmin.colOverrideUpdated'),
    key: 'override_updated',
    width: 170,
    render: row => (row.override ? formatDateTime(row.override.updated_at) : '—')
  },
  {
    title: $t('notificationAdmin.colActions'),
    key: 'actions',
    width: 220,
    render: row => [
      h(NButton, { text: true, type: 'primary', size: 'small', onClick: () => openEditor(row) }, { default: () => $t('common.edit') }),
      row.has_override
        ? h(
            NPopconfirm,
            { onPositiveClick: () => handleReset(row) },
            {
              trigger: () => h(NButton, { text: true, type: 'warning', size: 'small', style: 'margin-left:8px' }, { default: () => $t('common.reset') }),
              default: () => $t('notificationAdmin.resetConfirm')
            }
          )
        : null,
      h(
        NButton,
        { text: true, type: 'info', size: 'small', style: 'margin-left:8px', onClick: () => openPreview(row) },
        { default: () => $t('notificationAdmin.preview') }
      )
    ]
  }
];

async function loadList() {
  loading.value = true;
  try {
    const res = await fetchNotificationTemplates();
    if (!res.error && res.data) {
      records.value = res.data.items || [];
    }
  } finally {
    loading.value = false;
  }
}

function openEditor(row: FlatRow) {
  editForm.definition_key = row.definition_key;
  editForm.channel = row.channel;
  editForm.locale = row.locale;
  // 如果有覆盖，加载覆盖内容；否则加载默认内容作为编辑起点
  if (row.override) {
    editForm.subject = row.override.subject;
    editForm.body = row.override.body;
    editForm.enabled = row.override.enabled;
  } else {
    editForm.subject = row.default_subject;
    editForm.body = row.default_body;
    editForm.enabled = true;
  }
  editorVisible.value = true;
}

async function handleSave() {
  await formRef.value?.validate();
  editorSaving.value = true;
  try {
    const res = await fetchUpdateNotificationTemplate(editForm);
    if (!res.error) {
      window.$message?.success($t('common.modifySuccess'));
      editorVisible.value = false;
      await loadList();
    }
  } finally {
    editorSaving.value = false;
  }
}

async function handleReset(row: FlatRow) {
  const res = await fetchResetNotificationTemplate({
    definition_key: row.definition_key,
    channel: row.channel,
    locale: row.locale
  });
  if (!res.error) {
    window.$message?.success($t('common.modifySuccess'));
    await loadList();
  }
}

function openPreview(row: FlatRow) {
  previewForm.definition_key = row.definition_key;
  previewForm.channel = row.channel;
  previewForm.locale = row.locale;
  previewForm.variables = '{}';
  previewData.value = null;
  previewVisible.value = true;
}

async function handlePreview() {
  previewLoading.value = true;
  try {
    let variables: Record<string, any> = {};
    try {
      variables = JSON.parse(previewForm.variables);
    } catch {
      window.$message?.error($t('notificationAdmin.invalidJson'));
      return;
    }
    const res = await fetchPreviewNotificationTemplate({
      definition_key: previewForm.definition_key,
      channel: previewForm.channel,
      locale: previewForm.locale,
      variables
    });
    if (!res.error && res.data) {
      previewData.value = res.data;
    }
  } finally {
    previewLoading.value = false;
  }
}

onMounted(loadList);
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('notificationAdmin.templateTitle')" :bordered="false" class="card-wrapper">
      <template #header-extra>
        <NButton type="primary" size="small" @click="loadList">{{ $t('common.refresh') }}</NButton>
      </template>
      <NDataTable
        :columns="columns"
        :data="flatRows"
        :loading="loading"
        :bordered="false"
        :pagination="false"
        size="small"
      />
    </NCard>

    <!-- Edit drawer -->
    <NDrawer v-model:show="editorVisible" :width="520">
      <NDrawerContent :title="$t('notificationAdmin.editTemplate')" closable>
        <NForm ref="formRef" :model="editForm" label-placement="top">
          <NFormItem :label="$t('notificationAdmin.formDefinitionKey')">
            <NInput :value="editForm.definition_key" disabled />
          </NFormItem>
          <NSpace>
            <NFormItem :label="$t('notificationAdmin.colChannel')">
              <NInput :value="editForm.channel" disabled style="width: 140px" />
            </NFormItem>
            <NFormItem :label="$t('notificationAdmin.colLocale')">
              <NInput :value="editForm.locale" disabled style="width: 140px" />
            </NFormItem>
          </NSpace>
          <NFormItem :label="$t('notificationAdmin.formSubject')">
            <NInput v-model:value="editForm.subject" type="text" />
          </NFormItem>
          <NFormItem :label="$t('notificationAdmin.formBody')">
            <NInput v-model:value="editForm.body" type="textarea" :rows="8" />
          </NFormItem>
          <NFormItem :label="$t('notificationAdmin.formEnabled')">
            <NSwitch v-model:value="editForm.enabled" />
          </NFormItem>
        </NForm>
        <template #footer>
          <NSpace>
            <NButton @click="editorVisible = false">{{ $t('common.cancel') }}</NButton>
            <NButton type="primary" :loading="editorSaving" @click="handleSave">{{ $t('common.confirm') }}</NButton>
          </NSpace>
        </template>
      </NDrawerContent>
    </NDrawer>

    <!-- Preview drawer -->
    <NDrawer v-model:show="previewVisible" :width="600">
      <NDrawerContent :title="$t('notificationAdmin.previewTitle')" closable>
        <NForm label-placement="top">
          <NSpace>
            <NFormItem :label="$t('notificationAdmin.colChannel')">
              <NInput :value="previewForm.channel" disabled style="width: 140px" />
            </NFormItem>
            <NFormItem :label="$t('notificationAdmin.colLocale')">
              <NInput :value="previewForm.locale" disabled style="width: 140px" />
            </NFormItem>
          </NSpace>
          <NFormItem :label="$t('notificationAdmin.formVariables')">
            <NInput v-model:value="previewForm.variables" type="textarea" :rows="4" />
          </NFormItem>
          <NButton type="primary" :loading="previewLoading" @click="handlePreview">
            {{ $t('notificationAdmin.render') }}
          </NButton>
        </NForm>

        <div v-if="previewData" class="mt-16px">
          <NDivider>{{ $t('notificationAdmin.renderResult') }}</NDivider>
          <div class="preview-section">
            <div class="preview-label">{{ $t('notificationAdmin.formSource') }}: {{ previewData.source }} (v{{ previewData.version }})</div>
            <div class="preview-label">{{ $t('notificationAdmin.formSubject') }}:</div>
            <pre class="preview-block">{{ previewData.subject }}</pre>
            <div class="preview-label">{{ $t('notificationAdmin.formBody') }}:</div>
            <pre class="preview-block">{{ previewData.body }}</pre>
            <template v-if="previewData.title">
              <div class="preview-label">{{ $t('notificationAdmin.formTitle') }}:</div>
              <pre class="preview-block">{{ previewData.title }}</pre>
            </template>
            <template v-if="previewData.summary">
              <div class="preview-label">{{ $t('notificationAdmin.formSummary') }}:</div>
              <pre class="preview-block">{{ previewData.summary }}</pre>
            </template>
          </div>
        </div>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
.preview-section {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.preview-label {
  font-size: 12px;
  color: var(--n-text-color-3);
  margin-top: 8px;
}

.preview-block {
  background: rgba(0, 0, 0, 0.04);
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
</style>
