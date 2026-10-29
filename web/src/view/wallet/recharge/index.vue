<template>
  <div class="wallet-page">
    <div class="wallet-overview">
      <div><h2>系统人工充值单</h2><p>每笔充值绑定真实资金凭证；普通管理员双人复核，顶级超级管理员（ID 1）可自审。</p></div>
      <div class="wallet-metrics">
        <div><strong>{{ total }}</strong><span>筛选结果</span></div>
        <div><strong>{{ pageMetrics.pending }}</strong><span>本页待审核</span></div>
        <div><strong>{{ pageMetrics.credited }}</strong><span>本页已入账</span></div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="search" label-width="76px" @submit.prevent>
        <el-form-item label="用户账号"><el-input v-model.trim="search.accountNo" clearable placeholder="精确账号" @keyup.enter="submitSearch" /></el-form-item>
        <el-form-item label="手机号"><el-input v-model.trim="search.phone" clearable maxlength="20" placeholder="如 +60123456789" @keyup.enter="submitSearch" /></el-form-item>
        <el-form-item label="资金凭证"><el-input v-model.trim="search.externalOrderNo" clearable placeholder="精确凭证号" @blur="search.externalOrderNo = search.externalOrderNo.toUpperCase()" @keyup.enter="submitSearch" /></el-form-item>
        <el-form-item label="订单号"><el-input v-model.trim="search.orderNo" clearable placeholder="wa_..." @keyup.enter="submitSearch" /></el-form-item>
        <el-form-item label="状态"><el-select v-model="search.status" clearable placeholder="全部" style="width: 130px"><el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
        <el-form-item label="类型"><el-select v-model="search.kind" clearable placeholder="全部" style="width: 120px"><el-option label="充值" value="grant" /><el-option label="冲正" value="reverse" /></el-select></el-form-item>
        <el-form-item><el-button type="primary" icon="search" @click="submitSearch">查询</el-button><el-button icon="refresh" @click="resetSearch">重置</el-button></el-form-item>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="wallet-toolbar">
        <div><el-button type="primary" icon="plus" @click="openCreate">新建人工充值</el-button><el-button :loading="loading" @click="loadData">刷新</el-button></div>
        <span class="muted">同一真实资金凭证永久防重；驳回或冲正后也不会释放。</span>
      </div>
      <el-table v-loading="loading" class="mt-4" :data="rows" row-key="orderNo" stripe>
        <el-table-column label="订单/凭证" min-width="300">
          <template #default="scope"><div class="wallet-code">{{ scope.row.orderNo }}</div><div class="muted wallet-code">{{ scope.row.externalOrderNo }}</div></template>
        </el-table-column>
        <el-table-column label="用户" prop="accountNo" width="145" />
        <el-table-column label="类型" width="100"><template #default="scope"><el-tag :type="scope.row.kind === 'reverse' ? 'danger' : 'primary'" effect="plain">{{ kindLabel(scope.row.kind) }}</el-tag></template></el-table-column>
        <el-table-column label="金额" min-width="140"><template #default="scope"><span class="wallet-amount" :class="scope.row.kind === 'reverse' ? 'wallet-negative' : 'wallet-positive'">{{ scope.row.kind === 'reverse' ? '-' : '+' }}{{ formatWalletAmount(scope.row.amount) }}</span> <span class="muted">COIN</span></template></el-table-column>
        <el-table-column label="状态" width="105"><template #default="scope"><el-tag :type="statusMeta(scope.row.status).type">{{ statusMeta(scope.row.status).label }}</el-tag></template></el-table-column>
        <el-table-column label="申请/审核人" min-width="210"><template #default="scope"><div>申请：{{ scope.row.createdByDisplay || `管理员 [ ${scope.row.createdBy} ]` }}</div><div class="muted">审核：{{ scope.row.approvedByDisplay || scope.row.rejectedByDisplay || '-' }}</div></template></el-table-column>
        <el-table-column label="申请时间" width="175"><template #default="scope">{{ formatWalletTime(scope.row.createdAt) }}</template></el-table-column>
        <el-table-column label="原因" min-width="220" show-overflow-tooltip prop="reason" />
        <el-table-column label="操作" width="255" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="openDetail(scope.row)">详情</el-button>
            <template v-if="scope.row.status === 'pending'">
              <el-button link type="success" :disabled="isOwnOrder(scope.row)" @click="openReview(scope.row, 'approve')">通过</el-button>
              <el-button link type="danger" :disabled="isOwnOrder(scope.row)" @click="openReview(scope.row, 'reject')">驳回</el-button>
            </template>
            <el-button v-if="scope.row.kind === 'grant' && scope.row.status === 'credited' && !scope.row.reversalOrderNo" link type="warning" @click="openReverse(scope.row)">冲正</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="gva-pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[10, 20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next, jumper" @current-change="loadData" @size-change="changePageSize" /></div>
    </div>

    <el-dialog v-model="createVisible" title="新建系统人工充值" width="650px" :close-on-click-modal="false" :close-on-press-escape="!submitting" :show-close="!submitting" destroy-on-close>
      <el-alert class="wallet-alert" type="warning" :closable="false" title="必须先确认真实资金已经到账。资金凭证号需由收款渠道、收款账户和交易流水号按统一运营规则组成，不能填写随机编号。" />
      <el-form ref="createFormRef" :model="createForm" :rules="createRules" label-width="105px" :disabled="submitting">
        <el-form-item label="用户账号" prop="accountNo"><el-input v-model.trim="createForm.accountNo" maxlength="32" placeholder="完整公开账号" /></el-form-item>
        <el-form-item label="资金凭证号" prop="externalOrderNo"><el-input v-model.trim="createForm.externalOrderNo" maxlength="128" placeholder="如 BANK-MERCHANT01-20261028-0001" @blur="createForm.externalOrderNo = createForm.externalOrderNo.toUpperCase()" /><div class="wallet-form-tip">统一转成大写；跨管理员、用户、金额和请求编号全局防重。</div></el-form-item>
        <el-form-item label="充值金额" prop="amount"><el-input v-model.trim="createForm.amount" maxlength="19" inputmode="numeric" placeholder="正整数 COIN，不接受小数" /></el-form-item>
        <el-form-item label="凭证引用" prop="evidenceRef"><el-input v-model.trim="createForm.evidenceRef" maxlength="500" placeholder="受控文件系统中的凭证引用或内部文件地址" /><div class="wallet-form-tip">请先通过受控媒体库上传凭证，再粘贴引用；不要填写公开、可被替换的临时链接。</div></el-form-item>
        <el-form-item label="充值原因" prop="reason"><el-input v-model.trim="createForm.reason" type="textarea" :rows="4" maxlength="500" show-word-limit placeholder="说明资金来源、入账用途和核对信息" /></el-form-item>
        <el-form-item label="到账确认" prop="fundsVerified"><el-checkbox v-model="createForm.fundsVerified">我已核对收款账户、真实交易号和到账金额</el-checkbox></el-form-item>
        <el-form-item label="请求编号"><span class="wallet-code">{{ createForm.requestId }}</span></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="submitting" @click="createVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="submitCreate">提交审核</el-button></template>
    </el-dialog>

    <el-dialog v-model="reviewVisible" :title="reviewAction === 'approve' ? '审核通过并入账' : '驳回充值申请'" width="580px" :close-on-click-modal="false" :close-on-press-escape="!submitting" :show-close="!submitting">
      <el-alert class="wallet-alert" :type="reviewAction === 'approve' ? 'warning' : 'info'" :closable="false" :title="reviewAction === 'approve' ? '确认后将原子更新余额、流水、额度、订单及审计。请独立核对真实到账，不能只看申请描述。' : '驳回不会释放资金凭证号；后续不能使用同一凭证重新申请。'" />
      <el-descriptions v-if="selectedOrder" :column="2" border>
        <el-descriptions-item label="用户">{{ selectedOrder.accountNo }}</el-descriptions-item><el-descriptions-item label="金额">{{ formatWalletAmount(selectedOrder.amount) }} COIN</el-descriptions-item>
        <el-descriptions-item label="资金凭证" :span="2"><span class="wallet-code">{{ selectedOrder.externalOrderNo }}</span></el-descriptions-item>
        <el-descriptions-item label="凭证引用" :span="2"><span class="wallet-code">{{ selectedOrder.evidenceRef || '-' }}</span></el-descriptions-item>
      </el-descriptions>
      <el-form ref="reviewFormRef" class="mt-4" :model="reviewForm" :rules="reviewRules" label-width="82px" :disabled="submitting"><el-form-item label="审核意见" prop="reason"><el-input v-model.trim="reviewForm.reason" type="textarea" :rows="4" maxlength="500" show-word-limit /></el-form-item></el-form>
      <template #footer><el-button :disabled="submitting" @click="reviewVisible = false">取消</el-button><el-button :type="reviewAction === 'approve' ? 'success' : 'danger'" :loading="submitting" @click="submitReview">确认{{ reviewAction === 'approve' ? '入账' : '驳回' }}</el-button></template>
    </el-dialog>

    <el-dialog v-model="reverseVisible" title="创建全额冲正申请" width="580px" :close-on-click-modal="false" :close-on-press-escape="!submitting" :show-close="!submitting">
      <el-alert class="wallet-alert" type="warning" :closable="false" title="冲正只创建待审核申请，不会立即扣款。另一管理员审核后追加一条全额负数流水；余额不足或钱包停用时不能执行。" />
      <el-form ref="reverseFormRef" :model="reverseForm" :rules="reverseRules" label-width="92px" :disabled="submitting">
        <el-form-item label="原充值单"><span class="wallet-code">{{ selectedOrder?.orderNo }}</span></el-form-item>
        <el-form-item label="冲正金额">{{ formatWalletAmount(selectedOrder?.amount || '0') }} COIN</el-form-item>
        <el-form-item label="冲正原因" prop="reason"><el-input v-model.trim="reverseForm.reason" type="textarea" :rows="4" maxlength="500" show-word-limit /></el-form-item>
        <el-form-item label="补充凭证"><el-input v-model.trim="reverseForm.evidenceRef" maxlength="500" placeholder="可选，填写错误入账核查材料引用" /></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="submitting" @click="reverseVisible = false">取消</el-button><el-button type="danger" :loading="submitting" @click="submitReverse">提交冲正审核</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="充值单详情" size="820px" destroy-on-close>
      <div v-loading="detailLoading">
        <template v-if="detail">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="订单号" :span="2"><span class="wallet-code">{{ detail.order.orderNo }}</span></el-descriptions-item>
            <el-descriptions-item label="请求编号"><span class="wallet-code">{{ detail.order.requestId }}</span></el-descriptions-item><el-descriptions-item label="来源">{{ sourceLabel(detail.order.sourceType) }}</el-descriptions-item>
            <el-descriptions-item label="资金凭证" :span="2"><span class="wallet-code">{{ detail.order.externalOrderNo }}</span></el-descriptions-item>
            <el-descriptions-item label="用户">{{ detail.order.accountNo }}</el-descriptions-item><el-descriptions-item label="金额"><span class="wallet-amount">{{ formatWalletAmount(detail.order.amount) }} COIN</span></el-descriptions-item>
            <el-descriptions-item label="类型">{{ kindLabel(detail.order.kind) }}</el-descriptions-item><el-descriptions-item label="状态"><el-tag :type="statusMeta(detail.order.status).type">{{ statusMeta(detail.order.status).label }}</el-tag></el-descriptions-item>
            <el-descriptions-item label="申请原因" :span="2">{{ detail.order.reason }}</el-descriptions-item><el-descriptions-item label="凭证引用" :span="2"><span class="wallet-code">{{ detail.order.evidenceRef || '-' }}</span></el-descriptions-item>
            <el-descriptions-item label="申请人/IP">{{ detail.order.createdByDisplay || `管理员 [ ${detail.order.createdBy} ]` }} / {{ detail.order.createdIp }}</el-descriptions-item><el-descriptions-item label="申请时间">{{ formatWalletTime(detail.order.createdAt) }}</el-descriptions-item>
            <el-descriptions-item label="审核人/IP">{{ detail.order.approvedByDisplay || detail.order.rejectedByDisplay || '-' }} / {{ detail.order.reviewedIp || '-' }}</el-descriptions-item><el-descriptions-item label="审核时间">{{ formatWalletTime(detail.order.approvedAt || detail.order.rejectedAt) }}</el-descriptions-item>
            <el-descriptions-item label="审核意见" :span="2">{{ detail.order.reviewReason || '-' }}</el-descriptions-item>
            <el-descriptions-item label="原充值单"><span class="wallet-code">{{ detail.order.originalOrderNo || '-' }}</span></el-descriptions-item><el-descriptions-item label="冲正单"><span class="wallet-code">{{ detail.order.reversalOrderNo || '-' }}</span></el-descriptions-item>
          </el-descriptions>
          <div class="wallet-detail-section"><h3>关联流水</h3>
            <el-descriptions v-if="detail.ledger" :column="2" border><el-descriptions-item label="流水号" :span="2"><span class="wallet-code">{{ detail.ledger.ledgerNo }}</span></el-descriptions-item><el-descriptions-item label="金额">{{ detail.ledger.changeAmount }}</el-descriptions-item><el-descriptions-item label="版本">{{ detail.ledger.walletVersion }}</el-descriptions-item><el-descriptions-item label="变动前">{{ detail.ledger.balanceBefore }}</el-descriptions-item><el-descriptions-item label="变动后">{{ detail.ledger.balanceAfter }}</el-descriptions-item></el-descriptions>
            <el-empty v-else description="订单尚未产生流水" :image-size="70" />
          </div>
          <div class="wallet-detail-section"><h3>事务审计</h3><div class="wallet-audit"><div v-for="(item, index) in detail.audit" :key="`${item.createdAt}-${index}`" class="wallet-audit-item"><div><el-tag size="small" effect="plain">{{ auditLabel(item.action) }}</el-tag> <strong>{{ item.operatorDisplay || `管理员 [ ${item.operatorId} ]` }}</strong> <span class="muted">{{ item.operatorIp }} · {{ formatWalletTime(item.createdAt) }}</span></div><p>{{ item.reason }}</p></div></div></div>
        </template>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onDeactivated, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '@/pinia'
