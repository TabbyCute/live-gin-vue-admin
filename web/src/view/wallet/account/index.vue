<template>
  <div class="wallet-page">
    <div class="wallet-overview">
      <div><h2>钱包账户</h2><p>使用用户账号或完整手机号精确查询余额、控制钱包状态，并执行只读账务对账。</p></div>
      <div v-if="balance" class="wallet-metrics">
        <div><strong>{{ formatWalletAmount(balance.availableBalance) }}</strong><span>可用 COIN</span></div>
        <div><strong>{{ formatWalletAmount(balance.frozenBalance) }}</strong><span>冻结 COIN</span></div>
        <div><strong>{{ balance.version }}</strong><span>账务版本</span></div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="search" @submit.prevent>
        <el-form-item label="用户账号">
          <el-input v-model.trim="search.accountNo" clearable maxlength="32" placeholder="完整公开账号" @keyup.enter="loadAccount" />
        </el-form-item>
        <el-form-item label="手机号"><el-input v-model.trim="search.phone" clearable maxlength="20" placeholder="如 +60123456789" @keyup.enter="loadAccount" /></el-form-item>
        <el-form-item label="币种"><el-input model-value="COIN" disabled /></el-form-item>
        <el-form-item>
          <el-button type="primary" icon="search" :loading="loading" @click="loadAccount">查询</el-button>
          <el-button icon="refresh" @click="reset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="gva-table-box">
      <template v-if="balance">
        <div class="wallet-toolbar">
          <div>
            <el-tag :type="balance.status === 'normal' ? 'success' : 'danger'">{{ balance.status === 'normal' ? '正常' : '已停用' }}</el-tag>
            <span class="muted ml-3">更新时间：{{ formatWalletTime(balance.updatedAt) }}</span>
          </div>
          <div>
            <el-button :disabled="!walletOpened" :type="balance.status === 'normal' ? 'danger' : 'success'" @click="openStatusDialog">
              {{ balance.status === 'normal' ? '停用钱包' : '启用钱包' }}
            </el-button>
            <el-button type="primary" plain :disabled="reconciling" @click="runReconcile">完整对账</el-button>
            <el-button v-if="reconciling" type="danger" plain @click="cancelReconcile">停止对账</el-button>
          </div>
        </div>
        <el-alert v-if="!walletOpened" class="mt-4" type="info" :closable="false" title="该用户尚未开户，当前展示虚拟零余额。首次充值审核成功时会原子开户；状态操作不会创建钱包。" />
        <el-descriptions class="mt-4" :column="3" border>
          <el-descriptions-item label="公开账号">{{ balance.accountNo }}</el-descriptions-item>
          <el-descriptions-item label="绑定手机号">{{ balance.phone || '未绑定' }}</el-descriptions-item>
          <el-descriptions-item label="币种">{{ balance.currencyCode }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ balance.status }}</el-descriptions-item>
          <el-descriptions-item label="可用余额"><span class="wallet-amount">{{ formatWalletAmount(balance.availableBalance) }}</span></el-descriptions-item>
          <el-descriptions-item label="冻结余额"><span class="wallet-amount">{{ formatWalletAmount(balance.frozenBalance) }}</span></el-descriptions-item>
          <el-descriptions-item label="流水版本">{{ balance.version }}</el-descriptions-item>
        </el-descriptions>
      </template>
      <el-empty v-else class="wallet-empty" description="输入用户账号或完整手机号后查询" />
    </div>

    <div v-if="reconcileResult" class="gva-table-box">
      <div class="wallet-toolbar">
        <h3 class="m-0">对账结果</h3>
        <el-tag :type="reconcileResult.consistent ? 'success' : 'danger'">{{ reconcileResult.consistent ? '账务一致' : '发现异常' }}</el-tag>
      </div>
      <el-descriptions class="mt-4" :column="4" border>
        <el-descriptions-item label="已检查流水">{{ reconcileResult.checked }}</el-descriptions-item>
        <el-descriptions-item label="已检查订单">{{ reconcileResult.checkedOrders }}</el-descriptions-item>
        <el-descriptions-item label="快照版本">{{ reconcileResult.snapshotVersion || '-' }}</el-descriptions-item>
        <el-descriptions-item label="执行批次">{{ reconcileResult.batches }}</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="reconcileResult.cancelled" class="mt-4" type="warning" :closable="false" title="对账已停止，当前结果不是完整历史结论。" />
      <el-alert v-else-if="reconcileResult.consistent" class="mt-4" type="success" :closable="false" title="从起点到当前末尾的流水、订单和余额关系一致。并发新入账可能发生在批次之间。" />
      <el-table v-else class="mt-4" :data="reconcileResult.issues" stripe>
        <el-table-column label="异常" min-width="260"><template #default="scope">{{ issueLabel(scope.row) }}</template></el-table-column>
        <el-table-column label="原始标识" min-width="300"><template #default="scope"><span class="wallet-code">{{ scope.row }}</span></template></el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="statusVisible" :title="targetStatus === 'disabled' ? '停用钱包' : '启用钱包'" width="520px" :close-on-click-modal="false" :close-on-press-escape="!submitting" :show-close="!submitting">
      <el-alert class="wallet-alert" :type="targetStatus === 'disabled' ? 'warning' : 'info'" :closable="false" :title="targetStatus === 'disabled' ? '停用后充值审核和冲正都会被阻止，余额及历史流水保留。' : '启用前请确认用户账号仍有效且风险处置已经结束。'" />
      <el-form ref="statusFormRef" :model="statusForm" :rules="statusRules" label-width="82px" :disabled="submitting">
        <el-form-item label="用户账号"><el-input :model-value="statusTarget?.accountNo" disabled /></el-form-item>
        <el-form-item label="操作原因" prop="reason"><el-input v-model.trim="statusForm.reason" type="textarea" :rows="4" maxlength="500" show-word-limit /></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="submitting" @click="statusVisible = false">取消</el-button><el-button :type="targetStatus === 'disabled' ? 'danger' : 'primary'" :loading="submitting" @click="submitStatus">确认</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onDeactivated, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { checkWalletAccount, getWalletAccount, setWalletAccountStatus } from '@/api/wallet'
