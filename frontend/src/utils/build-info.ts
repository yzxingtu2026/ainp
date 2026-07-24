export interface BuildInfo {
  version: string;
  commit: string;
  buildTime: string;
}

export const buildInfo: Readonly<BuildInfo> = Object.freeze({
  version: import.meta.env.VITE_BUILD_VERSION || 'dev',
  commit: import.meta.env.VITE_BUILD_COMMIT || 'unknown',
  buildTime: import.meta.env.VITE_BUILD_TIME || 'unknown'
});

export function logBuildInfo() {
  console.info(
    `[AINP] version=${buildInfo.version} commit=${buildInfo.commit} build_time=${buildInfo.buildTime}`
  );
}
