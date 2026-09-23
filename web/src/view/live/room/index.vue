<template>
  <div class="live-room-page">
    <div class="room-overview">
      <div>
        <div class="overview-title">直播间管理</div>
        <div class="overview-desc">管理直播间固定资料、可见范围和使用状态。禁用或关闭活动直播间时，当前场次会进入结束流程。</div>
      </div>
      <div class="overview-counts">
        <div><strong>{{ total }}</strong><span>筛选结果</span></div>
        <div><strong>{{ livingCount }}</strong><span>本页直播中</span></div>
        <div><strong>{{ disabledCount }}</strong><span>本页禁用</span></div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="searchInfo" label-width="78px">
        <el-form-item label="房间编号"><el-input v-model.trim="searchInfo.roomNo" clearable placeholder="roomNo" @keyup.enter="onSubmit" /></el-form-item>
        <el-form-item label="主播编号"><el-input v-model.trim="searchInfo.anchorNo" clearable placeholder="anchorNo" @keyup.enter="onSubmit" /></el-form-item>
        <el-form-item label="标题"><el-input v-model.trim="searchInfo.title" clearable placeholder="直播间标题" @keyup.enter="onSubmit" /></el-form-item>
        <el-form-item label="分类">
          <el-select v-model="searchInfo.categoryId" clearable placeholder="全部" style="width: 170px">
            <el-option v-for="item in categoryOptions" :key="item.id" :label="item.label" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="房间状态">
          <el-select v-model="searchInfo.status" clearable placeholder="全部" style="width: 130px">
            <el-option v-for="item in roomStatuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item label="直播状态">
          <el-select v-model="searchInfo.liveStatus" clearable placeholder="全部" style="width: 130px">
            <el-option v-for="item in liveStatuses" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" icon="search" @click="onSubmit">查询</el-button>
          <el-button icon="refresh" @click="onReset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="gva-btn-list room-toolbar">
        <el-button :loading="loading" @click="refreshData">刷新</el-button>
        <span>0 禁用表示后台/风控封禁；2 关闭表示主播主动关闭或运营停用。</span>
      </div>
      <el-table v-loading="loading" :data="tableData" stripe>
        <el-table-column label="内部 ID" prop="id" width="90" />
        <el-table-column label="直播间" min-width="220">
          <template #default="scope">
            <div class="main-cell"><strong>{{ scope.row.title || '-' }}</strong><span>{{ scope.row.roomNo }}</span></div>
          </template>
        </el-table-column>
        <el-table-column label="主播" min-width="170">
          <template #default="scope"><div class="main-cell"><strong>{{ scope.row.anchorNickname || '-' }}</strong><span>{{ scope.row.anchorNo }} / ID {{ scope.row.anchorId }}</span></div></template>
        </el-table-column>
        <el-table-column label="分类" min-width="140"><template #default="scope">{{ categoryLabel(scope.row.categoryId) }}</template></el-table-column>
        <el-table-column label="房间状态" width="105"><template #default="scope"><el-tag :type="roomStatus(scope.row.status).type">{{ roomStatus(scope.row.status).label }}</el-tag></template></el-table-column>
        <el-table-column label="直播状态" width="105"><template #default="scope"><el-tag :type="liveStatus(scope.row.liveStatus).type">{{ liveStatus(scope.row.liveStatus).label }}</el-tag></template></el-table-column>
        <el-table-column label="可见范围" width="110"><template #default="scope">{{ visibilityLabel(scope.row.visibility) }}</template></el-table-column>
        <el-table-column label="推荐权重" prop="recommendWeight" width="105" sortable />
        <el-table-column label="当前场次 ID" width="120"><template #default="scope">{{ scope.row.currentSessionId || '-' }}</template></el-table-column>
        <el-table-column label="开播时间" width="178"><template #default="scope">{{ formatTimestamp(scope.row.liveStartedAt) }}</template></el-table-column>
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="showDetail(scope.row)">详情</el-button>
            <el-button link type="primary" @click="openEdit(scope.row)">编辑</el-button>
            <el-button link :type="scope.row.status === 0 ? 'success' : 'warning'" @click="openStatus(scope.row)">状态</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="gva-pagination">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[10, 20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next, jumper" @current-change="getTableData" @size-change="handleSizeChange" />
      </div>
    </div>

    <el-dialog v-model="editVisible" title="编辑直播间" width="620px" destroy-on-close>
      <el-form ref="editFormRef" :model="editForm" :rules="editRules" label-width="92px">
        <el-form-item label="房间编号" prop="roomNo"><el-input v-model.trim="editForm.roomNo" maxlength="32" show-word-limit /></el-form-item>
        <el-alert type="info" :closable="false" title="修改房间编号会改变客户端查询与分享地址，不会修改稳定推流名称。" class="dialog-alert" />
        <el-form-item label="分类" prop="categoryId"><el-select v-model="editForm.categoryId" style="width: 100%"><el-option label="未分类" :value="0" /><el-option v-for="item in categoryOptions" :key="item.id" :label="item.label" :value="item.id" /></el-select></el-form-item>
        <el-form-item label="标题" prop="title"><el-input v-model.trim="editForm.title" maxlength="128" show-word-limit /></el-form-item>
        <el-form-item label="封面地址"><el-input v-model.trim="editForm.coverUrl" maxlength="500" /></el-form-item>
        <el-form-item label="直播公告"><el-input v-model.trim="editForm.notice" type="textarea" :rows="3" maxlength="500" show-word-limit /></el-form-item>
        <el-form-item label="可见范围"><el-radio-group v-model="editForm.visibility"><el-radio-button :value="0">私密</el-radio-button><el-radio-button :value="1">公开</el-radio-button><el-radio-button :value="2">仅关注者</el-radio-button></el-radio-group></el-form-item>
        <el-form-item label="推荐权重"><el-input-number v-model="editForm.recommendWeight" :controls="false" style="width: 100%" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="editVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="submitEdit">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="statusVisible" title="修改直播间状态" width="520px" destroy-on-close>
      <el-alert type="warning" :closable="false" title="非正常状态会取消准备中的场次，或结束当前直播中的场次。" class="dialog-alert" />
      <el-form label-width="92px">
        <el-form-item label="目标状态"><el-radio-group v-model="statusForm.status"><el-radio-button :value="1">正常</el-radio-button><el-radio-button :value="2">关闭</el-radio-button><el-radio-button :value="0">禁用</el-radio-button></el-radio-group></el-form-item>
        <el-form-item label="原因" :required="statusForm.status === 0"><el-input v-model.trim="statusForm.statusReason" type="textarea" :rows="3" maxlength="255" show-word-limit /></el-form-item>
      </el-form>
      <template #footer><el-button @click="statusVisible = false">取消</el-button><el-button type="primary" :loading="submitting" @click="submitStatus">确认</el-button></template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="直播间详情" size="620px">
      <el-descriptions v-if="detail" :column="2" border>
        <el-descriptions-item label="内部 ID">{{ detail.id }}</el-descriptions-item><el-descriptions-item label="房间编号">{{ detail.roomNo }}</el-descriptions-item>
        <el-descriptions-item label="主播内部 ID">{{ detail.anchorId }}</el-descriptions-item><el-descriptions-item label="主播编号">{{ detail.anchorNo }}</el-descriptions-item>
        <el-descriptions-item label="主播展示名">{{ detail.anchorNickname || '-' }}</el-descriptions-item><el-descriptions-item label="分类">{{ categoryLabel(detail.categoryId) }}</el-descriptions-item>
        <el-descriptions-item label="标题" :span="2">{{ detail.title || '-' }}</el-descriptions-item><el-descriptions-item label="公告" :span="2">{{ detail.notice || '-' }}</el-descriptions-item>
        <el-descriptions-item label="封面" :span="2">{{ detail.coverUrl || '-' }}</el-descriptions-item><el-descriptions-item label="稳定流名称">{{ detail.streamName }}</el-descriptions-item>
        <el-descriptions-item label="密钥版本">{{ detail.streamKeyVersion }}</el-descriptions-item><el-descriptions-item label="房间状态">{{ roomStatus(detail.status).label }}</el-descriptions-item>
        <el-descriptions-item label="直播状态">{{ liveStatus(detail.liveStatus).label }}</el-descriptions-item><el-descriptions-item label="状态原因">{{ detail.statusReason || '-' }}</el-descriptions-item>
        <el-descriptions-item label="当前场次 ID">{{ detail.currentSessionId || '-' }}</el-descriptions-item><el-descriptions-item label="当前开播时间">{{ formatTimestamp(detail.liveStartedAt) }}</el-descriptions-item>
        <el-descriptions-item label="可见范围">{{ visibilityLabel(detail.visibility) }}</el-descriptions-item><el-descriptions-item label="推荐权重">{{ detail.recommendWeight }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatDateTime(detail.createdAt) }}</el-descriptions-item><el-descriptions-item label="更新时间">{{ formatDateTime(detail.updatedAt) }}</el-descriptions-item>
      </el-descriptions>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { formatDate } from '@/utils/format'
