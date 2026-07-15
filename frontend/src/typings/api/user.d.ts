declare namespace Api {
  namespace User {
    type ActivationStatus = 'enabled' | 'disabled';

    interface RoleBrief {
      id: string;
      code: string;
      name: string;
      status: ActivationStatus;
      is_builtin: boolean;
    }

    interface Record {
      id: string;
      username: string;
      display_name: string;
      status: ActivationStatus;
      org_id?: string;
      dept_id?: string;
      roles: RoleBrief[];
      is_admin: boolean;
      created_at: string;
      updated_at: string;
    }

    interface ListParams {
      current: number;
      size: number;
      keyword?: string;
      status?: ActivationStatus | '';
      role_id?: string;
    }

    interface ListResult {
      current: number;
      size: number;
      total: number;
      records: Record[];
    }

    interface CreateParams {
      username: string;
      display_name: string;
      password: string;
      status: ActivationStatus;
      role_ids: string[];
      org_id?: string;
      dept_id?: string;
    }

    interface UpdateParams {
      id: string;
      display_name: string;
      org_id?: string;
      dept_id?: string;
    }
  }
}
