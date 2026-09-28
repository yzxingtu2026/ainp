/** Resolve the page entry independently of Vite's immutable asset base. */
export function resolveWebEntry(pathname: string, assetBase: string, mount: string) {
  const versioned = assetBase.startsWith('/__web/') && pathname.startsWith(assetBase);
  return { base: versioned ? assetBase : mount, versioned };
}