import { CreateUUID } from '@/utils/format'
import { approveWalletAdjustment, createWalletRecharge, getWalletAccount, getWalletAdjustmentDetail, getWalletAdjustmentList, rejectWalletAdjustment, reverseWalletAdjustment } from '@/api/wallet'
import { formatWalletAmount, formatWalletTime, kindLabel, sourceLabel, statusMeta } from '../shared'
import { createRequestScope } from '../requestScope'
import '../wallet.scss'

defineOptions({ name: 'WalletRecharge' })

const statusOptions = [{ label: '待审核', value: 'pending' }, { label: '已入账', value: 'credited' }, { label: '已驳回', value: 'rejected' }, { label: '已冲正', value: 'reversed' }]
const route = useRoute()
const defaultSearch = () => ({ accountNo: '', phone: '', externalOrderNo: '', orderNo: '', status: '', kind: '' })
const search = reactive(defaultSearch())
const rows = ref([]); const total = ref(0); const page = ref(1); const pageSize = ref(20); const loading = ref(false); const submitting = ref(false)
const listRequests = createRequestScope(); const detailRequests = createRequestScope(); const actionRequests = createRequestScope()
const userStore = useUserStore()
const currentUserID = computed(() => String(userStore.userInfo?.ID || userStore.userInfo?.id || ''))
const pageMetrics = computed(() => ({ pending: rows.value.filter(item => item.status === 'pending').length, credited: rows.value.filter(item => item.status === 'credited').length }))
const buildRequestID = (prefix) => `${prefix}_${CreateUUID().replaceAll('-', '')}`
const queryParams = () => { const result = { page: page.value, pageSize: pageSize.value }; Object.entries(search).forEach(([key, value]) => { if (value) result[key] = value }); return result }
const loadData = async () => {
  const current = listRequests.begin(); const params = queryParams()
  loading.value = true; rows.value = []
  try {
    const res = await getWalletAdjustmentList(params)
    if (current() && res.code === 0) { rows.value = res.data.list || []; total.value = Number(res.data.total || 0); page.value = Number(res.data.page || params.page); pageSize.value = Number(res.data.pageSize || params.pageSize) }
  } finally { if (current()) loading.value = false }
}
const submitSearch = () => {
  if (search.accountNo && !/^[A-Za-z0-9_-]{1,32}$/.test(search.accountNo)) return ElMessage.warning('用户账号格式不正确')
  if (search.phone && !/^\+[1-9]\d{7,14}$/.test(search.phone)) return ElMessage.warning('手机号必须是E.164格式，例如 +60123456789')
  page.value = 1; loadData()
}
const resetSearch = () => { Object.assign(search, defaultSearch()); page.value = 1; loadData() }
const changePageSize = () => { page.value = 1; loadData() }
const isOwnOrder = (order) => currentUserID.value !== '1' && currentUserID.value && currentUserID.value === String(order.createdBy)

