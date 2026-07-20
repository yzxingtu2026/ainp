import { request } from '../request';
const post = <T>(path: string, data: Record<string, unknown> = {}) =>
  request<T>({ url: `/api/v1/ai/${path}`, method: 'post', data });
export const fetchProviderTypes = () =>
  post<Array<{ protocol: string; name: string; model_discovery: boolean }>>('provider-types/list');
export const fetchAIProviders = (params: Record<string, unknown>) =>
  post<Api.AI.Page<Api.AI.Provider>>('providers/list', params);
export const createAIProvider = (data: Record<string, unknown>) => post<Api.AI.Provider>('providers/create', data);
export const updateAIProvider = (data: Record<string, unknown>) => post<Api.AI.Provider>('providers/update', data);
export const updateAIProviderStatus = (id: string, status: Api.AI.Status) =>
  post<Api.AI.Provider>('providers/status/update', { id, status });
export const discoverAIModels = (providerId: string) =>
  post<{ items: Api.AI.RemoteModel[]; next_cursor?: string }>('providers/discover-models', {
    provider_id: providerId,
    limit: 100
  });
export const fetchAIDeployments = (params: Record<string, unknown>) =>
  post<Api.AI.Page<Api.AI.Deployment>>('deployments/list', params);
export const createAIDeployment = (data: Record<string, unknown>) =>
  post<Api.AI.Deployment>('deployments/create', data);
export const updateAIDeployment = (data: Record<string, unknown>) =>
  post<Api.AI.Deployment>('deployments/update', data);
export const importAIDeployments = (items: Record<string, unknown>[]) =>
  post<Api.AI.Deployment[]>('deployments/import', { items });
export const updateAIDeploymentStatus = (id: string, status: Api.AI.Status) =>
  post<Api.AI.Deployment>('deployments/status/update', { id, status });
export const deleteAIDeployment = (id: string) => post<{ deleted: boolean }>('deployments/delete', { id });
export const fetchAIAliases = () => post<Api.AI.Alias[]>('aliases/list');
export const upsertAIAlias = (code: string, deploymentId: string) =>
  post<Api.AI.Alias>('aliases/upsert', { code, deployment_id: deploymentId });
export const deleteAIAlias = (code: string) => post<{ deleted: boolean }>('aliases/delete', { code });
export const searchAITools = (data: Record<string, unknown>) => post<Api.AI.ToolSearchResponse>('tools/search', data);
export const loadAITools = (data: Record<string, unknown>) => post<Api.AI.ToolLoadResponse>('tools/load', data);
export const executeAITool = (data: Record<string, unknown>) => post<Api.AI.ToolResult>('tools/execute', data);
export const fetchAIToolConfirmations = (scopeId?: string) =>
  post<Api.AI.ToolConfirmation[]>('tool-confirmations/list', { scope_id: scopeId || '' });
export const approveAIToolConfirmation = (id: string) => post<Api.AI.ToolResult>('tool-confirmations/approve', { id });
export const rejectAIToolConfirmation = (id: string) =>
  post<{ id: string; status: string }>('tool-confirmations/reject', { id });
export const fetchAIAgents = (params: Record<string, unknown>) =>
  post<Api.AI.Page<Api.AI.Agent>>('agents/list', params);
export const fetchAIAgent = (id: string) => post<Api.AI.AgentDetail>('agents/get', { id });
export const createAIAgent = (data: Record<string, unknown>) => post<Api.AI.Agent>('agents/create', data);
export const updateAIAgent = (data: Record<string, unknown>) => post<Api.AI.Agent>('agents/update', data);
export const duplicateAIAgent = (data: Record<string, unknown>) => post<Api.AI.Agent>('agents/duplicate', data);
export const updateAIAgentStatus = (id: string, status: string, revision: number) =>
  post<Api.AI.Agent>('agents/status/update', { id, status, revision });
export const fetchAIAgentVersions = (id: string) => post<Api.AI.AgentVersion[]>('agent-versions/list', { id });
export const fetchAIAgentVersion = (id: string) => post<Api.AI.AgentVersion>('agent-versions/get', { id });
export const createAIAgentVersion = (data: Record<string, unknown>) =>
  post<Api.AI.AgentVersion>('agent-versions/create', data);