import { formatWalletAmount, formatWalletTime, issueLabel } from '../shared'
import { createRequestScope } from '../requestScope'
import '../wallet.scss'

defineOptions({ name: 'WalletAccount' })

const route = useRoute()
const search = reactive({ accountNo: '', phone: '' })
const balance = ref(null)
const loading = ref(false)
const submitting = ref(false)
const statusVisible = ref(false)
const statusTarget = ref(null)
const statusFormRef = ref()
const statusForm = reactive({ reason: '' })
const reconcileResult = ref(null)
const reconciling = ref(false)
const reconcileCancelled = ref(false)
const accountRequests = createRequestScope()
const reconcileRequests = createRequestScope()
const walletOpened = computed(() => Boolean(balance.value?.updatedAt))
const targetStatus = computed(() => statusTarget.value?.status || 'disabled')
const statusRules = { reason: [{ required: true, message: '请填写操作原因', trigger: 'blur' }, { min: 3, max: 500, message: '原因需为 3—500 个字符', trigger: 'blur' }] }

const identityParams = () => {
  if (!search.accountNo && !search.phone) { ElMessage.warning('请输入用户账号或完整手机号'); return null }
  if (search.accountNo && !/^[A-Za-z0-9_-]{1,32}$/.test(search.accountNo)) { ElMessage.warning('用户账号格式不正确'); return null }
  if (search.phone && !/^\+[1-9]\d{7,14}$/.test(search.phone)) { ElMessage.warning('手机号必须是E.164格式，例如 +60123456789'); return null }
  return { ...(search.accountNo ? { accountNo: search.accountNo } : {}), ...(search.phone ? { phone: search.phone } : {}), currencyCode: 'COIN' }
}

