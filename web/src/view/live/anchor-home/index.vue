<template>
  <div>
    <el-alert title="主播上传的照片审核通过后才对外展示；驳回或下架必须填写原因。" type="info" :closable="false" class="mb-4" />
    <el-alert v-if="error" :title="error" type="error" :closable="false" class="mb-4" />
    <el-alert v-if="pending" title="存在尚未确认的审核操作，请先核实原请求结果。" type="warning" :closable="false" class="mb-4">
      <el-button :loading="saving" @click="retryPending">使用原请求核实结果</el-button>
    </el-alert>
    <div class="gva-search-box">
      <el-form inline @submit.prevent="search">
        <el-form-item label="主播编号"><el-input v-model="query.anchorNo" maxlength="64" clearable /></el-form-item>
        <el-form-item label="状态"><el-select v-model="query.status" clearable class="w-40"><el-option v-for="(label, value) in photoStatuses" :key="value" :label="label" :value="value" /></el-select></el-form-item>
        <el-form-item><el-button type="primary" :loading="loading" @click="search">查询</el-button></el-form-item>
      </el-form>
    </div>
    <div class="gva-table-box">
      <el-table v-loading="loading" :data="rows" row-key="photoNo">
        <el-table-column label="照片编号" prop="photoNo" min-width="200" show-overflow-tooltip />
        <el-table-column label="主播编号" prop="anchorNo" min-width="170" show-overflow-tooltip />
        <el-table-column label="状态" width="100"><template #default="{ row }">{{ photoStatuses[row.status] || row.status }}</template></el-table-column>
        <el-table-column label="尺寸" width="130"><template #default="{ row }">{{ row.width }} × {{ row.height }}</template></el-table-column>
        <el-table-column label="上传时间" min-width="170"><template #default="{ row }">{{ time(row.createdAt) }}</template></el-table-column>
        <el-table-column label="审核原因" prop="reason" min-width="180" show-overflow-tooltip />
        <el-table-column label="操作" width="100"><template #default="{ row }"><el-button type="primary" link :disabled="!!pending || saving" @click="open(row)">查看审核</el-button></template></el-table-column>
      </el-table>
      <el-pagination v-model:current-page="query.page" v-model:page-size="query.pageSize" :total="total" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next" @current-change="load" @size-change="search" />
    </div>
    <el-dialog :model-value="visible" title="主播主页照片审核" width="min(90vw, 760px)" :close-on-click-modal="false" @update:model-value="closePreview">
      <div v-if="selected" v-loading="previewLoading">
        <img v-if="imageURL" class="album-review-image" :src="imageURL" alt="待审核的主播主页照片" />
        <el-descriptions :column="1" border class="my-4">
          <el-descriptions-item label="主播编号">{{ selected.anchorNo }}</el-descriptions-item>
          <el-descriptions-item label="当前状态">{{ photoStatuses[selected.status] }}</el-descriptions-item>
          <el-descriptions-item v-if="selected.reason" label="历史原因">{{ selected.reason }}</el-descriptions-item>
        </el-descriptions>
        <el-form v-if="['pending', 'approved'].includes(selected.status)" label-position="top" @submit.prevent="submit">
          <el-form-item label="审核决定">
            <el-radio-group v-model="decision" :disabled="saving || !!pending">
              <template v-if="selected.status === 'pending'"><el-radio value="approve">通过</el-radio><el-radio value="reject">驳回</el-radio></template>
              <el-radio v-else value="takedown">下架</el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="原因（最多 200 字，驳回或下架必填）"><el-input v-model="reason" type="textarea" :rows="3" :disabled="saving || !!pending" placeholder="填写可供主播理解的具体原因" /></el-form-item>
          <el-button type="primary" :loading="saving" :disabled="!reasonValid || !imageURL || !!pending" @click="submit">确认审核</el-button>
        </el-form>
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, onActivated, onBeforeUnmount, onDeactivated, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/pinia/modules/user'
import { CreateUUID } from '@/utils/format'
import { formatTimeToStr } from '@/utils/date'
import { listAnchorPhotos, previewAnchorPhoto, reviewAnchorPhoto } from '@/api/anchor-home'
import { useAlbumAdmin, photoStatuses } from './album-admin'

defineOptions({ name: 'AnchorHomeAlbum' })
const user = useUserStore()
const { query, rows, total, loading, error, visible, selected, imageURL, previewLoading, decision, reason, reasonValid, saving, pending, load, search, open, submit, retryPending, closePreview, clear } = useAlbumAdmin({
  list: listAnchorPhotos, preview: previewAnchorPhoto, review: reviewAnchorPhoto, newRequestId: CreateUUID,
  createURL: blob => URL.createObjectURL(blob), revokeURL: value => URL.revokeObjectURL(value), success: ElMessage.success
})
const time = value => value ? formatTimeToStr(value) : '—'
let active = false
const activate = () => { if (!active) { active = true; load() } }
watch([() => user.token, () => user.userInfo?.ID, () => user.userInfo?.authorityId], clear, { flush: 'sync' })
onMounted(activate)
onActivated(activate)
onDeactivated(() => { active = false; clear() })
onBeforeUnmount(clear)
</script>

<style scoped>
.album-review-image { width: 100%; max-height: 50vh; object-fit: contain; background: #f5f7fa; }
</style>
