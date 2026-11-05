<template>
 <div class="gva-table-box">
  <h2>公司出款账户</h2><el-alert title="平台单笔/每日限额不是银行余额；每日限额按UTC日累计领取金额，释放或失败不退回当日额度。账户资料创建后不可更改，不保存网银密码、验证码或U盾信息。" type="info" :closable="false"/>
  <div class="my-4"><el-input v-model="currency" placeholder="币种" style="width:140px"/><el-select v-model="status" clearable placeholder="状态" style="width:140px"><el-option value="active" label="启用"/><el-option value="disabled" label="停用"/></el-select><el-button @click="search" :loading="loading">查询</el-button><el-button v-if="caps['channel-account/create']" @click="openCreate">新增公司账户</el-button></div>
  <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  <el-table :data="rows" v-loading="loading"><el-table-column v-for="[key,label] in fields" :key="key" :prop="key" :label="label" min-width="140" show-overflow-tooltip/><el-table-column label="操作" min-width="260"><template #default="{row}"><el-button v-if="caps['channel-account/'+(row.status==='active'?'disable':'enable')]" @click="openChange(row)">{{row.status==='active'?'停用':'启用'}}</el-button><SensitiveReveal :target-no="row.channelAccountNo" company :allowed="!!caps['channel-account/reveal']&&!!caps['channel-account/reveal-grant']"/></template></el-table-column></el-table>
  <p>共 {{ total }} 条</p><el-pagination v-model:current-page="page" :page-size="20" :total="Math.min(Number(total),2000)" @current-change="load"/>
  <el-dialog v-model="visible" :title="action==='create'?'新增公司出款账户':'更改公司账户状态'" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy" @close="close">
   <el-form :disabled="busy||!!pending" label-width="140px" autocomplete="off">
    <template v-if="action==='create'"><el-form-item label="渠道">人工银行转账（manual_bank）</el-form-item><el-form-item v-for="[key,label] in createFields" :key="key" :label="label"><el-input v-model="form[key]" :maxlength="key==='account'?34:128" autocomplete="off"/></el-form-item></template>
    <template v-else><p>{{ target?.accountName }} · {{ target?.maskedAccount }}</p><el-form-item label="原因"><el-input v-model="form.reason" type="textarea" maxlength="500" placeholder="不得填写完整账号"/></el-form-item></template>
   </el-form><el-alert v-if="pending" title="结果未确认时请使用原请求重试；参数已锁定。" type="warning" :closable="false"/>
   <template #footer><el-button :loading="busy" type="primary" @click="submit">{{pending?'按原请求重试':'确认提交'}}</el-button></template>
  </el-dialog>
 </div>
</template>
<script setup>
import {ref,reactive,onMounted,onBeforeUnmount,onDeactivated,watch} from 'vue'
import {useUserStore} from '@/pinia/modules/user'
import {CreateUUID} from '@/utils/format'
import {getPayoutCapabilities,listCompanyAccounts,manualPayout} from '@/api/payout'
import {manualError} from '../shared'
import SensitiveReveal from '../SensitiveReveal.vue'
const user=useUserStore(),caps=ref({}),rows=ref([]),total=ref('0'),page=ref(1),currency=ref(''),status=ref(''),loading=ref(false),error=ref(''),visible=ref(false),action=ref('create'),target=ref(null),busy=ref(false),pending=ref(null)
const form=reactive({accountName:'',institutionName:'',currency:'',account:'',singleLimit:'',dailyLimit:'',reason:''})
const createFields=[['accountName','公司户名'],['institutionName','开户机构'],['currency','币种'],['account','完整公司账号'],['singleLimit','单笔限额（最小单位）'],['dailyLimit','每日限额（最小单位）']],fields=[['channelAccountNo','公司账户编号'],['accountName','公司户名'],['institutionName','开户机构'],['currency','币种'],['maskedAccount','脱敏账号'],['status','状态'],['singleLimit','单笔风控限额'],['dailyLimit','每日风控限额']]
let epoch=0,listSeq=0
function close(){if(busy.value)return;Object.keys(form).forEach(k=>form[k]='');pending.value=null;target.value=null;visible.value=false}
function clear(){epoch++;listSeq++;busy.value=false;loading.value=false;close();caps.value={};rows.value=[];total.value='0';error.value=''}
function openCreate(){if(busy.value)return;close();action.value='create';visible.value=true}
function openChange(row){if(busy.value)return;close();target.value=row;action.value=row.status==='active'?'disable':'enable';visible.value=true}
async function load(){const id=++listSeq,token=user.token;loading.value=true;try{const r=await listCompanyAccounts({page:page.value,pageSize:20,...(currency.value?{currency:currency.value}:{}),...(status.value?{status:status.value}:{})});if(id!==listSeq||token!==user.token)return;if(r.code!==0)throw r;rows.value=r.data.list;total.value=r.data.total}catch(e){if(id===listSeq){rows.value=[];error.value=manualError(e)}}finally{if(id===listSeq)loading.value=false}}
function search(){page.value=1;return load()}
async function submit(){if(busy.value)return;if(!pending.value)pending.value=action.value==='create'?{requestId:CreateUUID(),channelCode:'manual_bank',...Object.fromEntries(createFields.map(([k])=>[k,form[k]]))}:{requestId:CreateUUID(),channelAccountNo:target.value.channelAccountNo,reason:form.reason};const id=epoch,token=user.token;busy.value=true
 try{const r=await manualPayout('channel-account/'+action.value,pending.value);if(id!==epoch||token!==user.token)return;if(r.code!==0)throw r;busy.value=false;close();await load()}catch(e){if(id===epoch)error.value=manualError(e)}finally{if(id===epoch)busy.value=false}}
async function init(){const id=epoch,token=user.token;try{const r=await getPayoutCapabilities();if(id!==epoch||token!==user.token)return;if(r.code!==0)throw r;caps.value=r.data;if(caps.value['channel-account/list'])await load()}catch(e){if(id===epoch)error.value=manualError(e)}}
watch([()=>user.token,()=>user.userInfo?.ID,()=>user.userInfo?.authorityId],clear);onMounted(init);onBeforeUnmount(clear);onDeactivated(clear)
</script>
