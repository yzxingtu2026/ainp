<script setup lang="ts">
import { h, onMounted, reactive, ref, computed } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NCard, NDataTable, NDivider, NFormItem, NInput, NModal, NPopconfirm, NSelect, NSpace, NSwitch, NTab, NTag, NTabs } from 'naive-ui';
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
const activeChannel = ref('all');

/** Per-locale template data */
interface LocaleData {
  locale: string;
  default_subject: string;
  default_body: string;
  has_override: boolean;
  override: Api.Notification.TemplateOverrideInfo | null;
}

/** Merged row: one per (definition + channel) */
interface MergedRow {
  definition_key: string;
  name: string;
  description: string;
  category: string;
  priority: string;
  channel: string;
  locales: LocaleData[];
  variables: Api.Notification.VariableField[];
}

/** 从定义数据中提取实际使用的渠道列表 */
const availableChannels = computed<string[]>(() => {
  const channelSet = new Set<string>();
  for (const def of records.value) {
    for (const tpl of def.templates) {
      channelSet.add(tpl.channel);
    }
  }
  return Array.from(channelSet);
});

const mergedRows = computed<MergedRow[]>(() => {
  const rows: MergedRow[] = [];
  for (const def of records.value) {
    // Group templates by channel
    const channelMap = new Map<string, LocaleData[]>();
    for (const tpl of def.templates) {
      if (activeChannel.value !== 'all' && tpl.channel !== activeChannel.value) continue;
      const arr = channelMap.get(tpl.channel) || [];
      arr.push({
        locale: tpl.locale,
        default_subject: tpl.default_subject,
        default_body: tpl.default_body,
        has_override: tpl.has_override,
        override: tpl.override
      });
      channelMap.set(tpl.channel, arr);
    }
    for (const [channel, locales] of channelMap) {
      rows.push({
        definition_key: def.definition_key,
        name: def.name,
        description: def.description,
        category: def.category,
        priority: def.priority,
        channel,
        locales,
        variables: def.variables
      });
    }
  }
  return rows;
});

// ── Editor ──
const editorVisible = ref(false);
const editorSaving = ref(false);
const editorDefinitionKey = ref('');
const editorChannel = ref('');
const editorActiveLocale = ref('');
const editorVariables = ref<Api.Notification.VariableField[]>([]);

/** Per-locale edit form data, keyed by locale string */
interface LocaleFormData {
  subject: string;
  body: string;
  enabled: boolean;
  defaultSubject: string;
  defaultBody: string;
  hasOverride: boolean;
  dirty: boolean;
}
const editorLocales = ref<Record<string, LocaleFormData>>({});

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

// 值 → i18n key 映射
const priorityI18n: Record<string, string> = {
  low: 'notificationAdmin.priorityLow',
  normal: 'notificationAdmin.priorityNormal',
  high: 'notificationAdmin.priorityHigh',
  critical: 'notificationAdmin.priorityCritical'
};
const channelI18n: Record<string, string> = {
  in_app: 'notificationAdmin.channelInApp',
  email: 'notificationAdmin.channelEmail',
  sms: 'notificationAdmin.channelSms',
  webhook: 'notificationAdmin.channelWebhook',
  im: 'notificationAdmin.channelIm'
};
const categoryI18n: Record<string, string> = {
  user: 'notificationAdmin.categoryUser',
  security: 'notificationAdmin.categorySecurity',
  system: 'notificationAdmin.categorySystem'
};

/** Aggregate override status: true if any locale has override */
function anyOverride(row: MergedRow): boolean {
  return row.locales.some(l => l.has_override);
}

/** Latest override updated_at across locales */
function latestOverrideUpdated(row: MergedRow): string | null {
  let latest = '';
  for (const l of row.locales) {
    if (l.override && l.override.updated_at > latest) {
      latest = l.override.updated_at;
    }
  }
  return latest || null;
}

