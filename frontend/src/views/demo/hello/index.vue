<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { request } from '@/service/request';
import { useAuth } from '@/hooks/business/auth';
import { localStg } from '@/utils/storage';

defineOptions({ name: 'DemoHello' });

const { hasAuth } = useAuth();
const loading = ref(false);
const message = ref('');
const error = ref('');

async function callHello() {
  loading.value = true;
  error.value = '';
  message.value = '';
  try {
    const { data, error: err } = await request<{ message: string; tenant_id: string; user_id: string }>({
      url: '/api/v1/demo/hello',
      method: 'post',
      data: { name: 'AINP' }
    });
    if (err) {
      error.value = err.message || 'request failed';
    } else if (data) {
      message.value = `${data.message} (tenant=${data.tenant_id}, user=${data.user_id})`;
    }
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  // 确保已登录态下有 token
  if (localStg.get('token')) {
    callHello();
  }
});
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard title="Demo Hello" :bordered="false" class="card-wrapper">
      <NSpace vertical>
        <NAlert type="info" title="联调说明">
          本页对接后端
          <code>POST /api/v1/demo/hello</code>
          ，权限码
          <code>demo.hello</code>
          。
        </NAlert>
        <NSpace>
          <NButton type="primary" :loading="loading" :disabled="!hasAuth('demo.hello')" @click="callHello">
            打个招呼
          </NButton>
          <NTag v-if="hasAuth('demo.hello')" type="success">有按钮权限 demo.hello</NTag>
          <NTag v-else type="warning">无按钮权限 demo.hello</NTag>
        </NSpace>
        <NAlert v-if="message" type="success" :title="message" />
        <NAlert v-if="error" type="error" :title="error" />
      </NSpace>
    </NCard>
  </div>
</template>
