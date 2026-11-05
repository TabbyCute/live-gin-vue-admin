<template>
  <div class="wallet-page">
    <div class="wallet-overview"><div><h2>交易溯源</h2><p>查清一笔余额为什么变化、来自哪个业务、由谁触发，以及退了多少。仅供查询。</p></div></div>
    <div class="gva-search-box">
      <el-form :inline="true" @submit.prevent="searchTrace">
        <el-form-item label="查询方式"><el-select v-model="mode" style="width:180px"><el-option label="流水号 / 账务单号" value="no"/><el-option label="业务订单" value="business"/></el-select></el-form-item>
        <el-form-item v-if="mode === 'no'" label="编号"><el-input v-model.trim="form.no" maxlength="40" placeholder="wl_ / wt_ / wa_ 开头" style="width:340px"/></el-form-item>
        <template v-else>
          <el-form-item label="业务标识"><el-input v-model.trim="form.businessType" maxlength="32" placeholder="所属业务的固定标识"/></el-form-item>
          <el-form-item label="业务单号"><el-input v-model.trim="form.businessNo" maxlength="128"/></el-form-item>
          <el-form-item label="动作"><el-select v-model="form.kind" style="width:120px"><el-option label="扣款" value="debit"/><el-option label="退款" value="refund"/></el-select></el-form-item>
        </template>
        <el-form-item><el-button type="primary" native-type="submit" :loading="loading">查询</el-button></el-form-item>
      </el-form>
    </div>
    <div v-if="data" class="gva-table-box">
      <el-alert v-if="data.issues?.length" type="error" title="该交易的流水、退款或审计存在完整性异常。以下为现存记录，请停止相关资金操作并核查。" :closable="false"/>
      <el-alert v-if="!data.ledger && data.transaction" type="error" title="交易对应流水缺失，请停止相关资金操作并排查" :closable="false"/>
      <el-descriptions v-if="data.ledger" title="本次余额变动" :column="2" border>
        <el-descriptions-item label="流水号">{{ data.ledger.ledgerNo }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ businessLabel(data.ledger.businessType) }}</el-descriptions-item>
        <el-descriptions-item label="发生金额">{{ formatWalletAmount(data.ledger.changeAmount) }} COIN</el-descriptions-item>
        <el-descriptions-item label="余额前 → 后">{{ formatWalletAmount(data.ledger.balanceBefore) }} → {{ formatWalletAmount(data.ledger.balanceAfter) }}</el-descriptions-item>
        <el-descriptions-item label="操作者">{{ data.ledger.operatorDisplay }}</el-descriptions-item>
        <el-descriptions-item label="时间">{{ formatWalletTime(data.ledger.createdAt) }}</el-descriptions-item>
        <el-descriptions-item label="钱包版本">{{ data.ledger.walletVersion }}</el-descriptions-item>
        <el-descriptions-item label="原因">{{ data.ledger.reason }}</el-descriptions-item>
      </el-descriptions>
      <el-descriptions v-if="data.transaction" title="业务来源" :column="2" border class="mt-4">
        <el-descriptions-item label="账务交易号">{{ data.transaction.transactionNo }}</el-descriptions-item>
        <el-descriptions-item label="用户账号">{{ data.transaction.accountNo }}</el-descriptions-item>
        <el-descriptions-item label="所属业务">{{ data.transaction.businessType }}</el-descriptions-item>
        <el-descriptions-item label="业务单号">{{ data.transaction.businessNo }}</el-descriptions-item>
        <el-descriptions-item label="操作者">{{ data.transaction.actorDisplay }}</el-descriptions-item>
        <el-descriptions-item label="来源 IP">{{ data.transaction.actorIp }}</el-descriptions-item>
        <el-descriptions-item label="请求追踪号">{{ data.transaction.traceId }}</el-descriptions-item>
        <el-descriptions-item label="原扣款"><el-button v-if="data.original" link type="primary" @click="openNo(data.original.transactionNo)">{{ data.original.transactionNo }}</el-button><span v-else>本笔为原扣款</span></el-descriptions-item>
        <el-descriptions-item label="原消费金额">{{ formatWalletAmount((data.original || data.transaction).amount) }} COIN</el-descriptions-item>
        <el-descriptions-item label="累计已退">{{ formatWalletAmount((data.original || data.transaction).refundedAmount) }} COIN</el-descriptions-item>
      </el-descriptions>
      <el-descriptions v-if="data.adjustment" title="人工充值 / 冲正来源" :column="2" border class="mt-4">
        <el-descriptions-item v-for="field in adjustmentFields" :key="field.key" :label="field.label">{{ field.key === 'status' ? statusMeta(data.adjustment.order.status).label : data.adjustment.order[field.key] || '-' }}</el-descriptions-item>
      </el-descriptions>
      <template v-if="data.transaction">
        <h3 class="mt-4">原消费的退款记录</h3>
        <el-table :data="data.refunds" row-key="transactionNo" stripe>
          <el-table-column label="退款交易号" min-width="300"><template #default="{ row }"><el-button link type="primary" @click="openNo(row.transactionNo)">{{ row.transactionNo }}</el-button></template></el-table-column>
          <el-table-column label="业务退款单" prop="businessNo" min-width="220"/>
          <el-table-column label="退款金额"><template #default="{ row }">{{ formatWalletAmount(row.amount) }}</template></el-table-column>
          <el-table-column label="操作者" prop="actorDisplay" min-width="180"/>
          <el-table-column label="原因" prop="reason" min-width="180"/>
          <el-table-column label="时间" width="180"><template #default="{ row }">{{ formatWalletTime(row.createdAt) }}</template></el-table-column>
        </el-table>
        <el-button v-if="data.hasMore" class="mt-4" :loading="loading" @click="loadMore">继续加载退款</el-button>
      </template>
      <h3 class="mt-4">本笔交易审计</h3>
      <el-table :data="data.audit" stripe><el-table-column label="动作"><template #default="{ row }">{{ actionLabels[row.action] || row.action }}</template></el-table-column><el-table-column label="操作者" prop="operatorDisplay"/><el-table-column label="来源 IP" prop="operatorIp"/><el-table-column label="原因" prop="reason"/><el-table-column label="时间"><template #default="{ row }">{{ formatWalletTime(row.createdAt) }}</template></el-table-column></el-table>
    </div>
  </div>
