<template>
 <section>
  <el-alert title="人工出款不会自动调用银行。只有财务线下真实转账、提交凭证并独立复核后才记录付款成功；未知结果必须继续冻结。" type="warning" :closable="false" />
  <div class="my-4"><el-button v-for="[path,label] in actions" :key="path" :disabled="busy" @click="open(path,label)">{{ label }}</el-button>
   <SensitiveReveal :target-no="detail.order.payoutNo" :allowed="!!detail.allowedActions?.['order/reveal-payee']"/>
  </div>
  <div v-if="detail.allowedActions?.['evidence/upload']"><label>上传当前尝试凭证（PNG/JPEG，最多2MiB；上传后不能释放领取）<input type="file" accept="image/png,image/jpeg" :disabled="busy" @change="upload"/></label><el-button v-if="uploadPending" :loading="busy" @click="retryUpload">按原请求重试上传</el-button><p v-if="proofNo">已上传凭证编号：{{ proofNo }}</p></div>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <h3>出款尝试</h3><el-table :data="detail.attempts"><el-table-column v-for="[key,label] in attemptFields" :key="key" :prop="key" :label="label" min-width="160" show-overflow-tooltip/><el-table-column label="私有凭证" min-width="150"><template #default="{row}"><el-button v-if="row.evidenceReference && detail.allowedActions?.['evidence/read']" @click="viewEvidence(row.evidenceReference)">审计查看</el-button></template></el-table-column></el-table>
  <h3>调查记录</h3><p v-if="detail.investigationsTruncated">显示最近200条调查；完整记录保留在审计数据库。</p><el-table :data="detail.investigations"><el-table-column prop="investigationNo" label="调查编号"/><el-table-column prop="actorId" label="调查人"/><el-table-column prop="note" label="调查说明"/><el-table-column prop="evidenceNo" label="新凭证编号"/></el-table>
  <h3>内部对账（非银行自动对账）</h3><p v-if="detail.check">{{ detail.check.consistent?'内部账务链通过':detail.check.issues?.join('、') }}</p><el-table :data="detail.reconciliations"><el-table-column prop="reconciliationNo" label="对账编号"/><el-table-column prop="status" label="结果"/><el-table-column prop="snapshotHash" label="快照哈希"/></el-table>
  <el-dialog v-model="visible" :title="title" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy" @closed="close">
   <el-alert title="结果不明确时保持原参数、原请求号重试。流水号须来自真实银行记录，金额与收款账户由订单固定。" type="info" :closable="false"/>
   <el-form label-width="150px" :disabled="busy||!!pending">
    <el-form-item v-if="['order/start','order/retry'].includes(path)" label="公司出款账户"><el-select v-model="form.channelAccountNo" filterable><el-option v-for="c in companies" :key="c.channelAccountNo" :value="c.channelAccountNo" :label="`${c.accountName} · ${c.maskedAccount} · ${c.currency}`"/></el-select><el-button :disabled="companyPage===1||companyLoading" @click="loadCompanies(companyPage-1)">上一页</el-button><span>第 {{ companyPage }} 页 / 共 {{ companyTotal }} 个可用账户</span><el-button :disabled="companyPage*100>=Number(companyTotal)||companyLoading" @click="loadCompanies(companyPage+1)">下一页</el-button></el-form-item>
    <template v-if="path==='attempt/submit'"><el-form-item label="真实银行流水号"><el-input v-model="form.paymentReference" maxlength="128" autocomplete="off"/></el-form-item><el-form-item label="真实支付时间"><el-date-picker v-model="form.paymentTime" type="datetime" value-format="x"/></el-form-item></template>
    <el-form-item v-if="['attempt/submit','order/investigate'].includes(path)" label="本人当前尝试的凭证"><el-select v-model="form.evidenceNo"><el-option v-for="ev in ownEvidence" :key="ev.evidenceNo" :label="ev.evidenceNo" :value="ev.evidenceNo"/></el-select></el-form-item>
    <template v-if="path==='attempt/resolve-unknown'"><el-form-item label="本人新调查记录"><el-select v-model="form.investigationNo"><el-option v-for="v in ownInvestigations" :key="v.investigationNo" :label="v.investigationNo" :value="v.investigationNo"/></el-select></el-form-item><el-form-item label="确认结果"><el-select v-model="form.decision"><el-option label="确认付款成功" value="success"/><el-option label="确认未付款成功" value="failure"/></el-select></el-form-item></template>
    <el-form-item label="原因 / 核验说明"><el-input v-model="form.reason" type="textarea" maxlength="500" placeholder="记录核验依据；不要填写完整账号或其他敏感资料"/></el-form-item>
    <el-form-item><el-checkbox v-model="form.confirm">我已核验事实并确认本次操作；失败/取消表示确认未付款成功</el-checkbox></el-form-item>
   </el-form><el-alert v-if="pending" title="参数已锁定；如网络结果不明确，点击下方按钮按原请求重试。" type="warning" :closable="false"/>
   <template #footer><el-button :disabled="busy" @click="visible=false">关闭</el-button><el-button type="primary" :loading="busy" :disabled="!form.confirm" @click="submit">{{pending?'按原请求重试':'确认提交'}}</el-button></template>
  </el-dialog>
  <el-dialog v-model="imageOpen" title="私有支付凭证（查看已审计）" @close="clearImage"><img v-if="imageURL" :src="imageURL" alt="本次私有支付凭证" style="max-width:100%"/></el-dialog>
 </section>
