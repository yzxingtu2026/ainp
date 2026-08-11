import type { CustomAxiosRequestConfig } from '@sa/axios';

export type AinPEnvelope<T = unknown> = {
  code: number | string;
  message?: string;
  msg?: string;
  message_key?: string;
  data: T;
  request_id?: string;
};

export type RequestErrorOptions = Pick<CustomAxiosRequestConfig, 'errorMode'>;

export interface RequestInstanceState {
  /** the promise of refreshing token */
  refreshTokenPromise: Promise<boolean> | null;
  /** the request error message stack */
  errMsgStack: string[];
  [key: string]: unknown;
}
