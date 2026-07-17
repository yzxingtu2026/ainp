export const AI_ACTIONS = {
  providerList: 'ai.provider_list',
  providerCreate: 'ai.provider_create',
  providerUpdate: 'ai.provider_update',
  providerStatus: 'ai.provider_status_update',
  providerDiscover: 'ai.provider_discover_models',
  deploymentList: 'ai.deployment_list',
  deploymentCreate: 'ai.deployment_create',
  deploymentImport: 'ai.deployment_import',
  deploymentUpdate: 'ai.deployment_update',
  deploymentStatus: 'ai.deployment_status_update',
  deploymentDelete: 'ai.deployment_delete',
  aliasList: 'ai.alias_list',
  aliasUpsert: 'ai.alias_upsert',
  aliasDelete: 'ai.alias_delete',
  modelGenerate: 'ai.model_generate',
  modelStream: 'ai.model_stream',
  agentCreate: 'ai.agent_create',
  agentUpdate: 'ai.agent_update',
  agentDuplicate: 'ai.agent_duplicate',
  agentStatus: 'ai.agent_status_update',
  agentToolBind: 'ai.agent_tool_bind',
  agentToolUnbind: 'ai.agent_tool_unbind',
  agentBuiltinReconcile: 'ai.agent_builtin_reconcile'
} as const;
export const MODEL_TYPES = ['chat', 'embedding', 'rerank', 'image', 'speech', 'transcription'] as const;
export const MODEL_MODALITIES = ['text', 'image', 'audio', 'file'] as const;
export const MODEL_FEATURES = [
  'streaming',
  'tool_calling',
  'parallel_tool_calls',
  'structured_output',
  'vision',
  'embeddings',
  'image_generation',
  'audio_input',
  'audio_output'
] as const;