</template>
<script setup>
import { computed,ref,reactive,watch,onBeforeUnmount,onDeactivated } from 'vue'
import { useUserStore } from '@/pinia/modules/user'
import { CreateUUID } from '@/utils/format'
import { manualPayout,listCompanyAccounts,readPayoutEvidence } from '@/api/payout'
import { manualActions,manualBody,manualError } from './shared'
import SensitiveReveal from './SensitiveReveal.vue'
const props=defineProps({detail:{type:Object,required:true}}),emit=defineEmits(['changed']),user=useUserStore()
const actions=computed(()=>manualActions.filter(([p])=>props.detail.allowedActions?.[p])),ownInvestigations=computed(()=>(props.detail.investigations||[]).filter(i=>String(i.actorId)===String(user.userInfo?.ID)&&i.evidenceNo))
const ownEvidence=computed(()=>(props.detail.evidence||[]).filter(ev=>ev.attemptNo===props.detail.attempts.at(-1)?.attemptNo&&String(ev.createdBy)===String(user.userInfo?.ID)))
const visible=ref(false),path=ref(''),title=ref(''),busy=ref(false),error=ref(''),pending=ref(null),companies=ref([]),proofNo=ref(''),uploadPending=ref(null),imageOpen=ref(false),imageURL=ref('')
const companyPage=ref(1),companyTotal=ref('0'),companyLoading=ref(false)
const form=reactive({reason:'',confirm:false,channelAccountNo:'',paymentReference:'',paymentTime:'',evidenceNo:'',investigationNo:'',decision:''})
const attemptFields=[['attemptNo','尝试编号'],['sequence','次数'],['status','状态'],['channelAccountNo','公司账户编号'],['companyMasked','公司脱敏账号'],['operatorId','操作人'],['reviewerId','复核人'],['paymentReference','银行流水号'],['paymentTime','支付时间（毫秒）'],['evidenceHash','凭证SHA-256'],['reviewReason','复核说明']]
let epoch=0,imageEpoch=0,companyEpoch=0
function clearImage(){imageEpoch++;if(imageURL.value)URL.revokeObjectURL(imageURL.value);imageURL.value='';imageOpen.value=false}
function close(){if(busy.value)return;visible.value=false;/* Preserve an ambiguous request in memory until it is retried or this order closes. */}
function clear(){epoch++;companyEpoch++;companyLoading.value=false;companyPage.value=1;companyTotal.value='0';clearImage();busy.value=false;visible.value=false;pending.value=null;uploadPending.value=null;proofNo.value='';companies.value=[];error.value='';Object.keys(form).forEach(k=>form[k]=k==='confirm'?false:'')}
async function open(p,label){if(busy.value)return;if(pending.value&&path.value!==p){error.value='请先按原请求重试未确认的操作，或关闭详情后查询最新订单状态';return}path.value=p;title.value=label;visible.value=true;form.evidenceNo=proofNo.value||form.evidenceNo
 if(['order/start','order/retry'].includes(p))await loadCompanies(1)}
