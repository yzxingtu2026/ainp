const clipboardImageExtensions: Record<string, string> = {
  'image/gif': 'gif',
  'image/jpeg': 'jpg',
  'image/png': 'png',
  'image/webp': 'webp'
};

export const clipboardImageMimes = Object.keys(clipboardImageExtensions);
export const clipboardImageExts = Object.values(clipboardImageExtensions).map(extension => `.${extension}`);

export function normalizeClipboardImage(file: File, index: number, timestamp = Date.now()): File | null {
  const extension = clipboardImageExtensions[file.type.toLowerCase()];
  if (!extension) return null;

  return new File([file], `clipboard-${timestamp}-${index + 1}.${extension}`, {
    type: file.type.toLowerCase(),
    lastModified: timestamp
  });
}
