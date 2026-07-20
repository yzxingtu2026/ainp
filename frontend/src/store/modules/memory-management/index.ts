import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';
import {
  confirmMemory,
  deleteMemory,
  fetchMemory,
  fetchMemoryList,
  fetchMemoryProfile,
  rejectMemory
} from '@/service/api';
export const useMemoryManagementStore = defineStore('memory-management', () => {
  const loading = ref(false);
  const rows = ref<Api.Memory.Item[]>([]);
  const total = ref(0);
  const pendingTotal = ref(0);
  const revisionToken = ref('');
  const profile = ref<Api.Memory.Profile | null>(null);
  const detail = ref<Api.Memory.Item | null>(null);
  const filters = reactive({
    page: 1,
    page_size: 20,
    keyword: '',
    status: null as string | null,
    type: null as string | null,
    sensitivity: null as string | null,
    agent_id: null as string | null,
    date_from: null as string | null,
    date_to: null as string | null
  });
  async function load() {
    loading.value = true;
    try {
      const { data } = await fetchMemoryList({
        ...filters,
        status: filters.status || '',
        type: filters.type || '',
        sensitivity: filters.sensitivity || '',
        agent_id: filters.agent_id || '',
        date_from: filters.date_from || '',
        date_to: filters.date_to || ''
      });
      if (data) {
        rows.value = data.items || [];
        total.value = data.total;
        revisionToken.value = data.revision_token;
      }
    } finally {
      loading.value = false;
    }
  }
  async function loadProfile() {
    const { data } = await fetchMemoryProfile();
    if (data) profile.value = data;
  }
  async function loadPendingTotal() {
    const { data } = await fetchMemoryList({ page: 1, page_size: 1, status: 'pending_review' });
    pendingTotal.value = data?.total || 0;
  }
  async function open(id: string) {
    const { data } = await fetchMemory(id);
    if (data) detail.value = data;
  }
  async function decide(item: Api.Memory.Item, action: 'confirm' | 'reject' | 'delete') {
    const call = action === 'confirm' ? confirmMemory : action === 'reject' ? rejectMemory : deleteMemory;
    const { error } = await call(item.id, item.revision);
    if (!error) {
      detail.value = null;
      await Promise.all([load(), loadProfile(), loadPendingTotal()]);
      window.$message?.success('操作成功');
    }
  }
  return {
    loading,
    rows,
    total,
    pendingTotal,
    revisionToken,
    profile,
    detail,
    filters,
    load,
    loadProfile,
    loadPendingTotal,
    open,
    decide
  };
});