</template>
<script setup>
import { ref, reactive, watch, onDeactivated, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getWalletTrace } from '@/api/wallet'
import { businessLabel, formatWalletAmount, formatWalletTime, statusMeta } from '../shared'
import { createRequestScope } from '../requestScope'
import '../wallet.scss'
defineOptions({ name: 'WalletTrace' })
const route = useRoute()
const mode = ref('no'), data = ref(null), loading = ref(false)
const form = reactive({ no: '', businessType: '', businessNo: '', kind: 'debit' })
const requests = createRequestScope()
let applied = null
const actionLabels = { debit: '消费扣款', refund: '消费退款', create: '创建申请', approve: '审核通过', reject: '驳回申请', reverse: '创建冲正申请', wallet_open: '钱包开户', status_normal: '启用钱包', status_disabled: '停用钱包' }
const adjustmentFields = [{ key:'orderNo',label:'订单号' },{ key:'accountNo',label:'用户账号' },{ key:'amount',label:'金额 COIN' },{ key:'status',label:'状态' },{ key:'externalOrderNo',label:'资金凭证号' },{ key:'evidenceRef',label:'凭证引用' },{ key:'createdByDisplay',label:'申请人' },{ key:'approvedByDisplay',label:'审核人' },{ key:'originalOrderNo',label:'原充值单号' },{ key:'reversalOrderNo',label:'关联冲正单' }]
const load = async (append = false) => {
  const params = { ...applied, ...(append ? { afterRefundId: data.value.nextRefundId } : {}) }
  const current = requests.begin(); loading.value = true
  try { const res = await getWalletTrace(params); if (!current() || res.code !== 0) return
    data.value = append ? { ...res.data, refunds: [...data.value.refunds, ...res.data.refunds] } : res.data
  } finally { if (current()) loading.value = false }
}
const searchTrace = async () => {
  if (mode.value === 'no' ? !form.no : !form.businessType || !form.businessNo) return ElMessage.warning('请填写查询编号')
  applied = mode.value === 'no' ? { no: form.no } : { businessType: form.businessType, businessNo: form.businessNo, kind: form.kind }
  data.value = null; await load()
}
const openNo = async (no) => { mode.value = 'no'; form.no = no; await searchTrace() }
const loadMore = async () => { if (!loading.value && data.value?.hasMore) await load(true) }
const invalidate = () => { requests.invalidate(); loading.value = false }
onDeactivated(invalidate); onBeforeUnmount(invalidate)
watch(() => route.query.no, value => { if (typeof value === 'string' && value) openNo(value) }, { immediate: true })
</script>
