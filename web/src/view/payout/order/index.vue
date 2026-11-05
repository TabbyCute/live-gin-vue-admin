<template>
 <div>
  <div class="gva-search-box"><h2>出款订单</h2><el-alert title="人工出款不会自动调用银行。创建、领取和提交凭证期间资金保持冻结；独立复核成功后完成内部入账。" type="info" :closable="false" />
   <el-form :inline="true" class="mt-4" @submit.prevent="search">
    <el-form-item v-for="[key,label] in filters" :key="key" :label="label"><el-input v-model.trim="query[key]" clearable :maxlength="key==='paymentReference'?128:40" /></el-form-item>
    <el-form-item label="收款方式"><el-select v-model="query.payeeMethod" clearable class="w-40"><el-option v-for="v in ['bank_card','alipay','wechat']" :key="v" :value="v" :label="payeeMethodLabel(v)" /></el-select></el-form-item>
    <el-form-item label="状态"><el-select v-model="query.status" clearable class="w-48"><el-option v-for="(label,value) in payoutStates" :key="value" :label="label" :value="value" /></el-select></el-form-item>
    <el-form-item label="创建时间"><el-date-picker v-model="range" type="datetimerange" value-format="x" start-placeholder="开始" end-placeholder="结束（不包含）" /></el-form-item>
    <el-form-item><el-button type="primary" :loading="loading" @click="search">查询</el-button><el-button @click="reset">重置</el-button></el-form-item>
   </el-form><el-alert v-if="error" :title="error" type="error" :closable="false" />
  </div>
  <div class="gva-table-box"><el-table :data="rows" v-loading="loading"><el-table-column v-for="[key,label] in orderFields" :key="key" :label="label" min-width="150" show-overflow-tooltip><template #default="{row}">{{ display(key,row[key]) }}</template></el-table-column><el-table-column label="操作" fixed="right" width="100"><template #default="{row}"><el-button v-if="capabilities['order/detail']" link @click="inspect(row)">详情</el-button></template></el-table-column></el-table>
   <p>共 {{ total }} 条</p><el-pagination v-model:current-page="query.page" v-model:page-size="query.pageSize" :total="paginationTotal" :page-sizes="[20,50,100]" layout="prev,pager,next,sizes" @current-change="load" @size-change="search" />
  </div>
  <el-drawer v-model="detailOpen" title="出款详情与人工处理" size="85%" @close="closeDetail">
   <el-alert v-if="detailError" :title="detailError" type="error" :closable="false" />
   <div v-if="detail" v-loading="detailLoading">
    <el-alert :title="detail.consistent?'内部账务核验通过（不代表银行自动核验）':'内部核验发现异常，请检查对账结果'" :type="detail.consistent?'success':'warning'" :closable="false" />
    <ManualActions :detail="detail" @changed="refreshDetail"/>
    <el-descriptions :column="2" border class="mt-4"><el-descriptions-item v-for="[key,label] in orderFields" :key="key" :label="label">{{ display(key,detail.order[key]) }}</el-descriptions-item><el-descriptions-item label="加密快照哈希">{{ detail.order.payeeSnapshotHash }}</el-descriptions-item><el-descriptions-item label="当前账户状态">{{ payeeStateLabel(detail.currentPayee.state) }}</el-descriptions-item><el-descriptions-item label="与原快照一致">{{ detail.currentPayee.matchesSnapshot?'是':'否' }}</el-descriptions-item><el-descriptions-item label="来源提现状态">{{ statusLabel(detail.withdrawal.status) }}</el-descriptions-item><el-descriptions-item label="来源对账状态">{{ statusLabel(detail.withdrawal.reconciliationStatus) }}</el-descriptions-item></el-descriptions>
    <h3>创建时账务核验</h3><SnapshotPanel :value="detail.creationReconciliation" />
    <h3>状态历史</h3><el-table :data="detail.history"><el-table-column prop="eventNo" label="事件编号"/><el-table-column prop="sequence" label="顺序"/><el-table-column prop="fromStatus" label="原状态"/><el-table-column prop="toStatus" label="新状态"/><el-table-column prop="attemptNo" label="尝试编号"/><el-table-column prop="actorId" label="操作人"/><el-table-column prop="reason" label="原因"/><el-table-column label="时间"><template #default="{row}">{{ display('createdAt',row.createdAt) }}</template></el-table-column></el-table>
    <h3>操作审计</h3><p v-if="detail.auditTruncated">显示最近200条；完整审计永久保留。</p><el-table :data="detail.audit"><el-table-column prop="eventNo" label="事件编号"/><el-table-column prop="action" label="操作"/><el-table-column prop="actorId" label="操作人"/><el-table-column prop="traceId" label="请求追踪编号"/><el-table-column label="时间"><template #default="{row}">{{ display('createdAt',row.createdAt) }}</template></el-table-column></el-table>
   </div>
  </el-drawer>
 </div>
