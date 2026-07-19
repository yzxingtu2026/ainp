import { request } from '../request';

/**
 * Login — AINP POST /api/v1/system/auth/login
 */
export function fetchLogin(userName: string, password: string) {
  return request<Api.Auth.LoginToken>({
    url: '/api/v1/system/auth/login',
    method: 'post',
    data: {
      username: userName,
      password
    }
  }).then(res => {
    if (res.error || !res.data) {
      return res as typeof res & { data: Api.Auth.LoginToken | null };
    }

    // createFlatRequest 已 transform 为 data；此处兼容原始信封被误传的情况
    const payload = res.data as unknown as Api.Auth.AinPLoginData & Api.Auth.LoginToken;
    const token = payload.token || payload.access_token;
    const mapped: Api.Auth.LoginToken = {
      token,
      refreshToken: payload.refreshToken || payload.refresh_token || ''
    };
    return { data: mapped, error: null, response: res.response };
  });
}

/** Get user info — AINP GET /api/v1/system/auth/me */
export function fetchGetUserInfo() {
  return request<Api.Auth.UserInfo>({ url: '/api/v1/system/auth/me' }).then(async res => {
    if (res.error || !res.data) {
      return res as typeof res & { data: Api.Auth.UserInfo | null };
    }

    const profile = res.data as unknown as Api.Auth.AinPUserProfile & Api.Auth.UserInfo;

    // 并行拉菜单按钮权限
    const menusRes = await request<Api.Route.AinPMenusData>({ url: '/api/v1/menus' });
    const buttons =
      !menusRes.error && menusRes.data
        ? (menusRes.data.buttons || []).map(b => b.permission || b.bind_action || '').filter(Boolean)
        : profile.buttons || profile.permissions || [];

    const permissionButtons = Array.from(new Set([...(profile.permissions || []), ...buttons].filter(Boolean)));

    const mapped: Api.Auth.UserInfo = {
      realm: profile.realm,
      userId: profile.userId || profile.id,
      userName: profile.userName || profile.display_name || profile.username,
      roles: profile.roles || [],
      buttons: permissionButtons
    };

    return { data: mapped, error: null, response: res.response };
  });
}

/**
 * Phase 1 无 refresh token；失败即登出。
 */
export function fetchRefreshToken(_refreshToken: string) {
  return request<Api.Auth.LoginToken>({
    url: '/api/v1/system/auth/refresh',
    method: 'post',
    data: { refresh_token: _refreshToken }
  }).then(res => {
    if (res.error || !res.data) return res;
    const payload = res.data as unknown as Api.Auth.AinPLoginData;
    return {
      data: { token: payload.access_token, refreshToken: payload.refresh_token },
      error: null,
      response: res.response
    };
  });
}

export function fetchAuthMethods() {
  return request<{ methods: Api.Auth.AuthMethod[] }>({ url: '/api/v1/system/auth/methods' });
}

export function fetchLogout() {
  return request<{ revoked: boolean }>({ url: '/api/v1/system/auth/logout', method: 'post' });
}

export function fetchAuthSessions() {
  return request<{ sessions: Api.Auth.Session[] }>({ url: '/api/v1/system/auth/sessions' });
}

export function revokeAuthSession(sessionId: string) {
  return request<{ revoked: boolean }>({
    url: '/api/v1/system/auth/sessions/revoke', method: 'post', data: { session_id: sessionId }
  });
}

/** @deprecated mock only */
export function fetchCustomBackendError(code: string, msg: string) {
  return request({ url: '/auth/error', params: { code, msg } });
}
