-- Tenant data access closeout (Issue #113).
-- Composite parent keys make cross-tenant references impossible even when an
-- application query omits a tenant predicate.

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_tenant_id ON users (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_roles_tenant_id ON roles (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agents_tenant_id ON ai_agents (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agent_versions_tenant_id ON ai_agent_versions (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_conversations_tenant_id ON ai_conversations (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_messages_tenant_id ON ai_messages (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_assistant_streams_tenant_id ON ai_assistant_streams (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_context_snapshots_tenant_id ON ai_context_snapshots (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agent_runs_tenant_id ON ai_agent_runs (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_agent_steps_tenant_id ON ai_agent_steps (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_tool_scopes_tenant_id ON ai_tool_scopes (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_tool_confirmations_tenant_id ON ai_tool_confirmations (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ai_tool_invocations_tenant_id ON ai_tool_invocations (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_job_runs_tenant_id ON job_runs (tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_outbox_events_tenant_id ON outbox_events (tenant_id, id);

ALTER TABLE user_roles
  DROP CONSTRAINT IF EXISTS user_roles_user_id_fkey,
  DROP CONSTRAINT IF EXISTS user_roles_role_id_fkey,
  ADD CONSTRAINT fk_user_roles_user_tenant FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_user_roles_role_tenant FOREIGN KEY (tenant_id, role_id) REFERENCES roles (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE auth_identities
  DROP CONSTRAINT IF EXISTS auth_identities_user_id_fkey,
  ADD CONSTRAINT fk_auth_identities_user_tenant FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE auth_sessions
  DROP CONSTRAINT IF EXISTS auth_sessions_user_id_fkey,
  ADD CONSTRAINT fk_auth_sessions_user_tenant FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_agent_versions
  DROP CONSTRAINT IF EXISTS ai_agent_versions_agent_id_fkey,
  ADD CONSTRAINT fk_ai_agent_versions_agent_tenant FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_agent_versions_creator_tenant FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_agent_tool_bindings
  DROP CONSTRAINT IF EXISTS ai_agent_tool_bindings_agent_id_fkey,
  ADD CONSTRAINT fk_ai_agent_tools_agent_tenant FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_agent_tools_creator_tenant FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_conversations
  DROP CONSTRAINT IF EXISTS ai_conversations_agent_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_conversations_agent_version_id_fkey,
  ADD CONSTRAINT fk_ai_conversations_owner_tenant FOREIGN KEY (tenant_id, owner_user_id) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_conversations_agent_tenant FOREIGN KEY (tenant_id, agent_id) REFERENCES ai_agents (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_conversations_version_tenant FOREIGN KEY (tenant_id, agent_version_id) REFERENCES ai_agent_versions (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_messages
  DROP CONSTRAINT IF EXISTS ai_messages_conversation_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_messages_reply_to_message_id_fkey,
  ADD CONSTRAINT fk_ai_messages_conversation_tenant FOREIGN KEY (tenant_id, conversation_id) REFERENCES ai_conversations (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_messages_reply_tenant FOREIGN KEY (tenant_id, reply_to_message_id) REFERENCES ai_messages (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_assistant_streams
  DROP CONSTRAINT IF EXISTS ai_assistant_streams_conversation_id_fkey,
  ADD CONSTRAINT fk_ai_streams_conversation_tenant FOREIGN KEY (tenant_id, conversation_id) REFERENCES ai_conversations (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_assistant_stream_deltas
  DROP CONSTRAINT IF EXISTS ai_assistant_stream_deltas_stream_id_fkey,
  ADD CONSTRAINT fk_ai_stream_deltas_stream_tenant FOREIGN KEY (tenant_id, stream_id) REFERENCES ai_assistant_streams (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_context_snapshots
  DROP CONSTRAINT IF EXISTS ai_context_snapshots_conversation_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_context_snapshots_agent_version_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_context_snapshots_parent_snapshot_id_fkey,
  ADD CONSTRAINT fk_ai_snapshots_conversation_tenant FOREIGN KEY (tenant_id, conversation_id) REFERENCES ai_conversations (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_snapshots_version_tenant FOREIGN KEY (tenant_id, agent_version_id) REFERENCES ai_agent_versions (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_snapshots_parent_tenant FOREIGN KEY (tenant_id, parent_snapshot_id) REFERENCES ai_context_snapshots (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_tool_confirmations
  DROP CONSTRAINT IF EXISTS ai_tool_confirmations_scope_id_fkey,
  ADD CONSTRAINT fk_ai_tool_confirmations_scope_tenant FOREIGN KEY (tenant_id, scope_id) REFERENCES ai_tool_scopes (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_tool_invocations
  DROP CONSTRAINT IF EXISTS ai_tool_invocations_scope_id_fkey,
  ADD CONSTRAINT fk_ai_tool_invocations_scope_tenant FOREIGN KEY (tenant_id, scope_id) REFERENCES ai_tool_scopes (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_agent_runs
  DROP CONSTRAINT IF EXISTS ai_agent_runs_conversation_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_agent_runs_trigger_message_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_agent_runs_agent_version_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_agent_runs_context_snapshot_id_fkey,
  ADD CONSTRAINT fk_ai_runs_user_tenant FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_runs_conversation_tenant FOREIGN KEY (tenant_id, conversation_id) REFERENCES ai_conversations (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_runs_trigger_tenant FOREIGN KEY (tenant_id, trigger_message_id) REFERENCES ai_messages (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_runs_version_tenant FOREIGN KEY (tenant_id, agent_version_id) REFERENCES ai_agent_versions (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_runs_snapshot_tenant FOREIGN KEY (tenant_id, context_snapshot_id) REFERENCES ai_context_snapshots (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_agent_steps
  DROP CONSTRAINT IF EXISTS ai_agent_steps_run_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_agent_steps_parent_step_id_fkey,
  ADD CONSTRAINT fk_ai_steps_run_tenant FOREIGN KEY (tenant_id, run_id) REFERENCES ai_agent_runs (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_steps_parent_tenant FOREIGN KEY (tenant_id, parent_step_id) REFERENCES ai_agent_steps (tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE ai_agent_run_events
  DROP CONSTRAINT IF EXISTS ai_agent_run_events_run_id_fkey,
  DROP CONSTRAINT IF EXISTS ai_agent_run_events_step_id_fkey,
  ADD CONSTRAINT fk_ai_run_events_run_tenant FOREIGN KEY (tenant_id, run_id) REFERENCES ai_agent_runs (tenant_id, id) ON DELETE RESTRICT,
  ADD CONSTRAINT fk_ai_run_events_step_tenant FOREIGN KEY (tenant_id, step_id) REFERENCES ai_agent_steps (tenant_id, id) ON DELETE RESTRICT;
