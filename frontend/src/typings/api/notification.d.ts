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
  }
}
