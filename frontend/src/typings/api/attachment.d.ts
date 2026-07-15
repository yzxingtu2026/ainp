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
      created_at: string;
      updated_at: string;
    }

    interface TransferPrepare {
      attachment: Item;
      upload_url?: string;
      download_url?: string;
      token: string;
      expires_at: string;
      method: string;
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
      current?: number;
      size?: number;
      keyword?: string;
      status?: string;
      scope?: 'all' | 'mine';
      uploader_id?: string;
    }

    interface PageResult {
      current: number;
      size: number;
      total: number;
      records: Item[];
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
