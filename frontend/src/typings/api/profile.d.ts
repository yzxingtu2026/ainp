declare namespace Api {
  namespace Profile {
    interface SelfProfile {
      id: string;
      username: string;
      display_name: string;
      tenant_id: string;
      tenant_name: string;
      org_id?: string;
      dept_id?: string;
      roles: Api.Account.RoleBrief[];
      permissions: string[];
      realm: 'tenant';
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
  }
}
