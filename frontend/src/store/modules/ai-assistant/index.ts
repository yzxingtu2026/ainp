import { nextTick, ref } from 'vue';
import { defineStore } from 'pinia';
import { SetupStoreId } from '@/enum';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';

export type AiAssistantView = 'conversation' | 'history' | 'run';

export const useAiAssistantStore = defineStore(SetupStoreId.AiAssistant, () => {
  const visible = ref(false);
  const view = ref<AiAssistantView>('conversation');
  const controller = useAiConversationController();
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

  function show(target: AiAssistantView) {
    view.value = target;
  }

  function resetStore() {
    close();
    controller.reset();
  }

  return { visible, view, open, close, toggle, show, resetStore };
});
