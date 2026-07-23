<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { fetchAuthSessions, fetchMCPConnectionInfo, fetchSelfProfile, fetchUserPreferences, revokeAuthSession, updateSelfProfile, updateUserPreferences } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'SystemProfile' });

type Tab = 'basic' | 'preferences' | 'memory' | 'mcp' | 'security';
const route = useRoute();
const router = useRouter();
const tab = ref<Tab>((route.query.tab as Tab) || 'basic');
const loading = ref(false);
const saving = ref(false);
const profile = ref<Api.User.SelfProfile | null>(null);
const definition = ref<Api.Setting.Definition | null>(null);
const sessions = ref<Api.Auth.Session[]>([]);
const mcpConnection = ref<Api.User.MCPConnectionInfo | null>(null);
const displayName = ref('');
const values = reactive<Record<string, any>>({});

const address = computed(() => {
  const override = String(values.preferred_address_override || '').trim();
  return override
    ? { value: override, source: '手动设置' }
    : { value: profile.value?.effective_address || '-', source: profile.value?.address_source === 'memory_profile' ? '记忆画像' : '基本资料' };
});

const mcpJSONConfig = computed(() => JSON.stringify({
  mcpServers: {
    ainp: {
      type: 'streamable-http',
      url: mcpConnection.value?.url || ''
    }
  }
}, null, 2));

watch(tab, value => router.replace({ query: { ...route.query, tab: value } }));

async function load() {
  loading.value = true;
  try {
    const [profileResult, preferenceResult, sessionResult, mcpResult] = await Promise.all([fetchSelfProfile(), fetchUserPreferences(), fetchAuthSessions(), fetchMCPConnectionInfo()]);
    if (!profileResult.error && profileResult.data) {
      profile.value = profileResult.data;
      displayName.value = profileResult.data.display_name;
    }
    if (!preferenceResult.error && preferenceResult.data) {
      definition.value = preferenceResult.data;
      Object.assign(values, preferenceResult.data.values);
    }
    if (!sessionResult.error && sessionResult.data) sessions.value = sessionResult.data.sessions || [];
    if (!mcpResult.error && mcpResult.data) mcpConnection.value = mcpResult.data;
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

async function copyMCPURL() {
  if (!mcpConnection.value?.url) return;
  await navigator.clipboard.writeText(mcpConnection.value.url);
  window.$message?.success('MCP 地址已复制');
}

async function copyMCPJSON() {
  if (!mcpConnection.value?.url) return;
  await navigator.clipboard.writeText(mcpJSONConfig.value);
  window.$message?.success('MCP JSON 配置已复制');
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
            <NSpace><NButton type="primary" :loading="saving" @click="savePreferences">保存隐私设置</NButton><NButton @click="router.push({ path: '/profile/memory', query: { from: 'profile' } })">管理记忆</NButton></NSpace>
          </NSpace>
        </NTabPane>

        <NTabPane name="mcp" tab="MCP 连接">
          <div class="max-w-760px">
            <NAlert type="info" title="连接桌面 Agent" class="mb-20px">
              AINP 使用远程 Streamable HTTP 与浏览器 OAuth 授权。客户端保存连接后会打开 AINP 授权页，无需手动填写 Token 或请求头。
            </NAlert>

            <NDescriptions label-placement="left" :column="1" bordered>
              <NDescriptionsItem label="名称">{{ mcpConnection?.name || 'AINP' }}</NDescriptionsItem>
              <NDescriptionsItem label="类型">流式 HTTP</NDescriptionsItem>
              <NDescriptionsItem label="URL">
                <div class="flex items-center gap-10px">
                  <NInput :value="mcpConnection?.url || ''" readonly />
                  <NButton type="primary" secondary :disabled="!mcpConnection?.url" @click="copyMCPURL">复制地址</NButton>
                </div>
              </NDescriptionsItem>
              <NDescriptionsItem label="Bearer 令牌环境变量">留空</NDescriptionsItem>
              <NDescriptionsItem label="标头 / 环境变量标头">留空</NDescriptionsItem>
            </NDescriptions>

            <NDivider title-placement="left">在 Codex 中连接</NDivider>
            <ol class="m-0 pl-22px leading-30px">
              <li>打开“插件”，切换到“MCP”，选择“连接至自定义 MCP”。</li>
              <li>名称填写 AINP，类型选择“流式 HTTP”。</li>
              <li>粘贴上方 URL，其余认证字段保持为空，然后保存。</li>
              <li>在浏览器授权页确认账号、租户和所需权限范围。</li>
            </ol>

            <NDivider title-placement="left">JSON 配置</NDivider>
            <NAlert type="default" class="mb-12px">
              AINP 使用 Streamable HTTP，不是旧版 SSE。适用于识别 <NCode code="type: streamable-http" language="text" /> 的客户端；若客户端使用 <NCode code="type: http" language="text" /> 等私有枚举，请按其文档调整 type，URL 始终使用上方的 /mcp 地址。
            </NAlert>
            <div class="border border-#e5e7eb bg-#f8fafc p-16px">
              <div class="mb-12px flex items-center justify-between gap-12px">
                <NText strong>mcp.json</NText>
                <NButton type="primary" secondary :disabled="!mcpConnection?.url" @click="copyMCPJSON">复制 JSON</NButton>
              </div>
              <NCode :code="mcpJSONConfig" language="json" word-wrap />
            </div>

            <NDivider title-placement="left">授权范围</NDivider>
            <NSpace vertical :size="10">
              <div><NTag type="success" class="mr-10px">读取</NTag>查看当前账号有权访问的数据，默认启用。</div>
              <div><NTag type="warning" class="mr-10px">写入</NTag>创建或编辑有权管理的数据，需要主动勾选。</div>
              <div><NTag type="error" class="mr-10px">敏感</NTag>允许发起外部调用、费用或删除操作，执行时仍需逐次确认。</div>
            </NSpace>
          </div>
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
