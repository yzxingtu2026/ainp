declare namespace Api {
  namespace Role {
    type ActivationStatus = 'enabled' | 'disabled';

    interface Record {
      id: string;
      code: string;
      name: string;
      description: string;
      status: ActivationStatus;
      is_builtin: boolean;
      user_count: number;
      permissions?: string[];
      created_at: string;
      updated_at: string;
    }

    interface ListParams {
      current: number;
      size: number;
      keyword?: string;
      status?: ActivationStatus | '';
    }

    interface ListResult {
      current: number;
      size: number;
      total: number;
      records: Record[];
    }

    interface PermissionItem {
      permission: string;
      group: string;
      group_name: string;
      group_name_key?: string;
      action_ids: string[];
      name: string;
      name_key?: string;
      description?: string;
      description_key?: string;
      risk?: string;
      audit: boolean;
    }

    interface PermissionGroup {
      capability: string;
      name: string;
      name_key?: string;
      description?: string;
      description_key?: string;
      items: PermissionItem[];
    }
  }
}
