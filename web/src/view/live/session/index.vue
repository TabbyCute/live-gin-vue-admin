<template>
  <div class="live-session-page">
    <div class="session-overview">
      <div>
        <div class="overview-title">直播场次</div>
        <div class="overview-desc">每次逻辑开播对应一条场次。短暂断流重连沿用原场次，超过重连窗口才进入结束与结算。</div>
      </div>
      <div class="overview-counts"><div><strong>{{ total }}</strong><span>筛选结果</span></div><div><strong>{{ activeCount }}</strong><span>本页活动场次</span></div><div><strong>{{ endingCount }}</strong><span>本页结束中</span></div></div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="searchInfo" label-width="76px">
        <el-form-item label="场次编号"><el-input v-model.trim="searchInfo.sessionNo" clearable placeholder="sessionNo" @keyup.enter="onSubmit" /></el-form-item>
        <el-form-item label="房间编号"><el-input v-model.trim="searchInfo.roomNo" clearable placeholder="roomNo" @keyup.enter="onSubmit" /></el-form-item>
        <el-form-item label="主播编号"><el-input v-model.trim="searchInfo.anchorNo" clearable placeholder="anchorNo" @keyup.enter="onSubmit" /></el-form-item>
        <el-form-item label="分类"><el-select v-model="searchInfo.categoryId" clearable placeholder="全部" style="width: 170px"><el-option v-for="item in categoryOptions" :key="item.id" :label="item.label" :value="item.id" /></el-select></el-form-item>
        <el-form-item label="状态"><el-select v-model="searchInfo.status" clearable placeholder="全部" style="width: 130px"><el-option v-for="item in statuses" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
        <el-form-item label="开播时间"><el-date-picker v-model="searchInfo.startedAtRange" type="datetimerange" range-separator="至" start-placeholder="开始" end-placeholder="结束" value-format="x" style="width: 355px" /></el-form-item>
        <el-form-item><el-button type="primary" icon="search" @click="onSubmit">查询</el-button><el-button icon="refresh" @click="onReset">重置</el-button></el-form-item>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="gva-btn-list session-toolbar"><el-button :loading="loading" @click="refreshData">刷新</el-button><span>durationMs 是逻辑直播时长；成功重连前的短暂断流会计入，最终超时等待窗口不会计入。</span></div>
      <el-table v-loading="loading" :data="tableData" stripe>
        <el-table-column label="内部 ID" prop="id" width="90" />
        <el-table-column label="场次" min-width="215"><template #default="scope"><div class="main-cell"><strong>{{ scope.row.title || '-' }}</strong><span>{{ scope.row.sessionNo }}</span></div></template></el-table-column>
        <el-table-column label="直播间 / 主播" min-width="205"><template #default="scope"><div class="main-cell"><strong>{{ scope.row.roomNo }}</strong><span>{{ scope.row.anchorNo }}</span></div></template></el-table-column>
        <el-table-column label="状态" width="105"><template #default="scope"><el-tag :type="statusMeta(scope.row.status).type">{{ statusMeta(scope.row.status).label }}</el-tag></template></el-table-column>
        <el-table-column label="开始时间" width="178"><template #default="scope">{{ formatTimestamp(scope.row.startedAt) }}</template></el-table-column>
        <el-table-column label="结束时间" width="178"><template #default="scope">{{ formatTimestamp(scope.row.endedAt) }}</template></el-table-column>
        <el-table-column label="逻辑时长" width="125"><template #default="scope">{{ durationText(scope.row.durationMs) }}</template></el-table-column>
        <el-table-column label="断流次数" prop="disconnectCount" width="100" />
        <el-table-column label="观看 / 峰值" width="125"><template #default="scope">{{ scope.row.viewCount }} / {{ scope.row.peakOnlineCount }}</template></el-table-column>
        <el-table-column label="点赞" prop="likeCount" width="90" />
        <el-table-column label="礼物件数" prop="giftCount" width="100" />
        <el-table-column label="礼物金币" prop="giftCoinAmount" width="110" />
        <el-table-column label="操作" width="155" fixed="right"><template #default="scope"><el-button link type="primary" @click="showDetail(scope.row)">详情</el-button><el-button v-if="[0, 1].includes(scope.row.status)" link type="danger" @click="openEnd(scope.row)">强制结束</el-button></template></el-table-column>
      </el-table>
      <div class="gva-pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[10, 20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next, jumper" @current-change="getTableData" @size-change="handleSizeChange" /></div>
    </div>

    <el-dialog v-model="endVisible" title="强制结束直播场次" width="520px" destroy-on-close>
      <el-alert type="warning" :closable="false" :title="`将结束场次 ${endForm.sessionNo}`" class="dialog-alert" />
      <el-form label-width="84px"><el-form-item label="结束原因"><el-input v-model.trim="endForm.reason" type="textarea" :rows="4" maxlength="500" show-word-limit placeholder="建议填写，便于后续审计和主播问题排查" /></el-form-item></el-form>
      <template #footer><el-button @click="endVisible = false">取消</el-button><el-button type="danger" :loading="submitting" @click="submitEnd">确认结束</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="直播场次详情" size="680px">
      <template v-if="detail">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="内部场次 ID">{{ detail.id }}</el-descriptions-item><el-descriptions-item label="场次编号">{{ detail.sessionNo }}</el-descriptions-item>
          <el-descriptions-item label="内部房间 ID">{{ detail.roomId }}</el-descriptions-item><el-descriptions-item label="房间编号">{{ detail.roomNo }}</el-descriptions-item>
          <el-descriptions-item label="内部主播 ID">{{ detail.anchorId }}</el-descriptions-item><el-descriptions-item label="主播编号">{{ detail.anchorNo }}</el-descriptions-item>
          <el-descriptions-item label="标题" :span="2">{{ detail.title || '-' }}</el-descriptions-item><el-descriptions-item label="封面" :span="2">{{ detail.coverUrl || '-' }}</el-descriptions-item>
          <el-descriptions-item label="分类">{{ categoryLabel(detail.categoryId) }}</el-descriptions-item><el-descriptions-item label="状态"><el-tag :type="statusMeta(detail.status).type">{{ statusMeta(detail.status).label }}</el-tag></el-descriptions-item>
          <el-descriptions-item label="首次推流">{{ formatTimestamp(detail.startedAt) }}</el-descriptions-item><el-descriptions-item label="最终结束">{{ formatTimestamp(detail.endedAt) }}</el-descriptions-item>
          <el-descriptions-item label="逻辑时长">{{ durationText(detail.durationMs) }}</el-descriptions-item><el-descriptions-item label="断流次数">{{ detail.disconnectCount }}</el-descriptions-item>
          <el-descriptions-item label="最近断流">{{ formatTimestamp(detail.lastUnpublishAt) }}</el-descriptions-item><el-descriptions-item label="重连截止">{{ formatTimestamp(detail.reconnectDeadlineAt) }}</el-descriptions-item>
          <el-descriptions-item label="结束原因">{{ endReasonLabel(detail.endReason) }}</el-descriptions-item><el-descriptions-item label="统计完成">{{ formatTimestamp(detail.statsFinalizedAt) }}</el-descriptions-item>
          <el-descriptions-item label="失败/操作原因" :span="2">{{ detail.failureReason || '-' }}</el-descriptions-item>
        </el-descriptions>
        <div class="detail-section-title">本场统计快照</div>
        <div class="stat-grid">
          <div><strong>{{ detail.viewCount }}</strong><span>进入次数</span></div><div><strong>{{ detail.viewerCount }}</strong><span>去重观众</span></div><div><strong>{{ detail.peakOnlineCount }}</strong><span>峰值在线</span></div>
          <div><strong>{{ detail.likeCount }}</strong><span>点赞</span></div><div><strong>{{ detail.giftCount }}</strong><span>礼物件数</span></div><div><strong>{{ detail.giftCoinAmount }}</strong><span>礼物金币</span></div><div><strong>{{ detail.giftUserCount }}</strong><span>送礼人数</span></div>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { formatDate } from '@/utils/format'
