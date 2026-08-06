import { platformPage as page } from './page';

export const aiPages = [
  page('ai_agent-builder', () => import('@/views/ai/agent-builder/index.vue')),
  page('ai_agent-slots', () => import('@/views/ai/agent-slots/index.vue')),
  page('ai_agents', () => import('@/views/ai/agents/index.vue')),
  page('ai_knowledge', () => import('@/views/ai/knowledge/index.vue')),
  page('ai_models', () => import('@/views/ai/models/index.vue')),
  page('ai_providers', () => import('@/views/ai/providers/index.vue')),
  page('ai_workspace', () => import('@/views/ai/workspace/index.vue'))
] as const;