const loadAccount = async () => {
  const params = identityParams()
  if (!params) return
  const current = accountRequests.begin()
  reconcileRequests.invalidate(); reconciling.value = false
  balance.value = null; reconcileResult.value = null; statusVisible.value = false
  loading.value = true
  try {
    const res = await getWalletAccount(params)
    if (current() && res.code === 0) balance.value = res.data
  } finally { if (current()) loading.value = false }
}
const invalidateQueries = () => { accountRequests.invalidate(); reconcileRequests.invalidate(); loading.value = false; reconciling.value = false; statusVisible.value = false }
const reset = () => { invalidateQueries(); search.accountNo = ''; search.phone = ''; balance.value = null; reconcileResult.value = null }
const openStatusDialog = () => {
  if (submitting.value || !walletOpened.value) return
  statusTarget.value = { accountNo: balance.value.accountNo, currencyCode: 'COIN', status: balance.value.status === 'normal' ? 'disabled' : 'normal' }
  statusForm.reason = ''; statusVisible.value = true
}
const submitStatus = async () => {
  if (submitting.value || !statusTarget.value) return
  submitting.value = true
  const current = accountRequests.capture()
  const payload = { ...statusTarget.value, reason: statusForm.reason }
  try {
    await statusFormRef.value.validate()
    await ElMessageBox.confirm(`确认将 ${payload.accountNo} 的钱包设置为“${payload.status === 'disabled' ? '停用' : '正常'}”？`, '敏感资金操作', { type: 'warning', confirmButtonText: '确认执行', cancelButtonText: '取消' })
    if (!current()) return
    const res = await setWalletAccountStatus(payload)
    if (res.code === 0) {
      ElMessage.success('钱包状态已更新')
      if (current()) { statusVisible.value = false; balance.value = res.data }
    }
  } catch (error) { if (error instanceof Error) ElMessage.error(error.message || '状态操作失败，请查询后确认结果') } finally { submitting.value = false }
}
const cancelReconcile = () => { reconcileCancelled.value = true }
const runReconcile = async () => {
  if (!balance.value?.accountNo) return ElMessage.warning('请先查询用户')
  if (reconciling.value) return
  const current = reconcileRequests.begin()
  const accountNo = balance.value.accountNo
  reconcileCancelled.value = false; reconciling.value = true
  const result = { consistent: true, issues: [], checked: 0, checkedOrders: 0, snapshotVersion: '0', batches: 0, cancelled: false }
  let afterVersion = '0'; let afterOrderId = '0'
  try {
    while (result.batches < 1000 && !reconcileCancelled.value) {
      const res = await checkWalletAccount({ accountNo, currencyCode: 'COIN', afterVersion, afterOrderId, limit: 1000 })
      if (!current()) return
      if (res.code !== 0) return
      const data = res.data
      result.batches += 1; result.checked += Number(data.checked || 0); result.checkedOrders += Number(data.checkedOrders || 0)
      result.snapshotVersion = data.snapshotVersion || result.snapshotVersion
      result.consistent = result.consistent && Boolean(data.consistent)
      result.issues.push(...(data.issues || []))
      if (!data.hasMore) break
      if (data.nextVersion === afterVersion && data.nextOrderId === afterOrderId) throw new Error('对账游标未推进')
      afterVersion = data.nextVersion; afterOrderId = data.nextOrderId
    }
    result.cancelled = reconcileCancelled.value
    if (result.batches >= 1000) { result.cancelled = true; ElMessage.warning('对账达到 1000 批安全上限，请联系技术人员执行离线核查') }
    result.issues = [...new Set(result.issues)]
    reconcileResult.value = result
    if (!result.cancelled) ElMessage[result.consistent ? 'success' : 'error'](result.consistent ? '完整对账通过' : '对账发现异常，请立即停用钱包并排查')
  } catch (error) { if (current()) ElMessage.error(error?.message || '对账失败') } finally { if (current()) reconciling.value = false }
}

onDeactivated(invalidateQueries)
onBeforeUnmount(invalidateQueries)

watch(() => route.query.accountNo, (value) => {
  const accountNo = Array.isArray(value) ? value[0] : value
  if (!accountNo || !/^[A-Za-z0-9_-]{1,32}$/.test(accountNo)) return
  search.accountNo = accountNo; search.phone = ''; loadAccount()
}, { immediate: true })
</script>
