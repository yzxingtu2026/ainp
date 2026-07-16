declare namespace Api {
  namespace Operations {
    interface PageQuery {
      page?: number;
      page_size?: number;
    }
    interface JobRun {
      id: string;
      job_type: string;
      queue: string;
      status: string;
      progress: number;
      progress_message?: string;
      attempt: number;
      max_attempts: number;
      last_error?: string;
      created_at: string;
      updated_at: string;
      started_at?: string;
      finished_at?: string;
      cancel_requested_at?: string;
    }
    interface JobListData {
      items: JobRun[];
      total: number;
      page: number;
      page_size: number;
    }
    interface OutboxEvent {
      id: string;
      event_type: string;
      aggregate_type: string;
      aggregate_id: string;
      status: string;
      attempts: number;
      last_error?: string;
      occurred_at: string;
      available_at: string;
      published_at?: string;
    }
    interface OutboxListData {
      items: OutboxEvent[];
      total: number;
      page: number;
      page_size: number;
    }
    interface OutboxDelivery {
      id: string;
      handler_name: string;
      status: string;
      attempts: number;
      processed_at?: string;
      last_error?: string;
    }
    interface OutboxDetail {
      event: OutboxEvent & { payload: unknown; metadata: unknown };
      deliveries: OutboxDelivery[];
    }
    interface DeliveryItem {
      id: string;
      tenant_id: string;
      request_id: string;
      recipient_type: string;
      recipient_id: string;
      recipient_name: string;
      channel: string;
      provider: string;
      status: string;
      attempts: number;
      max_attempts: number;
      external_id: string;
      last_error: string;
      sent_at: string | null;
      delivered_at: string | null;
      failed_at: string | null;
      scheduled_at: string | null;
      created_at: string;
      updated_at: string;
    }
    interface DeliveryListData {
      items: DeliveryItem[];
      total: number;
      page: number;
      page_size: number;
    }
    interface DeliveryDetail extends DeliveryItem {
      locale: string;
      definition_key: string;
      variables: Record<string, unknown>;
      content: {
        title: string;
        summary: string;
        body: string;
      };
    }
    interface AuditLogQuery extends PageQuery {
      start_at?: string;
      end_at?: string;
      user_keyword?: string;
      actor_type?: string;
      action_id?: string;
      result?: string;
      resource_type?: string;
      resource_id?: string;
      request_id?: string;
      client?: string;
    }
    interface AuditLog {
      id: string;
      occurred_at: string;
      user_id: string;
      actor_name: string;
      action_name: string;
      actor_type: string;
      action_id: string;
      result: string;
      request_id: string;
      permission: string;
      error_code: number;
      resource_type: string;
      resource_id: string;
      client: string;
      summary: string;
      replay: boolean;
    }
    interface AuditLogListData {
      items: AuditLog[];
      total: number;
      page: number;
      page_size: number;
    }
    interface LogSource {
      name: string;
      display_name: string;
      capabilities: { tenant_filter: boolean; levels?: string[]; description?: string };
      available: boolean;
      status: string;
    }
    interface LogQuery extends PageQuery {
      source: string;
      start_at?: string;
      end_at?: string;
      level?: string;
      service?: string;
      component?: string;
      keyword?: string;
      request_id?: string;
      trace_id?: string;
      action_id?: string;
      job_type?: string;
      job_run_id?: string;
      event_id?: string;
    }
    interface LogEntry {
      id: string;
      timestamp: string;
      level: string;
      service?: string;
      component?: string;
      message: string;
      request_id?: string;
      trace_id?: string;
      tenant_id?: string;
      user_id?: string;
      action_id?: string;
      job_type?: string;
      job_run_id?: string;
      event_id?: string;
      source: string;
      attributes?: Record<string, unknown>;
    }
    interface LogPage {
      items: LogEntry[];
      total: number;
      page: number;
      page_size: number;
    }
  }
}
