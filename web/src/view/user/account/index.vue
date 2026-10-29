<template>
  <div class="account-page">
    <div class="account-overview">
      <div>
        <div class="overview-title">客户端用户管理</div>
        <div class="overview-desc">查询用户资料、管理账号状态并执行敏感安全操作</div>
      </div>
      <div class="overview-metrics">
        <div><strong>{{ total }}</strong><span>筛选结果</span></div>
        <div><strong>{{ pageMetrics.normal }}</strong><span>本页正常</span></div>
        <div><strong>{{ pageMetrics.restricted }}</strong><span>本页受限</span></div>
        <div><strong>{{ pageMetrics.verified }}</strong><span>本页实名</span></div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="searchInfo" label-width="82px">
        <el-form-item label="账号编号">
          <el-input v-model.trim="searchInfo.accountNo" clearable placeholder="前缀查询" @keyup.enter="onSubmit" />
        </el-form-item>
        <el-form-item label="内部用户名">
          <el-input v-model.trim="searchInfo.username" clearable placeholder="前缀查询" @keyup.enter="onSubmit" />
        </el-form-item>
        <el-form-item label="手机号">
          <el-input v-model.trim="searchInfo.phone" clearable placeholder="完整 E.164 号码" @keyup.enter="onSubmit" />
        </el-form-item>
        <el-form-item label="昵称">
          <el-input v-model.trim="searchInfo.nickname" clearable placeholder="模糊查询" @keyup.enter="onSubmit" />
        </el-form-item>
        <el-form-item label="账号状态">
          <el-select v-model="searchInfo.status" clearable placeholder="全部" style="width: 140px">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="实名状态">
          <el-select v-model="searchInfo.realNameStatus" clearable placeholder="全部" style="width: 140px">
            <el-option v-for="item in realNameOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" icon="search" @click="onSubmit">查询</el-button>
          <el-button icon="refresh" @click="onReset">重置</el-button>
          <el-button link type="primary" @click="advancedVisible = !advancedVisible">
            {{ advancedVisible ? '收起筛选' : '更多筛选' }}
          </el-button>
        </el-form-item>
        <div v-show="advancedVisible" class="advanced-search">
          <el-form-item label="注册来源">
            <el-input v-model.trim="searchInfo.registerSource" clearable maxlength="32" />
          </el-form-item>
          <el-form-item label="渠道 ID">
            <el-input-number v-model="searchInfo.registerChannelId" :min="1" :controls="false" clearable />
          </el-form-item>
          <el-form-item label="注册日期">
            <el-date-picker
              v-model="searchInfo.createdAtRange"
              type="daterange"
              value-format="YYYY-MM-DD"
              start-placeholder="开始日期"
              end-placeholder="结束日期"
              range-separator="至"
            />
          </el-form-item>
        </div>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="gva-btn-list account-toolbar">
        <el-button :loading="loading" @click="loadData">刷新</el-button>
        <span>本页显示完整绑定手机号和当前可用余额，请仅用于必要的后台业务处理。</span>
      </div>
      <el-table v-loading="loading" :data="tableData" stripe row-key="id">
        <el-table-column label="用户" min-width="230">
          <template #default="scope">
            <div class="user-cell">
              <el-avatar :size="40" :src="scope.row.avatar">{{ firstChar(scope.row.nickname) }}</el-avatar>
              <div class="user-copy">
                <strong>{{ scope.row.nickname || '-' }}</strong>
                <span>{{ scope.row.accountNo }}</span>
                <span>@{{ scope.row.username }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="手机号" min-width="145">
          <template #default="scope">
            <span>{{ scope.row.phone || '未绑定' }}</span>
            <el-tag v-if="scope.row.phoneVerified" class="inline-tag" size="small" type="success">已验证</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="当前余额" min-width="145">
          <template #default="scope">
            <div class="balance-cell"><strong>{{ formatWalletAmount(scope.row.walletAvailableBalance || '0') }}</strong><span>COIN</span></div>
            <el-tag v-if="!scope.row.walletOpened" size="small" type="info" effect="plain">未开户</el-tag>
            <el-tag v-else-if="scope.row.walletStatus === 'disabled'" size="small" type="danger" effect="plain">钱包停用</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="账号状态" width="105">
          <template #default="scope">
            <el-tag :type="statusMeta(scope.row.status).type">{{ statusMeta(scope.row.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="实名状态" width="105">
          <template #default="scope">
            <el-tag :type="realNameMeta(scope.row.realNameStatus).type" effect="plain">{{ realNameMeta(scope.row.realNameStatus).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源/渠道" min-width="125">
          <template #default="scope">
            <div>{{ scope.row.registerSource || '-' }}</div>
            <div class="muted-text">渠道 {{ scope.row.registerChannelId || '-' }}</div>
          </template>
        </el-table-column>
        <el-table-column label="最近登录" width="175">
          <template #default="scope">{{ formatTimestamp(scope.row.lastLoginAt) }}</template>
        </el-table-column>
        <el-table-column label="注册时间" width="175">
          <template #default="scope">{{ formatDateTime(scope.row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="500" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="openDetail(scope.row)">详情</el-button>
            <el-button link type="primary" @click="openProfile(scope.row)">编辑资料</el-button>
            <el-button link :type="scope.row.status === 1 ? 'danger' : 'warning'" @click="openStatus(scope.row)">状态</el-button>
            <el-button link type="primary" @click="goWallet(scope.row, 'walletAccount')">钱包账户</el-button>
            <el-button link type="primary" @click="goWallet(scope.row, 'walletLedger')">余额流水</el-button>
            <el-button link type="primary" @click="goWallet(scope.row, 'walletRecharge')">充值单</el-button>
            <el-dropdown trigger="click" @command="(command) => handleCommand(command, scope.row)">
              <el-button link type="primary">更多<el-icon class="el-icon--right"><arrow-down /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="logout">强制全部下线</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
      <div class="gva-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="loadData"
          @size-change="handleSizeChange"
        />
      </div>
    </div>

    <el-drawer v-model="detailVisible" title="用户详情" size="720px" destroy-on-close>
      <div v-loading="detailLoading">
        <template v-if="detail">
          <div class="detail-head">
            <el-avatar :size="64" :src="detail.avatar">{{ firstChar(detail.nickname) }}</el-avatar>
            <div><h3>{{ detail.nickname || '-' }}</h3><p>{{ detail.accountNo }} · @{{ detail.username }}</p></div>
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="内部 ID">{{ detail.id }}</el-descriptions-item>
            <el-descriptions-item label="手机号（完整）">{{ detail.phone || '未绑定' }}</el-descriptions-item>
            <el-descriptions-item label="账号状态"><el-tag :type="statusMeta(detail.status).type">{{ statusMeta(detail.status).label }}</el-tag></el-descriptions-item>
            <el-descriptions-item label="实名状态"><el-tag :type="realNameMeta(detail.realNameStatus).type">{{ realNameMeta(detail.realNameStatus).label }}</el-tag></el-descriptions-item>
            <el-descriptions-item label="签名" :span="2">{{ detail.signature || '-' }}</el-descriptions-item>
            <el-descriptions-item label="性别">{{ genderLabel(detail.gender) }}</el-descriptions-item>
            <el-descriptions-item label="生日">{{ detail.birthday || '-' }}</el-descriptions-item>
            <el-descriptions-item label="国家/地区">{{ detail.countryCode || '-' }}</el-descriptions-item>
            <el-descriptions-item label="地区/城市">{{ [detail.regionCode, detail.cityCode].filter(Boolean).join(' / ') || '-' }}</el-descriptions-item>
            <el-descriptions-item label="语言">{{ detail.language || '-' }}</el-descriptions-item>
            <el-descriptions-item label="封禁截止">{{ formatTimestamp(detail.banUntil, detail.status === 3 ? '永久封禁' : '-') }}</el-descriptions-item>
            <el-descriptions-item label="状态原因" :span="2">{{ detail.statusReason || '-' }}</el-descriptions-item>
            <el-descriptions-item label="注册来源">{{ detail.registerSource || '-' }}</el-descriptions-item>
            <el-descriptions-item label="渠道 ID">{{ detail.registerChannelId || '-' }}</el-descriptions-item>
            <el-descriptions-item label="最近登录">{{ formatTimestamp(detail.lastLoginAt) }}</el-descriptions-item>
            <el-descriptions-item label="修改密码">{{ formatTimestamp(detail.passwordChangedAt) }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDateTime(detail.createdAt) }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ formatDateTime(detail.updatedAt) }}</el-descriptions-item>
          </el-descriptions>
        </template>
      </div>
    </el-drawer>

    <el-dialog v-model="profileVisible" title="编辑用户资料" width="720px" destroy-on-close :close-on-click-modal="false">
      <el-form ref="profileFormRef" :model="profileForm" :rules="profileRules" label-width="90px">
        <el-row :gutter="18">
          <el-col :span="12"><el-form-item label="昵称" prop="nickname"><el-input v-model.trim="profileForm.nickname" maxlength="64" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="手机号" prop="phone"><el-input v-model.trim="profileForm.phone" maxlength="20" placeholder="例如 +60123456789" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="性别"><el-select v-model="profileForm.gender" style="width: 100%"><el-option label="未知" :value="0" /><el-option label="男" :value="1" /><el-option label="女" :value="2" /><el-option label="其他" :value="3" /></el-select></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="生日"><el-date-picker v-model="profileForm.birthday" type="date" value-format="YYYY-MM-DD" clearable style="width: 100%" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="语言"><el-input v-model.trim="profileForm.language" maxlength="16" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="国家代码"><el-input v-model.trim="profileForm.countryCode" maxlength="2" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="地区代码"><el-input v-model.trim="profileForm.regionCode" maxlength="32" /></el-form-item></el-col>
          <el-col :span="8"><el-form-item label="城市代码"><el-input v-model.trim="profileForm.cityCode" maxlength="32" /></el-form-item></el-col>
          <el-col :span="24"><el-form-item label="头像地址"><el-input v-model.trim="profileForm.avatar" maxlength="500" /></el-form-item></el-col>
          <el-col :span="24"><el-form-item label="个性签名"><el-input v-model="profileForm.signature" type="textarea" :rows="3" maxlength="255" show-word-limit /></el-form-item></el-col>
        </el-row>
        <el-alert type="warning" :closable="false" show-icon title="手机号是用户唯一登录凭证，必须填写；换号后用户需使用新手机号重新登录。" />
      </el-form>
      <template #footer><el-button @click="profileVisible = false">取消</el-button><el-button type="primary" :loading="submitLoading" @click="submitProfile">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="statusVisible" title="修改账号状态" width="560px" destroy-on-close :close-on-click-modal="false">
      <el-form :model="statusForm" label-width="96px">
        <el-form-item label="账号状态" required>
          <el-select v-model="statusForm.status" style="width: 100%" :disabled="activeRow?.status === 4">
            <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="statusForm.status !== 1" label="状态原因" required>
          <el-input v-model.trim="statusForm.statusReason" type="textarea" :rows="3" maxlength="255" show-word-limit />
        </el-form-item>
        <template v-if="statusForm.status === 3">
          <el-form-item label="永久封禁"><el-switch v-model="statusForm.banForever" /></el-form-item>
          <el-form-item v-if="!statusForm.banForever" label="封禁截止" required>
            <el-date-picker v-model="statusForm.banUntil" type="datetime" value-format="x" style="width: 100%" />
          </el-form-item>
        </template>
        <el-alert type="warning" :closable="false" show-icon title="修改状态会立即使该用户现有客户端登录失效；封禁或停用主播账号时，也会结束其活动直播场次。" />
      </el-form>
      <template #footer><el-button @click="statusVisible = false">取消</el-button><el-button type="primary" :loading="submitLoading" @click="submitStatus">确认修改</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onActivated, onDeactivated, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { formatDate } from '@/utils/format'
import {
  getLiveAccountDetail,
  getLiveAccountList,
  logoutLiveAccountAll,
  updateLiveAccountProfile,
  updateLiveAccountStatus
} from '@/api/live-account'
import { formatWalletAmount } from '@/view/wallet/shared'
import { createRequestScope } from '@/view/wallet/requestScope'

const router = useRouter()

const statusMap = {
  1: { label: '正常', type: 'success' }, 2: { label: '禁用', type: 'warning' },
  3: { label: '封禁', type: 'danger' }, 4: { label: '注销', type: 'info' }
}
const realNameMap = {
  0: { label: '未认证', type: 'info' }, 1: { label: '审核中', type: 'warning' },
  2: { label: '已认证', type: 'success' }, 3: { label: '未通过', type: 'danger' },
  4: { label: '已过期', type: 'info' }
}
const toOptions = (source) => Object.entries(source).map(([value, meta]) => ({ value: Number(value), label: meta.label }))
const statusOptions = toOptions(statusMap)
const realNameOptions = toOptions(realNameMap)
const statusMeta = (value) => statusMap[value] || { label: '未知', type: 'info' }
const realNameMeta = (value) => realNameMap[value] || { label: '未知', type: 'info' }
const genderLabel = (value) => ({ 0: '未知', 1: '男', 2: '女', 3: '其他' }[value] || '未知')

const defaultSearch = () => ({ accountNo: '', username: '', phone: '', nickname: '', status: null, realNameStatus: null, registerSource: '', registerChannelId: null, createdAtRange: [] })
const defaultProfile = () => ({ accountId: 0, phone: '', nickname: '', avatar: '', signature: '', gender: 0, birthday: null, countryCode: '', regionCode: '', cityCode: '', language: '' })
const defaultStatus = () => ({ accountId: 0, status: 1, statusReason: '', banForever: true, banUntil: null })

const searchInfo = reactive(defaultSearch())
const profileForm = reactive(defaultProfile())
const statusForm = reactive(defaultStatus())
const profileFormRef = ref()
const profileRules = {
  nickname: [{ required: true, message: '请输入昵称', trigger: 'blur' }],
  phone: [{ required: true, pattern: /^\+[1-9]\d{7,14}$/, message: '请输入E.164格式手机号，例如 +60123456789', trigger: 'blur' }]
}
const tableData = ref([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const submitLoading = ref(false)
const advancedVisible = ref(false)
const detailVisible = ref(false)
const detailLoading = ref(false)
const profileVisible = ref(false)
const statusVisible = ref(false)
const detail = ref(null)
const activeRow = ref(null)

const pageMetrics = computed(() => ({
  normal: tableData.value.filter((item) => item.status === 1).length,
  restricted: tableData.value.filter((item) => item.status !== 1).length,
  verified: tableData.value.filter((item) => item.realNameStatus === 2).length
}))

const firstChar = (value) => value?.trim()?.slice(0, 1) || '用'
const formatDateTime = (value) => value ? (formatDate(value) || '-') : '-'
const formatTimestamp = (value, empty = '-') => Number(value) > 0 ? formatDateTime(Number(value)) : empty
const goWallet = (row, name) => router.push({ name, query: { accountNo: row.accountNo } })

const buildParams = () => {
  const params = { page: page.value, pageSize: pageSize.value }
  for (const key of ['accountNo', 'username', 'phone', 'nickname', 'registerSource']) if (searchInfo[key]) params[key] = searchInfo[key]
  for (const key of ['status', 'realNameStatus', 'registerChannelId']) if (searchInfo[key] !== null && searchInfo[key] !== undefined) params[key] = searchInfo[key]
  if (searchInfo.createdAtRange?.length === 2) [params.createdAtStart, params.createdAtEnd] = searchInfo.createdAtRange
  return params
}

const listRequests = createRequestScope()
let needsRefresh = false
const loadData = async () => {
  const current = listRequests.begin()
  const params = buildParams()
  loading.value = true
  try {
    const res = await getLiveAccountList(params)
    if (current() && res.code === 0) {
      tableData.value = res.data.list || []
      total.value = Number(res.data.total || 0)
      page.value = Number(res.data.page || page.value)
      pageSize.value = Number(res.data.pageSize || pageSize.value)
    }
  } finally { if (current()) loading.value = false }
}
onDeactivated(() => { listRequests.invalidate(); loading.value = false; needsRefresh = true })
onBeforeUnmount(() => listRequests.invalidate())
onActivated(() => { if (needsRefresh) { needsRefresh = false; loadData() } })
const onSubmit = () => { page.value = 1; loadData() }
const onReset = () => { Object.assign(searchInfo, defaultSearch()); page.value = 1; loadData() }
const handleSizeChange = () => { page.value = 1; loadData() }

const fetchDetail = async (row) => {
  const res = await getLiveAccountDetail({ accountId: row.id })
  return res.code === 0 ? res.data : null
}
const openDetail = async (row) => {
  detailVisible.value = true
  detailLoading.value = true
  try { detail.value = await fetchDetail(row) } finally { detailLoading.value = false }
}
const openProfile = async (row) => {
  const data = await fetchDetail(row)
  if (!data) return
  Object.assign(profileForm, defaultProfile(), {
    accountId: data.id, phone: data.phone || '', nickname: data.nickname, avatar: data.avatar || '', signature: data.signature || '',
    gender: data.gender || 0, birthday: data.birthday || null, countryCode: data.countryCode || '',
    regionCode: data.regionCode || '', cityCode: data.cityCode || '', language: data.language || ''
  })
  profileVisible.value = true
}
const submitProfile = async () => {
  const valid = await profileFormRef.value?.validate().catch(() => false)
  if (!valid) return
  submitLoading.value = true
  try {
    const res = await updateLiveAccountProfile({ ...profileForm })
    if (res.code === 0) { ElMessage.success('资料已更新'); profileVisible.value = false; await loadData() }
  } finally { submitLoading.value = false }
}

const openStatus = (row) => {
  activeRow.value = row
  Object.assign(statusForm, defaultStatus(), {
    accountId: row.id, status: row.status, banForever: row.status !== 3 || !row.banUntil,
    banUntil: row.banUntil || null
  })
  statusVisible.value = true
}
const submitStatus = async () => {
  if (statusForm.status !== 1 && !statusForm.statusReason) return ElMessage.warning('请填写状态原因')
  if (statusForm.status === 3 && !statusForm.banForever && !statusForm.banUntil) return ElMessage.warning('请选择封禁截止时间')
  submitLoading.value = true
  try {
    const res = await updateLiveAccountStatus({
      accountId: statusForm.accountId, status: statusForm.status,
      statusReason: statusForm.status === 1 ? '' : statusForm.statusReason,
      banUntil: statusForm.status === 3 && !statusForm.banForever ? Number(statusForm.banUntil) : 0
    })
    if (res.code === 0) { ElMessage.success('账号状态已更新'); statusVisible.value = false; await loadData() }
  } finally { submitLoading.value = false }
}

const handleCommand = async (command, row) => {
  if (command === 'logout') {
    try { await ElMessageBox.confirm(`确认强制“${row.nickname}”的全部客户端登录失效吗？`, '强制下线', { type: 'warning' }) } catch { return }
    const res = await logoutLiveAccountAll({ accountId: row.id })
    if (res.code === 0) ElMessage.success('用户已强制下线')
    return
  }
}

onMounted(loadData)
</script>

<style scoped lang="scss">
.account-page {
  .account-overview { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 22px 24px; color: #fff; background: linear-gradient(120deg, #172554 0%, #2563eb 58%, #38bdf8 100%); border-radius: 10px; }
  .overview-title { font-size: 22px; font-weight: 700; }
  .overview-desc { margin-top: 7px; color: rgb(255 255 255 / 76%); font-size: 13px; }
  .overview-metrics { display: flex; gap: 10px; }
  .overview-metrics > div { display: flex; flex-direction: column; min-width: 86px; padding: 10px 14px; background: rgb(255 255 255 / 12%); border: 1px solid rgb(255 255 255 / 16%); border-radius: 8px; }
  .overview-metrics strong { font-size: 20px; }
  .overview-metrics span { color: rgb(255 255 255 / 72%); font-size: 12px; }
  .advanced-search { display: inline; }
  .account-toolbar { align-items: center; justify-content: space-between; color: var(--el-text-color-secondary); font-size: 12px; }
  .user-cell, .detail-head { display: flex; align-items: center; gap: 12px; }
  .user-copy { display: flex; flex-direction: column; min-width: 0; }
  .user-copy span, .muted-text, .detail-head p { color: var(--el-text-color-secondary); font-size: 12px; }
  .inline-tag { margin-left: 6px; }
  .balance-cell { display: flex; align-items: baseline; gap: 5px; }
  .balance-cell strong { color: var(--el-color-success); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .balance-cell span { color: var(--el-text-color-secondary); font-size: 11px; }
  .detail-head { margin-bottom: 20px; }
  .detail-head h3, .detail-head p { margin: 0; }
  .detail-head p { margin-top: 5px; }
}
@media (max-width: 900px) {
  .account-page {
    .account-overview { align-items: flex-start; flex-direction: column; }
    .overview-metrics { width: 100%; overflow-x: auto; }
    .overview-metrics > div { flex: 1; }
  }
}
</style>
