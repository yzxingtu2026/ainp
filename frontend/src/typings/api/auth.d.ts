declare namespace Api {
  namespace Auth {
    interface LoginToken {
      token: string;
      refreshToken: string;
    }

    interface UserInfo {
      realm: 'tenant' | 'platform';
      userId: string;
      userName: string;
      avatarAttachmentId?: string | null;
      roles: string[];
      buttons: string[];
    }

    /** AINP /api/v1/system/auth/login data */
    interface AinPLoginData {
      access_token: string;
      refresh_token: string;
      token_type: string;
      expires_in: number;
      expires_at: string;
      session_id: string;
      user: AinPUserProfile;
    }

    interface AuthMethod {
      method: string;
      capabilities: {
        challenge: boolean;
        redirect: boolean;
        label?: string;
        label_key?: string;
        icon?: string;
        order?: number;
      };
    }

    interface AuthChallenge {
      challenge_id: string;
      expires_in: number;
      data?: { authorize_url?: string; provider?: string };
    }

    interface UnifiedAuthToken extends AinPLoginData {
      session_id: string;
      client_surface: string;
    }

    interface Session {
      id: string;
      client: string;
      device_summary?: string;
      created_at: string;
      last_seen_at: string;
      expires_at: string;
      is_current: boolean;
    }

    interface AinPUserProfile {
      realm: 'tenant' | 'platform';
      id: string;
      username: string;
      display_name: string;
      avatar_attachment_id?: string | null;
      tenant_id: string;
      tenant_name?: string;
      org_id?: string;
      dept_id?: string;
      roles: string[];
      permissions: string[];
    }
  }
}