const createVisible = ref(false); const createFormRef = ref(); const createForm = reactive({ requestId: '', accountNo: '', externalOrderNo: '', amount: '', evidenceRef: '', reason: '', fundsVerified: false })
const createRules = {
  accountNo: [{ required: true, message: '请输入用户账号', trigger: 'blur' }, { pattern: /^[A-Za-z0-9_-]{1,32}$/, message: '账号格式无效', trigger: 'blur' }],
  externalOrderNo: [{ required: true, message: '请输入真实资金凭证号', trigger: 'blur' }, { pattern: /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$/, message: '凭证号格式无效', trigger: 'blur' }],
  amount: [{ required: true, message: '请输入充值金额', trigger: 'blur' }, { pattern: /^[1-9][0-9]{0,18}$/, message: '金额必须为无前导零的正整数', trigger: 'blur' }],
  evidenceRef: [{ required: true, message: '请填写凭证引用', trigger: 'blur' }], reason: [{ required: true, message: '请填写充值原因', trigger: 'blur' }, { min: 3, max: 500, message: '原因需为 3—500 个字符', trigger: 'blur' }],
  fundsVerified: [{ validator: (_rule, value, callback) => value ? callback() : callback(new Error('请先确认真实资金到账')), trigger: 'change' }]
}
const openCreate = async () => {
  if (submitting.value) return
  const current = actionRequests.begin()
  const identity = { accountNo: search.accountNo, phone: search.phone }
  let accountNo = identity.accountNo
  if (identity.accountNo && !/^[A-Za-z0-9_-]{1,32}$/.test(identity.accountNo)) return ElMessage.warning('用户账号格式不正确')
  if (identity.phone && !/^\+[1-9]\d{7,14}$/.test(identity.phone)) return ElMessage.warning('手机号必须是E.164格式，例如 +60123456789')
  if (identity.accountNo || identity.phone) {
    const res = await getWalletAccount({ ...identity, currencyCode: 'COIN' })
    if (!current() || res.code !== 0 || identity.accountNo !== search.accountNo || identity.phone !== search.phone) return
    accountNo = res.data.accountNo
  }
  reviewVisible.value = false; reverseVisible.value = false
  Object.assign(createForm, { requestId: buildRequestID('recharge'), accountNo, externalOrderNo: '', amount: '', evidenceRef: '', reason: '', fundsVerified: false })
  createVisible.value = true
}
const submitCreate = async () => {
  if (submitting.value) return
  submitting.value = true
  const current = actionRequests.capture()
  const { fundsVerified: _ignored, ...draft } = createForm
  const payload = { ...draft, externalOrderNo: draft.externalOrderNo.toUpperCase(), sourceType: 'manual', currencyCode: 'COIN' }
  try {
    await createFormRef.value.validate()
    await ElMessageBox.confirm(`确认已收到资金，并为 ${payload.accountNo} 申请充值 ${formatWalletAmount(payload.amount)} COIN？`, '资金到账确认', { type: 'warning', confirmButtonText: '确认提交', cancelButtonText: '取消' })
    if (!current()) return
    const res = await createWalletRecharge(payload)
    if (res.code === 0 && current()) { ElMessage.success('充值申请已创建，等待审核'); createVisible.value = false; page.value = 1; await loadData() }
  } catch (error) { reportSubmitError(error) } finally { submitting.value = false }
}

