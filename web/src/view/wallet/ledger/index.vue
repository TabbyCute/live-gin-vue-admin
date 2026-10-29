<template>
  <div class="wallet-page">
    <div class="wallet-overview">
      <div><h2>余额流水</h2><p>使用用户账号或完整手机号查询不可变账务流水；结果按最新版本倒序。</p></div>
      <div v-if="balance" class="wallet-metrics">
        <div><strong>{{ formatWalletAmount(balance.availableBalance) }}</strong><span>当前可用 COIN</span></div>
        <div><strong>{{ balance.version }}</strong><span>当前版本</span></div>
        <div><strong>{{ pageIndex + 1 }}</strong><span>当前游标页</span></div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="search" @submit.prevent>
        <el-form-item label="用户账号"><el-input v-model.trim="search.accountNo" clearable maxlength="32" placeholder="完整公开账号" @keyup.enter="queryFirstPage" /></el-form-item>
        <el-form-item label="手机号"><el-input v-model.trim="search.phone" clearable maxlength="20" placeholder="如 +60123456789" @keyup.enter="queryFirstPage" /></el-form-item>
        <el-form-item label="每页数量">
          <el-select v-model="search.limit" style="width: 120px"><el-option v-for="size in [20, 50, 100]" :key="size" :label="size" :value="size" /></el-select>
        </el-form-item>
        <el-form-item><el-button type="primary" icon="search" :loading="loading" @click="queryFirstPage">查询</el-button><el-button icon="refresh" @click="reset">重置</el-button></el-form-item>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="wallet-toolbar">
        <div><el-button :disabled="loading || !queried || pageIndex === 0" @click="previousPage">上一页</el-button><el-button type="primary" plain :disabled="loading || !hasMore" @click="nextPage">下一页</el-button><span v-if="balance" class="muted ml-3">当前用户：{{ balance.accountNo }}</span></div>
        <span class="muted">流水不可修改或删除；翻页期间如有新入账，请点击查询返回最新首页。</span>
      </div>
      <el-table v-loading="loading" class="mt-4" :data="rows" row-key="ledgerNo" stripe>
        <el-table-column label="版本" prop="walletVersion" width="90" />
        <el-table-column label="发生金额" min-width="130">
          <template #default="scope"><span class="wallet-amount" :class="scope.row.changeAmount.startsWith('-') ? 'wallet-negative' : 'wallet-positive'">{{ scope.row.changeAmount.startsWith('-') ? '' : '+' }}{{ formatWalletAmount(scope.row.changeAmount) }}</span></template>
        </el-table-column>
        <el-table-column label="变动前/后" min-width="210">
          <template #default="scope"><span class="wallet-amount">{{ formatWalletAmount(scope.row.balanceBefore) }}</span><span class="muted"> → </span><span class="wallet-amount">{{ formatWalletAmount(scope.row.balanceAfter) }}</span></template>
        </el-table-column>
        <el-table-column label="业务类型" width="120"><template #default="scope"><el-tag :type="scope.row.changeAmount.startsWith('-') ? 'danger' : 'success'" effect="plain">{{ businessLabel(scope.row.businessType) }}</el-tag></template></el-table-column>
        <el-table-column label="业务单号" min-width="300"><template #default="scope"><span class="wallet-code">{{ scope.row.businessNo }}</span></template></el-table-column>
        <el-table-column label="原流水号" min-width="300"><template #default="scope"><span class="wallet-code">{{ scope.row.originalLedgerNo || '-' }}</span></template></el-table-column>
        <el-table-column label="操作人" min-width="170"><template #default="scope">{{ scope.row.operatorDisplay || `管理员 [ ${scope.row.operatorId} ]` }}</template></el-table-column>
        <el-table-column label="内部原因" min-width="220" show-overflow-tooltip prop="reason" />
        <el-table-column label="发生时间" width="175"><template #default="scope">{{ formatWalletTime(scope.row.createdAt) }}</template></el-table-column>
        <el-table-column label="流水号" min-width="300"><template #default="scope"><span class="wallet-code">{{ scope.row.ledgerNo }}</span></template></el-table-column>
        <template #empty><el-empty :description="queried ? '该页没有流水' : '输入账号或手机号后查询'" /></template>
      </el-table>
    </div>
  </div>
