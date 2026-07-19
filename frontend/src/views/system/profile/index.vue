<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { fetchAuthSessions, fetchSelfProfile, fetchUserPreferences, revokeAuthSession, updateSelfProfile, updateUserPreferences } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'SystemProfile' });

type Tab = 'basic' | 'preferences' | 'memory' | 'security';
const route = useRoute();
const router = useRouter();
const tab = ref<Tab>((route.query.tab as Tab) || 'basic');
const loading = ref(false);
const saving = ref(false);
const profile = ref<Api.User.SelfProfile | null>(null);
const definition = ref<Api.Setting.Definition | null>(null);
const sessions = ref<Api.Auth.Session[]>([]);
const displayName = ref('');
const values = reactive<Record<string, any>>({});

const address = computed(() => {
  const override = String(values.preferred_address_override || '').trim();
  return override
    ? { value: override, source: '手动设置' }
    : { value: profile.value?.effective_address || '-', source: profile.value?.address_source === 'memory_profile' ? '记忆画像' : '基本资料' };
});

watch(tab, value => router.replace({ query: { ...route.query, tab: value } }));

async function load() {
  loading.value = true;
  try {
    const [profileResult, preferenceResult, sessionResult] = await Promise.all([fetchSelfProfile(), fetchUserPreferences(), fetchAuthSessions()]);
    if (!profileResult.error && profileResult.data) {
      profile.value = profileResult.data;
      displayName.value = profileResult.data.display_name;
    }
    if (!preferenceResult.error && preferenceResult.data) {
      definition.value = preferenceResult.data;
      Object.assign(values, preferenceResult.data.values);
    }
    if (!sessionResult.error && sessionResult.data) sessions.value = sessionResult.data.sessions || [];
  } finally { loading.value = false; }
}

async function revokeSession(session: Api.Auth.Session) {
  if (session.is_current) return;
  const { error } = await revokeAuthSession(session.id);
  if (!error) { sessions.value = sessions.value.filter(item => item.id !== session.id); window.$message?.success('会话已撤销'); }
}

async function saveBasic() {
  if (!displayName.value.trim()) return window.$message?.error('显示名称不能为空');
  saving.value = true;
  try {
    const { data, error } = await updateSelfProfile(displayName.value.trim());
    if (!error && data) { profile.value = data; window.$message?.success('个人资料已保存'); }
  } finally { saving.value = false; }
}

async function savePreferences() {
  if (!definition.value) return;
  saving.value = true;
  try {
    const { data, error } = await updateUserPreferences(definition.value.revision, { ...values });
    if (!error && data) { definition.value = data; Object.assign(values, data.values); window.$message?.success('对话偏好已保存'); }
  } finally { saving.value = false; }
}

const optionMap: Record<string, Array<{ label: string; value: string }>> = {
  timezone: ['Asia/Shanghai', 'UTC', 'America/New_York'].map(value => ({ label: value, value })),
  locale: [{ label: '简体中文', value: 'zh-CN' }, { label: 'English', value: 'en-US' }],
  response_style: [{ label: '简洁', value: 'concise' }, { label: '标准', value: 'standard' }, { label: '详细', value: 'detailed' }],
  date_format: ['YYYY-MM-DD', 'YYYY/MM/DD', 'DD/MM/YYYY'].map(value => ({ label: value, value })),
  time_format: [{ label: '24 小时', value: '24h' }, { label: '12 小时', value: '12h' }]
};

onMounted(load);
</script>

<template>
  <NSpin :show="loading">
    <NCard title="个人主页" :bordered="false" class="min-h-500px">
      <NTabs v-model:value="tab" type="line" animated>
        <NTabPane name="basic" tab="基本资料">
          <NForm class="max-w-680px" label-placement="top">
            <NFormItem label="显示名称" required><NInput v-model:value="displayName" maxlength="128" show-count /></NFormItem>
            <NFormItem label="用户名"><NInput :value="profile?.username" disabled /></NFormItem>
            <NFormItem label="组织 / 部门"><NInput :value="[profile?.org_id, profile?.dept_id].filter(Boolean).join(' / ') || '未设置'" disabled /></NFormItem>
            <NFormItem label="角色"><NSpace><NTag v-for="role in profile?.roles || []" :key="role.code">{{ role.name }}</NTag></NSpace></NFormItem>
            <NButton type="primary" :loading="saving" @click="saveBasic">保存基本资料</NButton>
          </NForm>
        </NTabPane>

        <NTabPane name="preferences" tab="对话偏好">
          <NForm class="max-w-680px" label-placement="top">
            <NFormItem label="AI 称呼我的方式" feedback="可留空；留空后依次使用记忆画像、显示名称和用户名">
              <NInput v-model:value="values.preferred_address_override" maxlength="64" clearable />
            </NFormItem>
            <NFormItem label="语言"><NSelect v-model:value="values.locale" :options="optionMap.locale" /></NFormItem>
            <NFormItem label="时区"><NSelect v-model:value="values.timezone" filterable :options="optionMap.timezone" /></NFormItem>
            <NFormItem label="回答风格"><NSelect v-model:value="values.response_style" :options="optionMap.response_style" /></NFormItem>
            <div class="grid gap-16px sm:grid-cols-2">
              <NFormItem label="日期格式"><NSelect v-model:value="values.date_format" :options="optionMap.date_format" /></NFormItem>
              <NFormItem label="时间格式"><NSelect v-model:value="values.time_format" :options="optionMap.time_format" /></NFormItem>
            </div>
            <NButton type="primary" :loading="saving" @click="savePreferences">保存对话偏好</NButton>
          </NForm>
        </NTabPane>

        <NTabPane name="memory" tab="记忆与隐私">
          <NSpace vertical size="large" class="max-w-680px">
            <NCard size="small" title="当前生效称呼"><NText strong>{{ address.value }}</NText><NTag class="ml-12px">{{ address.source }}</NTag></NCard>
            <NCard size="small" title="记忆画像称呼">{{ profile?.memory_preferred_address || '暂无记忆画像' }}</NCard>
            <div class="flex items-center gap-10px">
              <NSwitch v-model:value="values.auto_memory_enabled" size="small" aria-label="自动记忆" />
              <NText>{{ values.auto_memory_enabled ? '自动记忆已开启' : '自动记忆已关闭' }}</NText>
            </div>
            <NSpace><NButton type="primary" :loading="saving" @click="savePreferences">保存隐私设置</NButton><NButton disabled>管理记忆（#124）</NButton></NSpace>
          </NSpace>
        </NTabPane>

        <NTabPane name="security" tab="账号安全">
          <NSpace vertical size="large">
            <NAlert type="info" title="账号安全">登录会话沿用统一认证能力；密码能力不在此页面复制实现。</NAlert>
            <NList bordered>
              <template #header>登录会话</template>
              <NListItem v-for="session in sessions" :key="session.id">
                <NThing :title="session.device_summary || session.client || '未知设备'" :description="`最近活动：${formatDateTime(session.last_seen_at)}`" />
                <template #suffix>
                  <NTag v-if="session.is_current" type="success">当前会话</NTag>
                  <NButton v-else size="small" type="error" ghost @click="revokeSession(session)">撤销</NButton>
                </template>
              </NListItem>
              <NEmpty v-if="sessions.length === 0" description="暂无活动会话" class="py-24px" />
            </NList>
          </NSpace>
        </NTabPane>
      </NTabs>
    </NCard>
  </NSpin>
</template>
