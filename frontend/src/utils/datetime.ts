import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

export type DateTimeDisplayFormat = 'datetime' | 'date' | 'time' | 'relative';

const EMPTY_DISPLAY = '-';

/**
 * Format API machine timestamps for user-visible UI.
 * Uses browser local timezone; follows the current dayjs locale (synced with app lang).
 */
export function formatDateTime(
  value: string | number | Date | null | undefined,
  format: DateTimeDisplayFormat = 'datetime'
): string {
  if (value === null || value === undefined || value === '') {
    return EMPTY_DISPLAY;
  }

  const parsed = dayjs(value);
  if (!parsed.isValid()) {
    return EMPTY_DISPLAY;
  }

  switch (format) {
    case 'date':
      return parsed.format('YYYY-MM-DD');
    case 'time':
      return parsed.format('HH:mm:ss');
    case 'relative':
      return parsed.fromNow();
    case 'datetime':
    default:
      // Compact list/detail default — avoids raw RFC3339 wrapping in narrow columns.
      return parsed.format('YYYY-MM-DD HH:mm');
  }
}
