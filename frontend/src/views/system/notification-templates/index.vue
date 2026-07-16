<script setup lang="ts">
import { h, onMounted, reactive, ref } from 'vue';
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
const records = ref<Api.Notification.TemplateOverride[]>([]);

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

const previewVisible = ref(false);
const previewLoading = ref(false);
const previewData = ref<Api.Notification.PreviewData | null>(null);
const previewForm = reactive({
  definition_key: '',
  channel: '',
  locale: '',
  variables: '{}'
});


const columns: DataTableColumns<Api.Notification.TemplateOverride> = [
  { title: 'Definition Key', key: 'definition_key', width: 200, ellipsis: { tooltip: true } },
  {
    title: 'Channel',
    key: 'channel',
    width: 100,
    render: row => row.channel
  },
  { title: 'Locale', key: 'locale', width: 90 },
  { title: 'Subject', key: 'subject', minWidth: 200, ellipsis: { tooltip: true } },
  {
    title: 'Enabled',
    key: 'enabled',
    width: 90,
    render: row => h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small' }, { default: () => (row.enabled ? '✓' : '✗') })
  },
  {
    title: 'Updated At',
    key: 'updated_at',
    width: 170,
    render: row => formatDateTime(row.updated_at)
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 200,
    render: row => [
      h(NButton, { text: true, type: 'primary', size: 'small', onClick: () => openEditor(row) }, { default: () => $t('common.edit') }),
      h(
        NPopconfirm,
        { onPositiveClick: () => handleReset(row) },
        {
          trigger: () => h(NButton, { text: true, type: 'warning', size: 'small', style: 'margin-left:8px' }, { default: () => $t('common.reset') }),
          default: () => $t('notificationAdmin.resetConfirm')
        }
      ),
      h(NButton, { text: true, type: 'info', size: 'small', style: 'margin-left:8px', onClick: () => openPreview(row) }, { default: () => $t('notificationAdmin.preview') })
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

function openEditor(row: Api.Notification.TemplateOverride) {
  editForm.definition_key = row.definition_key;
  editForm.channel = row.channel;
  editForm.locale = row.locale;
  editForm.subject = row.subject;
  editForm.body = row.body;
  editForm.enabled = row.enabled;
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

async function handleReset(row: Api.Notification.TemplateOverride) {
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

function openPreview(row: Api.Notification.TemplateOverride) {
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
        :data="records"
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
          <NFormItem label="Definition Key">
            <NInput :value="editForm.definition_key" disabled />
          </NFormItem>
          <NSpace>
            <NFormItem label="Channel">
              <NInput :value="editForm.channel" disabled style="width: 140px" />
            </NFormItem>
            <NFormItem label="Locale">
              <NInput :value="editForm.locale" disabled style="width: 140px" />
            </NFormItem>
          </NSpace>
          <NFormItem label="Subject">
            <NInput v-model:value="editForm.subject" type="text" />
          </NFormItem>
          <NFormItem label="Body">
            <NInput v-model:value="editForm.body" type="textarea" :rows="8" />
          </NFormItem>
          <NFormItem label="Enabled">
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
            <NFormItem label="Channel">
              <NInput :value="previewForm.channel" disabled style="width: 140px" />
            </NFormItem>
            <NFormItem label="Locale">
              <NInput :value="previewForm.locale" disabled style="width: 140px" />
            </NFormItem>
          </NSpace>
          <NFormItem label="Variables (JSON)">
            <NInput v-model:value="previewForm.variables" type="textarea" :rows="4" />
          </NFormItem>
          <NButton type="primary" :loading="previewLoading" @click="handlePreview">
            {{ $t('notificationAdmin.render') }}
          </NButton>
        </NForm>

        <div v-if="previewData" class="mt-16px">
          <NDivider>{{ $t('notificationAdmin.renderResult') }}</NDivider>
          <div class="preview-section">
            <div class="preview-label">Source: {{ previewData.source }} (v{{ previewData.version }})</div>
            <div class="preview-label">Subject:</div>
            <pre class="preview-block">{{ previewData.subject }}</pre>
            <div class="preview-label">Body:</div>
            <pre class="preview-block">{{ previewData.body }}</pre>
            <template v-if="previewData.title">
              <div class="preview-label">Title:</div>
              <pre class="preview-block">{{ previewData.title }}</pre>
            </template>
            <template v-if="previewData.summary">
              <div class="preview-label">Summary:</div>
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
  background: var(--n-color-target);
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}
</style>
