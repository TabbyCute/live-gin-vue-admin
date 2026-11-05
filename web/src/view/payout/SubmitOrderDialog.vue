<template>
 <el-dialog v-model="visible" title="提交打款中心" width="650px" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy">
  <el-alert title="本操作只创建待出款订单，不会实际打款；金额继续保持冻结。" type="warning" :closable="false" />
  <el-descriptions v-if="target" :column="1" border class="mt-4">
   <el-descriptions-item label="提现单号">{{ target.withdrawalNo }}</el-descriptions-item><el-descriptions-item label="主播">{{ target.anchorNo }}</el-descriptions-item>
   <el-descriptions-item label="金额">{{ target.amount }} {{ target.currency }} 最小单位</el-descriptions-item><el-descriptions-item label="收款方式">{{ payeeMethodLabel(target.payeeMethod) }}</el-descriptions-item>
   <el-descriptions-item label="脱敏账户">{{ target.accountMasked }}</el-descriptions-item><el-descriptions-item label="账户版本">{{ target.payeeVersion }}</el-descriptions-item>
  </el-descriptions>
  <el-alert v-if="error" :title="error" type="error" :closable="false" class="mt-4" />
  <el-alert v-if="result" :title="`已提交打款中心，出款单号：${result.payoutNo}，状态：待出款`" type="success" :closable="false" class="mt-4" />
  <template #footer><el-button :disabled="busy" @click="visible=false">关闭</el-button><el-button v-if="!result" type="primary" :loading="busy" @click="submit">{{ attempted ? '按原请求重试' : '确认创建待出款订单' }}</el-button></template>
 </el-dialog>
</template>
<script setup>
import { ref, watch, onBeforeUnmount } from 'vue'
import { CreateUUID } from '@/utils/format'
import { useUserStore } from '@/pinia/modules/user'
import { createPayoutOrder } from '@/api/payout'
import { payeeMethodLabel } from '@/view/live/earning/shared'
import { canSubmitPayout, payoutError } from './shared'
const emit=defineEmits(['created']),user=useUserStore()
const visible=ref(false),busy=ref(false),attempted=ref(false),target=ref(null),result=ref(null),error=ref('')
let request=null,generation=0,ownerToken=''
function open(row){if(busy.value||!canSubmitPayout(row))return;generation++;ownerToken=user.token;target.value={...row};request=Object.freeze({requestId:CreateUUID(),withdrawalNo:row.withdrawalNo});result.value=null;error.value='';attempted.value=false;visible.value=true}
async function submit(){if(busy.value||!request||result.value||!visible.value||ownerToken!==user.token)return;busy.value=true;error.value='';attempted.value=true;const id=generation,body=request,token=user.token
 try{const r=await createPayoutOrder(body);if(id!==generation||token!==user.token)return;if(r.code!==0)throw r
 if(r.data?.businessNo!==body.withdrawalNo||r.data?.status!=='ready'||!/^po_[a-f0-9]{32}$/.test(r.data?.payoutNo||''))throw new Error('Invalid order result')
 result.value=r.data;emit('created',r.data)
 }catch(e){if(id===generation&&token===user.token)error.value=payoutError(e)}finally{if(id===generation)busy.value=false}}
function clear(){generation++;visible.value=false;busy.value=false;request=null;target.value=null;result.value=null;error.value=''}
watch(()=>user.token,clear);onBeforeUnmount(clear);defineExpose({open})
</script>