const columns = computed<DataTableColumns<MergedRow>>(() => [
  {
    title: $t('notificationAdmin.colDefinition'),
    key: 'definition_key',
    minWidth: 200,
    ellipsis: { tooltip: true },
    render: row => [
      h('div', { class: 'text-13px font-medium' }, row.name || row.definition_key),
      h('div', { class: 'text-12px opacity-60' }, $t((categoryI18n[row.category] || '') as any) || row.category)
    ]
  },
  {
    title: $t('notificationAdmin.colPriority'),
    key: 'priority',
    width: 100,
    render: row => h(NTag, { type: priorityTagType[row.priority] || 'default', size: 'small' }, { default: () => $t((priorityI18n[row.priority] || '') as any) || row.priority })
  },
  {
    title: $t('notificationAdmin.colLocale'),
    key: 'locales',
    width: 140,
    render: row => h(NSpace, { size: 4 }, {
      default: () => row.locales.map(l => h(NTag, { size: 'small', bordered: false }, { default: () => l.locale }))
    })
  },
  {
    title: $t('notificationAdmin.colDefaultSubject'),
    key: 'default_subject',
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: row => row.locales[0]?.default_subject || '—'
  },
  {
    title: $t('notificationAdmin.colOverride'),
    key: 'has_override',
    width: 110,
    render: row => {
      if (!anyOverride(row)) {
        return h(NTag, { size: 'small', type: 'default' }, { default: () => $t('notificationAdmin.noOverride') });
      }
      const enabledLocales = row.locales.filter(l => l.has_override && l.override?.enabled);
      return h(
        NTag,
        { size: 'small', type: enabledLocales.length > 0 ? 'success' : 'error' },
        { default: () => enabledLocales.length > 0 ? $t('notificationAdmin.overrideOn') : $t('notificationAdmin.overrideOff') }
      );
    }
  },
  {
    title: $t('notificationAdmin.colOverrideUpdated'),
    key: 'override_updated',
    width: 170,
    render: row => {
      const ts = latestOverrideUpdated(row);
      return ts ? formatDateTime(ts) : '—';
    }
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 240,
    fixed: 'right',
    render: row => h(NSpace, null, {
      default: () => [
        h(NButton, { size: 'small', onClick: () => openEditor(row) }, { default: () => $t('common.edit') }),
        anyOverride(row)
          ? h(
              NPopconfirm,
              { onPositiveClick: () => handleResetAll(row) },
              {
                trigger: () => h(NButton, { size: 'small', type: 'warning' }, { default: () => $t('common.reset') }),
                default: () => $t('notificationAdmin.resetConfirm')
              }
            )
          : null,
        h(NButton, { size: 'small', type: 'info', ghost: true, onClick: () => openPreview(row) }, { default: () => $t('notificationAdmin.preview') })
      ].filter(Boolean)
    })
  }
]);

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

function openEditor(row: MergedRow) {
  editorDefinitionKey.value = row.definition_key;
  editorChannel.value = row.channel;
  editorVariables.value = row.variables;
  editorActiveLocale.value = row.locales[0]?.locale || '';

  const formMap: Record<string, LocaleFormData> = {};
  for (const l of row.locales) {
    formMap[l.locale] = {
      subject: l.override ? l.override.subject : l.default_subject,
      body: l.override ? l.override.body : l.default_body,
      enabled: l.override ? l.override.enabled : true,
      defaultSubject: l.default_subject,
      defaultBody: l.default_body,
      hasOverride: l.has_override,
      dirty: false
    };
  }
  editorLocales.value = formMap;
  editorVisible.value = true;
}

async function handleSave() {
  editorSaving.value = true;
  try {
    // Save each locale that is dirty
    const promises: Promise<any>[] = [];
    for (const [locale, data] of Object.entries(editorLocales.value)) {
      if (!data.dirty) continue;
      promises.push(
        fetchUpdateNotificationTemplate({
          definition_key: editorDefinitionKey.value,
          channel: editorChannel.value,
          locale,
          subject: data.subject,
          body: data.body,
          enabled: data.enabled
        })
      );
    }
    if (promises.length === 0) {
      editorVisible.value = false;
      return;
    }
    const results = await Promise.all(promises);
    const hasError = results.some(r => r.error);
    if (!hasError) {
      window.$message?.success($t('common.modifySuccess'));
      editorVisible.value = false;
      await loadList();
    }
  } finally {
    editorSaving.value = false;
  }
}

