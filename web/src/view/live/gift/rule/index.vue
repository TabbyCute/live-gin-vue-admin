<template>
  <div class="gva-table-box"><el-alert title="规则按主播→礼物→分类→全局匹配。超级 root（ID 1）可自行复核；其他管理员的创建人与复核人必须不同，同范围有效期不能重叠。无有效规则时禁止送礼。已创建规则不可修改，请新增版本。" :closable="false" />
    <el-button type="primary" @click="open">提交新版本</el-button><el-button @click="load">刷新</el-button><el-alert v-if="error" :title="error" type="error" />
    <el-table :data="rows" v-loading="loading">
      <el-table-column v-for="[prop,label] in columns" :key="prop" :prop="prop" :label="label" min-width="120" />
      <el-table-column label="有效期" min-width="340"><template #default="{row}">{{ formatDate(new Date(row.effectiveFrom)) }} — {{ formatDate(new Date(row.effectiveTo)) }}</template></el-table-column>
      <el-table-column label="提前终止" min-width="300"><template #default="{row}">
        <template v-if="row.termination">
          <div>{{ terminationLabel(row.termination.status) }} · {{ row.termination.terminationNo }}</div>
          <div>原因：{{ row.termination.reason }}</div>
          <div>申请人：{{ row.termination.createdBy }} / 复核人：{{ row.termination.reviewedBy }}</div>
          <div v-if="row.termination.reviewedAt">复核时间：{{ formatDate(new Date(row.termination.reviewedAt)) }}</div>
        </template><span v-else>—</span>
      </template></el-table-column>
      <el-table-column label="操作" fixed="right" width="200"><template #default="{row}">
        <template v-if="row.status==='pending'"><el-button link :disabled="writing" @click="review(row,true)">通过</el-button><el-button link type="danger" :disabled="writing" @click="review(row,false)">拒绝</el-button></template>
        <template v-else-if="row.termination?.status==='pending'"><el-button link type="danger" :disabled="writing" @click="reviewTermination(row,true)">通过终止</el-button><el-button link :disabled="writing" @click="reviewTermination(row,false)">拒绝终止</el-button></template>
        <el-button v-else-if="row.status==='active' && row.termination?.status!=='approved'" link type="danger" :disabled="writing" @click="openTermination(row)">申请提前终止</el-button>
      </template></el-table-column>
    </el-table>
    <el-pagination v-model:current-page="page" :page-size="20" :total="total" layout="total,prev,pager,next" @change="load" />
    <el-dialog v-model="dialog" title="新建分成规则版本" width="620px"><el-form label-width="120px" @submit.prevent="create"><el-form-item label="名称"><el-input v-model="form.name" maxlength="64" /></el-form-item><el-form-item label="适用范围"><el-select v-model="form.scopeType"><el-option v-for="[value,label] in scopes" :key="value" :value="value" :label="label" /></el-select></el-form-item><el-form-item v-if="form.scopeType!=='global'" label="范围公开编号"><el-input v-model="form.scopeNo" /></el-form-item><el-form-item label="主播万分比"><el-input v-model="form.anchorShareBps" placeholder="例如4000表示40%" /></el-form-item><el-form-item label="优先级"><el-input-number v-model="form.priority" :min="0" :max="2147483647" /></el-form-item><el-form-item label="有效期"><el-date-picker v-model="range" type="datetimerange" value-format="x" /></el-form-item><el-alert v-if="writeError" :title="writeError" type="error" /><el-button type="primary" native-type="submit" :loading="writing">提交复核</el-button></el-form></el-dialog>
    <el-dialog v-model="terminationDialog" title="申请提前终止分成规则" width="600px" :close-on-click-modal="false" :before-close="closeTermination">
      <el-alert title="超级 root（ID 1）可自行复核，其他管理员须由另一位管理员复核后生效。新订单停止匹配此规则，可能回退至其他范围规则；历史订单及退款继续使用原快照。" :closable="false" />
      <p>规则：{{ terminationDraft.ruleNo }}</p>
      <el-input v-model="terminationDraft.reason" type="textarea" maxlength="500" placeholder="填写终止原因" :disabled="writing||terminationSubmitted" />
      <el-alert v-if="terminationError" :title="terminationError" type="error" :closable="false" />
      <el-button type="danger" :loading="writing" @click="terminate">{{ terminationSubmitted?'按原请求重试':'提交终止复核' }}</el-button>
    </el-dialog>
  </div>
