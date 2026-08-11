import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const DEFAULT_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

export function findAttachmentPreviewViolations(root = DEFAULT_ROOT) {
  const violations = [];
  const read = relativePath => fs.readFileSync(path.join(root, relativePath), 'utf8');
  const component = read('src/components/common/attachment-preview.vue');
  const hook = read('src/hooks/business/attachment.ts');
  const consumers = ['src/components/common/attachment-list.vue', 'src/views/system/attachments/index.vue'];

  for (const [mime, tag] of [
    ['image', '<img'],
    ['pdf', '<iframe'],
    ['video', '<video']
  ]) {
    if (!component.includes(tag)) violations.push(`AttachmentPreview 缺少 ${mime} 渲染分支`);
  }
  if (!component.includes('useAttachmentPreview')) violations.push('AttachmentPreview 必须复用 useAttachmentPreview');
  if (!component.includes('noopener,noreferrer')) violations.push('新窗口预览必须使用 noopener,noreferrer');
  if (component.includes('icon="mdi:close"')) violations.push('AttachmentPreview 禁止重复添加 Modal 已内置的关闭按钮');
  if (!component.includes('@wheel.prevent="zoomImage"')) violations.push('图片预览必须保留鼠标滚轮缩放');
  const pdfIframe = component.match(/<iframe[\s\S]*?\/>/)?.[0] || '';
  if (/\bsandbox(?:\s|=|>)/.test(pdfIframe)) violations.push('PDF iframe 禁止 sandbox，以兼容 Chrome 原生 PDF Viewer');
  if (!pdfIframe.includes('referrerpolicy="no-referrer"')) violations.push('PDF iframe 必须使用 no-referrer');
  if (!hook.includes("normalized === 'application/pdf'")) violations.push('PDF MIME 必须精确匹配 application/pdf');
  if (!hook.includes('ATTACHMENT_PREVIEW_MAX_BLOB_SIZE_BYTES')) violations.push('Blob 预览必须声明大小上限');

  for (const relativePath of consumers) {
    const source = read(relativePath);
    if (!source.includes('<AttachmentPreview')) violations.push(`${relativePath} 未复用 AttachmentPreview`);
    if (source.includes('useAttachmentPreview'))
      violations.push(`${relativePath} 绕过 AttachmentPreview 直接使用 Hook`);
    for (const tag of ['<img', '<iframe', '<video']) {
      if (source.includes(tag)) violations.push(`${relativePath} 保留了局部 ${tag} 预览分支`);
    }
  }

  return violations;
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  const violations = findAttachmentPreviewViolations();
  if (violations.length) {
    console.error(violations.map(item => `- ${item}`).join('\n'));
    process.exitCode = 1;
  } else {
    console.log('AttachmentPreview architecture check passed.');
  }
}