</template>
<script setup>
import { computed,onMounted,onBeforeUnmount,reactive,ref,watch } from 'vue'
import { useUserStore } from '@/pinia/modules/user'
import { formatDate } from '@/utils/format'
import { listPayoutOrders,getPayoutOrder,getPayoutCapabilities } from '@/api/payout'
import { sequence,payeeMethodLabel,payeeStateLabel,statusLabel } from '@/view/live/earning/shared'
import SnapshotPanel from '@/view/live/earning/SnapshotPanel.vue'
import { orderFields,payoutError,payoutStates } from '../shared'
import ManualActions from '../ManualActions.vue'
const user=useUserStore(),filters=[['payoutNo','出款单号'],['withdrawalNo','提现单号'],['anchorNo','主播编号'],['currency','币种'],['channelAccountNo','公司账户编号'],['operatorId','操作人ID'],['reviewerId','复核人ID'],['paymentReference','银行流水号（需指定公司账户）']]
const initial=()=>({page:1,pageSize:20,payoutNo:'',withdrawalNo:'',anchorNo:'',currency:'',payeeMethod:'',status:'',channelAccountNo:'',operatorId:'',reviewerId:'',paymentReference:''})
const capabilities=ref({})
const query=reactive(initial()),range=ref([]),rows=ref([]),total=ref('0'),loading=ref(false),error=ref(''),detail=ref(null),detailOpen=ref(false),detailLoading=ref(false),detailError=ref('')
const listSeq=sequence(),detailSeq=sequence(),paginationTotal=computed(()=>Math.min(Number(total.value),query.pageSize*10000))
const display=(key,value)=>key==='createdAt'?formatDate(new Date(Number(value)),'yyyy-MM-dd hh:mm:ss'):key==='status'?(payoutStates[value]||value):key==='payeeMethod'?payeeMethodLabel(value):key==='businessType'?(value==='anchor_withdrawal'?'主播提现':value):value
async function load(){const id=listSeq.next(),token=user.token;loading.value=true;error.value='';const params={...query};for(const key of Object.keys(params))if(params[key]==='')delete params[key];if(range.value?.length===2){params.from=Number(range.value[0]);params.to=Number(range.value[1])}
 try{const r=await listPayoutOrders(params);if(!listSeq.valid(id)||token!==user.token)return;if(r.code!==0)throw r;rows.value=r.data.list;total.value=r.data.total}catch(e){if(listSeq.valid(id)&&token===user.token){rows.value=[];total.value='0';error.value=payoutError(e)}}finally{if(listSeq.valid(id))loading.value=false}}
function search(){query.page=1;return load()}
function reset(){Object.assign(query,initial());range.value=[];return load()}
async function inspect(row){const id=detailSeq.next(),token=user.token;detail.value=null;detailOpen.value=true;detailError.value='';detailLoading.value=true;try{const r=await getPayoutOrder(row.payoutNo);if(!detailSeq.valid(id)||token!==user.token)return;if(r.code!==0)throw r;if(r.data.order.payoutNo!==row.payoutNo)throw new Error('Mismatched order');detail.value=r.data}catch(e){if(detailSeq.valid(id)&&token===user.token)detailError.value=payoutError(e)}finally{if(detailSeq.valid(id))detailLoading.value=false}}
function closeDetail(){detailSeq.invalidate();detail.value=null;detailLoading.value=false}
async function refreshDetail(){const no=detail.value?.order.payoutNo;if(!no)return;const id=detailSeq.next(),token=user.token;detailLoading.value=true;try{const r=await getPayoutOrder(no);if(!detailSeq.valid(id)||token!==user.token)return;if(r.code!==0)throw r;detail.value=r.data;load()}catch(e){if(detailSeq.valid(id))detailError.value=payoutError(e)}finally{if(detailSeq.valid(id))detailLoading.value=false}}
function clear(){capabilities.value={};listSeq.invalidate();closeDetail();rows.value=[];total.value='0';detailOpen.value=false;loading.value=false;error.value='';detailError.value=''}
watch([()=>user.token,()=>user.userInfo?.ID,()=>user.userInfo?.authorityId],clear);onMounted(async()=>{const token=user.token;try{const r=await getPayoutCapabilities();if(token===user.token&&r.code===0)capabilities.value=r.data}catch{};if(token===user.token)await load()});onBeforeUnmount(clear)
</script>
