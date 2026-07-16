declare namespace Api.AI {
  type Status='enabled'|'disabled';
  interface Provider { id:string;code:string;name:string;protocol:string;base_url:string;credential_configured:boolean;status:Status;config_json:Record<string,unknown>;created_at:string;updated_at:string }
  interface Deployment { id:string;provider_id:string;code:string;remote_model_id:string;display_name:string;model_type:string;capabilities_json:{input_modalities:string[];output_modalities:string[];features:string[]};limits_json:{context_window?:number;max_output_tokens?:number};status:Status;created_at:string;updated_at:string }
  interface Alias { id:string;code:string;deployment_id:string;created_at:string;updated_at:string }
  interface RemoteModel { id:string;display_name:string;suggested_type?:string;suggested_capabilities:{input_modalities:string[];output_modalities:string[];features:string[]};metadata_confirmed:boolean;imported:boolean }
  interface Page<T>{items:T[];total:number;page:number;page_size:number}
}
