declare namespace Api {
  namespace Operations {
    interface PageQuery { current?: number; size?: number }
    interface JobRun {
      id: string; job_type: string; queue: string; status: string; progress: number;
      progress_message?: string; attempt: number; max_attempts: number; last_error?: string;
      created_at: string; updated_at: string; started_at?: string; finished_at?: string; cancel_requested_at?: string;
    }
    interface JobListData { items: JobRun[]; total: number; current: number; size: number }
    interface OutboxEvent {
      id: string; event_type: string; aggregate_type: string; aggregate_id: string; status: string;
      attempts: number; last_error?: string; occurred_at: string; available_at: string; published_at?: string;
    }
    interface OutboxListData { items: OutboxEvent[]; total: number; current: number; size: number }
    interface OutboxDelivery {
      id: string; handler_name: string; status: string; attempts: number; processed_at?: string; last_error?: string;
    }
    interface OutboxDetail {
      event: OutboxEvent & { payload: unknown; metadata: unknown };
      deliveries: OutboxDelivery[];
    }
  }
}