</template>

<script setup>
import { onDeactivated, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getWalletAccount, getWalletLedger } from '@/api/wallet'
import { businessLabel, formatWalletAmount, formatWalletTime } from '../shared'
import { createRequestScope } from '../requestScope'
import '../wallet.scss'

defineOptions({ name: 'WalletLedger' })

const route = useRoute()
const search = reactive({ accountNo: '', phone: '', limit: 20 })
const balance = ref(null)
const rows = ref([])
const hasMore = ref(false)
const nextCursor = ref('')
const cursorHistory = ref([''])
const pageIndex = ref(0)
const queried = ref(false)
const loading = ref(false)
const requests = createRequestScope()
let appliedSearch = null

const identityParams = () => {
  if (!search.accountNo && !search.phone) { ElMessage.warning('请输入用户账号或完整手机号'); return null }
  if (search.accountNo && !/^[A-Za-z0-9_-]{1,32}$/.test(search.accountNo)) { ElMessage.warning('用户账号格式不正确'); return null }
  if (search.phone && !/^\+[1-9]\d{7,14}$/.test(search.phone)) { ElMessage.warning('手机号必须是E.164格式，例如 +60123456789'); return null }
  return { ...(search.accountNo ? { accountNo: search.accountNo } : {}), ...(search.phone ? { phone: search.phone } : {}), currencyCode: 'COIN' }
}
const loadPage = async (cursor, index) => {
  if (!appliedSearch) return
  const identity = { ...appliedSearch }
  const current = requests.begin()
  loading.value = true
  try {
    const [ledgerRes, balanceRes] = await Promise.all([
      getWalletLedger({ ...identity, ...(cursor ? { cursor } : {}) }),
      getWalletAccount({ accountNo: identity.accountNo, phone: identity.phone, currencyCode: identity.currencyCode })
    ])
    if (current() && ledgerRes.code === 0 && balanceRes.code === 0) {
      rows.value = ledgerRes.data.list || []; hasMore.value = Boolean(ledgerRes.data.hasMore); nextCursor.value = ledgerRes.data.nextCursor || ''
      balance.value = balanceRes.data; pageIndex.value = index; queried.value = true
      cursorHistory.value = cursorHistory.value.slice(0, index + 1)
      cursorHistory.value[index] = cursor
    }
  } finally { if (current()) loading.value = false }
}
const queryFirstPage = async () => {
  const identity = identityParams()
  if (!identity) return
  appliedSearch = { ...identity, limit: search.limit }
  balance.value = null; rows.value = []; hasMore.value = false; queried.value = false
  cursorHistory.value = ['']; await loadPage('', 0)
}
const nextPage = async () => {
  if (loading.value || !hasMore.value || !nextCursor.value) return
  const nextIndex = pageIndex.value + 1
  await loadPage(nextCursor.value, nextIndex)
}
const previousPage = async () => { if (!loading.value && pageIndex.value > 0) await loadPage(cursorHistory.value[pageIndex.value - 1], pageIndex.value - 1) }
const invalidateQueries = () => { requests.invalidate(); loading.value = false }
const reset = () => { invalidateQueries(); appliedSearch = null; search.accountNo = ''; search.phone = ''; search.limit = 20; balance.value = null; rows.value = []; queried.value = false; hasMore.value = false; nextCursor.value = ''; cursorHistory.value = ['']; pageIndex.value = 0 }
onDeactivated(invalidateQueries)
onBeforeUnmount(invalidateQueries)

watch(() => route.query.accountNo, (value) => {
  const accountNo = Array.isArray(value) ? value[0] : value
  if (!accountNo || !/^[A-Za-z0-9_-]{1,32}$/.test(accountNo)) return
  search.accountNo = accountNo; search.phone = ''; queryFirstPage()
}, { immediate: true })
</script>
