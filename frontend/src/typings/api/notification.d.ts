declare namespace Api {
  namespace Notification {
    /** Single notification item */
    interface NotificationItem {
      id: string;
      title: string;
      summary: string;
      body: string;
      icon: string;
      local_icon: string;
      action_json: {
        type: 'route' | 'action';
        target: string;
        label?: string;
      } | null;
      read_at: string | null;
      archived_at: string | null;
      created_at: string;
    }

    /** Paginated notification list response */
    interface ListData {
      items: NotificationItem[];
      total: number;
      current: number;
      size: number;
    }

    /** Unread count response */
    interface UnreadCountData {
      unread_count: number;
    }

    /** Mark read response */
    interface MarkReadData {
      id: string;
      read_at: string;
    }

    /** Mark all read response */
    interface MarkAllReadData {
      affected: number;
    }

    /** Template override item */
    interface TemplateOverride {
      id: string;
      tenant_id: string;
      definition_key: string;
      channel: string;
      locale: string;
      version: number;
      subject: string;
      body: string;
      enabled: boolean;
      updated_by: string;
      created_at: string;
      updated_at: string;
    }

    /** Template list response */
    interface TemplateListData {
      items: TemplateOverride[];
    }

    /** Template preview response */
    interface PreviewData {
      source: string;
      version: number;
      channel: string;
      locale: string;
      subject: string;
      body: string;
      title: string;
      summary: string;
      text_body: string;
      html_body: string;
      sms_text: string;
      im_text: string;
    }

    /** Delivery record item */
    interface DeliveryItem {
      id: string;
      tenant_id: string;
      request_id: string;
      recipient_type: string;
      recipient_id: string;
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

    /** Delivery list response */
    interface DeliveryListData {
      items: DeliveryItem[];
      total: number;
    }
  }
}