import { getCategoryTree } from '@/api/live-category'
import { getLiveRoomDetail, getLiveRoomList, updateLiveRoom, updateLiveRoomStatus } from '@/api/live-room'

const roomStatuses = [{ value: 0, label: '禁用', type: 'danger' }, { value: 1, label: '正常', type: 'success' }, { value: 2, label: '关闭', type: 'info' }]
const liveStatuses = [{ value: 0, label: '未开播', type: 'info' }, { value: 1, label: '准备中', type: 'warning' }, { value: 2, label: '直播中', type: 'success' }, { value: 3, label: '结束中', type: 'warning' }]
const defaultSearch = () => ({ roomNo: '', anchorNo: '', title: '', categoryId: null, status: null, liveStatus: null })
const searchInfo = reactive(defaultSearch())
const page = ref(1); const pageSize = ref(20); const total = ref(0); const loading = ref(false); const submitting = ref(false)
const tableData = ref([]); const categoryTree = ref([]); const detail = ref(null)
const editVisible = ref(false); const statusVisible = ref(false); const detailVisible = ref(false); const editFormRef = ref()
const editForm = reactive({ roomId: 0, roomNo: '', categoryId: 0, title: '', coverUrl: '', notice: '', visibility: 1, recommendWeight: 0 })
const statusForm = reactive({ roomId: 0, status: 1, statusReason: '' })
const editRules = {
  roomNo: [{ required: true, whitespace: true, message: '请输入房间编号', trigger: 'blur' }],
  title: [{ required: true, message: '请输入直播间标题', trigger: 'blur' }]
}
const livingCount = computed(() => tableData.value.filter((item) => item.liveStatus === 2).length)
const disabledCount = computed(() => tableData.value.filter((item) => item.status === 0).length)
const flatten = (items, depth = 0) => items.flatMap((item) => [{ id: item.id, label: `${'　'.repeat(depth)}${item.name}` }, ...flatten(item.children || [], depth + 1)])
const categoryOptions = computed(() => flatten(categoryTree.value))
const categoryLabel = (id) => id ? (categoryOptions.value.find((item) => item.id === id)?.label.trim() || `分类 ID ${id}`) : '未分类'
const roomStatus = (value) => roomStatuses.find((item) => item.value === value) || { label: `未知(${value})`, type: 'info' }
const liveStatus = (value) => liveStatuses.find((item) => item.value === value) || { label: `未知(${value})`, type: 'info' }
const visibilityLabel = (value) => ['私密', '公开', '仅关注者'][value] || `未知(${value})`
const formatDateTime = (value) => value ? (formatDate(value) || '-') : '-'
const formatTimestamp = (value) => Number(value) ? (formatDate(Number(value)) || '-') : '-'
const params = () => { const result = { page: page.value, pageSize: pageSize.value }; Object.entries(searchInfo).forEach(([key, value]) => { if (value !== '' && value !== null && value !== undefined) result[key] = value }); return result }
const getTableData = async () => { loading.value = true; try { const res = await getLiveRoomList(params()); if (res.code === 0) { tableData.value = res.data.list || []; total.value = Number(res.data.total || 0) } } finally { loading.value = false } }
const loadCategories = async () => { const res = await getCategoryTree(); if (res.code === 0) categoryTree.value = res.data || [] }
const refreshData = () => Promise.all([getTableData(), loadCategories()])
const onSubmit = () => { page.value = 1; getTableData() }
const onReset = () => { Object.assign(searchInfo, defaultSearch()); page.value = 1; getTableData() }
const handleSizeChange = () => { page.value = 1; getTableData() }
const openEdit = (row) => { Object.assign(editForm, { roomId: row.id, roomNo: row.roomNo, categoryId: row.categoryId, title: row.title, coverUrl: row.coverUrl, notice: row.notice, visibility: row.visibility, recommendWeight: row.recommendWeight }); editVisible.value = true }
const submitEdit = async () => { await editFormRef.value.validate(); submitting.value = true; try { const res = await updateLiveRoom({ ...editForm }); if (res.code === 0) { ElMessage.success('保存成功'); editVisible.value = false; getTableData() } } finally { submitting.value = false } }
const openStatus = (row) => { Object.assign(statusForm, { roomId: row.id, status: row.status, statusReason: row.statusReason || '' }); statusVisible.value = true }
const submitStatus = async () => { if (statusForm.status === 0 && !statusForm.statusReason) return ElMessage.warning('禁用直播间必须填写原因'); submitting.value = true; try { const res = await updateLiveRoomStatus({ ...statusForm }); if (res.code === 0) { ElMessage.success('状态已更新'); statusVisible.value = false; getTableData() } } finally { submitting.value = false } }
const showDetail = async (row) => { const res = await getLiveRoomDetail({ roomId: row.id }); if (res.code === 0) { detail.value = res.data; detailVisible.value = true } }
onMounted(refreshData)
</script>

<style scoped lang="scss">
.live-room-page { padding: 0; }
.room-overview { display: flex; justify-content: space-between; align-items: center; gap: 24px; margin-bottom: 16px; padding: 22px 24px; color: #fff; border-radius: 12px; background: linear-gradient(125deg, #172554, #1d4ed8 62%, #0ea5e9); }
.overview-title { font-size: 22px; font-weight: 700; }.overview-desc { margin-top: 7px; opacity: .82; }.overview-counts { display: flex; gap: 30px; }.overview-counts div { display: flex; flex-direction: column; align-items: center; }.overview-counts strong { font-size: 23px; }.overview-counts span { margin-top: 3px; font-size: 12px; opacity: .8; }
.room-toolbar { display: flex; align-items: center; justify-content: space-between; color: #909399; font-size: 13px; }.main-cell { display: flex; flex-direction: column; gap: 4px; }.main-cell span { color: #909399; font-size: 12px; }.dialog-alert { margin-bottom: 18px; }
@media (max-width: 900px) { .room-overview { align-items: flex-start; flex-direction: column; }.overview-counts { width: 100%; justify-content: space-around; } }
</style>
