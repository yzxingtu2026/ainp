export function pendingThumbnailIds(items: Api.Attachment.Item[]): Set<string> {
  return new Set(
    items
      .filter(
        item =>
          item.thumb_status === 'pending' &&
          (item.content_type.toLowerCase().startsWith('image/') ||
            item.content_type.toLowerCase().startsWith('video/'))
      )
      .map(item => item.id)
  );
}

export function mergePendingThumbnailStatuses(
  items: Api.Attachment.Item[],
  refreshed: Api.Attachment.Item[]
): Api.Attachment.Item[] {
  const pending = pendingThumbnailIds(items);
  const updates = new Map(refreshed.filter(item => pending.has(item.id)).map(item => [item.id, item]));
  if (updates.size === 0) return items;

  return items.map(item => (updates.has(item.id) ? { ...item, ...updates.get(item.id)! } : item));
}
