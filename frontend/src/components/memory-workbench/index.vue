<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { clearMemories, correctMemory, fetchUserPreferences, updateUserPreferences } from '@/service/api';
import { useAppStore } from '@/store/modules/app';
import { useMemoryManagementStore } from '@/store/modules/memory-management';
import { formatDateTime } from '@/utils/datetime';

const route = useRoute();
const router = useRouter();
const appStore = useAppStore();
const store = useMemoryManagementStore();
const view = ref(String(route.query.view || 'profile'));
const detailOpen = ref(false);
const correctionOpen = ref(false);
const clearOpen = ref(false);
const clearConfirm = ref('');
const preference = ref<Api.Setting.Definition | null>(null);
const autoMemoryEnabled = ref(true);
const preferenceSaving = ref(false);
const correction = reactive({
  summary: '',
  content: '',
  local_date: null as string | null,
  date_precision: 'exact_day'
});

const pending = computed(() => store.rows.filter(item => item.status === 'pending_review'));
const timeline = computed(() => {
  const groups = new Map<string, Api.Memory.Item[]>();
  for (const item of store.rows.filter(row => ['active', 'superseded'].includes(row.status))) {
    const key = item.local_date?.slice(0, 10) || '日期未知';
    groups.set(key, [...(groups.get(key) || []), item]);
  }
  return [...groups.entries()];
});
const profileEntries = computed(() => Object.entries(store.profile?.profile || {}));
const drawerWidth = computed(() => (appStore.isMobile ? '100%' : 560));
const statusLabel: Record<string, string> = {
  active: '当前有效',
  pending_review: '待确认',
  superseded: '历史版本',
  revoked: '已删除',
  deleted: '已清除',
  rejected: '已拒绝',
  candidate: '候选',
  expired: '已过期'
};

watch(view, async value => {
  await router.replace({ query: { ...route.query, view: value } });
  store.filters.page = 1;
  store.filters.status = value === 'pending' ? 'pending_review' : null;
  await store.load();
});

async function loadPreference() {
  const { data } = await fetchUserPreferences();
  if (data) {
    preference.value = data;
    autoMemoryEnabled.value = Boolean(data.values.auto_memory_enabled);
  }
}

async function saveAutoMemory(value: boolean) {
  if (!preference.value) return;
  preferenceSaving.value = true;
  const values = { ...preference.value.values, auto_memory_enabled: value };
  try {
    const { data, error } = await updateUserPreferences(preference.value.revision, values);
    if (error) {
      autoMemoryEnabled.value = !value;
      return;
    }
    if (data) preference.value = data;
    window.$message?.success(value ? '自动记忆已开启' : '自动记忆已关闭');
  } finally {
    preferenceSaving.value = false;
  }
}

async function show(item: Api.Memory.Item) {
  await store.open(item.id);
  detailOpen.value = true;
}

function startCorrection() {
  if (!store.detail) return;
  Object.assign(correction, {
    summary: store.detail.summary,
    content: store.detail.content,
    local_date: store.detail.local_date?.slice(0, 10) || null,
    date_precision: store.detail.date_precision
  });
  correctionOpen.value = true;
}

async function saveCorrection() {
  if (!store.detail) return;
  const { error } = await correctMemory({
    id: store.detail.id,
    revision: store.detail.revision,
    ...correction,
    local_date: correction.local_date || ''
  });
  if (!error) {
    correctionOpen.value = false;
    detailOpen.value = false;
    await Promise.all([store.load(), store.loadProfile()]);
    window.$message?.success('纠正已保存，历史记录已保留');
  }
}

async function clearAll() {
  if (clearConfirm.value !== '清空记忆') return;
  const { error } = await clearMemories({ scope: 'all', revision_token: store.revisionToken });
  if (!error) {
    clearOpen.value = false;
    clearConfirm.value = '';
    await Promise.all([store.load(), store.loadProfile()]);
    window.$message?.success('已有记忆已退出召回');
  }
}

async function deleteDetail() {
  if (!store.detail) return;
  await store.decide(store.detail, 'delete');
  detailOpen.value = false;
}

function search() {
  store.filters.page = 1;
  store.load();
}

