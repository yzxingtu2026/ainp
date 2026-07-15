<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import type { FormInst, FormRules, MenuOption } from 'naive-ui';
import { $t } from '@/locales';
import { SYSTEM_ACTIONS } from '@/constants/system';
import SvgIcon from '@/components/custom/svg-icon.vue';
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

const formRules = computed<FormRules>(() => {
  const rules: FormRules = {};
  const definition = current.value;
  if (!definition) return rules;
  definition.fields.forEach(field => {
    if (!field.required) return;
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

function applyDefinition(definition: Api.Setting.Definition | null) {
  current.value = definition;
  Object.keys(formModel).forEach(key => delete formModel[key]);
  if (!definition) return;
  definition.fields.forEach(field => {
    const value = definition.values[field.key];
    formModel[field.key] = value === undefined ? field.default : value;
  });
}

async function loadSettings() {
  loading.value = true;
  try {
    const { data, error } = await fetchSettingList();
    if (error || !data) {
      categories.value = [];
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
      category.definitions.find(item => item.key === activeDefinitionKey.value) || category.definitions[0];
    activeDefinitionKey.value = definition?.key || '';
    applyDefinition(definition || null);
  } finally {
    loading.value = false;
  }
}

function handleCategoryChange(key: string) {
  activeCategoryKey.value = key;
  const category = categories.value.find(item => item.key === key);
  activeDefinitionKey.value = category?.definitions[0]?.key || '';
  applyDefinition(category?.definitions[0] || null);
}

watch(activeDefinitionKey, key => {
  const category = activeCategory.value;
  const definition = category?.definitions.find(item => item.key === key) || null;
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
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('page.setting.title')" :bordered="false" class="card-wrapper" :loading="loading">
      <NEmpty v-if="!loading && categories.length === 0" :description="$t('page.setting.empty')" />
      <div v-else class="flex gap-16px lt-md:flex-col">
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
            <NAlert class="mb-16px" type="info" :bordered="false">
              {{ current.source === 'tenant' ? $t('page.setting.sourceTenant') : $t('page.setting.sourceDefault') }}
              · revision {{ current.revision }}
            </NAlert>
            <NForm ref="formRef" :model="formModel" :rules="formRules" label-placement="left" label-width="140">
              <NFormItem
                v-for="field in current.fields"
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
                    :options="selectOptions(field)"
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
            <NSpace>
              <NButton v-if="can(SYSTEM_ACTIONS.settingUpdate)" type="primary" :loading="saving" @click="handleSave">
                {{ $t('common.confirm') }}
              </NButton>
              <NPopconfirm v-if="can(SYSTEM_ACTIONS.settingReset)" @positive-click="handleReset">
                <template #trigger>
                  <NButton :disabled="current.source === 'default'" :loading="saving">
                    {{ $t('page.setting.reset') }}
                  </NButton>
                </template>
                {{ $t('page.setting.resetConfirm') }}
              </NPopconfirm>
            </NSpace>
          </template>
          <NEmpty v-else :description="$t('page.setting.empty')" />
        </div>
      </div>
    </NCard>
  </div>
</template>
