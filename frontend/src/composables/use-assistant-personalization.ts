import { computed, ref } from 'vue';
import { fetchAssistantPersonalization } from '@/service/api';

const snapshot = ref<Api.AI.AssistantPersonalization | null>(null);
const loading = ref(false);
const frozen = ref(false);
const pendingPrompt = ref('');
let identity = '';

export function useAssistantPersonalization() {
  async function load(currentIdentity = '', allowReplace = true) {
    if (currentIdentity !== identity) {
      identity = currentIdentity;
      snapshot.value = null;
      frozen.value = false;
    }
    if (loading.value || (frozen.value && snapshot.value)) return;
    loading.value = true;
    try {
      const { data } = await fetchAssistantPersonalization();
      if (data && (allowReplace || !snapshot.value) && !frozen.value) snapshot.value = data;
    } finally {
      loading.value = false;
    }
  }

  function freeze() {
    frozen.value = true;
  }

  function choose(prompt: string) {
    pendingPrompt.value = prompt;
    freeze();
  }

  function consumePrompt() {
    const prompt = pendingPrompt.value;
    pendingPrompt.value = '';
    return prompt;
  }

  function reset() {
    snapshot.value = null;
    frozen.value = false;
    pendingPrompt.value = '';
    identity = '';
  }

  return {
    snapshot: computed(() => snapshot.value),
    greeting: computed(() => snapshot.value?.greeting || ''),
    suggestions: computed(() => snapshot.value?.suggestions || []),
    loading: computed(() => loading.value),
    frozen: computed(() => frozen.value),
    load,
    freeze,
    choose,
    consumePrompt,
    reset
  };
}
