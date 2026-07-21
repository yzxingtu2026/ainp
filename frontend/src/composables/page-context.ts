import { onActivated, onBeforeUnmount, onDeactivated, onMounted } from 'vue';
import { useRoute } from 'vue-router';

export type PageContextProvider = () => Api.AI.PageContextEnvelope | null;

let activeProvider: { token: symbol; provider: PageContextProvider } | null = null;

export function registerPageContextProvider(provider: PageContextProvider) {
  const token = Symbol('page-context-provider');
  activeProvider = { token, provider };
  return () => {
    if (activeProvider?.token === token) activeProvider = null;
  };
}

export function capturePageContext(): Api.AI.PageContextEnvelope | undefined {
  const snapshot = activeProvider?.provider();
  if (!snapshot) return undefined;
  return structuredClone(snapshot);
}

export function usePageContextProvider(provider: PageContextProvider) {
  const route = useRoute();
  let unregister: (() => void) | undefined;
  const routeBoundProvider: PageContextProvider = () => {
    const snapshot = provider();
    if (!snapshot || snapshot.route_name !== String(route.name || '')) return null;
    return snapshot;
  };
  const register = () => {
    unregister?.();
    unregister = registerPageContextProvider(routeBoundProvider);
  };
  const remove = () => {
    unregister?.();
    unregister = undefined;
  };
  onMounted(register);
  onActivated(register);
  onDeactivated(remove);
  onBeforeUnmount(remove);
}
