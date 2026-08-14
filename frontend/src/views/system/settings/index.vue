<script setup lang="tsx">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import type { FormInst, FormRules, MenuOption } from 'naive-ui';
import { $t } from '@/locales';
import { SYSTEM_ACTIONS } from '@/constants/system';
import SvgIcon from '@/components/custom/svg-icon.vue';
import RemoteSelect from '@/components/common/remote-select.vue';
import {
  fetchNavigationIndexStatus,
  fetchSettingList,
  fetchToolIndexStatus,
  rebuildNavigationIndex,
  rebuildToolIndex,
  resetSetting,
  updateSetting,
  updateSettingSecret
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { buildSettingUpdatePayload } from './secret-state';

defineOptions({ name: 'SystemSettings' });

const authStore = useAuthStore();
const loading = ref(false);
const saving = ref(false);
const categories = ref<Api.Setting.Category[]>([]);
const activeCategoryKey = ref('');
const activeDefinitionKey = ref('');
const formRef = ref<FormInst | null>(null);
const formModel = reactive<Record<string, unknown>>({});
const baseline = ref<Record<string, unknown>>({});
const secretInputs = reactive<Record<string, string>>({});
const clearedSecretFields = reactive(new Set<string>());
const current = ref<Api.Setting.Definition | null>(null);
const semanticIndexStatus = ref<Api.Setting.NavigationIndexStatus | null>(null);
const indexStatusLoading = ref(false);
const rebuildingIndex = ref(false);
let indexPollTimer: ReturnType<typeof setTimeout> | undefined;
let indexRequestSequence = 0;

const isNavigationSearchSetting = computed(() => current.value?.key === 'ai.navigation_search');
const isToolSearchSetting = computed(() => current.value?.key === 'ai.tool_search');
const isSemanticSearchSetting = computed(() => isNavigationSearchSetting.value || isToolSearchSetting.value);
const indexTranslationKey = computed(() =>
  isToolSearchSetting.value ? 'page.setting.toolIndex' : 'page.setting.navigationIndex'
);
const navigationStateType = computed(() => {
  const state = semanticIndexStatus.value?.state;
  if (state === 'ready') return 'success';
  if (state === 'failed') return 'error';
  if (state === 'pending' || state === 'degraded') return 'warning';
  return 'default';
});

function clearIndexPoll() {
  if (indexPollTimer) clearTimeout(indexPollTimer);
  indexPollTimer = undefined;
}

async function loadNavigationIndexStatus(scheduleNext = true) {
  clearIndexPoll();
  if (!isSemanticSearchSetting.value) {
    semanticIndexStatus.value = null;
    return;
  }
  const sequence = ++indexRequestSequence;
  indexStatusLoading.value = true;
  try {
    const request = isToolSearchSetting.value ? fetchToolIndexStatus : fetchNavigationIndexStatus;
    const { data, error } = await request();
    if (sequence !== indexRequestSequence || error || !data) return;
    semanticIndexStatus.value = data;
    if (scheduleNext && data.state === 'pending') indexPollTimer = setTimeout(loadNavigationIndexStatus, 2000);
  } finally {
    if (sequence === indexRequestSequence) indexStatusLoading.value = false;
  }
}

async function handleRebuildNavigationIndex() {
  if (!can(SYSTEM_ACTIONS.settingUpdate)) return;
  rebuildingIndex.value = true;
  try {
    const request = isToolSearchSetting.value ? rebuildToolIndex : rebuildNavigationIndex;
    const { data, error } = await request();
    if (!error && data) {
      semanticIndexStatus.value = data;
      window.$message?.success($t(`${indexTranslationKey.value}.rebuildQueued`));
      loadNavigationIndexStatus();
    }
  } finally {
    rebuildingIndex.value = false;
  }
}

function can(permission: string) {
  return authStore.userInfo.buttons.includes(permission) || authStore.userInfo.buttons.includes('*');
}

const activeCategory = computed(() => categories.value.find(item => item.key === activeCategoryKey.value) || null);

const categoryMenuOptions = computed<MenuOption[]>(() =>
  categories.value.map(category => ({
    label: category.title,
    key: category.key,
    icon: () => h(SvgIcon, { icon: category.icon, localIcon: category.local_icon })
  }))
);

const isDirty = computed(() => {
  if (!current.value) return false;
  return current.value.fields.some(field => {
    if (field.type === 'secret') return Boolean(secretInputs[field.key]) || clearedSecretFields.has(field.key);
    return !isSameValue(formModel[field.key], baseline.value[field.key]);
  });
});

const showActions = computed(
  () =>
    isDirty.value && Boolean(current.value) && (can(SYSTEM_ACTIONS.settingUpdate) || can(SYSTEM_ACTIONS.settingReset))
);

const formRules = computed<FormRules>(() => {
  const rules: FormRules = {};
  const definition = current.value;
  if (!definition) return rules;
  definition.fields.forEach(field => {
    if (!field.required || !isFieldVisible(field.key)) return;
    rules[field.key] = [
      {
        required: true,
        trigger: ['blur', 'change'],
        message: $t('page.setting.required'),
        validator: (_rule, value) => {
          if (field.type === 'secret') return field.secret_configured || Boolean(secretInputs[field.key]);
          if (field.type === 'boolean') return true;
          if (field.type === 'multi_select') return Array.isArray(value) && value.length > 0;
          return !(value === null || value === undefined || value === '');
        }
      }
    ];
  });
  return rules;
});

function valuesEqual(left: unknown, right: unknown) {
  return JSON.stringify(normalizeValue(left)) === JSON.stringify(normalizeValue(right));
}

function resolveFieldValue(fieldKey: string) {
  if (Object.prototype.hasOwnProperty.call(formModel, fieldKey)) {
    return formModel[fieldKey];
  }
  const field = current.value?.fields.find(item => item.key === fieldKey);
  return field?.default ?? null;
}

function isFieldVisible(fieldKey: string, visiting: Set<string> = new Set()): boolean {
  const definition = current.value;
  if (!definition) return false;
  const field = definition.fields.find(item => item.key === fieldKey);
  if (!field) return false;
  const when = field.visible_when;
  if (!when) return true;
  if (visiting.has(fieldKey)) return false;
  visiting.add(fieldKey);
  if (!isFieldVisible(when.field, visiting)) return false;
  const actual = resolveFieldValue(when.field);
  if (when.eq !== undefined) {
    return valuesEqual(actual, when.eq);
  }
  if (Array.isArray(when.in) && when.in.length > 0) {
    return when.in.some(candidate => valuesEqual(actual, candidate));
  }
  return false;
}

function isSameValue(left: unknown, right: unknown) {
  return JSON.stringify(normalizeValue(left)) === JSON.stringify(normalizeValue(right));
}

function normalizeValue(value: unknown) {
  if (Array.isArray(value)) return [...value].map(String).sort();
  return value ?? null;
}

function cloneValues(values: Record<string, unknown>) {
  return JSON.parse(JSON.stringify(values)) as Record<string, unknown>;
}

function snapshotValues(definition: Api.Setting.Definition) {
  const values: Record<string, unknown> = {};
  definition.fields.forEach(field => {
    if (field.type === 'secret') return;
    const value = definition.values[field.key];
    values[field.key] = value === undefined ? field.default : value;
  });
  return values;
}

function applyDefinition(definition: Api.Setting.Definition | null) {
  current.value = definition;
  Object.keys(formModel).forEach(key => Reflect.deleteProperty(formModel, key));
  Object.keys(secretInputs).forEach(key => Reflect.deleteProperty(secretInputs, key));
  clearedSecretFields.clear();
  if (!definition) {
    baseline.value = {};
    return;
  }
  const values = snapshotValues(definition);
  baseline.value = cloneValues(values);
  Object.assign(formModel, cloneValues(values));
}

async function loadSettings() {
  loading.value = true;
  try {
    const { data, error } = await fetchSettingList();
    if (error || !data) {
      categories.value = [];
      applyDefinition(null);
      return;
    }
    categories.value = data.categories || [];
    const category = categories.value.find(item => item.key === activeCategoryKey.value) || categories.value[0];
    if (!category) {
      activeCategoryKey.value = '';
      activeDefinitionKey.value = '';
      applyDefinition(null);
      return;
    }
    activeCategoryKey.value = category.key;
    const definition =
      category.definitions.find(item => item.key === activeDefinitionKey.value) || category.definitions[0] || null;
    activeDefinitionKey.value = definition?.key || '';
    applyDefinition(definition);
  } finally {
    loading.value = false;
  }
}

function handleCategoryChange(key: string) {
  activeCategoryKey.value = key;
  const category = categories.value.find(item => item.key === key);
  const definition = category?.definitions[0] || null;
  activeDefinitionKey.value = definition?.key || '';
  applyDefinition(definition);
}

watch(activeDefinitionKey, key => {
  if (!key) return;
  const category = activeCategory.value;
  const definition = category?.definitions.find(item => item.key === key) || null;
  if (definition?.key === current.value?.key) return;
  applyDefinition(definition);
});

watch(
  () => current.value?.key,
  () => loadNavigationIndexStatus()
);

async function handleSave() {
  if (!current.value || !can(SYSTEM_ACTIONS.settingUpdate)) return;
  await formRef.value?.validate();
  saving.value = true;
  try {
    const payload = buildSettingUpdatePayload(current.value, formModel, secretInputs, clearedSecretFields);
    const request = payload.hasSecretFields
      ? updateSettingSecret(
          current.value.key,
          current.value.revision,
          payload.values,
          payload.secrets,
          payload.clearSecretFields
        )
      : updateSetting(current.value.key, current.value.revision, payload.values);
    const { data, error } = await request;
    if (!error && data) {
      window.$message?.success($t('common.updateSuccess'));
      activeDefinitionKey.value = data.key;
      await loadSettings();
    }
  } finally {
    saving.value = false;
  }
}

function clearSecret(field: Api.Setting.Field) {
  secretInputs[field.key] = '';
  clearedSecretFields.add(field.key);
}

function restoreSecret(field: Api.Setting.Field) {
  clearedSecretFields.delete(field.key);
}

async function handleReset() {
  if (!current.value || !can(SYSTEM_ACTIONS.settingReset)) return;
  saving.value = true;
  try {
    const { data, error } = await resetSetting(current.value.key, current.value.revision);
    if (!error && data) {
      window.$message?.success($t('page.setting.resetSuccess'));
      activeDefinitionKey.value = data.key;
      await loadSettings();
    }
  } finally {
    saving.value = false;
  }
}

function selectOptions(field: Api.Setting.Field) {
  return (field.options || []).map(option => ({ label: option.label || option.value, value: option.value }));
}

onMounted(loadSettings);
onBeforeUnmount(clearIndexPoll);
</script>

<template>
  <div class="h-full min-h-0 flex-col-stretch gap-16px overflow-x-hidden overflow-y-auto">
    <NCard :title="$t('page.setting.title')" :bordered="false" class="card-wrapper" :loading="loading">
      <template #header-extra>
        <NSpace v-if="showActions">
          <NButton v-if="can(SYSTEM_ACTIONS.settingUpdate)" type="primary" :loading="saving" @click="handleSave">
            {{ $t('common.confirm') }}
          </NButton>
          <NPopconfirm v-if="can(SYSTEM_ACTIONS.settingReset)" @positive-click="handleReset">
            <template #trigger>
              <NButton :loading="saving">{{ $t('page.setting.reset') }}</NButton>
            </template>
            {{ $t('page.setting.resetConfirm') }}
          </NPopconfirm>
        </NSpace>
      </template>

      <NEmpty v-if="!loading && categories.length === 0" :description="$t('page.setting.empty')" />
      <div v-else-if="categories.length > 0" class="flex gap-16px lt-md:flex-col">
        <div class="w-220px shrink-0 lt-md:w-full">
          <NMenu :value="activeCategoryKey" :options="categoryMenuOptions" @update:value="handleCategoryChange" />
        </div>
        <div class="min-w-0 flex-1">
          <NSpace v-if="activeCategory" class="mb-12px" vertical :size="8">
            <div class="text-16px font-600">{{ activeCategory.title }}</div>
            <div v-if="activeCategory.description" class="text-13px text-gray-500">
              {{ activeCategory.description }}
            </div>
            <NTabs
              v-if="activeCategory.definitions.length > 1"
              type="line"
              :value="activeDefinitionKey"
              @update:value="value => (activeDefinitionKey = String(value))"
            >
              <NTab v-for="definition in activeCategory.definitions" :key="definition.key" :name="definition.key">
                {{ definition.title }}
              </NTab>
            </NTabs>
          </NSpace>

          <template v-if="current">
            <NAlert v-if="isSemanticSearchSetting" class="mb-16px" :show-icon="false">
              <div class="flex flex-wrap items-center justify-between gap-12px">
                <div class="flex flex-wrap items-center gap-8px">
                  <span class="font-500">{{ $t(`${indexTranslationKey}.title`) }}</span>
                  <NTag :type="navigationStateType" size="small">
                    {{ $t(`${indexTranslationKey}.states.${semanticIndexStatus?.state || 'not_indexed'}`) }}
                  </NTag>
                  <span v-if="semanticIndexStatus" class="text-12px text-gray-500">
                    {{ $t(`${indexTranslationKey}.summary`, semanticIndexStatus) }}
                  </span>
                </div>
                <NButton
                  size="small"
                  :loading="rebuildingIndex || indexStatusLoading"
                  :disabled="!can(SYSTEM_ACTIONS.settingUpdate) || isDirty"
                  @click="handleRebuildNavigationIndex"
                >
                  {{ $t(`${indexTranslationKey}.rebuild`) }}
                </NButton>
              </div>
            </NAlert>
            <NForm ref="formRef" :model="formModel" :rules="formRules" label-placement="left" label-width="140">
              <NFormItem
                v-for="field in current.fields.filter(item => isFieldVisible(item.key))"
                :key="field.key"
                :label="field.label"
                :path="field.key"
              >
                <div class="w-full">
                  <NInput
                    v-if="field.type === 'text'"
                    v-model:value="(formModel[field.key] as string)"
                    :maxlength="field.max_length"
                  />
                  <NInput
                    v-else-if="field.type === 'textarea'"
                    v-model:value="(formModel[field.key] as string)"
                    type="textarea"
                    :maxlength="field.max_length"
                  />
                  <NInputNumber
                    v-else-if="field.type === 'number'"
                    v-model:value="(formModel[field.key] as number)"
                    class="w-full"
                    :min="field.min"
                    :max="field.max"
                    :step="field.step || 1"
                  />
                  <NSwitch v-else-if="field.type === 'boolean'" v-model:value="(formModel[field.key] as boolean)" />
                  <AppSelect
                    v-else-if="field.type === 'select'"
                    v-model:value="(formModel[field.key] as string)"
                    :options="selectOptions(field)"
                  />
                  <AppSelect
                    v-else-if="field.type === 'multi_select'"
                    v-model:value="(formModel[field.key] as string[])"
                    multiple
                    filterable
                    :tag="field.allow_custom_values"
                    :options="selectOptions(field)"
                  />
                  <RemoteSelect
                    v-else-if="field.type === 'remote_select' && field.remote_source"
                    v-model:value="(formModel[field.key] as string)"
                    :source="field.remote_source"
                  />
                  <NColorPicker
                    v-else-if="field.type === 'color'"
                    v-model:value="(formModel[field.key] as string)"
                    :show-alpha="false"
                  />
                  <div v-else-if="field.type === 'secret'" class="flex-col-stretch gap-8px">
                    <NInput
                      v-model:value="secretInputs[field.key]"
                      type="password"
                      show-password-on="click"
                      :maxlength="field.max_length"
                      autocomplete="new-password"
                      :placeholder="
                        $t(
                          field.secret_configured ? 'page.setting.secretConfigured' : 'page.setting.secretNotConfigured'
                        )
                      "
                      @update:value="restoreSecret(field)"
                    />
                    <div class="flex items-center justify-between gap-8px text-12px text-gray-500">
                      <span>
                        {{
                          $t(
                            field.secret_configured ? 'page.setting.secretKeepHint' : 'page.setting.secretNotConfigured'
                          )
                        }}
                      </span>
                      <NPopconfirm
                        v-if="field.secret_configured && !field.required && !clearedSecretFields.has(field.key)"
                        @positive-click="clearSecret(field)"
                      >
                        <template #trigger>
                          <NButton text type="error">{{ $t('page.setting.secretClear') }}</NButton>
                        </template>
                        {{ $t('page.setting.secretClearConfirm') }}
                      </NPopconfirm>
                      <NButton v-else-if="clearedSecretFields.has(field.key)" text @click="restoreSecret(field)">
                        {{ $t('common.cancel') }}
                      </NButton>
                    </div>
                  </div>
                  <div v-if="field.description" class="mt-4px text-12px text-gray-500">{{ field.description }}</div>
                </div>
              </NFormItem>
            </NForm>
          </template>
          <NEmpty v-else-if="!loading" :description="$t('page.setting.empty')" />
        </div>
      </div>
    </NCard>
  </div>
</template>
