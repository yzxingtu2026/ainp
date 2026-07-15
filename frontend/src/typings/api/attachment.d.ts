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
  }
}
