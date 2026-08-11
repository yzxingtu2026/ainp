// @vitest-environment happy-dom

import { defineComponent, h, nextTick } from 'vue';
import { mount, type VueWrapper } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchAttachment, prepareAttachmentDownload } from '@/service/api';
import { getAttachmentTransferBlob } from '@/service/attachment/transfer';
import {
  ATTACHMENT_PREVIEW_IMAGE_ZOOM_MAX,
  ATTACHMENT_PREVIEW_IMAGE_ZOOM_MIN,
  clampAttachmentPreviewImageOffset,
  nextAttachmentPreviewImageZoom
} from '@/components/common/attachment-preview-image';
import {
  ATTACHMENT_PREVIEW_MAX_BLOB_SIZE_BYTES,
  attachmentPreviewKind,
  canPreviewAttachment,
  useAttachmentPreview
} from './attachment';

vi.mock('@/service/api', () => ({
  fetchAttachment: vi.fn(),
  prepareAttachmentDownload: vi.fn()
}));

vi.mock('@/service/attachment/transfer', () => ({
  getAttachmentTransferBlob: vi.fn()
}));

vi.mock('@/locales', () => ({
  $t: (key: string) => key
}));

vi.mock('@/service/request', () => ({
  getRequestErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
  getStandaloneErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback)
}));

const mockedFetchAttachment = vi.mocked(fetchAttachment);
const mockedPrepareDownload = vi.mocked(prepareAttachmentDownload);
const mockedTransferBlob = vi.mocked(getAttachmentTransferBlob);
const wrappers: VueWrapper[] = [];
let objectUrlSequence = 0;

function item(
  id: string,
  contentType = 'image/png',
  overrides: Partial<Api.Attachment.Item> = {}
): Api.Attachment.Item {
  return {
    id,
    original_name: `${id}.bin`,
    content_type: contentType,
    size: 1024,
    status: 'ready',
    uploader_id: 'user-1',
    created_at: '2026-08-11T00:00:00Z',
    updated_at: '2026-08-11T00:00:00Z',
    ...overrides
  };
}

function mountPreview(options?: { maxBlobSizeBytes?: number }) {
  let preview!: ReturnType<typeof useAttachmentPreview>;
  const wrapper = mount(
    defineComponent({
      setup() {
        preview = useAttachmentPreview(options);
        return () => h('div');
      }
    })
  );
  wrappers.push(wrapper);
  return preview;
}

function prepareResult(attachment: Api.Attachment.Item) {
  return {
    data: {
      attachment,
      download_url: `/transfer/${attachment.id}`,
      token: `token-${attachment.id}`,
      expires_at: '2026-08-11T00:01:00Z',
      method: 'GET'
    },
    error: null
  } as Awaited<ReturnType<typeof prepareAttachmentDownload>>;
}

