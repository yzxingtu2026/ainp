<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { localStg } from '@/utils/storage';
import { getServiceBaseURL } from '@/utils/service';

defineOptions({ name: 'OAuthAuthorize' });

type AuthorizationInfo = { client_id: string; client_name: string; redirect_uri: string; requested_scope: string[]; user_id: string; tenant_id: string };
const route = useRoute();
const router = useRouter();
const loading = ref(true);
const submitting = ref(false);
const info = ref<AuthorizationInfo | null>(null);
const write = ref(false);
const sensitive = ref(false);
const { baseURL } = getServiceBaseURL(import.meta.env, import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y');
const query = computed(() => new URLSearchParams(Object.entries(route.query).flatMap(([key, value]) => typeof value === 'string' ? [[key, value]] : [])));

async function loadAuthorization() {
  const token = localStg.get('token');
  if (!token) { await router.replace({ name: 'login', query: { redirect: route.fullPath } }); return; }
  const response = await fetch(`${baseURL}/api/v1/mcp/oauth/authorize?${query.value}`, { headers: { Authorization: `Bearer ${token}` } });
  if (response.status === 401) { await router.replace({ name: 'login', query: { redirect: route.fullPath } }); return; }
  if (!response.ok) throw new Error('授权请求无效或已过期');
  info.value = await response.json();
  loading.value = false;
}

function callback(error?: string, code?: string) {
  if (!info.value) return;
  const target = new URL(info.value.redirect_uri);
  if (typeof route.query.state === 'string') target.searchParams.set('state', route.query.state);
  if (error) target.searchParams.set('error', error);
  if (code) target.searchParams.set('code', code);
  window.location.assign(target.toString());
}

async function approve() {
  if (!info.value) return;
  submitting.value = true;
  const scopes = ['actions:read'];
  if (write.value) scopes.push('actions:write');
  if (sensitive.value) scopes.push('actions:sensitive');
  const response = await fetch(`${baseURL}/api/v1/mcp/oauth/authorize`, { method: 'POST', headers: { Authorization: `Bearer ${localStg.get('token')}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ client_id: info.value.client_id, redirect_uri: info.value.redirect_uri, scope: scopes.join(' '), code_challenge: route.query.code_challenge, code_challenge_method: route.query.code_challenge_method }) });
  const body = await response.json();
  if (!response.ok || !body.code) { submitting.value = false; window.$message?.error('授权失败，请重新发起连接'); return; }
  callback(undefined, body.code);
}

onMounted(() => void loadAuthorization().catch(error => { loading.value = false; window.$message?.error(error instanceof Error ? error.message : '无法读取授权请求'); }));
</script>

<template>
  <div class="min-h-screen bg-#f4f6f8 px-16px py-40px text-#202124">
    <main class="mx-auto max-w-620px bg-white p-32px shadow-sm ring-1 ring-#dfe3e8 sm:p-40px">
      <NSpin :show="loading">
        <template v-if="info">
          <div class="mb-28px flex items-center gap-14px">
            <div class="h-44px w-44px flex-center bg-#111827 text-20px text-white">AI</div>
            <div><h1 class="m-0 text-22px font-600">授权 {{ info.client_name }} 连接 AINP</h1><p class="mb-0 mt-4px text-13px text-#667085">{{ info.user_id }} · {{ info.tenant_id }}</p></div>
          </div>
          <div class="mb-24px border-y border-#eaecf0 py-18px text-13px">
            <div class="grid grid-cols-[88px_1fr] gap-y-8px"><span class="text-#667085">回调地址</span><span class="break-all">{{ info.redirect_uri }}</span><span class="text-#667085">有效期</span><span>30 天，可随时撤销</span></div>
          </div>
          <NSpace vertical :size="14">
            <NCheckbox :checked="true" disabled><span class="font-500">读取数据</span><span class="ml-8px text-#667085">查看当前有权访问的数据</span></NCheckbox>
            <NCheckbox v-model:checked="write"><span class="font-500">创建和编辑数据</span><span class="ml-8px text-#667085">可创建、编辑或变更当前有权管理的数据</span></NCheckbox>
            <div class="border border-#f04438 bg-#fff5f4 p-14px"><NCheckbox v-model:checked="sensitive"><span class="font-600 text-#b42318">敏感操作</span></NCheckbox><p class="mb-0 mt-8px text-13px leading-20px text-#b42318">可能触发外部调用、费用或数据删除。此授权仅允许发起，每次执行仍需你单独确认。</p></div>
          </NSpace>
          <div class="mt-30px flex justify-end gap-12px"><NButton @click="callback('access_denied')">取消</NButton><NButton type="primary" :loading="submitting" @click="approve">确认授权</NButton></div>
        </template>
      </NSpin>
    </main>
  </div>
</template>
