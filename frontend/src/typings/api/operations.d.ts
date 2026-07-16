declare namespace Api {
  namespace Operations {
    interface PageQuery { page?: number; page_size?: number }
    interface JobRun {
      id: string; job_type: string; queue: string; status: string; progress: number;
      progress_message?: string; attempt: number; max_attempts: number; last_error?: string;
      created_at: string; updated_at: string; started_at?: string; finished_at?: string; cancel_requested_at?: string;
    }
    interface JobListData { items: JobRun[]; total: number; page: number; page_size: number }
    interface OutboxEvent {
      id: string; event_type: string; aggregate_type: string; aggregate_id: string; status: string;
      attempts: number; last_error?: string; occurred_at: string; available_at: string; published_at?: string;
    }
    interface OutboxListData { items: OutboxEvent[]; total: number; page: number; page_size: number }
    interface OutboxDelivery {
      id: string; handler_name: string; status: string; attempts: number; processed_at?: string; last_error?: string;
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
  }
}
