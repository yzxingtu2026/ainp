import { ref } from 'vue';
import { defineStore } from 'pinia';
import { fetchAIConversation, fetchAIConversations, fetchAIMessages } from '@/service/api';

export const useAIConversationStore = defineStore('ai-conversation', () => {
  const conversations = ref<Api.AI.Conversation[]>([]);
  const current = ref<Api.AI.Conversation | null>(null);
  const messages = ref<Api.AI.ConversationMessage[]>([]);
  const nextSequence = ref(0);
  const hasMore = ref(false);
  const loading = ref(false);

  async function loadConversations(params: Record<string, unknown> = {}) {
    loading.value = true;
    const { data, error } = await fetchAIConversations(params);
    loading.value = false;
    if (!error && data) conversations.value = data.items;
    return { data, error };
  }

  async function open(id: string) {
    loading.value = true;
    const [{ data: conversation, error }, { data: page }] = await Promise.all([
      fetchAIConversation(id),
      fetchAIMessages(id)
    ]);
    loading.value = false;
    if (!error && conversation) current.value = conversation;
    messages.value = page?.items || [];
    nextSequence.value = page?.next_sequence || 0;
    hasMore.value = Boolean(page?.has_more);
    return { data: conversation, error };
  }

  async function loadMore() {
    if (!current.value || !hasMore.value) return;
    const { data } = await fetchAIMessages(current.value.id, nextSequence.value);
    if (!data) return;
    messages.value.push(...data.items);
    nextSequence.value = data.next_sequence;
    hasMore.value = data.has_more;
  }

  function clear() {
    current.value = null;
    messages.value = [];
    nextSequence.value = 0;
    hasMore.value = false;
  }

  return { conversations, current, messages, nextSequence, hasMore, loading, loadConversations, open, loadMore, clear };
});
