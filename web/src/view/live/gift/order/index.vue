<template>
  <div>
    <div class="gva-search-box"><el-form inline @submit.prevent="search">
      <el-form-item v-for="[key,label] in filterFields" :key="key" :label="label"><el-input v-model="query[key]" clearable /></el-form-item>
      <el-form-item label="送礼时间"><el-date-picker v-model="range" type="datetimerange" value-format="x" /></el-form-item>
      <el-form-item label="退款"><el-select v-model="query.refunded" clearable style="width:130px"><el-option label="有退款" value="yes" /><el-option label="未退款" value="no" /></el-select></el-form-item>
      <el-button native-type="submit" :loading="loading">查询</el-button>
    </el-form></div>
    <div class="gva-table-box">
      <el-alert v-if="error" :title="error" type="error" :closable="false" />
      <el-table :data="rows" v-loading="loading" @sort-change="sortChanged">
        <el-table-column prop="orderNo" label="订单号" min-width="310" /><el-table-column prop="senderAccountNo" label="用户账号" min-width="120" /><el-table-column prop="anchorNo" label="主播编号" /><el-table-column prop="roomNo" label="房间编号" /><el-table-column prop="giftName" label="礼物" />
        <el-table-column prop="quantity" label="件数" sortable="custom" /><el-table-column prop="grossCoinAmount" label="消费 COIN" sortable="custom" /><el-table-column prop="refundedCoinAmount" label="退款 COIN" sortable="custom" />
        <el-table-column prop="status" label="状态" /><el-table-column label="送礼时间" min-width="170"><template #default="{row}">{{ formatDate(new Date(row.createdAt)) }}</template></el-table-column>
        <el-table-column label="操作" fixed="right" width="160"><template #default="{row}"><el-button link @click="inspect(row)">详情</el-button><el-button link type="danger" @click="openRefund(row)">退款</el-button></template></el-table-column>
      </el-table><el-pagination v-model:current-page="query.page" :page-size="query.pageSize" :total="total" layout="total,prev,pager,next" @change="load" />
    </div>
    <el-drawer v-model="detailOpen" title="礼物订单与账务事实" size="80%">
      <template v-if="detail"><el-alert :type="detail.consistent ? 'success' : 'error'" :title="detail.consistent ? '礼物订单、退款、收入、收益及钱包交易金额核对一致' : detail.issues.join('；')" :closable="false" />
      <el-descriptions :column="2" border><el-descriptions-item v-for="[key,label] in detailFields" :key="key" :label="label">{{ detail.order[key] }}</el-descriptions-item></el-descriptions>
      <el-button @click="walletTrace">查看钱包交易溯源</el-button>
      <el-tabs><el-tab-pane v-for="[key,title,columns] in tables" :key="key" :label="title"><el-table :data="detail[key]"><el-table-column v-for="[prop,label] in columns" :key="prop" :prop="prop" :label="label" min-width="150" show-overflow-tooltip /></el-table></el-tab-pane></el-tabs>
      <el-alert title="广播事件 pending 表示尚未接入 IM，守护权益待接入守护服务；不会伪造完成状态。" :closable="false" />
      </template>
    </el-drawer>
    <el-dialog v-model="refundOpen" title="退回原用户 COIN 钱包" width="580px" :close-on-click-modal="false">
      <el-alert title="退款同时冲减礼物收入与主播待结算收益。网络失败后请使用本窗口原参数重试。" type="warning" :closable="false" />
      <el-form label-width="100px"><el-form-item label="订单号">{{ refundDraft.orderNo }}</el-form-item><el-form-item label="退款件数"><el-input v-model="refundDraft.quantity" :disabled="submitted || writing" /></el-form-item><el-form-item label="退款原因"><el-input v-model="refundDraft.reason" :disabled="submitted || writing" type="textarea" maxlength="500" /></el-form-item><el-form-item label="退款 COIN">{{ preview }}</el-form-item></el-form>
      <el-alert v-if="refundError" :title="refundError" type="error" :closable="false" /><el-button type="danger" :loading="writing" @click="refund">{{ submitted ? '原参数安全重试' : '确认退款' }}</el-button>
    </el-dialog>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { giftQuery, giftWrite } from '@/api/gift'
