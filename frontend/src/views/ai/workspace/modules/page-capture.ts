import { LAYOUT_SCROLL_EL_ID } from '@sa/materials';
import { domToCanvas } from 'modern-screenshot';
import { useAttachmentUploadQueue } from '@/hooks/business/attachment-upload-queue';

export const PAGE_CAPTURE_EVENT_REQUESTED = 'run.capture_requested.v1';
export const PAGE_CAPTURE_EVENT_SETTLED = 'run.capture_settled.v1';
export const PAGE_CAPTURE_KIND = 'page.capture';
export const PAGE_CAPTURE_MAX_EDGE = 1600;
export const PAGE_CAPTURE_MAX_PIXELS = 2_000_000;
export const PAGE_CAPTURE_MAX_BYTES = 4 * 1024 * 1024;
export const PAGE_CAPTURE_JPEG_QUALITY = 0.84;

const AI_ASSISTANT_PANEL_SELECTOR = '[data-ainp-ai-assistant="true"], .global-ai-assistant';
const MAIN_CONTENT_SELECTOR = '[data-ainp-main-content="true"]';
const ACTIVE_OVERLAY_SELECTOR = ['.n-modal-body-wrapper', '.n-drawer-body-content-wrapper', '.n-dialog'].join(', ');

export interface PageCaptureRequestEventPayload {
  captureRequestId: string;
  kind: string;
  toolCallId?: string;
  expiresAt?: string;
}

export function parsePageCaptureRequestEvent(event: Api.AI.AgentRunEvent): PageCaptureRequestEventPayload | null {
  if (event.type !== PAGE_CAPTURE_EVENT_REQUESTED) return null;
  const payload = event.payload || {};
  const captureRequestId = typeof payload.capture_request_id === 'string' ? payload.capture_request_id.trim() : '';
  const kind = typeof payload.kind === 'string' ? payload.kind.trim() : '';
  if (!captureRequestId || kind !== PAGE_CAPTURE_KIND) return null;
  return {
    captureRequestId,
    kind,
    toolCallId: typeof payload.tool_call_id === 'string' ? payload.tool_call_id : undefined,
    expiresAt: typeof payload.expires_at === 'string' ? payload.expires_at : undefined
  };
}

export function computeDownscaledDimensions(
  width: number,
  height: number,
  maxEdge = PAGE_CAPTURE_MAX_EDGE,
  maxPixels = PAGE_CAPTURE_MAX_PIXELS
): { width: number; height: number; scale: number } {
  const safeWidth = Math.max(1, Math.round(width));
  const safeHeight = Math.max(1, Math.round(height));
  let scale = 1;
  const longestEdge = Math.max(safeWidth, safeHeight);
  if (longestEdge > maxEdge) {
    scale = Math.min(scale, maxEdge / longestEdge);
  }
  const pixels = safeWidth * safeHeight;
  if (pixels > maxPixels) {
    scale = Math.min(scale, Math.sqrt(maxPixels / pixels));
  }
  return {
    width: Math.max(1, Math.round(safeWidth * scale)),
    height: Math.max(1, Math.round(safeHeight * scale)),
    scale
  };
}

export function isExcludedAiAssistantNode(node: Node): boolean {
  if (!(node instanceof Element)) return false;
  if (node.matches(AI_ASSISTANT_PANEL_SELECTOR)) return true;
  return Boolean(node.closest(AI_ASSISTANT_PANEL_SELECTOR));
}

export function resolveMainContentContainer(doc: Document = document): HTMLElement | null {
  const marked = doc.querySelector<HTMLElement>(MAIN_CONTENT_SELECTOR);
  if (marked && !isExcludedAiAssistantNode(marked)) return marked;
  const fallback = doc.getElementById(LAYOUT_SCROLL_EL_ID);
  if (fallback instanceof HTMLElement && !isExcludedAiAssistantNode(fallback)) return fallback;
  return null;
}

function findVisibleBusinessOverlays(doc: Document, mainRect: DOMRect): HTMLElement[] {
  const candidates = Array.from(doc.querySelectorAll<HTMLElement>(ACTIVE_OVERLAY_SELECTOR));
  const overlays: HTMLElement[] = [];
  for (const el of candidates) {
    if (isExcludedAiAssistantNode(el)) continue;
    const style = window.getComputedStyle(el);
    if (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity || '1') <= 0.02) continue;
    const rect = el.getBoundingClientRect();
    if (rect.width < 24 || rect.height < 24) continue;
    const overlapsHorizontally = rect.right > mainRect.left && rect.left < mainRect.right;
    const overlapsVertically = rect.bottom > mainRect.top && rect.top < mainRect.bottom;
    if (!overlapsHorizontally || !overlapsVertically) continue;
    if (overlays.some(existing => existing.contains(el))) continue;
    overlays.push(el);
  }
  return overlays;
}

function maskPasswordInputs(cloned: Node) {
  if (!(cloned instanceof HTMLInputElement)) return;
  if (cloned.type?.toLowerCase() === 'password' && cloned.value) {
    cloned.value = '••••••••';
  }
}

function canvasToBlob(canvas: HTMLCanvasElement, mimeType: string, quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      blob => {
        if (blob && blob.size > 0) resolve(blob);
        else reject(new Error('empty capture image'));
      },
      mimeType,
      quality
    );
  });
}