const selectedOrder = ref(null); const reviewVisible = ref(false); const reviewAction = ref('approve'); const reviewFormRef = ref(); const reviewForm = reactive({ reason: '' }); const reviewRules = { reason: [{ required: true, message: '请填写审核意见', trigger: 'blur' }, { min: 3, max: 500, message: '审核意见需为 3—500 个字符', trigger: 'blur' }] }
const openReview = (order, action) => { if (submitting.value) return; actionRequests.invalidate(); createVisible.value = false; reverseVisible.value = false; selectedOrder.value = { ...order }; reviewAction.value = action; reviewForm.reason = ''; reviewVisible.value = true }
const submitReview = async () => {
  if (submitting.value || !selectedOrder.value) return
  submitting.value = true
  const current = actionRequests.capture(); const order = { ...selectedOrder.value }; const action = reviewAction.value
  const payload = { orderNo: order.orderNo, reason: reviewForm.reason }
  try {
    await reviewFormRef.value.validate()
    const selfReview = currentUserID.value === String(order.createdBy)
    await ElMessageBox.confirm(`确认${action === 'approve' ? '审核通过并立即入账' : '驳回'} ${order.accountNo} 的 ${formatWalletAmount(order.amount)} COIN 申请（${order.orderNo}）？${selfReview ? ' 本次申请和审核将记录为同一管理员。' : ''}`, selfReview ? '顶级超级管理员自审确认' : '双人审核确认', { type: 'warning', confirmButtonText: '确认执行', cancelButtonText: '取消' })
    if (!current()) return
    const api = action === 'approve' ? approveWalletAdjustment : rejectWalletAdjustment
    const res = await api(payload)
    if (res.code === 0 && current()) { ElMessage.success(action === 'approve' ? '审核完成，余额已入账' : '申请已驳回'); reviewVisible.value = false; await loadData() }
  } catch (error) { reportSubmitError(error) } finally { submitting.value = false }
}