beforeEach(() => {
  objectUrlSequence = 0;
  vi.stubGlobal('URL', {
    ...URL,
    createObjectURL: vi.fn(() => `blob:preview-${++objectUrlSequence}`),
    revokeObjectURL: vi.fn()
  });
  mockedPrepareDownload.mockImplementation(async id => prepareResult(item(id)));
  mockedTransferBlob.mockResolvedValue(new Blob(['preview']));
});

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount());
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe('attachment preview support matrix', () => {
  it('clamps image wheel zoom and pan offsets', () => {
    let zoom = 1;
    for (let index = 0; index < 100; index += 1) zoom = nextAttachmentPreviewImageZoom(zoom, -1);
    expect(zoom).toBe(ATTACHMENT_PREVIEW_IMAGE_ZOOM_MAX);
    for (let index = 0; index < 200; index += 1) zoom = nextAttachmentPreviewImageZoom(zoom, 1);
    expect(zoom).toBe(ATTACHMENT_PREVIEW_IMAGE_ZOOM_MIN);
    expect(nextAttachmentPreviewImageZoom(1, 0)).toBe(1);
    expect(
      clampAttachmentPreviewImageOffset(
        { x: 600, y: -400 },
        { width: 800, height: 600 },
        { width: 1000, height: 700 },
        2
      )
    ).toEqual({ x: 300, y: -250 });
  });

  it('supports ready images, exact PDF MIME, and videos only', () => {
    expect(attachmentPreviewKind('image/webp')).toBe('image');
    expect(attachmentPreviewKind('application/pdf')).toBe('pdf');
    expect(attachmentPreviewKind('video/mp4')).toBe('video');
    expect(attachmentPreviewKind('application/x-pdf')).toBe('unsupported');
    expect(canPreviewAttachment(item('image'))).toBe(true);
    expect(canPreviewAttachment(item('pdf', 'application/pdf'))).toBe(true);
    expect(canPreviewAttachment(item('video', 'video/webm'))).toBe(true);
    expect(canPreviewAttachment(item('pending', 'image/png', { status: 'pending' }))).toBe(false);
    expect(canPreviewAttachment(item('text', 'text/plain'))).toBe(false);
  });

  it.each([
    ['image/png', 'image'],
    ['application/pdf', 'pdf'],
    ['video/mp4', 'video']
  ] as const)('loads and revokes a %s Blob preview', async (contentType, kind) => {
    const attachment = item(kind, contentType);
    mockedPrepareDownload.mockResolvedValueOnce(prepareResult(attachment));
    const preview = mountPreview();

    await preview.open(attachment);

    expect(preview.state.value).toBe('ready');
    expect(preview.previewKind.value).toBe(kind);
    expect(preview.objectUrl.value).toBe('blob:preview-1');
    preview.close();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:preview-1');
  });

  it('loads metadata for an attachment ID before previewing', async () => {
    const attachment = item('by-id', 'application/pdf');
    mockedFetchAttachment.mockResolvedValueOnce({
      data: attachment as Api.Attachment.Detail,
      error: null
    } as unknown as Awaited<ReturnType<typeof fetchAttachment>>);
    mockedPrepareDownload.mockResolvedValueOnce(prepareResult(attachment));
    const preview = mountPreview();

    await preview.open(attachment.id);

    expect(mockedFetchAttachment).toHaveBeenCalledWith(
      attachment.id,
      expect.objectContaining({ errorMode: 'custom', signal: expect.any(AbortSignal) })
    );
    expect(preview.previewKind.value).toBe('pdf');
  });

  it('fails closed before prepare for non-ready, unsupported, and oversized files', async () => {
    const preview = mountPreview();

    await preview.open(item('pending', 'image/png', { status: 'pending' }));
    expect(preview.state.value).toBe('not-ready');
    await preview.open(item('unsupported', 'text/plain'));
    expect(preview.state.value).toBe('unsupported');
    await preview.open(item('large', 'video/mp4', { size: ATTACHMENT_PREVIEW_MAX_BLOB_SIZE_BYTES + 1 }));
    expect(preview.state.value).toBe('too-large');
    expect(mockedPrepareDownload).not.toHaveBeenCalled();
    expect(mockedTransferBlob).not.toHaveBeenCalled();
  });

  it('reports transfer failures without leaking an Object URL', async () => {
    mockedTransferBlob.mockRejectedValueOnce(new Error('transfer failed'));
    const preview = mountPreview();

    await preview.open(item('failed'));

    expect(preview.state.value).toBe('error');
    expect(preview.error.value).toContain('transfer failed');
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });

  it('aborts stale work when switching and keeps the latest result', async () => {
    let resolveFirst!: (blob: Blob) => void;
    const firstTransfer = new Promise<Blob>(resolve => {
      resolveFirst = resolve;
    });
    let firstSignal: AbortSignal | undefined;
    mockedTransferBlob
      .mockImplementationOnce(async options => {
        firstSignal = options.signal;
        return firstTransfer;
      })
      .mockResolvedValueOnce(new Blob(['second']));
    mockedPrepareDownload
      .mockResolvedValueOnce(prepareResult(item('first')))
      .mockResolvedValueOnce(prepareResult(item('second', 'video/mp4')));
    const preview = mountPreview();

    const firstOpen = preview.open(item('first'));
    await nextTick();
    const secondOpen = preview.open(item('second', 'video/mp4'));
    await secondOpen;
    resolveFirst(new Blob(['first']));
    await firstOpen;

    expect(firstSignal?.aborted).toBe(true);
    expect(preview.previewName.value).toBe('second.bin');
    expect(preview.previewKind.value).toBe('video');
    expect(preview.objectUrl.value).toBe('blob:preview-1');
  });

  it('aborts in-flight work on close and revokes loaded URLs on unmount', async () => {
    let pendingSignal: AbortSignal | undefined;
    mockedTransferBlob.mockImplementationOnce(
      options =>
        new Promise<Blob>(() => {
          pendingSignal = options.signal;
        })
    );
    const pendingPreview = mountPreview();
    void pendingPreview.open(item('pending-close'));
    await nextTick();
    pendingPreview.close();
    expect(pendingSignal?.aborted).toBe(true);
    expect(pendingPreview.state.value).toBe('idle');

    const loadedPreview = mountPreview();
    await loadedPreview.open(item('loaded'));
    wrappers.pop()?.unmount();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:preview-1');
  });
});
