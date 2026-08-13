declare namespace Api.AI {
  type AgentSource = 'builtin' | 'tenant' | 'personal';
  type AgentVisibility = 'private' | 'tenant';
  interface Agent {
    id: string;
    code: string;
    name: string;
    description: string;
    avatar_attachment_id?: string | null;
    owner_user_id?: string;
    contract_key?: string;
    contract_version?: string;
    source: AgentSource;
    visibility: AgentVisibility;
    status: 'draft' | 'enabled' | 'disabled';
    current_version: number;
    /** Builder 草稿：与元数据一同保存，正式发布后清空；Workspace 不消费 */
    has_draft?: boolean;
    draft_tenant_instructions?: string;
    draft_model_alias?: string;
    draft_generation_config?: Record<string, unknown>;
    draft_tool_policy?: Record<string, unknown>;
    draft_memory_policy?: MemoryPolicy | null;
    revision: number;
    created_at: string;
    updated_at: string;
  }
  interface AgentDetail extends Agent {
    base_instructions?: string;
    managed_fields?: string[];
    required_tool_selectors?: string[];
  }
  type AgentVersionKind = 'release' | 'preview';
  interface AgentVersion {
    id: string;
    agent_id: string;
    version: number;
    kind?: AgentVersionKind;
    content_hash?: string;
    model_alias: string;
    tenant_instructions?: string;
    generation_config: Record<string, unknown>;
    tool_policy: Record<string, unknown>;
    memory_policy: MemoryPolicy;
    memory_policy_source: 'agent_version' | 'tenant_default' | 'system_default';
    created_by: string;
    created_at: string;
  }
  interface MemoryPolicy {
    enabled: boolean;
    extraction_model_alias: string;
    consolidation_model_alias?: string;
    auto_save_low_sensitive: boolean;
    write_approval_mode: 'always' | 'sensitive_only' | 'never';
    agent_specific_scope: boolean;
    max_candidates_per_turn: number;
    max_consolidation_items: number;
    max_recall_items: number;
    max_recall_tokens: number;
    policy_version: string;
    prompt_version: string;
    schema_version: string;
  }
  interface AgentToolBinding {
    id: string;
    agent_id: string;
    tool_id: string;
    source: string;
    status: string;
    created_by: string;
    created_at: string;
    required: boolean;
    name?: string;
    binding_mode?: 'forbidden' | 'optional';
    loading_mode?: 'direct' | 'conditional' | 'progressive';
    binding_group?: string;
  }
  interface AgentKnowledgeBinding {
    id: string;
    agent_id: string;
    knowledge_base_id: string;
    created_by: string;
    created_at: string;
  }
  interface AgentDiagnosticIssue {
    code: string;
    resource?: string;
    message: string;
  }
  interface AgentDiagnostics {
    healthy: boolean;
    issues: AgentDiagnosticIssue[];
  }
  interface AgentSlot {
    key: string;
    version: string;
    module: string;
    title: string;
    description: string;
    default_contract_key: string;
    default_contract_version: string;
    required_capabilities: string[];
    tenant_override: boolean;
    user_override: boolean;
    fallback_policy: 'fail_closed';
    upgrade_policy: 'pinned' | 'compatible';
  }
  interface AgentSlotResolution {
    slot: AgentSlot;
    agent?: Agent;
    agent_version?: AgentVersion;
    source: 'user' | 'tenant' | 'builtin';
    healthy: boolean;
    diagnostics: AgentDiagnosticIssue[];
    binding_revision?: number;
    preference_revision?: number;
  }
  type ConversationVisibility = 'private' | 'tenant';
  type ConversationPurpose = 'workspace' | 'builder_preview';
  interface Conversation {
    id: string;
    owner_user_id: string;
    title: string;
    agent_id: string;
    agent_version_id: string;
    purpose?: ConversationPurpose;
    visibility: ConversationVisibility;
    status: 'active' | 'archived';
    last_message_at?: string;
    revision: number;
    created_at: string;
    updated_at: string;
  }
  interface PersonalizationSuggestion {
    title: string;
    prompt: string;
    source_type: 'general' | 'recent_work' | 'recent_conversation' | 'memory_profile';
    target_ref?: string | null;
  }
  interface AssistantPersonalization {
    greeting: string;
    suggestions: PersonalizationSuggestion[];
    freshness: 'fresh' | 'stale' | 'missing' | 'disabled';
    generated_at?: string | null;
    context_scope: 'global' | 'page';
  }
  interface ConversationContentPart {
    type: string;
    text?: string;
    image_attachment_id?: string;
    file_attachment_id?: string;
    attachment_revision?: string;
    detail?: string;
    tool_call_id?: string;
    tool_name?: string;
    presentation?: ToolPresentation;
    arguments?: unknown;
    result?: unknown;
  }
  interface ConversationMessage {
    id: string;
    conversation_id: string;
    role: 'user' | 'assistant' | 'system' | 'tool';
    status: 'pending' | 'committed' | 'failed' | 'cancelled';
    sequence: number;
    reply_to_message_id?: string;
    run_id?: string;
    author_user_id?: string;
    content: ConversationContentPart[];
    created_at: string;
    committed_at?: string;
  }
  interface MessageCursorPage {
    items: ConversationMessage[];
    next_sequence: number;
    has_more: boolean;
  }
  type AgentRunStatus = 'queued' | 'running' | 'waiting_confirmation' | 'paused' | 'completed' | 'failed' | 'cancelled';
  type AgentRunExecutionMode = 'normal' | 'completion_only';
  interface AgentRunBudget {
    schema_version: number;
    max_steps: number;
    max_model_calls: number;
    max_tool_calls: number;
    max_compactions: number;
    max_duration_seconds: number;
    run_token_budget: number;
  }
  interface PageContextResourceRef {
    resource_type: string;
    resource_id: string;
    revision?: string;
  }
  interface PageContextEnvelope {
    schema_version: '1.0';
    page_type: string;
    route_name: string;
    mode: 'list' | 'detail' | 'create' | 'edit';
    resource_refs?: PageContextResourceRef[];
    ui_state?: Record<string, string | number | boolean | null | Array<string | number | boolean | null>>;
    title?: string;
    summary?: string;
    tool_hints?: { capabilities?: string[]; tags?: string[]; queries?: string[] };
  }
  interface AgentRun {
    id: string;
    user_id: string;
    conversation_id: string;
    conversation_revision: number;
    trigger_message_id: string;
    agent_version_id: string;
    context_snapshot_id?: string;
    status: AgentRunStatus;
    execution_mode?: AgentRunExecutionMode;
    completion_reason?: string;
    completion_attempted?: boolean;
    completion_reserved_input_tokens?: number;
    completion_reserved_output_tokens?: number;
    completion_safety_margin_tokens?: number;
    budget: AgentRunBudget;
    current_step: number;
    model_calls: number;
    tool_calls: number;
    compactions: number;
    input_tokens: number;
    output_tokens: number;
    error_category?: string;
    revision: number;
    created_at: string;
    updated_at: string;
    started_at?: string;
    finished_at?: string;
    page_context_type?: string;
    page_context_status: 'none' | 'resolved';
    page_context_bytes: number;
  }
  interface AgentStep {
    id: string;
    run_id: string;
    step_index: number;
    type: string;
    status: string;
    parent_step_id?: string;
    model_invocation_id?: string;
    tool_invocation_id?: string;
    confirmation_id?: string;
    context_snapshot_id?: string;
    input_summary: Record<string, unknown>;
    output_summary: Record<string, unknown>;
    error_category?: string;
    created_at: string;
    started_at?: string;
    finished_at?: string;
  }
  interface AgentRunDetail {
    run: AgentRun;
    steps: AgentStep[];
  }
  interface AgentRunEvent {
    id: string;
    run_id: string;
    sequence: number;
    type: string;
    step_id?: string;
    payload: Record<string, unknown>;
    created_at: string;
  }
  interface AgentRunEventPage {
    items: AgentRunEvent[];
    next_sequence: number;
    has_more: boolean;
  }
  type Status = 'enabled' | 'disabled';
  interface Provider {
    id: string;
    code: string;
    name: string;
    protocol: string;
    base_url: string;
    credential_configured: boolean;
    status: Status;
    config_json: Record<string, unknown>;
    created_at: string;
    updated_at: string;
  }
  interface Deployment {
    id: string;
    provider_id: string;
    code: string;
    remote_model_id: string;
    display_name: string;
    model_type: string;
    capabilities_json: {
      input_modalities: string[];
      output_modalities: string[];
      features: string[];
      structured_output_strategies?: string[];
    };
    limits_json: { context_window?: number; max_output_tokens?: number };
    status: Status;
    created_at: string;
    updated_at: string;
  }
  interface Alias {
    id: string;
    code: string;
    deployment_id: string;
    created_at: string;
    updated_at: string;
  }
  interface RemoteModel {
    id: string;
    display_name: string;
    suggested_type?: string;
    suggested_capabilities: { input_modalities: string[]; output_modalities: string[]; features: string[] };
    metadata_confirmed: boolean;
    imported: boolean;
  }
  interface ToolCandidate {
    id: string;
    name: string;
    summary: string;
    source: string;
    capability: string;
    risk: string;
    side_effect: string;
    input_hints: string[];
    binding_mode: 'forbidden' | 'optional';
    loading_mode: 'direct' | 'conditional' | 'progressive';
    binding_group?: string;
    score: number;
  }
  interface ToolDefinition {
    id: string;
    source: string;
    name: string;
    title_for_human: string;
    description_for_llm: string;
    description_for_human: string;
    capability: string;
    tags?: string[];
    input_schema: Record<string, unknown>;
    output_schema?: Record<string, unknown>;
    risk: string;
    side_effect: string;
    requires_confirmation: boolean;
    binding_mode: 'forbidden' | 'optional';
    loading_mode: 'direct' | 'conditional' | 'progressive';
    binding_group?: string;
    schema_version: number;
    schema_hash: string;
    status: string;
  }
  interface LoadedTool {
    definition: ToolDefinition;
    handle: string;
    expires_at: number;
  }
  interface ToolSearchResponse {
    scope_id: string;
    items: ToolCandidate[];
  }
  interface ToolLoadResponse {
    scope_id: string;
    tools: LoadedTool[];
    newly_loaded_tool_ids: string[];
    already_loaded_tool_ids: string[];
  }
  interface ToolResult {
    call_id: string;
    status: string;
    summary: string;
    data?: unknown;
    resource_refs?: string[];
    truncated: boolean;
    error_category?: string;
  }
  interface ToolPresentation {
    title: string;
    description?: string;
  }
  interface ConfirmationPresentationDetail {
    label: string;
    value: string;
    sensitive?: boolean;
  }
  interface ConfirmationPresentation {
    tool: ToolPresentation;
    summary: string;
    details?: ConfirmationPresentationDetail[];
  }
  interface ToolConfirmation {
    id: string;
    scope_id: string;
    invocation_id: string;
    tool_call_id: string;
    tool_id: string;
    arguments_summary: Record<string, unknown>;
    presentation_version: number;
    presentation?: ConfirmationPresentation;
    risk: string;
    side_effect: string;
    status: string;
    expires_at: string;
    created_at: string;
  }
  interface Page<T> {
    items: T[];
    total: number;
    page: number;
    page_size: number;
  }
  type KnowledgeBaseStatus = 'draft' | 'enabled' | 'disabled';
  type KnowledgeVisibility = 'private' | 'tenant';
  interface ChunkingConfig {
    strategy: 'fixed_tokens';
    chunk_size: number;
    chunk_overlap: number;
  }
  interface RetrievalConfig {
    top_k: number;
    min_score: number;
    max_context_tokens: number;
  }
  interface KnowledgeBase {
    id: string;
    owner_user_id: string;
    code: string;
    name: string;
    description: string;
    visibility: KnowledgeVisibility;
    status: KnowledgeBaseStatus;
    embedding_alias: string;
    chunking_config: ChunkingConfig;
    retrieval_config: RetrievalConfig;
    current_index_version_id?: string | null;
    revision: number;
    created_at: string;
    updated_at: string;
  }
  interface KnowledgeBaseCreate {
    name: string;
    description: string;
    visibility: KnowledgeVisibility;
    embedding_alias: string;
    chunking_config: ChunkingConfig;
    retrieval_config: RetrievalConfig;
  }
  interface KnowledgeBaseUpdate extends KnowledgeBaseCreate {
    id: string;
    revision: number;
  }
  type KnowledgeDocumentStatus = 'pending' | 'processing' | 'ready' | 'failed';
  interface KnowledgeDocument {
    id: string;
    knowledge_base_id: string;
    attachment_id: string;
    source_revision: string;
    content_type: string;
    original_name: string;
    ingestion_status: KnowledgeDocumentStatus;
    ingestion_error_category?: string;
    current_index_version_id?: string | null;
    revision: number;
    created_at: string;
    updated_at: string;
  }
  interface Citation {
    knowledge_base_id: string;
    document_id: string;
    chunk_id: string;
    index_version_id: string;
    ordinal: number;
    source_revision: string;
    page_number?: number;
    position_start?: number;
    position_end?: number;
    heading_path?: string;
    title: string;
    attachment_id: string;
    stale: boolean;
  }
  interface RetrievalResult {
    citation: Citation;
    score: number;
    text: string;
    untrusted: boolean;
  }
  interface CitationLookup {
    citation: Citation;
    available: boolean;
    stale: boolean;
  }
}
