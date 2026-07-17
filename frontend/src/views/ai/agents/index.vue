<script setup lang="tsx">
import { h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NSpace, NTag } from 'naive-ui';
import { createAIAgent, fetchAIAgentDiagnostics, fetchAIAgents, fetchAIAgentVersions, reconcileBuiltinAIAgent, updateAIAgentStatus } from '@/service/api';

defineOptions({ name: 'AIAgents' });
const rows=ref<Api.AI.Agent[]>([]); const total=ref(0); const loading=ref(false);
const query=reactive({page:1,page_size:20,keyword:'',source:'',status:''});
const createVisible=ref(false); const detailVisible=ref(false); const detail=ref<Api.AI.Agent|null>(null);
const diagnostics=ref<Api.AI.AgentDiagnostics|null>(null); const versions=ref<Api.AI.AgentVersion[]>([]);
const form=reactive({code:'',name:'',description:'',source:'personal',visibility:'private',model_alias:'',user_instructions:''});
async function load(){loading.value=true;try{const {data}=await fetchAIAgents(query);rows.value=data?.items||[];total.value=data?.total||0}finally{loading.value=false}}
async function create(){const {error}=await createAIAgent({...form,generation_config:{},tool_policy:{}});if(!error){createVisible.value=false;await load()}}
async function toggle(row:Api.AI.Agent){await updateAIAgentStatus(row.id,row.status==='enabled'?'disabled':'enabled');await load()}
async function inspect(row:Api.AI.Agent){detail.value=row;detailVisible.value=true;const [d,v]=await Promise.all([fetchAIAgentDiagnostics(row.id),fetchAIAgentVersions(row.id)]);diagnostics.value=d.data||null;versions.value=v.data||[]}
async function reconcile(){const {error}=await reconcileBuiltinAIAgent();if(!error)await load()}
const columns:DataTableColumns<Api.AI.Agent>=[
  {title:'名称',key:'name',render:r=>h(NSpace,{vertical:true,size:2},{default:()=>[h('span',r.name),r.contract_key?h(NTag,{size:'small',bordered:false},{default:()=>`托管: ${r.contract_key}@${r.contract_version}`}):null]})},
  {title:'代码',key:'code'}, {title:'来源',key:'source',render:r=>h(NTag,{size:'small'},{default:()=>({builtin:'内置',tenant:'租户',personal:'个人'}[r.source])})},
  {title:'可见性',key:'visibility',render:r=>r.visibility==='tenant'?'租户共享':'仅自己'},
  {title:'状态',key:'status',render:r=>h(NTag,{type:r.status==='enabled'?'success':r.status==='draft'?'warning':'default'},{default:()=>({draft:'草稿',enabled:'已启用',disabled:'已停用'}[r.status])})},
  {title:'版本',key:'current_version'},
  {title:'操作',key:'actions',render:r=>h(NSpace,null,{default:()=>[h(NButton,{size:'small',onClick:()=>inspect(r)},{default:()=> '详情'}),h(NButton,{size:'small',onClick:()=>toggle(r)},{default:()=>r.status==='enabled'?'停用':'启用'})]})}
];
onMounted(load);
</script>

<template>
  <div class="h-full min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard title="智能体" :bordered="false" size="small" class="card-wrapper">
      <template #header-extra><NSpace><NButton @click="reconcile">同步内置智能体</NButton><NButton type="primary" @click="createVisible=true">新建智能体</NButton></NSpace></template>
      <NSpace class="mb-16px"><NInput v-model:value="query.keyword" clearable placeholder="名称或代码" @keyup.enter="load"/><NSelect v-model:value="query.source" class="w-140px" :options="[{label:'全部来源',value:''},{label:'内置',value:'builtin'},{label:'租户',value:'tenant'},{label:'个人',value:'personal'}]" @update:value="load"/><NButton @click="load">查询</NButton></NSpace>
      <NDataTable remote :loading="loading" :columns="columns" :data="rows" :pagination="{page:query.page,pageSize:query.page_size,itemCount:total,onChange:(p:number)=>{query.page=p;load()}}" />
    </NCard>
    <NModal v-model:show="createVisible" preset="card" title="新建智能体" class="w-600px max-w-90vw"><NForm label-placement="left" label-width="100"><NFormItem label="名称"><NInput v-model:value="form.name"/></NFormItem><NFormItem label="代码"><NInput v-model:value="form.code" placeholder="my-agent"/></NFormItem><NFormItem label="Model Alias"><NInput v-model:value="form.model_alias"/></NFormItem><NFormItem label="类型"><NSelect v-model:value="form.source" :options="[{label:'个人',value:'personal'},{label:'租户共享',value:'tenant'}]" @update:value="v=>form.visibility=v==='tenant'?'tenant':'private'"/></NFormItem><NFormItem label="补充指令"><NInput v-model:value="form.user_instructions" type="textarea" :maxlength="12000" show-count/></NFormItem></NForm><template #footer><NSpace justify="end"><NButton @click="createVisible=false">取消</NButton><NButton type="primary" @click="create">创建</NButton></NSpace></template></NModal>
    <NDrawer v-model:show="detailVisible" :width="520"><NDrawerContent :title="detail?.name" closable><NAlert :type="diagnostics?.healthy?'success':'warning'" :title="diagnostics?.healthy?'运行条件正常':'存在运行问题'"><div v-for="issue in diagnostics?.issues" :key="issue.code">{{issue.code}}: {{issue.message}}</div></NAlert><NDivider>版本历史</NDivider><NList bordered><NListItem v-for="v in versions" :key="v.id"><NThing :title="`版本 ${v.version}`" :description="`Model Alias: ${v.model_alias}`"/></NListItem></NList></NDrawerContent></NDrawer>
  </div>
</template>
