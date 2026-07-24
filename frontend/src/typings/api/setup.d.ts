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
      redis_addr: string;
      redis_reachable: boolean;
      redis_error?: string;
    }

    /** DB 测试结果 */
    interface TestDBResult {
      ok: boolean;
      error?: string;
    }

    interface RedisOptions {
      addr: string;
      db: number;
      jobs_enabled: boolean;
      embedded_worker: boolean;
    }

    /** 初始化请求参数 */
    interface InitParams {
      dsn?: string;
      tenant_name: string;
      username: string;
      password: string;
      redis_addr: string;
      redis_db: number;
      jobs_enabled: boolean;
      embedded_worker: boolean;
    }

    /** 初始化结果 */
    interface InitResult {
      installed: boolean;
      installed_at: string;
      jobs_enabled: boolean;
      embedded_worker: boolean;
    }
  }
}
