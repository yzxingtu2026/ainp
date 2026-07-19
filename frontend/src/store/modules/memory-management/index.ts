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
  const revisionToken = ref('');
  const profile = ref<Api.Memory.Profile | null>(null);
  const detail = ref<Api.Memory.Item | null>(null);
  const filters = reactive({
    page: 1,
    page_size: 20,
    keyword: '',
    status: '',
    type: '',
    sensitivity: '',
    agent_id: '',
    date_from: '',
    date_to: ''
  });
  async function load() {
    loading.value = true;
    try {
      const { data } = await fetchMemoryList({ ...filters });
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
  async function open(id: string) {
    const { data } = await fetchMemory(id);
    if (data) detail.value = data;
  }
  async function decide(item: Api.Memory.Item, action: 'confirm' | 'reject' | 'delete') {
    const call = action === 'confirm' ? confirmMemory : action === 'reject' ? rejectMemory : deleteMemory;
    const { error } = await call(item.id, item.revision);
    if (!error) {
      detail.value = null;
      await Promise.all([load(), loadProfile()]);
      window.$message?.success('操作成功');
    }
  }
  return { loading, rows, total, revisionToken, profile, detail, filters, load, loadProfile, open, decide };
});
