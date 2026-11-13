import { nextTick, ref, watch } from 'vue';
import { defineStore } from 'pinia';
import { SetupStoreId } from '@/enum';
import { useAppStore } from '@/store/modules/app';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';

export type AiAssistantView = 'conversation' | 'history' | 'run';

export const useAiAssistantStore = defineStore(SetupStoreId.AiAssistant, () => {
  const appStore = useAppStore();
  const visible = ref(false);
  const view = ref<AiAssistantView>('conversation');
  const controller = useAiConversationController();
  let returnFocus: HTMLElement | null = null;
  let siderCollapseToRestore: boolean | null = null;

  // the panel only auto-collapses the sider; any expansion while it is open is a manual choice
  watch(
    () => appStore.siderCollapse,
    collapsed => {
      if (visible.value && siderCollapseToRestore !== null && !collapsed) {
        siderCollapseToRestore = null;
      }
    }
  );

  function open(target: AiAssistantView = 'conversation') {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    view.value = target;
    const wasVisible = visible.value;
    visible.value = true;
    if (!wasVisible && !appStore.isMobile && !appStore.fullContent && !appStore.siderCollapse) {
      siderCollapseToRestore = appStore.siderCollapse;
      appStore.setSiderCollapse(true);
    }
    void controller.initialize();
  }

  function close() {
    visible.value = false;
    view.value = 'conversation';
    const restore = siderCollapseToRestore;
    siderCollapseToRestore = null;
    if (restore !== null && !appStore.isMobile && !appStore.fullContent) {
      appStore.setSiderCollapse(restore);
    }
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

  return {
    visible,
    view,
    open,
    close,
    toggle,
    show,
    resetStore
  };
});