const reverseVisible = ref(false); const reverseFormRef = ref(); const reverseForm = reactive({ requestId: '', reason: '', evidenceRef: '' }); const reverseRules = { reason: [{ required: true, message: '请填写冲正原因', trigger: 'blur' }, { min: 3, max: 500, message: '冲正原因需为 3—500 个字符', trigger: 'blur' }] }
const openReverse = (order) => { if (submitting.value) return; actionRequests.invalidate(); createVisible.value = false; reviewVisible.value = false; selectedOrder.value = { ...order }; Object.assign(reverseForm, { requestId: buildRequestID('reverse'), reason: '', evidenceRef: '' }); reverseVisible.value = true }
const submitReverse = async () => {
  if (submitting.value || !selectedOrder.value) return
  submitting.value = true
  const current = actionRequests.capture(); const payload = { ...reverseForm, orderNo: selectedOrder.value.orderNo }
  try {
    await reverseFormRef.value.validate()
    await ElMessageBox.confirm(`确认对 ${payload.orderNo} 创建全额冲正申请？提交后仍需审核。`, '冲正确认', { type: 'warning', confirmButtonText: '确认提交', cancelButtonText: '取消' })
    if (!current()) return
    const res = await reverseWalletAdjustment(payload)
    if (res.code === 0 && current()) { ElMessage.success('冲正申请已创建'); reverseVisible.value = false; await loadData() }
  } catch (error) { reportSubmitError(error) } finally { submitting.value = false }
}

