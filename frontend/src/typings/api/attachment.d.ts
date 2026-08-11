declare namespace Api {
  namespace Attachment {
    type Status = 'pending' | 'ready' | 'deleting' | 'deleted' | 'failed';

    interface Item {
      id: string;
      original_name: string;
      content_type: string;
      size: number;
      sha256?: string;
      status: Status;
      uploader_id: string;
      /** 展示用昵称；缺失时前端回退 uploader_id */
      uploader_name?: string;
      /** 缩略图派生状态（#36）；缺省表示尚未支持派生 */
      thumb_status?: 'pending' | 'ready' | 'failed' | 'skipped';
      created_at: string;
      updated_at: string;
    }

    type DownloadVariant = 'original' | 'thumb' | 'preview';

    type UploadTransport = 'raw' | 'multipart';

    interface TransferPrepare {
      attachment: Item;
      upload_url?: string;
      download_url?: string;
      token: string;
      expires_at: string;
      method: string;
      transport?: UploadTransport;
      field_name?: string;
    }

    interface BindResult {
      link_id: string;
      attachment: Item;
    }

    interface ListResult {
      items: Item[];
    }

    interface DeleteResult {
      deleted: boolean;
      object_pending?: boolean;
    }

    interface UnbindResult {
      unbound: boolean;
    }

    interface UploadPolicy {
      max_size_bytes: number;
      allowed_ext: string[];
      allowed_mime: string[];
      temp_retain_sec: number;
      transfer_ttl_sec: number;
      max_uploads_once: number;
    }

    interface ListParams {
      page?: number;
      page_size?: number;
      keyword?: string;
      status?: string;
      scope?: 'all' | 'mine';
      uploader_id?: string;
    }

    interface PageResult {
      page: number;
      page_size: number;
      total: number;
      items: Item[];
    }

    interface Link {
      id: string;
      attachment_id: string;
      resource_type: string;
      resource_id: string;
      field: string;
      created_at: string;
    }

    interface Detail extends Item {
      provider: string;
      object_key?: string;
      links: Link[];
    }
  }
}
