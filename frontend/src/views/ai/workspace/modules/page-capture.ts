export const PAGE_CAPTURE_EVENT_REQUESTED = 'run.capture_requested.v1';
export const PAGE_CAPTURE_EVENT_SETTLED = 'run.capture_settled.v1';
export const PAGE_CAPTURE_KIND = 'page.capture';
export const PAGE_CAPTURE_MAX_EDGE = 1600;
export const PAGE_CAPTURE_MAX_PIXELS = 2_000_000;
export const PAGE_CAPTURE_MAX_BYTES = 4 * 1024 * 1024;
export const PAGE_CAPTURE_JPEG_QUALITY = 0.84;

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
