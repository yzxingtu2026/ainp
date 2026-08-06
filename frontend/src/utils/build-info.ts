export interface BuildInfo {
  product: string;
  baselineVersion: string;
  version: string;
  commit: string;
  buildTime: string;
}

export const buildInfo: Readonly<BuildInfo> = Object.freeze({
  product: import.meta.env.VITE_APP_TITLE || 'AINP',
  baselineVersion: import.meta.env.VITE_AINP_BASELINE_VERSION || import.meta.env.VITE_BUILD_VERSION || 'dev',
  version: import.meta.env.VITE_BUILD_VERSION || 'dev',
  commit: import.meta.env.VITE_BUILD_COMMIT || 'unknown',
  buildTime: import.meta.env.VITE_BUILD_TIME || 'unknown'
});

export function logBuildInfo() {
  console.info(
    `[${buildInfo.product}] version=${buildInfo.version} ainp_baseline=${buildInfo.baselineVersion} commit=${buildInfo.commit} build_time=${buildInfo.buildTime}`
  );
}
