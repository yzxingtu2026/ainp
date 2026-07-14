declare namespace Api {
  namespace CodeSet {
    type Semantic = 'default' | 'info' | 'success' | 'warning' | 'danger' | 'muted';

    interface Item {
      value: string;
      label: string;
      label_key: string;
      description?: string;
      description_key?: string;
      semantic: Semantic;
      icon?: string;
      local_icon?: string;
      order: number;
      deprecated: boolean;
    }

    interface Definition {
      key: string;
      name: string;
      name_key: string;
      description?: string;
      description_key?: string;
      items: Item[];
      order: number;
      surface: 'tenant' | 'platform';
    }

    interface ListResponse {
      version: string;
      items: Definition[];
    }
  }
}