async function handleResetAll(row: MergedRow) {
  // Reset all locales that have override
  const promises = row.locales
    .filter(l => l.has_override)
    .map(l =>
      fetchResetNotificationTemplate({
        definition_key: row.definition_key,
        channel: row.channel,
        locale: l.locale
      })
    );
  const results = await Promise.all(promises);
  if (!results.some(r => r.error)) {
    window.$message?.success($t('common.modifySuccess'));
    await loadList();
  }
}

function openPreview(row: MergedRow) {
  previewForm.definition_key = row.definition_key;
  previewForm.channel = row.channel;
  previewForm.locale = row.locales[0]?.locale || '';
  previewForm.variables = '{}';
  previewData.value = null;
  previewVisible.value = true;
}

/** Locale options for preview selector */
const previewLocaleOptions = computed(() => {
  const row = mergedRows.value.find(r => r.definition_key === previewForm.definition_key && r.channel === previewForm.channel);
  if (!row) return [];
  return row.locales.map(l => ({ label: l.locale, value: l.locale }));
});

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
      <NTabs v-model:value="activeChannel" type="line" class="mb-12px">
        <NTab name="all">{{ $t('notificationAdmin.tabAll') }}</NTab>
        <NTab v-for="ch in availableChannels" :key="ch" :name="ch">
          {{ $t((channelI18n[ch] || ch) as any) || ch }}
        </NTab>
      </NTabs>
      <NDataTable :columns="columns" :data="mergedRows" :loading="loading" :row-key="row => `${row.definition_key}:${row.channel}`" :scroll-x="1100" />
    </NCard>

    <!-- Edit modal with locale tabs -->
    <NModal v-model:show="editorVisible" preset="card" class="w-640px max-w-90vw" :title="$t('notificationAdmin.editTemplate')">
      <div class="mb-12px flex items-center gap-8px text-13px opacity-70">
        <span>{{ $t('notificationAdmin.formDefinitionKey') }}: <strong>{{ editorDefinitionKey }}</strong></span>
        <span>·</span>
        <span>{{ $t('notificationAdmin.colChannel') }}: <strong>{{ $t((channelI18n[editorChannel] || editorChannel) as any) || editorChannel }}</strong></span>
      </div>
      <NTabs v-model:value="editorActiveLocale" type="bar" animated>
        <template v-for="(formData, locale) in editorLocales" :key="locale">
          <NTab :name="String(locale)">
            <NFormItem :label="$t('notificationAdmin.formSubject')">
              <NInput
                :value="formData.subject"
                type="text"
                @update:value="(v: string) => { formData.subject = v; formData.dirty = true }"
              />
            </NFormItem>
            <NFormItem :label="$t('notificationAdmin.formBody')">
              <NInput
                :value="formData.body"
                type="textarea"
                :rows="6"
                @update:value="(v: string) => { formData.body = v; formData.dirty = true }"
              />
            </NFormItem>
            <NFormItem :label="$t('notificationAdmin.formEnabled')">
              <NSwitch
                :value="formData.enabled"
                @update:value="(v: boolean) => { formData.enabled = v; formData.dirty = true }"
              />
            </NFormItem>
          </NTab>
        </template>
      </NTabs>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="editorVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="editorSaving" @click="handleSave">{{ $t('common.confirm') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Preview modal with locale selector -->
    <NModal v-model:show="previewVisible" preset="card" class="w-640px max-w-90vw" :title="$t('notificationAdmin.previewTitle')">
      <div class="mb-12px flex items-center gap-8px text-13px opacity-70">
        <span>{{ $t('notificationAdmin.formDefinitionKey') }}: <strong>{{ previewForm.definition_key }}</strong></span>
        <span>·</span>
        <span>{{ $t('notificationAdmin.colChannel') }}: <strong>{{ $t((channelI18n[previewForm.channel] || previewForm.channel) as any) || previewForm.channel }}</strong></span>
      </div>
      <NFormItem :label="$t('notificationAdmin.colLocale')">
        <NSelect v-model:value="previewForm.locale" :options="previewLocaleOptions" class="w-200px" />
      </NFormItem>
      <NFormItem :label="$t('notificationAdmin.formVariables')">
        <NInput v-model:value="previewForm.variables" type="textarea" :rows="3" />
      </NFormItem>
      <NButton type="primary" :loading="previewLoading" @click="handlePreview">
        {{ $t('notificationAdmin.render') }}
      </NButton>

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
    </NModal>
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
