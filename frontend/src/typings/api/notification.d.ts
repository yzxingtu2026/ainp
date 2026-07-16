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

    /** Template override info (from database) */
    interface TemplateOverrideInfo {
      subject: string;
      body: string;
      enabled: boolean;
      updated_at: string;
    }

    /** Template variant with default + override */
    interface TemplateVariantItem {
      channel: string;
      locale: string;
      default_subject: string;
      default_body: string;
      has_override: boolean;
      override: TemplateOverrideInfo | null;
    }

    /** Variable field definition */
    interface VariableField {
      name: string;
      type: string;
      required: boolean;
      sensitive: boolean;
    }

    /** Merged template definition (code default + tenant override) */
    interface TemplateDefinitionItem {
      definition_key: string;
      name: string;
      description: string;
      category: string;
      name_key: string;
      description_key: string;
      priority: string;
      allowed_channels: string[];
      default_channels: string[];
      variables: VariableField[];
      templates: TemplateVariantItem[];
    }

    /** Template list response */
    interface TemplateListData {
      items: TemplateDefinitionItem[];
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

    /** Delivery list response */
    interface DeliveryListData {
      items: DeliveryItem[];
      total: number;
    }

    /** Delivery detail with message content */
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