onMounted(() => Promise.all([store.load(), store.loadProfile(), loadPreference()]));
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px">
    <header class="flex flex-wrap items-center justify-between gap-12px">
      <div>
        <h1 class="m-0 text-24px font-semibold">记忆管理</h1>
        <NText depth="3">查看和管理 AI 为你保留的长期记忆</NText>
      </div>
      <NSpace align="center">
        <NSwitch v-model:value="autoMemoryEnabled" :loading="preferenceSaving" @update:value="saveAutoMemory" />
        <NText>{{ autoMemoryEnabled ? '自动记忆' : '自动记忆已关闭' }}</NText>
        <NButton quaternary @click="router.back()">
          <template #icon><SvgIcon icon="ph:arrow-left" /></template>
          返回
        </NButton>
        <NButton type="error" ghost @click="clearOpen = true">清空记忆</NButton>
      </NSpace>
    </header>

    <NCard :bordered="false">
      <NTabs v-model:value="view" type="line" animated>
        <NTabPane name="profile" tab="当前画像">
          <NSpin :show="store.loading">
            <NDescriptions v-if="profileEntries.length" bordered label-placement="left" :column="1">
              <NDescriptionsItem v-for="[key, value] in profileEntries" :key="key" :label="key">
                <NText strong>{{ typeof value === 'object' ? JSON.stringify(value) : value }}</NText>
              </NDescriptionsItem>
            </NDescriptions>
            <NEmpty v-else description="暂时没有稳定画像" />
          </NSpin>
        </NTabPane>

        <NTabPane name="timeline" tab="按日时间线">
          <div class="mb-16px grid gap-12px md:grid-cols-4">
            <NDatePicker
              v-model:formatted-value="store.filters.date_from"
              value-format="yyyy-MM-dd"
              type="date"
              clearable
            />
            <NDatePicker
              v-model:formatted-value="store.filters.date_to"
              value-format="yyyy-MM-dd"
              type="date"
              clearable
            />
            <NSelect
              v-model:value="store.filters.type"
              clearable
              placeholder="记忆类型"
              :options="[
                { label: '偏好', value: 'preference' },
                { label: '事实', value: 'fact' },
                { label: '上下文', value: 'context' },
                { label: '事件', value: 'event' }
              ]"
            />
            <NButton @click="search">筛选</NButton>
          </div>
          <NSpin :show="store.loading">
            <section v-for="[date, items] in timeline" :key="date" class="mb-24px">
              <h3 class="sticky top-0 z-1 bg-[var(--n-color)] py-8px text-16px">{{ date }}</h3>
              <NList bordered>
                <NListItem v-for="item in items" :key="item.id" class="cursor-pointer" @click="show(item)">
                  <NThing :title="item.summary" :description="item.content">
                    <template #header-extra>
                      <NTag size="small">{{ item.date_precision }}</NTag>
                    </template>
                  </NThing>
                </NListItem>
              </NList>
            </section>
            <NEmpty v-if="!timeline.length" description="所选日期内没有记忆" />
          </NSpin>
        </NTabPane>

        <NTabPane name="pending">
          <template #tab>
            待确认
            <NBadge :value="store.total" :max="99" />
          </template>
          <NList v-if="pending.length" bordered>
            <NListItem v-for="item in pending" :key="item.id">
              <NThing :title="item.summary">
                <template #description>
                  <div class="line-clamp-3 break-words">{{ item.content }}</div>
                  <NText depth="3">
                    {{ item.sensitivity === 'normal' ? '需要你确认后才会生效' : '涉及敏感信息，需要明确确认' }}
                  </NText>
                </template>
              </NThing>
              <template #suffix>
                <NSpace>
                  <NButton size="small" @click="store.decide(item, 'reject')">拒绝</NButton>
                  <NButton size="small" type="primary" @click="store.decide(item, 'confirm')">确认</NButton>
                  <NButton size="small" @click="show(item)">详情</NButton>
                </NSpace>
              </template>
            </NListItem>
          </NList>
          <NEmpty v-else description="没有待确认记忆" />
        </NTabPane>

        <NTabPane name="all" tab="全部记忆">
          <div class="mb-16px grid gap-12px md:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_minmax(180px,1fr)_96px]">
            <NInput v-model:value="store.filters.keyword" clearable placeholder="搜索记忆" @keyup.enter="search" />
            <NSelect
              v-model:value="store.filters.status"
              clearable
              placeholder="状态"
              :options="Object.entries(statusLabel).map(([value, label]) => ({ value, label }))"
            />
            <NSelect
              v-model:value="store.filters.sensitivity"
              clearable
              placeholder="敏感级别"
              :options="[
                { label: '普通', value: 'normal' },
                { label: '敏感', value: 'sensitive' },
                { label: '受限', value: 'restricted' }
              ]"
            />
            <NButton class="w-full" @click="search">
              <template #icon><SvgIcon icon="ph:magnifying-glass" /></template>
              搜索
            </NButton>
          </div>
          <NList bordered>
            <NListItem v-for="item in store.rows" :key="item.id" class="cursor-pointer" @click="show(item)">
              <NThing :title="item.summary" :description="item.content" />
              <template #suffix>
                <NTag
                  size="small"
                  :type="
                    item.status === 'active' ? 'success' : item.status === 'pending_review' ? 'warning' : 'default'
                  "
                >
                  {{ statusLabel[item.status] || item.status }}
                </NTag>
              </template>
            </NListItem>
          </NList>
          <NEmpty v-if="!store.rows.length" description="没有符合条件的记忆" />
          <div v-if="store.total > store.filters.page_size" class="mt-16px flex justify-end">
            <NPagination
              v-model:page="store.filters.page"
              :page-size="store.filters.page_size"
              :item-count="store.total"
              @update:page="store.load"
            />
          </div>
        </NTabPane>
      </NTabs>
    </NCard>

    <NDrawer v-model:show="detailOpen" :width="drawerWidth" placement="right">
      <NDrawerContent title="记忆详情" closable>
        <template v-if="store.detail">
          <NSpace vertical size="large">
            <div>
              <NTag>{{ statusLabel[store.detail.status] }}</NTag>
              <NTag class="ml-8px">{{ store.detail.type }}</NTag>
            </div>
            <h2 class="m-0 break-words text-20px">{{ store.detail.summary }}</h2>
            <NText class="break-words">{{ store.detail.content }}</NText>
            <NDescriptions bordered :column="1" label-placement="left">
              <NDescriptionsItem label="发生时间">
                {{
                  store.detail.occurred_at
                    ? formatDateTime(store.detail.occurred_at)
                    : store.detail.local_date || '未知'
                }}
              </NDescriptionsItem>
              <NDescriptionsItem label="获知时间">{{ formatDateTime(store.detail.observed_at) }}</NDescriptionsItem>
              <NDescriptionsItem label="记录时间">{{ formatDateTime(store.detail.recorded_at) }}</NDescriptionsItem>
              <NDescriptionsItem label="时间精度">{{ store.detail.date_precision }}</NDescriptionsItem>
              <NDescriptionsItem label="有效区间">
                {{ store.detail.valid_from ? formatDateTime(store.detail.valid_from) : '未限定' }} -
                {{ store.detail.valid_to ? formatDateTime(store.detail.valid_to) : '持续有效' }}
              </NDescriptionsItem>
              <NDescriptionsItem label="置信度">{{ Math.round(store.detail.confidence * 100) }}%</NDescriptionsItem>
            </NDescriptions>
            <div>
              <h3>来源</h3>
              <NList bordered>
                <NListItem v-for="source in store.detail.evidence || []" :key="source.id">
                  <NThing
                    :title="source.source_kind"
                    :description="
                      source.revoked_at ? '来源已撤权，正文不可访问' : `记录于 ${formatDateTime(source.created_at)}`
                    "
                  />
                </NListItem>
              </NList>
            </div>
          </NSpace>
        </template>
        <template #footer>
          <NSpace v-if="store.detail" justify="end">
            <NButton v-if="store.detail.status === 'active'" @click="startCorrection">纠正</NButton>
            <NPopconfirm v-if="store.detail.status === 'active'" @positive-click="deleteDetail">
              <template #trigger><NButton type="error" ghost>删除</NButton></template>
              删除后将立即停止召回，是否继续？
            </NPopconfirm>
          </NSpace>
        </template>
      </NDrawerContent>
    </NDrawer>

    <NModal v-model:show="correctionOpen" preset="card" title="纠正记忆" class="w-620px max-w-95vw">
      <NAlert type="info" class="mb-16px">纠正会创建新的当前事实，原记录会保留在历史中。</NAlert>
      <NForm label-placement="top">
        <NFormItem label="摘要"><NInput v-model:value="correction.summary" maxlength="512" /></NFormItem>
        <NFormItem label="内容">
          <NInput v-model:value="correction.content" type="textarea" :autosize="{ minRows: 3, maxRows: 8 }" />
        </NFormItem>
        <div class="grid gap-12px sm:grid-cols-2">
          <NFormItem label="发生日期">
            <NDatePicker
              v-model:formatted-value="correction.local_date"
              value-format="yyyy-MM-dd"
              type="date"
              clearable
            />
          </NFormItem>
          <NFormItem label="日期精度">
            <NSelect
              v-model:value="correction.date_precision"
              :options="[
                { label: '精确日期', value: 'exact_day' },
                { label: '近似日期', value: 'approximate' },
                { label: '未知', value: 'unknown' }
              ]"
            />
          </NFormItem>
        </div>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="correctionOpen = false">取消</NButton>
          <NButton type="primary" @click="saveCorrection">保存纠正</NButton>
        </NSpace>
      </template>
    </NModal>

    <NModal v-model:show="clearOpen" preset="card" title="清空已有记忆" class="w-480px max-w-95vw">
      <NAlert type="error" class="mb-16px">清空后所有当前及待确认记忆会立即停止召回。自动记忆开关不会随之关闭。</NAlert>
      <NInput v-model:value="clearConfirm" placeholder="输入“清空记忆”确认" />
      <template #footer>
        <NSpace justify="end">
          <NButton @click="clearOpen = false">取消</NButton>
          <NButton type="error" :disabled="clearConfirm !== '清空记忆'" @click="clearAll">确认清空</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
