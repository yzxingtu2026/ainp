export async function copyPlainText(value: string): Promise<boolean> {
  if (!value) return false;
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Fall through to selection-based copy when Clipboard API is restricted.
  }

  if (typeof document === 'undefined') return false;
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', 'true');
  textarea.style.position = 'fixed';
  textarea.style.top = '-9999px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  try {
    textarea.select();
    return typeof document.execCommand === 'function' ? document.execCommand('copy') : false;
  } catch {
    return false;
  } finally {
    textarea.remove();
  }
}
