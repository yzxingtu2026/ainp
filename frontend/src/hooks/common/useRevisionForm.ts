import { ref, computed } from 'vue';

/**
 * Composable for edit forms that use optimistic locking via revision.
 * - Tracks the current revision from the loaded resource
 * - Provides expectedRevision for submit
 * - Handles revision conflict (40004) by prompting user to refresh
 */
export function useRevisionForm<T extends { revision?: number }>() {
  const revision = ref<number>(0);
  const hasConflict = ref(false);

  /** Load revision from a fetched resource */
  function loadRevision(resource: T | null | undefined): void {
    revision.value = resource?.revision ?? 0;
    hasConflict.value = false;
  }

  /** The expected revision to send with the next update request */
  const expectedRevision = computed(() => revision.value);

  /**
   * Handle a revision conflict response.
   * Call this when the API returns error code 40004 (CodeRevisionConflict).
   */
  function handleRevisionConflict(): void {
    hasConflict.value = true;
  }

  /** Reset conflict state — call when user refreshes the data */
  function clearConflict(): void {
    hasConflict.value = false;
  }

  return {
    revision,
    expectedRevision,
    hasConflict,
    loadRevision,
    handleRevisionConflict,
    clearConflict
  };
}
