declare namespace Api {
  namespace Setting {
    type FieldType = 'text' | 'textarea' | 'number' | 'boolean' | 'select' | 'multi_select' | 'color';
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
  }
}
