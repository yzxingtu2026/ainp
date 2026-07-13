<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { request } from '@/service/request';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'SystemProfile' });

const authStore = useAuthStore();
const loading = ref(false);
const profile = ref<Api.Auth.AinPUserProfile | null>(null);

async function loadProfile() {
  loading.value = true;
  try {
    const { data, error } = await request<Api.Auth.AinPUserProfile>({
      url: '/api/v1/system/auth/me'
    });
    if (!error && data) {
      profile.value = data;
    }
  } finally {
    loading.value = false;
  }
}

onMounted(loadProfile);
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard title="个人中心" :bordered="false" class="card-wrapper" :loading="loading">
      <NDescriptions v-if="profile" label-placement="left" bordered :column="1">
        <NDescriptionsItem label="用户 ID">{{ profile.id }}</NDescriptionsItem>
        <NDescriptionsItem label="用户名">{{ profile.username }}</NDescriptionsItem>
        <NDescriptionsItem label="显示名">{{ profile.display_name }}</NDescriptionsItem>
        <NDescriptionsItem label="租户">{{ profile.tenant_name || profile.tenant_id }}</NDescriptionsItem>
        <NDescriptionsItem label="角色">{{ (profile.roles || []).join(', ') }}</NDescriptionsItem>
        <NDescriptionsItem label="权限">
          <NSpace>
            <NTag v-for="p in profile.permissions || []" :key="p" size="small">{{ p }}</NTag>
          </NSpace>
        </NDescriptionsItem>
        <NDescriptionsItem label="前端按钮权限缓存">
          <NSpace>
            <NTag v-for="b in authStore.userInfo.buttons" :key="b" size="small" type="info">{{ b }}</NTag>
          </NSpace>
        </NDescriptionsItem>
      </NDescriptions>
      <NButton class="mt-16px" type="primary" @click="loadProfile">刷新资料</NButton>
    </NCard>
  </div>
</template>
