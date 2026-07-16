import { useAuthStore } from '@/store/modules/auth';
import { localStg } from '@/utils/storage';
import { sessionStg } from '@/utils/storage';
import { getServiceBaseURL } from '@/utils/service';
import type { RequestInstanceState } from './type';

export function getAuthorization() {
  const token = localStg.get('token');
  const Authorization = token ? `Bearer ${token}` : null;

  return Authorization;
}

async function handleRefreshToken() {
  const { resetStore } = useAuthStore();
  const refreshToken = sessionStg.get('refreshToken');
  if (!refreshToken) {
    await resetStore();
    return false;
  }
  const isProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
  const { baseURL } = getServiceBaseURL(import.meta.env, isProxy);
  try {
    const response = await fetch(`${baseURL}/api/v1/system/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept-Language': localStg.get('lang') || 'zh-CN' },
      body: JSON.stringify({ refresh_token: refreshToken })
    });
    const envelope = await response.json();
    if (!response.ok || String(envelope.code) !== String(import.meta.env.VITE_SERVICE_SUCCESS_CODE))
      throw new Error('refresh failed');
    localStg.set('token', envelope.data.access_token);
    sessionStg.set('refreshToken', envelope.data.refresh_token);
    return true;
  } catch {
    await resetStore();
    return false;
  }
}

export async function handleExpiredRequest(state: RequestInstanceState) {
  if (!state.refreshTokenPromise) {
    state.refreshTokenPromise = handleRefreshToken();
  }

  const success = await state.refreshTokenPromise;

  setTimeout(() => {
    state.refreshTokenPromise = null;
  }, 1000);

  return success;
}

export function showErrorMsg(state: RequestInstanceState, message: string) {
  if (!state.errMsgStack?.length) {
    state.errMsgStack = [];
  }

  const isExist = state.errMsgStack.includes(message);

  if (!isExist) {
    state.errMsgStack.push(message);

    window.$message?.error(message, {
      onLeave: () => {
        state.errMsgStack = state.errMsgStack.filter(msg => msg !== message);

        setTimeout(() => {
          state.errMsgStack = [];
        }, 5000);
      }
    });
  }
}
