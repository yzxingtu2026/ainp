import { computed, ref } from 'vue';
import { defineStore } from 'pinia';
import { adminApplication } from '@admin-application';
import { SetupStoreId } from '@/enum';
import { downloadAttachmentById } from '@/hooks/business/attachment';
import { fetchSetting } from '@/service/api';

export const BRAND_SETTING_KEY = 'general.brand';

export const useBrandStore = defineStore(SetupStoreId.Brand, () => {
  const nameOverride = ref('');
  const logoAttachmentId = ref('');
  const logoObjectUrl = ref('');
  const loading = ref(false);
  let requestVersion = 0;

  const name = computed(() => nameOverride.value.trim() || adminApplication.brand.name);
  const logo = computed(() => logoObjectUrl.value || adminApplication.brand.logo);

  function updateDocumentTitle() {
    const routeTitle = document.title.split(' - ')[0]?.trim();
    document.title = routeTitle && routeTitle !== adminApplication.name ? `${routeTitle} - ${name.value}` : name.value;
  }

  function revokeLogo() {
    if (logoObjectUrl.value) URL.revokeObjectURL(logoObjectUrl.value);
    logoObjectUrl.value = '';
  }

  function resetStore() {
    requestVersion += 1;
    revokeLogo();
    nameOverride.value = '';
    logoAttachmentId.value = '';
    updateDocumentTitle();
  }

  function fallbackLogo() {
    requestVersion += 1;
    revokeLogo();
  }

  async function apply(values: Record<string, unknown>) {
    const version = ++requestVersion;
    nameOverride.value = typeof values.name === 'string' ? values.name.trim() : '';
    updateDocumentTitle();
    logoAttachmentId.value = typeof values.logo_attachment_id === 'string' ? values.logo_attachment_id : '';
    revokeLogo();
    if (logoAttachmentId.value) {
      try {
        const result = await downloadAttachmentById(logoAttachmentId.value);
        if (version !== requestVersion || !result.contentType.startsWith('image/')) return;
        logoObjectUrl.value = URL.createObjectURL(result.blob);
      } catch {
        logoObjectUrl.value = '';
      }
    }
  }

  async function load() {
    loading.value = true;
    try {
      const { data, error } = await fetchSetting(BRAND_SETTING_KEY);
      if (error || !data) {
        resetStore();
        return;
      }
      await apply(data.values);
    } finally {
      loading.value = false;
    }
  }

  return { name, logo, logoAttachmentId, loading, load, apply, fallbackLogo, resetStore };
});