import { getCategoryTree } from '@/api/live-category'
import { endLiveSession, getLiveSessionDetail, getLiveSessionList } from '@/api/live-room'

const statuses = [{ value: 0, label: '准备中', type: 'warning' }, { value: 1, label: '直播中', type: 'success' }, { value: 2, label: '结束中', type: 'warning' }, { value: 3, label: '已结束', type: 'info' }, { value: 4, label: '已取消', type: 'info' }, { value: 5, label: '失败', type: 'danger' }]
const endReasons = ['未知', '主播结束', '管理员结束', '断流超时', '主播封禁/权限关闭', '系统异常']
const defaultSearch = () => ({ sessionNo: '', roomNo: '', anchorNo: '', categoryId: null, status: null, startedAtRange: [] })
const searchInfo = reactive(defaultSearch()); const page = ref(1); const pageSize = ref(20); const total = ref(0)
const loading = ref(false); const submitting = ref(false); const tableData = ref([]); const categoryTree = ref([]); const detail = ref(null)
const detailVisible = ref(false); const endVisible = ref(false); const endForm = reactive({ sessionId: 0, sessionNo: '', reason: '' })
const activeCount = computed(() => tableData.value.filter((item) => [0, 1].includes(item.status)).length)
const endingCount = computed(() => tableData.value.filter((item) => item.status === 2).length)
const flatten = (items, depth = 0) => items.flatMap((item) => [{ id: item.id, label: `${'　'.repeat(depth)}${item.name}` }, ...flatten(item.children || [], depth + 1)])
const categoryOptions = computed(() => flatten(categoryTree.value))
const categoryLabel = (id) => id ? (categoryOptions.value.find((item) => item.id === id)?.label.trim() || `分类 ID ${id}`) : '未分类'
const statusMeta = (value) => statuses.find((item) => item.value === value) || { label: `未知(${value})`, type: 'info' }
const endReasonLabel = (value) => endReasons[value] || `未知(${value})`
const formatTimestamp = (value) => Number(value) ? (formatDate(Number(value)) || '-') : '-'
const durationText = (milliseconds) => { const seconds = Math.floor(Number(milliseconds || 0) / 1000); if (seconds < 60) return `${seconds} 秒`; if (seconds < 3600) return `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`; return `${Math.floor(seconds / 3600)} 小时 ${Math.floor((seconds % 3600) / 60)} 分` }
const params = () => { const result = { page: page.value, pageSize: pageSize.value }; ['sessionNo', 'roomNo', 'anchorNo', 'categoryId', 'status'].forEach((key) => { const value = searchInfo[key]; if (value !== '' && value !== null && value !== undefined) result[key] = value }); if (searchInfo.startedAtRange?.length === 2) { result.startedAtStart = Number(searchInfo.startedAtRange[0]); result.startedAtEnd = Number(searchInfo.startedAtRange[1]) } return result }
const getTableData = async () => { loading.value = true; try { const res = await getLiveSessionList(params()); if (res.code === 0) { tableData.value = res.data.list || []; total.value = Number(res.data.total || 0) } } finally { loading.value = false } }
const loadCategories = async () => { const res = await getCategoryTree(); if (res.code === 0) categoryTree.value = res.data || [] }
const refreshData = () => Promise.all([getTableData(), loadCategories()])
const onSubmit = () => { page.value = 1; getTableData() }; const onReset = () => { Object.assign(searchInfo, defaultSearch()); page.value = 1; getTableData() }; const handleSizeChange = () => { page.value = 1; getTableData() }
const showDetail = async (row) => { const res = await getLiveSessionDetail({ sessionId: row.id }); if (res.code === 0) { detail.value = res.data; detailVisible.value = true } }
const openEnd = (row) => { Object.assign(endForm, { sessionId: row.id, sessionNo: row.sessionNo, reason: '' }); endVisible.value = true }
const submitEnd = async () => { if (!endForm.reason) return ElMessage.warning('请填写强制结束原因'); submitting.value = true; try { const res = await endLiveSession({ sessionId: endForm.sessionId, reason: endForm.reason }); if (res.code === 0) { ElMessage.success('场次已进入结束流程'); endVisible.value = false; getTableData() } } finally { submitting.value = false } }
onMounted(refreshData)
</script>