export const publishAIAgentVersion = (data: Record<string, unknown>) =>
  post<Api.AI.AgentVersion>('agent-versions/publish', data);
export const materializeAIAgentPreview = (data: Record<string, unknown>) =>
  post<Api.AI.AgentVersion>('agents/preview/materialize', data);
export const searchAIAgentTools = (data: Record<string, unknown>) =>
  post<Api.AI.ToolCandidate[]>('agent-tools/search', data);
export const fetchAIAgentTools = (id: string) => post<Api.AI.AgentToolBinding[]>('agent-tools/list', { id });
export const bindAIAgentTool = (agentId: string, toolId: string) =>
  post<Api.AI.AgentToolBinding>('agent-tools/bind', { agent_id: agentId, tool_id: toolId });
export const unbindAIAgentTool = (agentId: string, toolId: string) =>
  post<{ deleted: boolean }>('agent-tools/unbind', { agent_id: agentId, tool_id: toolId });
export const fetchAIAgentDiagnostics = (id: string) => post<Api.AI.AgentDiagnostics>('agents/diagnostics', { id });
export const reconcileBuiltinAIAgent = () =>
  post<{ created: boolean; agent: Api.AI.Agent }>('agents/builtin/reconcile');
export const fetchAIAgentSlots = (params: Record<string, unknown>) =>
  post<Api.AI.Page<Api.AI.AgentSlotResolution>>('agent-slots/list', params);
export const fetchAIAgentSlot = (slotKey: string) =>
  post<Api.AI.AgentSlotResolution>('agent-slots/get', { slot_key: slotKey });
export const updateAIAgentSlot = (slotKey: string, agentId: string, revision = 0) =>
  post<Api.AI.AgentSlotResolution>('agent-slots/update', { slot_key: slotKey, agent_id: agentId, revision });
export const resetAIAgentSlot = (slotKey: string, revision: number) =>
  post<Api.AI.AgentSlotResolution>('agent-slots/reset', { slot_key: slotKey, revision });
export const reconcileAIAgentSlots = () => post<{ changed: boolean }>('agent-slots/reconcile');
export const createAIConversationForSlot = (slotKey: string, data: Record<string, unknown> = {}) =>
  post<Api.AI.Conversation>('conversations/create-for-slot', { slot_key: slotKey, ...data });
export const fetchAIConversations = (params: Record<string, unknown> = {}) =>
  post<Api.AI.Page<Api.AI.Conversation>>('conversations/list', params);
export const fetchAIConversation = (id: string) => post<Api.AI.Conversation>('conversations/get', { id });
export const createAIConversation = (data: Record<string, unknown>) =>
  post<Api.AI.Conversation>('conversations/create', data);
export const updateAIConversation = (id: string, title: string, revision: number) =>
  post<Api.AI.Conversation>('conversations/update', { id, title, revision });
export const archiveAIConversation = (id: string, revision: number) =>
  post<Api.AI.Conversation>('conversations/archive', { id, revision });
export const restoreAIConversation = (id: string, revision: number) =>
  post<Api.AI.Conversation>('conversations/restore', { id, revision });
export const fetchAIMessages = (conversationId: string, afterSequence = 0, limit = 50) =>
  post<Api.AI.MessageCursorPage>('messages/list', {
    conversation_id: conversationId,
    after_sequence: afterSequence,
    limit
  });
export const fetchAIMessage = (id: string) => post<Api.AI.ConversationMessage>('messages/get', { id });
export const createAIMessage = (data: Record<string, unknown>, idempotencyKey: string) =>
  request<Api.AI.ConversationMessage>({
    url: '/api/v1/ai/messages/create',
    method: 'post',
    data,
    _idempotencyKey: idempotencyKey
  } as any);
export const startAIAgentRun = (data: Record<string, unknown>, idempotencyKey: string) =>
  request<Api.AI.AgentRun>({
    url: '/api/v1/ai/agent-runs/start',
    method: 'post',
    data,
    _idempotencyKey: idempotencyKey
  } as any);
