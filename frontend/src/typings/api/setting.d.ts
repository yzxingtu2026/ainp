declare namespace Api {
  namespace Setting {
    type FieldType = 'text' | 'textarea' | 'number' | 'boolean' | 'select' | 'multi_select' | 'remote_select' | 'color';
    type Source = 'default' | 'tenant';

    interface FieldOption {
      value: string;
      label: string;
      label_key?: string;
    }

    interface VisibleWhen {
      field: string;
      eq?: unknown;
      in?: unknown[];
    }

    interface RemoteOptionSource {
      endpoint: string;
      params?: Record<string, unknown>;
      response_path?: string;
      label_field: string;
      value_field: string;
    }

    interface Field {
      key: string;
      type: FieldType;
      label: string;
      label_key?: string;
      description?: string;
      description_key?: string;
      required: boolean;
      default?: unknown;
      min_length?: number;
      max_length?: number;
      min?: number;
      max?: number;
      step?: number;
      options?: FieldOption[];
      remote_source?: RemoteOptionSource;
      allow_custom_values?: boolean;
      custom_value_pattern?: string;
      visible_when?: VisibleWhen;
    }

    interface Definition {
      key: string;
      category_key: string;
      title: string;
      title_key?: string;
      description?: string;
      description_key?: string;
      permission?: string;
      order: number;
      schema_version: number;
      revision: number;
      source: Source;
      values: Record<string, unknown>;
      fields: Field[];
    }

    interface Category {
      key: string;
      title: string;
      title_key?: string;
      description?: string;
      description_key?: string;
      icon?: string;
      local_icon?: string;
      order: number;
      definitions: Definition[];
    }

    interface ListResult {
      categories: Category[];
    }

    type NavigationIndexState = 'disabled' | 'not_indexed' | 'pending' | 'ready' | 'failed' | 'degraded';
    interface NavigationIndexStatus {
      state: NavigationIndexState;
      model_alias?: string;
      total: number;
      pending: number;
      ready: number;
      failed: number;
      stale: number;
      last_updated_at?: string;
    }
    type ToolIndexStatus = NavigationIndexStatus;
  }
}
