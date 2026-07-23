<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { decideMCPConfirmation, fetchMCPConfirmation } from '@/service/api';
import { localStg } from '@/utils/storage';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'MCPConfirm' });

const route = useRoute();
const router = useRouter();
const loading = ref(true);
const deciding = ref(false);
const confirmation = ref<Api.User.MCPConfirmation | null>(null);
const token = computed(() => typeof route.query.token === 'string' ? route.query.token : '');

async function load() {
  if (!localStg.get('token')) {
    await router.replace({ name: 'login', query: { redirect: route.fullPath } });
    return;
  }
  if (!token.value) throw new Error('确认链接无效');
  const { data, error } = await fetchMCPConfirmation(token.value);
  if (error || !data) throw new Error('确认请求不存在或已失效');
  confirmation.value = data;
  loading.value = false;
}

async function decide(decision: 'approve' | 'reject') {
  deciding.value = true;
  try {
    const { data, error } = await decideMCPConfirmation(token.value, decision);
    if (!error && data) {
      confirmation.value = data;
      window.$message?.success(decision === 'approve' ? '已批准，请返回 MCP 客户端重试调用' : '已拒绝本次操作');
    }
  } finally { deciding.value = false; }
}

onMounted(() => void load().catch(error => { loading.value = false; window.$message?.error(error instanceof Error ? error.message : '无法读取确认请求'); }));
</script>

<template>
  <div class="min-h-screen bg-#f4f6f8 px-16px py-40px text-#202124">
    <main class="mx-auto max-w-620px bg-white p-32px shadow-sm ring-1 ring-#dfe3e8 sm:p-40px">
      <NSpin :show="loading">
        <NResult v-if="confirmation && confirmation.status !== 'pending'" :status="confirmation.status === 'approved' ? 'success' : 'info'" :title="confirmation.status === 'approved' ? '操作已批准' : '操作未执行'" description="可以关闭此页面并返回 MCP 客户端。" />
        <template v-else-if="confirmation">
          <div class="mb-24px flex items-start justify-between gap-16px">
            <div><h1 class="m-0 text-22px font-600">确认敏感操作</h1><p class="mb-0 mt-6px text-#667085">{{ confirmation.client_name }} 请求通过 AINP 执行操作</p></div>
            <NTag type="error">{{ confirmation.side_effect }}</NTag>
          </div>
          <NAlert type="error" title="仅批准这一次调用" class="mb-20px">批准只对当前参数有效，链接过期、参数变化、权限变化或授权撤销后均无法执行。</NAlert>
          <NDescriptions :column="1" bordered label-placement="left">
            <NDescriptionsItem label="操作">{{ confirmation.title }}</NDescriptionsItem>
            <NDescriptionsItem label="说明">{{ confirmation.description }}</NDescriptionsItem>
            <NDescriptionsItem label="风险">{{ confirmation.risk }}</NDescriptionsItem>
            <NDescriptionsItem label="有效期">{{ formatDateTime(confirmation.expires_at) }}</NDescriptionsItem>
          </NDescriptions>
          <div class="mt-28px flex justify-end gap-12px"><NButton :disabled="deciding" @click="decide('reject')">拒绝</NButton><NButton type="error" :loading="deciding" @click="decide('approve')">批准本次操作</NButton></div>
        </template>
      </NSpin>
    </main>
  </div>
</template>