import { formatDate, CreateUUID } from '@/utils/format'
import { refundAmount, requestSequence } from '../shared'
defineOptions({ name: 'GiftOrder' })
const router=useRouter(), seq=requestSequence(), detailSeq=requestSequence()
const filterFields=[['no','订单号'],['accountNo','用户账号'],['anchorNo','主播编号'],['roomNo','房间编号'],['sessionNo','场次编号'],['categoryNo','分类编号'],['giftNo','礼物编号'],['status','状态'],['minAmount','最低 COIN'],['maxAmount','最高 COIN']]
const query=reactive({page:1,pageSize:20,refunded:'',...Object.fromEntries(filterFields.map(([key])=>[key,'']))}),range=ref([]),rows=ref([]),total=ref(0),loading=ref(false),error=ref('')
const detailOpen=ref(false),detail=ref(null),refundOpen=ref(false),refundDraft=ref({}),selected=ref(null),writing=ref(false),submitted=ref(false),refundError=ref('')
const preview=computed(()=>{try{return selected.value ? refundAmount(selected.value,refundDraft.value.quantity) : ''}catch(e){return e.message}})
const detailFields=[['orderNo','订单号'],['senderAccountNo','送礼账号'],['anchorNo','主播'],['roomNo','房间'],['sessionNo','场次'],['categoryName','分类快照'],['giftName','礼物快照'],['unitPrice','单价 COIN'],['quantity','件数'],['grossCoinAmount','原始 COIN'],['refundedCoinAmount','退款 COIN'],['estimatedAnchorEarningCoin','原始预计收益 COIN'],['revenueRuleNo','分成规则'],['anchorShareBps','分成万分比'],['walletTransactionNo','钱包交易'],['benefitSnapshot','权益规则快照']]
const tables=[['refunds','退款',[['refundNo','退款单号'],['refundQuantity','件数'],['refundCoinAmount','COIN'],['reason','原因'],['walletRefundTransactionNo','钱包退款'],['benefitCompensationStatus','权益补偿']]],['revenue','收入事实',[['revenueNo','流水号'],['entryType','类型'],['grossCoinAmount','COIN']]],['earnings','收益事实',[['earningNo','流水号'],['entryType','类型'],['earningCoinAmount','COIN'],['status','状态']]],['benefits','权益',[['grantNo','权益号'],['benefitType','类型'],['status','状态'],['lastError','异常']]],['compensations','权益补偿',[['compensationNo','补偿号'],['durationDays','天数'],['status','状态']]],['events','广播事件',[['eventId','事件号'],['deliveryStatus','状态'],['retryCount','重试'],['lastError','异常']]]]
async function load(){const id=seq.next();loading.value=true;error.value='';try{const r=await giftQuery('order/list',{...query,from:Number(range.value?.[0]||0),to:Number(range.value?.[1]||0)});if(!seq.valid(id))return;if(r.code!==0)throw new Error(r.msg);rows.value=r.data.list;total.value=Number(r.data.total)}catch(e){if(seq.valid(id))error.value=e.message||'查询失败'}finally{if(seq.valid(id))loading.value=false}}
function search(){query.page=1;load()}
function sortChanged({prop,order}){query.sort=order?({quantity:'quantity',grossCoinAmount:'amount',refundedCoinAmount:'refund'})[prop]:'';query.desc=order==='descending';search()}
async function inspect(row){const id=detailSeq.next();try{const r=await giftQuery('order/detail',{no:row.orderNo});if(!detailSeq.valid(id))return;if(r.code!==0)throw new Error(r.msg);detail.value=r.data;detailOpen.value=true}catch(e){ElMessage.error(e.message)}}
function walletTrace(){router.push({name:'walletTrace',query:{no:detail.value.order.walletTransactionNo}})}
function openRefund(row){if(writing.value)return;if(submitted.value){refundOpen.value=true;return}selected.value={...row};refundDraft.value={orderNo:row.orderNo,quantity:'1',reason:'',requestId:CreateUUID()};refundError.value='';refundOpen.value=true}
async function refund(){if(writing.value)return;writing.value=true;try{const data={...refundDraft.value};const amount=refundAmount(selected.value,data.quantity);if(data.reason.trim().length<3)throw new Error('请输入至少3字退款原因');if(!submitted.value)await ElMessageBox.confirm(`确认退回 ${amount} COIN？`,'退款确认');refundDraft.value=data;submitted.value=true;const r=await giftWrite('refund',data);if(r.code!==0)throw new Error(r.msg);submitted.value=false;refundOpen.value=false;await load()}catch(e){if(e instanceof Error)refundError.value=e.message}finally{writing.value=false}}
onMounted(load);onBeforeUnmount(()=>{seq.invalidate();detailSeq.invalidate()})
</script>
