import { computed, ref } from 'vue';
import { defineStore } from 'pinia';
import { fetchCodeSets } from '@/service/api';
import { getLocale } from '@/locales';

export const useCodeSetStore = defineStore('codeset', () => {
  const definitions = ref<Record<string, Api.CodeSet.Definition>>({});
  const version = ref('');
  const loadedLocale = ref('');
  const loading = ref(false);
  const error = ref<Error | null>(null);

  const all = computed(() => Object.values(definitions.value));

  async function load(keys: string[] = [], force = false) {
    const locale = getLocale();
    const missing = keys.filter(key => !definitions.value[key]);
    if (!force && loadedLocale.value === locale && keys.length > 0 && missing.length === 0) return;

    loading.value = true;
    error.value = null;
    const { data, error: requestError } = await fetchCodeSets(keys);
    loading.value = false;
    if (requestError || !data) {
      error.value = requestError || new Error('codeset response is empty');
      return;
    }
    if (loadedLocale.value !== locale || keys.length === 0) definitions.value = {};
    for (const definition of data.items) definitions.value[definition.key] = definition;
    version.value = data.version;
    loadedLocale.value = locale;
  }

  function get(key: string) {
    return definitions.value[key];
  }

  function clear() {
    definitions.value = {};
    version.value = '';
    loadedLocale.value = '';
    error.value = null;
  }

  return { definitions, all, version, loading, error, load, get, clear };
});