export const fetchAIAgentRun = (id: string) => post<Api.AI.AgentRunDetail>('agent-runs/get', { id });
export const fetchAIAgentRuns = (params: Record<string, unknown> = {}) =>
  post<Api.AI.Page<Api.AI.AgentRun>>('agent-runs/list', params);
export const cancelAIAgentRun = (id: string, revision: number) =>
  post<Api.AI.AgentRunDetail>('agent-runs/cancel', { id, revision });
export const resumeAIAgentRun = (id: string, revision: number) =>
  post<Api.AI.AgentRunDetail>('agent-runs/resume', { id, revision });
export const retryAIAgentRun = (id: string, revision: number) =>
  post<Api.AI.AgentRunDetail>('agent-runs/retry', { id, revision });
export const fetchAIAgentRunEvents = (id: string, afterSequence = 0, limit = 200) =>
  post<Api.AI.AgentRunEventPage>('agent-runs/events', { id, after_sequence: afterSequence, limit });
export const fetchKnowledgeBases = (params: Record<string, unknown>) =>
  post<Api.AI.Page<Api.AI.KnowledgeBase>>('knowledge-bases/list', params);
export const fetchKnowledgeBase = (id: string) => post<Api.AI.KnowledgeBase>('knowledge-bases/get', { id });
export const createKnowledgeBase = (data: Api.AI.KnowledgeBaseCreate) =>
  post<Api.AI.KnowledgeBase>('knowledge-bases/create', data as unknown as Record<string, unknown>);
export const updateKnowledgeBase = (data: Api.AI.KnowledgeBaseUpdate) =>
  post<Api.AI.KnowledgeBase>('knowledge-bases/update', data as unknown as Record<string, unknown>);
export const updateKnowledgeBaseStatus = (id: string, status: Api.AI.KnowledgeBaseStatus, revision: number) =>
  post<Api.AI.KnowledgeBase>('knowledge-bases/status/update', { id, status, revision });
export const deleteKnowledgeBase = (id: string, revision: number) =>
  post<{ deleted: boolean }>('knowledge-bases/delete', { id, revision });
export const fetchKnowledgeDocuments = (params: Record<string, unknown>) =>
  post<Api.AI.Page<Api.AI.KnowledgeDocument>>('knowledge-documents/list', params);
export const createKnowledgeDocument = (knowledgeBaseId: string, attachmentId: string) =>
  post<Api.AI.KnowledgeDocument>('knowledge-documents/create', {
    knowledge_base_id: knowledgeBaseId,
    attachment_id: attachmentId
  });
export const updateKnowledgeDocument = (id: string, attachmentId: string, revision: number) =>
  post<Api.AI.KnowledgeDocument>('knowledge-documents/update', { id, attachment_id: attachmentId, revision });
export const deleteKnowledgeDocument = (id: string, revision: number) =>
  post<{ deleted: boolean }>('knowledge-documents/delete', { id, revision });
export const retryKnowledgeDocument = (id: string) =>
  post<{ requested: boolean; document_id: string; revision: number }>('knowledge-documents/ingestion/retry', { id });
export const testKnowledgeRetrieval = (data: { knowledge_base_id: string; document_ids?: string[]; query: string }) =>
  post<{ items: Api.AI.RetrievalResult[] }>('knowledge-retrieval/test', data);
export const resolveKnowledgeCitation = (citation: Api.AI.Citation) =>
  post<Api.AI.CitationLookup>('knowledge-citations/resolve', { citation });
export const fetchAIAgentKnowledge = (agentId: string) =>
  post<Api.AI.Page<Api.AI.AgentKnowledgeBinding>>('agent-knowledge/list', {
    agent_id: agentId,
    page: 1,
    page_size: 100
  });
export const bindAIAgentKnowledge = (agentId: string, knowledgeBaseId: string) =>
  post<Api.AI.AgentKnowledgeBinding>('agent-knowledge/bind', {
    agent_id: agentId,
    knowledge_base_id: knowledgeBaseId
  });
export const unbindAIAgentKnowledge = (agentId: string, knowledgeBaseId: string) =>
  post<{ deleted: boolean }>('agent-knowledge/unbind', {
    agent_id: agentId,
    knowledge_base_id: knowledgeBaseId
  });