<style scoped lang="scss">
.live-session-page { padding: 0; }.session-overview { display: flex; justify-content: space-between; align-items: center; gap: 24px; margin-bottom: 16px; padding: 22px 24px; color: #fff; border-radius: 12px; background: linear-gradient(125deg, #312e81, #7c3aed 62%, #c026d3); }.overview-title { font-size: 22px; font-weight: 700; }.overview-desc { margin-top: 7px; opacity: .82; }.overview-counts { display: flex; gap: 30px; }.overview-counts div { display: flex; flex-direction: column; align-items: center; }.overview-counts strong { font-size: 23px; }.overview-counts span { margin-top: 3px; font-size: 12px; opacity: .8; }
.session-toolbar { display: flex; align-items: center; justify-content: space-between; color: #909399; font-size: 13px; }.main-cell { display: flex; flex-direction: column; gap: 4px; }.main-cell span { color: #909399; font-size: 12px; }.dialog-alert { margin-bottom: 18px; }.detail-section-title { margin: 24px 0 12px; font-size: 16px; font-weight: 700; }.stat-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }.stat-grid div { display: flex; min-height: 78px; padding: 12px; border: 1px solid #ebeef5; border-radius: 8px; flex-direction: column; justify-content: center; align-items: center; }.stat-grid strong { font-size: 19px; }.stat-grid span { margin-top: 5px; color: #909399; font-size: 12px; }
@media (max-width: 900px) { .session-overview { align-items: flex-start; flex-direction: column; }.overview-counts { width: 100%; justify-content: space-around; }.stat-grid { grid-template-columns: repeat(2, 1fr); } }
</style>