async function loadCompanies(page){const id=++companyEpoch,token=user.token;companyLoading.value=true;try{const r=await listCompanyAccounts({page,pageSize:100,status:'active',currency:props.detail.order.currency});if(id!==companyEpoch||token!==user.token)return;if(r.code!==0)throw r;companies.value=r.data.list;companyPage.value=page;companyTotal.value=r.data.total}catch(e){if(id===companyEpoch)error.value=manualError(e)}finally{if(id===companyEpoch)companyLoading.value=false}}
async function submit(){if(busy.value||!form.confirm)return;if(form.reason.trim().length<3){error.value='请填写至少3个字符的核验说明';return;}if(!pending.value)pending.value=manualBody(path.value,form,props.detail,CreateUUID());const id=epoch,token=user.token;busy.value=true;error.value=''
 try{const r=await manualPayout(path.value,pending.value);if(id!==epoch||token!==user.token)return;if(r.code!==0)throw r;pending.value=null;visible.value=false;emit('changed')}catch(e){if(id===epoch){error.value=manualError(e);if(Number(e?.code)===49201)pending.value=null}}finally{if(id===epoch)busy.value=false}}
async function upload(event){const file=event.target.files?.[0];event.target.value='';if(!file||busy.value)return;if(uploadPending.value){error.value='请先重试上次上传';return}if(!['image/png','image/jpeg'].includes(file.type)||file.size>2*1024*1024){error.value='只允许不超过2MiB的PNG/JPEG原始凭证';return}const id=epoch,token=user.token;busy.value=true
 try{const bytes=new Uint8Array(await file.arrayBuffer());if(id!==epoch||token!==user.token)return;let binary='';for(const b of bytes)binary+=String.fromCharCode(b);uploadPending.value={requestId:CreateUUID(),attemptNo:props.detail.attempts.at(-1).attemptNo,mime:file.type,content:btoa(binary)};bytes.fill(0)}finally{if(id===epoch)busy.value=false}if(id===epoch)await retryUpload()}
async function retryUpload(){if(busy.value||!uploadPending.value)return;const id=epoch,token=user.token;busy.value=true;try{const r=await manualPayout('evidence/upload',uploadPending.value);if(id!==epoch||token!==user.token)return;if(r.code!==0)throw r;proofNo.value=r.data.evidenceNo;form.evidenceNo=proofNo.value;uploadPending.value=null;emit('changed')}catch(e){if(id===epoch)error.value=manualError(e)}finally{if(id===epoch)busy.value=false}}
async function viewEvidence(no){clearImage();const id=imageEpoch,token=user.token;try{const blob=await readPayoutEvidence({requestId:CreateUUID(),evidenceNo:no,reason:'后台独立核验支付凭证'});if(id!==imageEpoch||token!==user.token)return;imageURL.value=URL.createObjectURL(blob);imageOpen.value=true}catch(e){if(id===imageEpoch)error.value=manualError(e)}}
watch([()=>user.token,()=>user.userInfo?.ID,()=>user.userInfo?.authorityId,()=>props.detail.order.payoutNo],clear)
onBeforeUnmount(clear);onDeactivated(clear)
</script>
