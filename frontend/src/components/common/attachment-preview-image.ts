export const ATTACHMENT_PREVIEW_IMAGE_ZOOM_MIN = 0.25;
export const ATTACHMENT_PREVIEW_IMAGE_ZOOM_MAX = 5;
const ATTACHMENT_PREVIEW_IMAGE_ZOOM_FACTOR = 1.15;

export function nextAttachmentPreviewImageZoom(current: number, deltaY: number): number {
  if (deltaY === 0) return current;
  const factor = deltaY < 0 ? ATTACHMENT_PREVIEW_IMAGE_ZOOM_FACTOR : 1 / ATTACHMENT_PREVIEW_IMAGE_ZOOM_FACTOR;
  const next = Math.round(current * factor * 10_000) / 10_000;
  return Math.min(ATTACHMENT_PREVIEW_IMAGE_ZOOM_MAX, Math.max(ATTACHMENT_PREVIEW_IMAGE_ZOOM_MIN, next));
}

export function clampAttachmentPreviewImageOffset(
  offset: { x: number; y: number },
  image: { width: number; height: number },
  viewport: { width: number; height: number },
  zoom: number
): { x: number; y: number } {
  const maxX = Math.max(0, (image.width * zoom - viewport.width) / 2);
  const maxY = Math.max(0, (image.height * zoom - viewport.height) / 2);
  return {
    x: Math.min(maxX, Math.max(-maxX, offset.x)),
    y: Math.min(maxY, Math.max(-maxY, offset.y))
  };
}
