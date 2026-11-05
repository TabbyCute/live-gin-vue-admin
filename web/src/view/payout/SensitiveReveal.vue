<template>
 <el-button v-if="allowed" @click="open">{{ company?'查看完整公司账号':'查看完整收款资料' }}</el-button>
 <el-dialog v-model="visible" title="敏感资料 · 单次查看" :close-on-click-modal="false" @close="clear">
  <el-alert title="仅用于当前人工出款。查看会留下审计；明文最多展示30秒，关闭后立即清除。" type="warning" :closable="false" />
  <template v-if="!data"><el-input v-model="reason" maxlength="500" placeholder="填写本次查看原因，不填写账号" :disabled="busy"/><el-button :loading="busy" @click="reveal">申请并查看</el-button></template>
  <el-descriptions v-else :column="1" border><el-descriptions-item v-for="[key,label] in fields" :key="key" :label="label">{{ data[key] || '—' }}</el-descriptions-item></el-descriptions>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
 </el-dialog>
</template>
<script setup>
import { ref,watch,onBeforeUnmount,onDeactivated } from 'vue'
import { useUserStore } from '@/pinia/modules/user'
import { CreateUUID } from '@/utils/format'
import { manualPayout } from '@/api/payout'
import { manualError } from './shared'
const props=defineProps({targetNo:String,company:Boolean,allowed:Boolean}),user=useUserStore()
const visible=ref(false),data=ref(null),reason=ref(''),busy=ref(false),error=ref('')
const fields=[['accountName','户名'],['account','完整账号'],['method','方式'],['currency','币种'],['bankCode','银行代码'],['institutionName','机构']]
let epoch=0,timer
function clear(){epoch++;clearTimeout(timer);data.value=null;reason.value='';busy.value=false;error.value='';visible.value=false}
function open(){clear();visible.value=true}
async function reveal(){if(busy.value||reason.value.trim().length<3)return;const id=++epoch,token=user.token,target=props.targetNo;busy.value=true;error.value='';const path=props.company?'channel-account/reveal':'order/reveal-payee'
 try{const grant=await manualPayout(path+'-grant',{requestId:CreateUUID(),targetNo:target,reason:reason.value});if(id!==epoch||token!==user.token||target!==props.targetNo)return;if(grant.code!==0)throw grant
 const r=await manualPayout(path,{requestId:CreateUUID(),grantNo:grant.data.grantNo,token:grant.data.token});if(id!==epoch||token!==user.token||target!==props.targetNo)return;if(r.code!==0)throw r;data.value=r.data;timer=setTimeout(clear,30000)
 }catch(e){if(id===epoch)error.value=manualError(e)}finally{if(id===epoch)busy.value=false}}
watch(()=>[user.token,user.userInfo?.ID,user.userInfo?.authorityId,props.targetNo,props.allowed],clear)
onBeforeUnmount(clear);onDeactivated(clear)
</script>