const reportSubmitError = (error) => {
  // 表单错误由表单展示，取消确认也不是提交失败；请求失败必须保留原请求编号。
  if (error instanceof Error) ElMessage.error('操作结果未确认，请查询原单或保留原请求编号重试')
}

const detailVisible = ref(false); const detailLoading = ref(false); const detail = ref(null)
const openDetail = async (order) => { const current = detailRequests.begin(); detailVisible.value = true; detailLoading.value = true; detail.value = null; try { const res = await getWalletAdjustmentDetail({ orderNo: order.orderNo }); if (current() && res.code === 0) detail.value = res.data } finally { if (current()) detailLoading.value = false } }
const auditLabel = (action) => ({ create: '创建申请', create_reverse: '创建冲正', approve_and_credit: '审核并记账', reject: '驳回' }[action] || action)
const invalidateQueries = () => { listRequests.invalidate(); detailRequests.invalidate(); actionRequests.invalidate(); loading.value = false; detailLoading.value = false; createVisible.value = false; reviewVisible.value = false; reverseVisible.value = false; detailVisible.value = false }
onDeactivated(invalidateQueries)
onBeforeUnmount(invalidateQueries)

const initialAccountNo = Array.isArray(route.query.accountNo) ? route.query.accountNo[0] : route.query.accountNo
if (initialAccountNo && /^[A-Za-z0-9_-]{1,32}$/.test(initialAccountNo)) search.accountNo = initialAccountNo
watch(() => route.query.accountNo, (value) => {
  const accountNo = Array.isArray(value) ? value[0] : value
  if (!accountNo || !/^[A-Za-z0-9_-]{1,32}$/.test(accountNo) || accountNo === search.accountNo) return
  invalidateQueries()
  search.accountNo = accountNo; search.phone = ''; submitSearch()
})
loadData()
</script>
