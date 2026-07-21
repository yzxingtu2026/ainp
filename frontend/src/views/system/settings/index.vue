<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import type { FormInst, FormRules, MenuOption } from 'naive-ui';
import { $t } from '@/locales';
import { SYSTEM_ACTIONS } from '@/constants/system';
import SvgIcon from '@/components/custom/svg-icon.vue';
import RemoteSelect from '@/components/common/remote-select.vue';
import { fetchSettingList, resetSetting, updateSetting } from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';

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
const current = ref<Api.Setting.Definition | null>(null);

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
  return current.value.fields.some(field => !isSameValue(formModel[field.key], baseline.value[field.key]));
});

const showActions = computed(
  () =>
    isDirty.value &&
    Boolean(current.value) &&
    (can(SYSTEM_ACTIONS.settingUpdate) || can(SYSTEM_ACTIONS.settingReset))
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
    const value = definition.values[field.key];
    values[field.key] = value === undefined ? field.default : value;
  });
  return values;
}

function applyDefinition(definition: Api.Setting.Definition | null) {
  current.value = definition;
  Object.keys(formModel).forEach(key => delete formModel[key]);
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

async function handleSave() {
  if (!current.value || !can(SYSTEM_ACTIONS.settingUpdate)) return;
  await formRef.value?.validate();
  saving.value = true;
  try {
    const values: Record<string, unknown> = {};
    current.value.fields.forEach(field => {
      values[field.key] = formModel[field.key];
    });
    const { data, error } = await updateSetting(current.value.key, current.value.revision, values);
    if (!error && data) {
      window.$message?.success($t('common.updateSuccess'));
      activeDefinitionKey.value = data.key;
      await loadSettings();
    }
  } finally {
    saving.value = false;
  }
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
            <div v-if="activeCategory.description" class="text-13px text-gray-500">{{ activeCategory.description }}</div>
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
                  <NSelect
                    v-else-if="field.type === 'select'"
                    v-model:value="(formModel[field.key] as string)"
                    :options="selectOptions(field)"
                  />
                  <NSelect
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
