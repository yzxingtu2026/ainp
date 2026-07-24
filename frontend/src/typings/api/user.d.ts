declare namespace Api {
  namespace User {
    interface SelfProfile extends Record {
      avatar_attachment_id?: string | null;
      memory_preferred_address: string;
      effective_address: string;
      address_source: 'memory_profile' | 'display_name' | 'username';
    }

    interface MCPConnectionInfo {
      name: string;
      transport: 'streamable_http';
      url: string;
      oauth_enabled: boolean;
      scopes: Array<'actions:read' | 'actions:write' | 'actions:sensitive'>;
    }

    interface MCPGrant {
      id: string;
      client_id: string;
      client_name: string;
      scopes: Array<'actions:read' | 'actions:write' | 'actions:sensitive'>;
      status: 'active' | 'revoked' | 'expired';
      expires_at: string;
      last_used_at?: string;
      created_at: string;
    }

    interface MCPConfirmation {
      id: string;
      client_name: string;
      title: string;
      description: string;
      risk: 'low' | 'medium' | 'high' | 'critical';
      side_effect: 'external' | 'paid' | 'destructive';
      status: 'pending' | 'approved' | 'rejected' | 'consumed' | 'revoked' | 'expired';
      expires_at: string;
      decided_at?: string;
    }
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
      page: number;
      page_size: number;
      keyword?: string;
      status?: ActivationStatus | '';
      role_id?: string;
    }

    interface ListResult {
      page: number;
      page_size: number;
      total: number;
      items: Record[];
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
