declare namespace Api.Memory {
  type Status =
    | 'candidate'
    | 'pending_review'
    | 'active'
    | 'superseded'
    | 'revoked'
    | 'expired'
    | 'deleted'
    | 'rejected';
  type ItemType = 'preference' | 'fact' | 'context' | 'event';
  interface Evidence {
    id: string;
    source_kind: string;
    conversation_id?: string;
    message_id?: string;
    run_id?: string;
    agent_id?: string;
    agent_version_id?: string;
    revoked_at?: string;
    created_at: string;
  }
  interface Item {
    id: string;
    user_id: string;
    agent_id?: string;
    type: ItemType;
    status: Status;
    summary: string;
    content: string;
    confidence: number;
    sensitivity: 'normal' | 'sensitive' | 'restricted';
    importance: number;
    revision: number;
    occurred_at?: string;
    occurred_end_at?: string;
    occurred_timezone: string;
    local_date?: string;
    date_precision: string;
    observed_at: string;
    recorded_at: string;
    valid_from?: string;
    valid_to?: string;
    expires_at?: string;
    supersedes_item_id?: string;
    is_current: boolean;
    stale: boolean;
    evidence?: Evidence[];
  }
  interface ListResult {
    items: Item[];
    total: number;
    page: number;
    page_size: number;
    revision_token: string;
  }
  interface Profile {
    profile: Record<string, unknown>;
    source_items: Array<{ id: string; revision: number }>;
    source_revision?: number;
    updated_at?: string;
  }
}
