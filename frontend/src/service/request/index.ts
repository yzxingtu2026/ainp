import type { AxiosError, AxiosResponse } from 'axios';
import { BACKEND_ERROR_CODE, createFlatRequest, createRequest } from '@sa/axios';
import { useAuthStore } from '@/store/modules/auth';
import { localStg } from '@/utils/storage';
import { getServiceBaseURL } from '@/utils/service';
import { $t } from '@/locales';
import { getAuthorization, handleExpiredRequest, showErrorMsg } from './shared';
import type { RequestInstanceState } from './type';

const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
const { baseURL, otherBaseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

type AinPEnvelope<T = unknown> = {
  code: number | string;
  message?: string;
  msg?: string;
  message_key?: string;
  data: T;
  request_id?: string;
};

function backendMessage(payload?: AinPEnvelope | null): string {
  return payload?.message || payload?.msg || '';
}

/** 读取当前前端语言，返回后端可识别的 Accept-Language 值 */
function getAcceptLanguage(): string {
  const lang = localStg.get('lang') || 'zh-CN';
  return lang;
}

export const request = createFlatRequest(
  {
    baseURL,
    headers: {}
  },
  {
    defaultState: {
      errMsgStack: [],
      refreshTokenPromise: null
    } as RequestInstanceState,
    transform(response: AxiosResponse<AinPEnvelope<any>>) {
      return response.data.data;
    },
    async onRequest(config) {
      const Authorization = getAuthorization();
      Object.assign(config.headers, { Authorization, 'Accept-Language': getAcceptLanguage() });

      return config;
    },
    isBackendSuccess(response) {
      return String(response.data.code) === String(import.meta.env.VITE_SERVICE_SUCCESS_CODE);
    },
    async onBackendFail(response, instance) {
      const authStore = useAuthStore();
      const responseCode = String(response.data.code);
      const msg = backendMessage(response.data);

      function handleLogout() {
        authStore.resetStore();
      }

      function logoutAndCleanup() {
        handleLogout();
        window.removeEventListener('beforeunload', handleLogout);
        request.state.errMsgStack = request.state.errMsgStack.filter(item => item !== msg);
      }

      const logoutCodes = import.meta.env.VITE_SERVICE_LOGOUT_CODES?.split(',').filter(Boolean) || [];
      if (logoutCodes.includes(responseCode)) {
        handleLogout();
        return null;
      }

      const modalLogoutCodes = import.meta.env.VITE_SERVICE_MODAL_LOGOUT_CODES?.split(',').filter(Boolean) || [];
      if (modalLogoutCodes.includes(responseCode) && msg && !request.state.errMsgStack?.includes(msg)) {
        request.state.errMsgStack = [...(request.state.errMsgStack || []), msg];
        window.addEventListener('beforeunload', handleLogout);
        window.$dialog?.error({
          title: $t('common.error'),
          content: msg,
          positiveText: $t('common.confirm'),
          maskClosable: false,
          closeOnEsc: false,
          onPositiveClick() {
            logoutAndCleanup();
          },
          onClose() {
            logoutAndCleanup();
          }
        });
        return null;
      }

      const expiredTokenCodes = import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(',').filter(Boolean) || [];
      if (expiredTokenCodes.includes(responseCode)) {
        const success = await handleExpiredRequest(request.state);
        if (success) {
          const Authorization = getAuthorization();
          Object.assign(response.config.headers, { Authorization, 'Accept-Language': getAcceptLanguage() });
          return instance.request(response.config) as Promise<AxiosResponse>;
        }
      }

      return null;
    },
    onError(error) {
      const authStore = useAuthStore();
      const axiosError = error as AxiosError<AinPEnvelope>;
      const payload = axiosError.response?.data;

      if (axiosError.response?.status === 401) {
        authStore.resetStore();
        showErrorMsg(request.state, backendMessage(payload) || 'unauthorized');
        return;
      }

      if (axiosError.response?.status === 403) {
        showErrorMsg(request.state, backendMessage(payload) || 'forbidden');
        return;
      }

      // HTTP 409/其它业务失败走 Axios error 通道时，优先展示信封 message
      let message = backendMessage(payload) || error.message;
      let backendErrorCode = '';

      if (error.code === BACKEND_ERROR_CODE) {
        message = backendMessage(payload) || message;
        backendErrorCode = String(payload?.code || '');
      } else if (payload && payload.code !== undefined && payload.code !== null) {
        backendErrorCode = String(payload.code);
      }

      const modalLogoutCodes = import.meta.env.VITE_SERVICE_MODAL_LOGOUT_CODES?.split(',').filter(Boolean) || [];
      if (modalLogoutCodes.includes(backendErrorCode)) {
        return;
      }

      const expiredTokenCodes = import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(',').filter(Boolean) || [];
      if (expiredTokenCodes.includes(backendErrorCode)) {
        return;
      }

      showErrorMsg(request.state, message);
    }
  }
);

export const demoRequest = createRequest(
  {
    baseURL: otherBaseURL.demo
  },
  {
    transform(response: AxiosResponse<App.Service.DemoResponse>) {
      return response.data.result;
    },
    async onRequest(config) {
      const { headers } = config;
      const token = localStg.get('token');
      const Authorization = token ? `Bearer ${token}` : null;
      Object.assign(headers, { Authorization, 'Accept-Language': getAcceptLanguage() });
      return config;
    },
    isBackendSuccess(response) {
      return response.data.status === '200';
    },
    async onBackendFail(_response) {},
    onError(error) {
      let message = error.message;
      if (error.code === BACKEND_ERROR_CODE) {
        message = error.response?.data?.message || message;
      }
      window.$message?.error(message);
    }
  }
);
