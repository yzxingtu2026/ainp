import { nextTick, ref } from 'vue';
import { defineStore } from 'pinia';
import { SetupStoreId } from '@/enum';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';

export type AiAssistantView = 'conversation' | 'history' | 'run';

export interface PendingWorkspaceMessage {
  id: string;
  content: string;
  created_at: string;
  status: 'pending' | 'sending' | 'failed';
}

export const useAiAssistantStore = defineStore(SetupStoreId.AiAssistant, () => {
  const visible = ref(false);
  const view = ref<AiAssistantView>('conversation');
  const controller = useAiConversationController();
  const pendingWorkspaceMessage = ref<PendingWorkspaceMessage | null>(null);
  let returnFocus: HTMLElement | null = null;

  function open(target: AiAssistantView = 'conversation') {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    view.value = target;
    visible.value = true;
    void controller.initialize();
  }

  function close() {
    visible.value = false;
    view.value = 'conversation';
    const target = returnFocus;
    returnFocus = null;
    if (target) void nextTick(() => target.focus());
  }

  function toggle() {
    if (visible.value) close();
    else open();
  }

  function queueWorkspaceMessage(content: string) {
    const normalized = content.trim();
    if (!normalized || pendingWorkspaceMessage.value?.status === 'sending') return null;
    const id = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    pendingWorkspaceMessage.value = {
      id,
      content: normalized,
      created_at: new Date().toISOString(),
      status: 'pending'
    };
    return pendingWorkspaceMessage.value;
  }

  async function consumeWorkspaceMessage() {
    const pending = pendingWorkspaceMessage.value;
    if (!pending || pending.status === 'sending') return;

    pending.status = 'sending';
    await controller.initialize();
    if (!controller.slot.value?.healthy) {
      pending.status = 'failed';
      return;
    }

    controller.newConversation();
    controller.input.value = pending.content;
    const sent = await controller.send(pending.id);
    if (pendingWorkspaceMessage.value?.id !== pending.id) return;
    if (sent) pendingWorkspaceMessage.value = null;
    else pending.status = 'failed';
  }

  function show(target: AiAssistantView) {
    view.value = target;
  }

  function resetStore() {
    close();
    pendingWorkspaceMessage.value = null;
    controller.reset();
  }

  return {
    visible,
    view,
    pendingWorkspaceMessage,
    open,
    close,
    toggle,
    show,
    queueWorkspaceMessage,
    consumeWorkspaceMessage,
    resetStore
  };
});
