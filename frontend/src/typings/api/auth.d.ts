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
      capabilities: { challenge: boolean; redirect: boolean; label_key?: string };
    }

    interface AinPUserProfile {
      realm: 'tenant' | 'platform';
      id: string;
      username: string;
      display_name: string;
      tenant_id: string;
      tenant_name?: string;
      org_id?: string;
      dept_id?: string;
      roles: string[];
      permissions: string[];
    }
  }
}
