export const CODESET_KEYS = {
  activationStatus: 'common.activation_status',
  attachmentStatus: 'system.attachment_status',
  agentRunStatus: 'ai.agent_run_status',
  knowledgeBaseStatus: 'ai.knowledge_base_status',
  knowledgeDocumentIngestionStatus: 'ai.knowledge_document_ingestion_status',
  knowledgeIndexVersionStatus: 'ai.knowledge_index_version_status'
} as const;

export const ACTIVATION_STATUS = {
  enabled: 'enabled',
  disabled: 'disabled'
} as const;

export type ActivationStatus = (typeof ACTIVATION_STATUS)[keyof typeof ACTIVATION_STATUS];
