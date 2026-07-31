<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns, FormInst } from 'naive-ui';
import { NButton, NSpace, NTag } from 'naive-ui';
import { AI_ACTIONS } from '@/constants/ai';
import { createAIProvider, discoverAIModels, fetchAIProviders, fetchProviderTypes, updateAIProvider, updateAIProviderStatus } from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({name:'AIProviders'});
const auth=useAuthStore();const loading=ref(false);const saving=ref(false);const rows=ref<Api.AI.Provider[]>([]);const total=ref(0);const query=reactive({page:1,page_size:20,keyword:'',status:''});
const visible=ref(false);const editing=ref<Api.AI.Provider|null>(null);const formRef=ref<FormInst|null>(null);const protocols=ref<Array<{protocol:string;name:string;model_discovery:boolean}>>([]);
const form=reactive({code:'',name:'',protocol:'openai',base_url:'',credential:'',status:'enabled' as Api.AI.Status});
const endpointPlaceholder=computed(()=>form.protocol==='openai_compatible'?'例如：https://api.example.com（结尾不带 /v1）':'留空使用官方地址；自定义示例：https://api.example.com（不带 /v1）');
const discovered=ref<Api.AI.RemoteModel[]>([]);const discoveryVisible=ref(false);const discoveryLoading=ref(false);const can=(p:string)=>auth.userInfo.buttons.includes(p)||auth.userInfo.buttons.includes('*');
async function load(){loading.value=true;try{const {data}=await fetchAIProviders(query);if(data){rows.value=data.items;total.value=data.total}}finally{loading.value=false}}
function edit(row?:Api.AI.Provider){editing.value=row||null;Object.assign(form,row?{code:row.code,name:row.name,protocol:row.protocol,base_url:row.base_url,credential:'',status:row.status}:{code:'',name:'',protocol:'openai',base_url:'',credential:'',status:'enabled'});visible.value=true}
async function save(){await formRef.value?.validate();saving.value=true;try{const r=editing.value?await updateAIProvider({id:editing.value.id,name:form.name,base_url:form.base_url,credential:form.credential,config:{}}):await createAIProvider({...form,config:{}});if(!r.error){visible.value=false;window.$message?.success('保存成功');await load()}}finally{saving.value=false}}
async function toggle(row:Api.AI.Provider){const status=row.status==='enabled'?'disabled':'enabled';if(!(await updateAIProviderStatus(row.id,status)).error)await load()}
async function discover(row:Api.AI.Provider){discovered.value=[];discoveryVisible.value=true;discoveryLoading.value=true;try{const {data,error}=await discoverAIModels(row.id);if(!error&&data)discovered.value=data.items}finally{discoveryLoading.value=false}}
const columns=computed<DataTableColumns<Api.AI.Provider>>(()=>[
 {title:'代码',key:'code',minWidth:130},{title:'名称',key:'name',minWidth:140},{title:'协议',key:'protocol',width:150},{title:'Endpoint',key:'base_url',minWidth:220,ellipsis:{tooltip:true}},
 {title:'凭证',key:'credential_configured',width:110,render:r=>h(NTag,{type:r.credential_configured?'success':'warning'},{default:()=>r.credential_configured?'已配置':'未配置'})},{title:'状态',key:'status',width:90,render:r=>h(NTag,{type:r.status==='enabled'?'success':'default'},{default:()=>r.status==='enabled'?'启用':'停用'})},
 {title:'操作',key:'actions',width:260,render:r=>h(NSpace,null,{default:()=>[can(AI_ACTIONS.providerUpdate)?h(NButton,{size:'small',onClick:()=>edit(r)},{default:()=> '编辑'}):null,can(AI_ACTIONS.providerDiscover)?h(NButton,{size:'small',disabled:r.status!=='enabled',onClick:()=>discover(r)},{default:()=> '获取模型'}):null,can(AI_ACTIONS.providerStatus)?h(NButton,{size:'small',type:r.status==='enabled'?'warning':'success',onClick:()=>toggle(r)},{default:()=>r.status==='enabled'?'停用':'启用'}):null].filter(Boolean)})}
]);
onMounted(async()=>{const {data}=await fetchProviderTypes();protocols.value=data||[];await load()});
</script>
<template>
 <div class="flex-col-stretch gap-16px"><NCard title="模型供应商" :bordered="false"><NSpace><NInput v-model:value="query.keyword" placeholder="搜索代码或名称" clearable @keyup.enter="load"/><NButton @click="load">查询</NButton><NButton v-if="can(AI_ACTIONS.providerCreate)" type="primary" @click="edit()">新增供应商</NButton></NSpace></NCard>
 <NCard :bordered="false"><NDataTable :columns="columns" :data="rows" :loading="loading" :scroll-x="1100"/><NPagination v-model:page="query.page" class="mt-16px justify-end" :item-count="total" :page-size="query.page_size" @update:page="load"/></NCard>
 <NModal v-model:show="visible" preset="card" :title="editing?'编辑供应商':'新增供应商'" class="w-600px max-w-90vw"><NForm ref="formRef" :model="form" label-placement="left" label-width="100"><NFormItem label="代码" path="code" :rule="{required:true,message:'请输入代码'}"><NInput v-model:value="form.code" :disabled="Boolean(editing)"/></NFormItem><NFormItem label="名称"><NInput v-model:value="form.name"/></NFormItem><NFormItem label="协议"><AppSelect v-model:value="form.protocol" :disabled="Boolean(editing)" :options="protocols.map(x=>({label:x.name,value:x.protocol}))"/></NFormItem><NFormItem label="Endpoint"><NInput v-model:value="form.base_url" :placeholder="endpointPlaceholder"/></NFormItem><NFormItem label="凭证" path="credential" :rule="editing?undefined:{required:true,message:'请输入供应商凭证'}"><NInput v-model:value="form.credential" type="password" show-password-on="click" :placeholder="editing?'留空表示保留现有凭证':'请输入 API Key'"/></NFormItem></NForm><template #footer><NSpace justify="end"><NButton @click="visible=false">取消</NButton><NButton type="primary" :loading="saving" @click="save">保存</NButton></NSpace></template></NModal>
 <NModal v-model:show="discoveryVisible" preset="card" title="远端候选模型" class="w-760px max-w-90vw"><NAlert type="info" class="mb-12px">候选模型不会自动开放，需在模型目录确认类型与能力后导入。</NAlert><NDataTable :loading="discoveryLoading" max-height="min(460px, 58vh)" virtual-scroll :columns="[{title:'模型 ID',key:'id'},{title:'名称',key:'display_name'},{title:'能力已确认',key:'metadata_confirmed',render:(r:any)=>r.metadata_confirmed?'是':'否'}]" :data="discovered"/></NModal></div>
</template>
