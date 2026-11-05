<template>
  <div>
    <div class="gva-search-box">
      <h2>{{ title }}</h2>
      <el-alert v-if="kind === 'withdrawal'" type="info" :closable="false" title="审核通过后可提交打款中心创建待出款订单。金额继续保持冻结，不会实际打款。" />
      <el-alert v-if="kind === 'exchange'" type="info" :closable="false">
        <template #title>怎么填写：先决定“多少收益 COIN 可以换 1 元”</template>
        <p><strong>假设运营决定 1000 收益 COIN 可以换人民币 1 元：</strong></p>
        <p>① 结算币种填写 CNY；②“每多少收益 COIN”填写 1000；③“可兑换多少分”填写 100，因为人民币 1 元等于 100 分。</p>
        <p>按这个示例计算：1000 COIN = 1 元，5000 COIN = 5 元，10000 COIN = 10 元。</p>
        <p>这只是帮助理解的示例，不是系统默认比例，请按实际运营方案填写。“主播分成规则”负责计算主播获得多少收益 COIN，本页只负责把收益 COIN 换算成可提现金额。</p>
        <p>超级 root（ID 1）可自行复核，其他管理员须由另一位管理员复核后才会生效。</p>
      </el-alert>
      <el-form :inline="true" :model="query" class="mt-4" @submit.prevent="search">
        <el-form-item v-if="kind !== 'exchange'" label="主播编号"><el-input v-model.trim="query.anchorNo" clearable maxlength="40" /></el-form-item>
        <el-form-item v-if="kind !== 'account'" :label="kind === 'reconciliation' ? '提现单号' : '单号'"><el-input v-model.trim="query.no" clearable maxlength="40" /></el-form-item>
        <el-form-item v-if="kind === 'withdrawal'" label="收款账户编号"><el-input v-model.trim="query.payeeAccountNo" clearable maxlength="35" placeholder="pa_ 开头，用于去向排查" /></el-form-item>
        <el-form-item label="币种"><el-input v-model.trim="query.currency" clearable maxlength="8" placeholder="如 CNY" /></el-form-item>
        <el-form-item v-if="kind !== 'account'" label="状态"><el-select v-model="query.status" clearable style="width:150px"><el-option v-for="s in statuses" :key="s" :value="s" :label="statusLabel(s)" /></el-select></el-form-item>
        <el-form-item label="时间"><el-date-picker v-model="range" type="datetimerange" value-format="x" start-placeholder="开始" end-placeholder="结束（不包含）" /></el-form-item>
        <el-form-item><el-button type="primary" :loading="loading" @click="search">查询</el-button><el-button @click="reset">重置</el-button></el-form-item>
      </el-form>
      <el-button v-if="kind === 'exchange' || kind === 'settlement'" type="primary" :disabled="writing" @click="openCreate">{{ kind === 'exchange' ? '创建兑换规则' : '生成结算申请' }}</el-button>
      <el-button v-if="kind === 'reconciliation'" type="primary" :disabled="writing" @click="openReconcile">发起提现对账</el-button>
      <el-alert v-if="error" class="mt-4" type="error" :closable="false" :title="error" />
    </div>
    <div class="gva-table-box">
      <el-table :data="rows" v-loading="loading" @sort-change="sortChanged">
        <el-table-column v-for="[key,label] in columns[kind]" :key="key" :prop="key" :label="label" min-width="150" show-overflow-tooltip :sortable="['amount','createdAt'].includes(key) ? 'custom' : false"><template #default="{row}">{{ display(key,row[key]) }}</template></el-table-column>
        <el-table-column label="操作" fixed="right" :width="kind === 'withdrawal' ? 340 : 240"><template #default="{row}">
          <el-button link :disabled="writing" @click="inspect(row)">{{ kind === 'account' ? '流水' : '详情' }}</el-button>
          <template v-if="['exchange','settlement','withdrawal'].includes(kind) && ['pending','pending_review'].includes(row.status)"><el-button link type="success" :disabled="writing" @click="openReview(row,true)">通过</el-button><el-button link type="danger" :disabled="writing" @click="openReview(row,false)">驳回</el-button></template>
          <el-button v-if="kind === 'account'" link :disabled="writing" @click="check(row)">完整对账</el-button>
          <el-button v-if="kind === 'withdrawal'" link :disabled="writing" @click="runReconcile(row.withdrawalNo)">自动对账</el-button>
          <template v-if="kind === 'withdrawal'">
            <el-button v-if="payoutCapabilities['order/create'] && (canSubmitPayout(row) || row.payoutNo)" link type="primary" :disabled="writing || !!row.payoutNo" @click="payoutSubmitter?.open(row)">提交打款中心</el-button>
            <span v-if="row.payoutNo">已提交打款中心：{{ row.payoutNo }} · {{ payoutStates[row.payoutStatus] || row.payoutStatus }}</span>
          </template>
        </template></el-table-column>
      </el-table>
      <div class="mt-4">共 {{ total }} 条</div>
      <el-pagination v-model:current-page="query.page" v-model:page-size="query.pageSize" :total="paginationTotal" :page-sizes="[20,50,100]" layout="prev,pager,next,sizes" @current-change="load" @size-change="search" />
    </div>
    <el-dialog v-model="dialog" :title="dialogTitle" width="620px" :close-on-click-modal="false" :close-on-press-escape="!writing" :show-close="!writing">
      <el-form label-width="130px" :disabled="writing || submitted" @submit.prevent>
        <template v-if="action === 'create' && kind === 'exchange'">
          <el-form-item label="规则名称"><el-input v-model.trim="draft.name" maxlength="80" /></el-form-item>
          <el-form-item label="收益单位"><el-input model-value="COIN" disabled /></el-form-item>
          <el-form-item label="结算币种"><el-input v-model.trim="draft.currency" maxlength="8" placeholder="由业务确认币种，不预设" /></el-form-item>
          <el-form-item label="每多少收益 COIN"><el-input v-model.trim="draft.coinUnits" placeholder="例如：1000" /></el-form-item>
          <el-form-item label="可兑换多少最小单位"><el-input v-model.trim="draft.minorUnits" placeholder="CNY 按分填写：1 元填 100" /></el-form-item>
          <el-alert v-if="exchangePreviewItems.length" type="success" :closable="false" show-icon>
            <template #title>收益预览（结算审核通过后进入可提现账户）</template>
            <p v-for="item in exchangePreviewItems" :key="item">{{ item }}</p>
          </el-alert>
          <el-form-item label="生效时间"><el-date-picker v-model="draft.effectiveFrom" type="datetime" value-format="x" /></el-form-item>
          <el-form-item label="截止时间"><el-date-picker v-model="draft.effectiveTo" type="datetime" value-format="x" /></el-form-item>
        </template>
        <template v-if="action === 'review'"><el-form-item label="审核单号"><el-input :model-value="draft.no" disabled /></el-form-item><el-form-item label="决定"><el-tag>{{ draft.approve ? '通过' : '驳回' }}</el-tag></el-form-item><el-form-item label="审核原因"><el-input v-model.trim="draft.reason" type="textarea" maxlength="500" show-word-limit /></el-form-item></template>
        <el-form-item v-if="action === 'reconcile'" label="提现单号"><el-input v-model.trim="draft.withdrawalNo" maxlength="40" /></el-form-item>
      </el-form>
      <el-alert v-if="writeError" class="mt-4" type="error" :closable="false" :title="writeError" />
      <el-alert v-if="submitted" class="mt-4" type="info" :closable="false" title="请求已提交。结果不明确时请按原参数重试，避免重复创建申请。" />
      <template #footer><el-button :disabled="writing" @click="dialog=false">关闭</el-button><el-button type="primary" :loading="writing" @click="submit">{{ submitted ? '按原参数重试' : '确认提交' }}</el-button></template>
    </el-dialog>
    <SettlementCreateDialog ref="settlementCreator" @created="load" />
    <SubmitOrderDialog ref="payoutSubmitter" @created="load" />
    <el-drawer v-model="detailOpen" title="账务详情" size="85%">
      <el-alert v-if="detailError" type="error" :title="detailError" :closable="false" />
      <template v-if="detail">
        <el-alert v-if="typeof detail.consistent === 'boolean'" :type="detail.consistent ? 'success' : 'error'" :title="detail.consistent ? '账务核验通过' : (detail.issue || '账务核验异常')" :closable="false" />
        <el-descriptions :column="2" border class="mt-4"><el-descriptions-item v-for="[key,value] in detailEntries" :key="key" :label="labels[key] || key">{{ display(key,value) }}</el-descriptions-item></el-descriptions>
        <section v-if="kind === 'withdrawal' && detail.currentPayee" class="mt-4">
          <h3>当前绑定账户核验</h3>
          <el-alert :closable="false" :type="detail.currentPayee.usable ? 'info' : 'warning'" :title="payeeStateLabel(detail.currentPayee.state)" />
          <el-descriptions :column="2" border class="mt-4">
            <el-descriptions-item label="与申请快照一致">{{ detail.currentPayee.matchesSnapshot ? '是' : '否' }}</el-descriptions-item>
            <el-descriptions-item label="当前绑定可用">{{ detail.currentPayee.usable ? '是（不代表允许实际出款）' : '否' }}</el-descriptions-item>
            <el-descriptions-item label="当前脱敏账户">{{ detail.currentPayee.account?.maskedAccount || '—' }}</el-descriptions-item>
            <el-descriptions-item label="当前账户版本">{{ detail.currentPayee.account?.version || '—' }}</el-descriptions-item>
          </el-descriptions>
          <p>上方为申请时不可变快照。审核通过前服务端会重新执行账务和账户核验。账户不可用时须按后续取消流程释放冻结后重新申请，不可更换历史单收款账户。</p>
        </section>
        <SnapshotPanel v-if="detail.snapshot" :value="detail.snapshot" class="mt-4" />
        <template v-for="proof in detail.reconciliations || []" :key="proof.reconciliationNo"><h3>{{ proof.reconciliationNo }}</h3><SnapshotPanel :value="proof.snapshot" /></template>
        <template v-for="[key,title] in [['items','结算收益明细'],['ledger','账户流水'],['audit','审计记录']]" :key="key"><template v-if="detail[key]?.length"><h3>{{ title }}</h3><el-table :data="detail[key]" max-height="400"><el-table-column v-for="field in tableFields(detail[key])" :key="field" :label="labels[field] || field" :prop="field" min-width="160" show-overflow-tooltip><template #default="{row}">{{ display(field,row[field]) }}</template></el-table-column></el-table></template></template>
        <el-pagination v-if="kind === 'account' && ledgerTarget" v-model:current-page="ledgerPage" :total="ledgerTotal" :page-size="50" layout="prev,pager,next" @current-change="loadLedger" />
      </template>
    </el-drawer>
  </div>