</template>
<script setup>
import {ref,onMounted,onBeforeUnmount} from 'vue'
import {ElMessage,ElMessageBox} from 'element-plus'
import {giftQuery,giftWrite} from '@/api/gift'
import {formatDate,CreateUUID} from '@/utils/format'
import {requestSequence} from '../shared'
defineOptions({name:'GiftRule'})
const scopes=[['global','全局'],['category','分类'],['gift','礼物'],['anchor','主播']],columns=[['ruleNo','规则编号'],['name','名称'],['scopeType','范围'],['scopeNo','范围编号'],['anchorShareBps','万分比'],['version','版本'],['status','状态'],['createdBy','创建人'],['approvedBy','复核人']]
const rows=ref([]),total=ref(0),page=ref(1),loading=ref(false),error=ref(''),writing=ref(false),dialog=ref(false),form=ref({}),range=ref([]),writeError=ref(''),seq=requestSequence()
async function load(){const id=seq.next();loading.value=true;error.value='';try{const r=await giftQuery('rule/list',{page:page.value,pageSize:20});if(!seq.valid(id))return;if(r.code!==0)throw new Error(r.msg);rows.value=r.data.list;total.value=Number(r.data.total)}catch(e){if(seq.valid(id))error.value=e.message||'查询失败'}finally{if(seq.valid(id))loading.value=false}}
function open(){form.value={name:'',scopeType:'global',scopeNo:'',anchorShareBps:'',priority:0};range.value=[];writeError.value='';dialog.value=true}
async function create(){if(writing.value)return;writing.value=true;try{if(!/^(0|[1-9][0-9]{0,4})$/.test(form.value.anchorShareBps)||BigInt(form.value.anchorShareBps)>BigInt('10000'))throw new Error('万分比范围0至10000');const r=await giftWrite('rule/create',{...form.value,scopeNo:form.value.scopeType==='global'?'':form.value.scopeNo,effectiveFrom:Number(range.value?.[0]||0),effectiveTo:Number(range.value?.[1]||0)});if(r.code!==0)throw new Error(r.msg);dialog.value=false;await load()}catch(e){writeError.value=e.message}finally{writing.value=false}}
async function review(row,approve){if(writing.value)return;writing.value=true;const body={ruleNo:row.ruleNo,approve};try{await ElMessageBox.confirm(`${approve?'通过':'拒绝'}规则 ${row.name}，主播分成 ${row.anchorShareBps} 万分比？`,'复核确认');const r=await giftWrite('rule/review',body);if(r.code!==0)throw new Error(r.msg);await load()}catch(e){if(e instanceof Error)ElMessage.error(e.message)}finally{writing.value=false}}
const terminationDialog=ref(false),terminationDraft=ref({}),terminationError=ref(''),terminationSubmitted=ref(false)
let terminationRequest=null
const terminationLabel=status=>({pending:'待复核终止',approved:'已提前终止',rejected:'终止申请已拒绝'}[status]||status)
function openTermination(row){if(writing.value)return;if(terminationSubmitted.value){terminationDialog.value=true;return}terminationDraft.value={ruleNo:row.ruleNo,requestId:CreateUUID(),reason:''};terminationError.value='';terminationDialog.value=true}
function closeTermination(done){if(!writing.value)done()}
async function terminate(){
  if(writing.value)return
  writing.value=true
  const body=terminationRequest||{...terminationDraft.value}
  try{
    if(!body.reason.trim())throw new Error('请填写终止原因')
    if(!terminationSubmitted.value)await ElMessageBox.confirm(`申请终止规则 ${body.ruleNo}？超级 root（ID 1）可自行复核，其他管理员须由另一位管理员复核。`,'终止确认')
    terminationRequest=body;terminationSubmitted.value=true;terminationDraft.value={...body}
    const r=await giftWrite('rule/terminate',body)
    if(r.code!==0)throw new Error(r.msg)
    terminationSubmitted.value=false;terminationRequest=null;terminationDialog.value=false;await load()
  }catch(e){if(e instanceof Error)terminationError.value=e.message}finally{writing.value=false}
}
async function reviewTermination(row,approve){
  if(writing.value)return
  writing.value=true
  const body={terminationNo:row.termination.terminationNo,approve}
  try{await ElMessageBox.confirm(`${approve?'通过':'拒绝'}终止申请 ${body.terminationNo}？${approve?'通过后新订单立即停止匹配该规则。':''}`,'终止复核');const r=await giftWrite('rule/terminate/review',body);if(r.code!==0)throw new Error(r.msg);await load()}catch(e){if(e instanceof Error)ElMessage.error(e.message)}finally{writing.value=false}
}
onMounted(load);onBeforeUnmount(()=>seq.invalidate())
</script>
