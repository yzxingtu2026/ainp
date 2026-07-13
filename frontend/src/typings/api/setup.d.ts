declare namespace Api {
  namespace Setup {
    /** 安装状态 */
    interface Status {
      installed: boolean;
      dsn_source: string;
      dsn_connected: boolean;
    }

    /** 环境检查 */
    interface CheckEnv {
      go_version: string;
      os: string;
      arch: string;
      dsn_source: string;
      dsn_present: boolean;
      i18n_langs: number;
      i18n_keys: number;
    }

    /** DB 测试结果 */
    interface TestDBResult {
      ok: boolean;
      error?: string;
    }

    /** 初始化请求参数 */
    interface InitParams {
      dsn?: string;
      tenant_name: string;
      username: string;
      password: string;
    }

    /** 初始化结果 */
    interface InitResult {
      installed: boolean;
      installed_at: string;
    }
  }
}