export async function renderBusinessMainContentFile(captureRequestId: string): Promise<File> {
  const mainEl = resolveMainContentContainer(document);
  if (!mainEl) {
    throw new Error('main content container unavailable');
  }
  const rect = mainEl.getBoundingClientRect();
  const viewportHeight = Math.max(1, window.innerHeight || document.documentElement.clientHeight || 1);
  const rawWidth = Math.max(1, Math.round(rect.width || mainEl.clientWidth || 1));
  const visibleBottom = Math.min(rect.bottom, viewportHeight);
  const visibleTop = Math.max(0, rect.top);
  const rawHeight = Math.max(
    1,
    Math.round(visibleBottom > visibleTop ? visibleBottom - visibleTop : rect.height || mainEl.clientHeight || 1)
  );
  const target = computeDownscaledDimensions(rawWidth, rawHeight);
  const backgroundColor =
    window.getComputedStyle(mainEl).backgroundColor &&
    window.getComputedStyle(mainEl).backgroundColor !== 'rgba(0, 0, 0, 0)'
      ? window.getComputedStyle(mainEl).backgroundColor
      : window.getComputedStyle(document.body).backgroundColor || '#ffffff';

  const scrollTop = mainEl.scrollTop || 0;
  const scrollLeft = mainEl.scrollLeft || 0;
  const baseCanvas = await domToCanvas(mainEl, {
    width: rawWidth,
    height: rawHeight,
    scale: target.scale,
    backgroundColor,
    timeout: 12000,
    filter: node => !isExcludedAiAssistantNode(node),
    onCloneEachNode: maskPasswordInputs,
    features: {
      restoreScrollPosition: scrollTop > 0 || scrollLeft > 0
    }
  });

  const outCanvas = document.createElement('canvas');
  outCanvas.width = target.width;
  outCanvas.height = target.height;
  const ctx = outCanvas.getContext('2d');
  if (!ctx) {
    throw new Error('canvas 2d context unavailable');
  }
  ctx.fillStyle = backgroundColor;
  ctx.fillRect(0, 0, target.width, target.height);
  ctx.drawImage(baseCanvas, 0, 0, target.width, target.height);

  // Teleported Naive UI modals/drawers belonging to the business page live on document.body;
  // composite any visible business overlay intersecting the main content bounds while still excluding the AI panel.
  const overlays = findVisibleBusinessOverlays(document, rect);
  if (overlays.length > 0) {
    ctx.fillStyle = 'rgba(0, 0, 0, 0.28)';
    ctx.fillRect(0, 0, target.width, target.height);
    for (const overlay of overlays) {
      try {
        const overlayRect = overlay.getBoundingClientRect();
        const ow = Math.max(1, Math.round(overlayRect.width));
        const oh = Math.max(1, Math.round(overlayRect.height));
        const overlayCanvas = await domToCanvas(overlay, {
          width: ow,
          height: oh,
          scale: target.scale,
          timeout: 8000,
          filter: node => !isExcludedAiAssistantNode(node),
          onCloneEachNode: maskPasswordInputs
        });
        const dx = Math.round((overlayRect.left - rect.left) * target.scale);
        const dy = Math.round((overlayRect.top - visibleTop) * target.scale);
        const dw = Math.max(1, Math.round(ow * target.scale));
        const dh = Math.max(1, Math.round(oh * target.scale));
        ctx.drawImage(overlayCanvas, dx, dy, dw, dh);
      } catch {
        // Best-effort overlay composite: never fail the primary main-content capture if an overlay cannot be cloned.
      }
    }
  }

  let blob = await canvasToBlob(outCanvas, 'image/jpeg', PAGE_CAPTURE_JPEG_QUALITY);
  if (blob.size > PAGE_CAPTURE_MAX_BYTES) {
    blob = await canvasToBlob(outCanvas, 'image/jpeg', 0.65);
  }
  if (blob.size > PAGE_CAPTURE_MAX_BYTES) {
    throw new Error('captured image exceeds size budget');
  }
  const safeId = captureRequestId.replace(/[^a-zA-Z0-9_-]/g, '').slice(0, 24) || 'page';
  return new File([blob], `page-capture-${safeId}.jpg`, { type: 'image/jpeg' });
}

export async function uploadCapturedPageImage(file: File): Promise<Api.Attachment.Item> {
  return new Promise((resolve, reject) => {
    const queue = useAttachmentUploadQueue({
      concurrency: 1,
      maxCount: 1,
      maxSizeBytes: PAGE_CAPTURE_MAX_BYTES,
      allowedExt: ['.jpg', '.jpeg', '.png', '.webp'],
      allowedMime: ['image/jpeg', 'image/png', 'image/webp'],
      onSuccess(item) {
        if (item.attachment && item.attachment.status === 'ready' && item.attachment.sha256) {
          resolve(item.attachment);
        } else {
          reject(new Error('capture attachment missing revision'));
        }
      }
    });
    queue.enqueue([file]);
    const poll = window.setInterval(() => {
      const current = queue.items.value[0];
      if (!current) return;
      if (current.status === 'failed' || current.status === 'cancelled') {
        window.clearInterval(poll);
        queue.reset();
        reject(new Error(current.error || 'capture upload failed'));
      } else if (current.status === 'success') {
        window.clearInterval(poll);
        queue.reset();
      }
    }, 60);
  });
}
