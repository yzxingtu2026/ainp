import { ref } from 'vue';

/**
 * Generate a stable idempotency key based on action ID and business payload.
 * Uses a simple hash of the action + key fields to ensure the same
 * user intent always produces the same key.
 */
export function generateIdempotencyKey(actionId: string, payload: Record<string, unknown>): string {
  const raw = `${actionId}:${JSON.stringify(sortObject(payload))}`;
  return simpleHash(raw);
}

/**
 * Composable for managing idempotency key lifecycle.
 * - Double-click / network timeout retry: reuse the same key
 * - User explicitly re-initiates: generate a new key
 */
export function useIdempotentAction(actionId: string) {
  const currentKey = ref<string | null>(null);
  const isSubmitting = ref(false);

  /** Get or create the current idempotency key */
  function getKey(payload: Record<string, unknown>): string {
    if (!currentKey.value) {
      currentKey.value = generateIdempotencyKey(actionId, payload);
    }
    return currentKey.value;
  }

  /** Reset the key — call when user explicitly re-initiates the action */
  function resetKey(): void {
    currentKey.value = null;
  }

  /**
   * Execute an async action with idempotency protection.
   * On success or user-initiated retry, resets the key.
   * On network timeout / double-click, reuses the same key.
   */
  async function execute<T>(
    payload: Record<string, unknown>,
    fn: (idempotencyKey: string) => Promise<T>
  ): Promise<T> {
    if (isSubmitting.value) {
      throw new Error('Action already in progress');
    }
    isSubmitting.value = true;
    const key = getKey(payload);
    try {
      const result = await fn(key);
      resetKey();
      return result;
    } finally {
      // Failures intentionally retain the key for a network retry.
      isSubmitting.value = false;
    }
  }

  return {
    currentKey,
    isSubmitting,
    getKey,
    resetKey,
    execute
  };
}

// --- internal helpers ---

function sortObject(obj: unknown): unknown {
  if (obj === null || obj === undefined) return obj;
  if (Array.isArray(obj)) return obj.map(sortObject);
  if (typeof obj === 'object') {
    const sorted: Record<string, unknown> = {};
    for (const key of Object.keys(obj as Record<string, unknown>).sort()) {
      sorted[key] = sortObject((obj as Record<string, unknown>)[key]);
    }
    return sorted;
  }
  return obj;
}

/** Simple non-cryptographic hash (djb2) for client-side key generation */
function simpleHash(str: string): string {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = ((hash << 5) + hash + str.charCodeAt(i)) & 0xffffffff;
  }
  return `${actionIdPrefix(str)}_${(hash >>> 0).toString(36)}`;
}

function actionIdPrefix(s: string): string {
  return s.slice(0, 32).replace(/[^a-zA-Z0-9_.-]/g, '_');
}
