export interface CursorPage<T> {
  items: T[];
  next_sequence: number;
  has_more: boolean;
}

export async function loadAllCursorPages<T>(
  fetchPage: (afterSequence: number) => Promise<CursorPage<T>>
): Promise<T[]> {
  const items: T[] = [];
  let afterSequence = 0;

  while (true) {
    const page = await fetchPage(afterSequence);
    items.push(...(page.items || []));
    if (!page.has_more) return items;
    if (page.next_sequence <= afterSequence) throw new Error('message cursor did not advance');
    afterSequence = page.next_sequence;
  }
}

export function createLatestRequestGate() {
  let version = 0;
  return {
    begin() {
      version += 1;
      return version;
    },
    invalidate() {
      version += 1;
    },
    isCurrent(requestVersion: number) {
      return requestVersion === version;
    }
  };
}