</template>
<script setup>
import { computed, onMounted, onBeforeUnmount, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { earningQuery, earningWrite } from '@/api/earning'
import { CreateUUID, formatDate } from '@/utils/format'
import { columns, labels, exchangePreview, integerAmount, sequence, statusLabel, payeeMethodLabel, payeeStateLabel } from './shared'
import SnapshotPanel from './SnapshotPanel.vue'
import SettlementCreateDialog from './SettlementCreateDialog.vue'
import SubmitOrderDialog from '@/view/payout/SubmitOrderDialog.vue'
import { canSubmitPayout,payoutStates } from '@/view/payout/shared'
import { getPayoutCapabilities } from '@/api/payout'
const payoutCapabilities=ref({})
const payoutSubmitter=ref(null)
const settlementCreator = ref(null)
const props=defineProps({ kind:{type:String,required:true}, title:{type:String,required:true} })
const kind=props.kind
const query=reactive({page:1,pageSize:20,anchorNo:'',no:'',payeeAccountNo:'',currency:'',status:'',sort:'',desc:true})
const range=ref([]),rows=ref([]),total=ref('0'),loading=ref(false),error=ref('')
const listSeq=sequence(),detailSeq=sequence()
const statuses=computed(()=>kind==='withdrawal'?['pending_review','approved','rejected','paid','cancelled']:kind==='reconciliation'?['passed','abnormal']:kind==='exchange'?['pending','active','rejected']:['pending','approved','rejected'])
const paginationTotal=computed(()=>Math.min(Number(total.value),1000000))
const display=(key,value)=>key==='payeeMethod'?payeeMethodLabel(value):key.endsWith('At')||['effectiveFrom','effectiveTo','periodStart','periodEnd'].includes(key)?(value?formatDate(new Date(Number(value)),'yyyy-MM-dd hh:mm:ss'):'—'):key.toLowerCase().includes('status')||key==='entryType'?statusLabel(value):value??'—'
async function load(){const id=listSeq.next();loading.value=true;error.value='';const params={...query};for(const key of Object.keys(params))if(params[key]==='')delete params[key];if(range.value?.length===2){params.from=Number(range.value[0]);params.to=Number(range.value[1])}try{const r=await earningQuery(kind+'/list',params);if(!listSeq.valid(id))return;if(r.code!==0)throw new Error(r.msg);rows.value=r.data.list||[];total.value=r.data.total}catch(e){if(listSeq.valid(id))error.value=e.message}finally{if(listSeq.valid(id))loading.value=false}}
function search(){query.page=1;return load()}
function reset(){Object.assign(query,{page:1,pageSize:20,anchorNo:'',no:'',payeeAccountNo:'',currency:'',status:'',sort:'',desc:true});range.value=[];return load()}
function sortChanged({prop,order}){query.sort=order?prop:'';query.desc=order==='descending';return search()}
const dialog=ref(false),action=ref(''),draft=ref({}),writing=ref(false),submitted=ref(false),writeError=ref('')
const exchangePreviewItems=computed(()=>kind==='exchange'&&action.value==='create'?['600','1000','1500'].map(sample=>exchangePreview(draft.value.coinUnits,draft.value.minorUnits,draft.value.currency,sample)).filter(Boolean):[])
let confirmed=null
const dialogTitle=computed(()=>action.value==='review'?'人工复核':action.value==='reconcile'?'发起自动对账':kind==='exchange'?'创建兑换规则':'生成结算申请')
function start(next,value){if(writing.value)return;action.value=next;draft.value=value;confirmed=null;submitted.value=false;writeError.value='';dialog.value=true}
function openCreate(){if(writing.value)return;if(kind==='settlement')return settlementCreator.value?.open(query.anchorNo);start('create',{requestId:CreateUUID(),name:'',sourceUnit:'COIN',currency:'',coinUnits:'',minorUnits:'',roundingMode:'floor',effectiveFrom:'',effectiveTo:''})}
function openReview(row,approve){start('review',{no:kind==='exchange'?row.ruleNo:kind==='settlement'?row.settlementNo:row.withdrawalNo,approve,reason:''})}
function openReconcile(){start('reconcile',{withdrawalNo:''})}
async function submit(){if(writing.value)return;writing.value=true;writeError.value='';try{
 if(!confirmed){const data=JSON.parse(JSON.stringify(draft.value));let path=kind+'/'+action.value
 if(action.value==='create'&&kind==='exchange'){if(!integerAmount(data.coinUnits)||!integerAmount(data.minorUnits)||!data.name||!/^[A-Z]{3,8}$/.test(data.currency)||data.currency==='COIN')throw new Error('请填写规则名称、真实结算币种与有效整数兑换比例');data.effectiveFrom=Number(data.effectiveFrom);data.effectiveTo=Number(data.effectiveTo);if(data.effectiveTo<=data.effectiveFrom)throw new Error('请选择有效生效区间')}
 if(action.value==='review'&&!data.reason)throw new Error('请填写审核原因')
 if(action.value==='reconcile'){if(!data.withdrawalNo)throw new Error('请填写提现单号');path='reconciliation/run'}
 await ElMessageBox.confirm(action.value==='review'?(data.approve?'确认审核通过？提现通过后仍保持冻结。':'确认驳回？提现冻结金额将释放，并优先抵扣欠额。'):'确认按当前参数提交？','确认操作',{type:'warning'});confirmed={path,data}
 }
 submitted.value=true;const r=await earningWrite(confirmed.path,confirmed.data);if(r.code!==0)throw new Error(r.msg);dialog.value=false;submitted.value=false;confirmed=null;await load()
 }catch(e){if(e!=='cancel'&&e!=='close')writeError.value=e.message||'提交失败，请查询结果或按原参数重试'}finally{writing.value=false}}
const detailOpen=ref(false),detail=ref(null),detailError=ref(''),ledgerTarget=ref(null),ledgerPage=ref(1),ledgerTotal=ref(0)
const detailEntries=computed(()=>Object.entries(detail.value?.settlement||detail.value?.withdrawal||detail.value?.account||detail.value||{}).filter(([key,value])=>value!==null&&typeof value!=='object'&&!['snapshot','ruleSnapshot','itemsHash','consistent','payoutEnabled','issue'].includes(key)))
const tableFields=rows=>Object.keys(rows[0]||{}).filter(key=>!['snapshot','incomeHash','requestId'].includes(key))
async function inspect(row){if(writing.value)return;detailSeq.invalidate();detailOpen.value=true;detail.value=null;detailError.value='';ledgerTarget.value=null;if(kind==='account'){ledgerTarget.value={anchorNo:row.anchorNo,currency:row.currency};ledgerPage.value=1;return loadLedger()};if(!['settlement','withdrawal'].includes(kind)){detail.value=JSON.parse(JSON.stringify(row));return};const id=detailSeq.next();try{const r=await earningQuery(kind+'/detail',{no:row.settlementNo||row.withdrawalNo});if(!detailSeq.valid(id))return;if(r.code!==0)throw new Error(r.msg);detail.value=r.data}catch(e){if(detailSeq.valid(id))detailError.value=e.message}}
async function loadLedger(){const id=detailSeq.next();try{const r=await earningQuery('account/ledger',{...ledgerTarget.value,page:ledgerPage.value,pageSize:50});if(!detailSeq.valid(id))return;if(r.code!==0)throw new Error(r.msg);detail.value={account:{...ledgerTarget.value},ledger:r.data.list};ledgerTotal.value=Math.min(Number(r.data.total),1000000)}catch(e){if(detailSeq.valid(id))detailError.value=e.message}}
async function check(row){const id=detailSeq.next();detailOpen.value=true;detail.value=null;detailError.value='';ledgerTarget.value=null;try{const r=await earningQuery('account/check',{anchorNo:row.anchorNo,currency:row.currency});if(!detailSeq.valid(id))return;if(r.code!==0)throw new Error(r.msg);detail.value=r.data}catch(e){if(detailSeq.valid(id))detailError.value=e.message}}
async function runReconcile(no){if(writing.value)return;writing.value=true;try{const r=await earningWrite('reconciliation/run',{withdrawalNo:no});if(r.code!==0)throw new Error(r.msg);detailSeq.invalidate();detail.value=r.data;detailOpen.value=true;await load()}catch(e){ElMessage.error(e.message)}finally{writing.value=false}}
onMounted(load)
onMounted(async()=>{if(kind==='withdrawal'){try{const r=await getPayoutCapabilities();if(r.code===0)payoutCapabilities.value=r.data}catch{payoutCapabilities.value={}}}})
onBeforeUnmount(()=>{listSeq.invalidate();detailSeq.invalidate()})
</script>
