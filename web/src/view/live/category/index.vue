<template>
  <div class="category-page">
    <div class="category-overview">
      <div>
        <div class="overview-title">直播分类</div>
        <div class="overview-desc">维护主播和直播间共用的内容分类；客户端只会读取已启用的分类树。</div>
      </div>
      <div class="overview-counts">
        <div><strong>{{ total }}</strong><span>筛选结果</span></div>
        <div><strong>{{ enabledCount }}</strong><span>本页启用</span></div>
        <div><strong>{{ disabledCount }}</strong><span>本页停用</span></div>
      </div>
    </div>

    <div class="gva-search-box">
      <el-form :inline="true" :model="searchInfo" label-width="76px">
        <el-form-item label="关键字">
          <el-input v-model.trim="searchInfo.keyword" clearable placeholder="分类编码或名称" @keyup.enter="onSubmit" />
        </el-form-item>
        <el-form-item label="父分类">
          <el-select v-model="searchInfo.parentId" clearable placeholder="全部层级" style="width: 210px">
            <el-option v-for="item in parentFilterOptions" :key="item.id" :label="item.label" :value="item.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="searchInfo.status" clearable placeholder="全部" style="width: 130px">
            <el-option label="启用" :value="1" />
            <el-option label="停用" :value="0" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" icon="search" @click="onSubmit">查询</el-button>
          <el-button icon="refresh" @click="onReset">重置</el-button>
        </el-form-item>
      </el-form>
    </div>

    <div class="gva-table-box">
      <div class="gva-btn-list category-toolbar">
        <div>
          <el-button type="primary" icon="plus" @click="openCreate()">新增分类</el-button>
          <el-button :loading="loading" @click="refreshData">刷新</el-button>
        </div>
        <span class="toolbar-tip">停用父分类会级联停用全部子分类；编码创建后不可修改。</span>
      </div>

      <el-table v-loading="loading" :data="tableData" row-key="id" stripe>
        <el-table-column label="ID" prop="id" width="80" />
        <el-table-column label="分类" min-width="230">
          <template #default="scope">
            <div class="category-cell">
              <el-image v-if="scope.row.icon" :src="scope.row.icon" fit="cover" class="category-icon">
                <template #error><div class="image-error">无图</div></template>
              </el-image>
              <div v-else class="category-icon image-error">无图</div>
              <div class="category-main">
                <div class="category-name">{{ scope.row.name }}</div>
                <div class="muted-text">{{ scope.row.code }}</div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="父分类" min-width="150">
          <template #default="scope">
            <span v-if="scope.row.parentId">{{ scope.row.parentName || `ID ${scope.row.parentId}` }}</span>
            <el-tag v-else size="small" effect="plain">顶级分类</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="排序" prop="sort" width="90" sortable />
        <el-table-column label="状态" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.status === 1 ? 'success' : 'info'">
              {{ scope.row.status === 1 ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="更新时间" width="180">
          <template #default="scope">{{ formatDateTime(scope.row.updatedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="300" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="openEdit(scope.row)">编辑</el-button>
            <el-button link type="primary" @click="openCreate(scope.row)">新增子类</el-button>
            <el-button
              link
              :type="scope.row.status === 1 ? 'warning' : 'success'"
              @click="changeStatus(scope.row)"
            >{{ scope.row.status === 1 ? '停用' : '启用' }}</el-button>
            <el-button link type="danger" @click="removeCategory(scope.row)">删除</el-button>
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
          @current-change="getTableData"
          @size-change="handleSizeChange"
        />
      </div>
    </div>

    <el-dialog v-model="dialogVisible" :title="editingId ? '编辑直播分类' : '新增直播分类'" width="560px" destroy-on-close>
      <el-alert
        v-if="editingId"
        class="form-alert"
        type="info"
        :closable="false"
        title="分类编码是稳定标识，创建后不可修改。状态请在列表中单独操作。"
      />
      <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
        <el-form-item label="父分类">
          <el-tree-select
            v-model="form.parentId"
            :data="formParentOptions"
            :props="treeProps"
            node-key="id"
            check-strictly
            clearable
            :render-after-expand="false"
            placeholder="不选择表示顶级分类"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="分类编码" prop="code">
          <el-input
            v-model.trim="form.code"
            :disabled="Boolean(editingId)"
            maxlength="32"
            placeholder="如 music、mobile-game"
            @blur="form.code = form.code.toLowerCase()"
          />
        </el-form-item>
        <el-form-item label="分类名称" prop="name">
          <el-input v-model.trim="form.name" maxlength="64" show-word-limit placeholder="请输入分类名称" />
        </el-form-item>
        <el-form-item label="图标地址" prop="icon">
          <el-input v-model.trim="form.icon" maxlength="500" placeholder="可填写 HTTPS 或站内图片地址" />
        </el-form-item>
        <el-form-item label="排序权重">
          <el-input-number v-model="form.sort" :controls="false" style="width: 100%" />
          <div class="form-tip">数值越大，在客户端分类树中越靠前。</div>
        </el-form-item>
        <el-form-item v-if="!editingId" label="初始状态" prop="status">
          <el-radio-group v-model="form.status">
            <el-radio-button :value="1">启用</el-radio-button>
            <el-radio-button :value="0">停用</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="submitForm">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { formatDate } from '@/utils/format'
import {
  createCategory,
  deleteCategory,
  getCategoryList,
  getCategoryTree,
  updateCategory,
  updateCategoryStatus
} from '@/api/live-category'

const defaultSearchInfo = () => ({ keyword: '', parentId: null, status: null })
const defaultForm = () => ({ id: 0, parentId: null, code: '', name: '', icon: '', sort: 0, status: 1 })

const searchInfo = reactive(defaultSearchInfo())
const form = reactive(defaultForm())
const formRef = ref()
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const tableData = ref([])
const categoryTree = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const submitLoading = ref(false)
const editingId = ref(0)

const enabledCount = computed(() => tableData.value.filter((item) => item.status === 1).length)
const disabledCount = computed(() => tableData.value.filter((item) => item.status === 0).length)

const flattenTree = (items, depth = 0) => items.flatMap((item) => [
  { id: item.id, label: `${'　'.repeat(depth)}${item.name}（${item.code}）` },
  ...flattenTree(item.children || [], depth + 1)
])
const parentFilterOptions = computed(() => [
  { id: 0, label: '仅顶级分类' },
  ...flattenTree(categoryTree.value)
])

const collectBranchIds = (items, targetId, collecting = false, result = new Set()) => {
  for (const item of items) {
    const inBranch = collecting || item.id === targetId
    if (inBranch) result.add(item.id)
    collectBranchIds(item.children || [], targetId, inBranch, result)
  }
  return result
}
const decorateTree = (items, excludedIds) => items.map((item) => ({
  ...item,
  label: `${item.name}（${item.code}）${item.status === 0 ? ' · 已停用' : ''}`,
  disabled: item.status !== 1 || excludedIds.has(item.id),
  children: decorateTree(item.children || [], excludedIds)
}))
const formParentOptions = computed(() => {
  const excludedIds = editingId.value ? collectBranchIds(categoryTree.value, editingId.value) : new Set()
  return decorateTree(categoryTree.value, excludedIds)
})
const treeProps = { label: 'label', children: 'children', disabled: 'disabled', value: 'id' }

const rules = {
  code: [
    { required: true, message: '请输入分类编码', trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9_-]{0,31}$/, message: '编码必须以小写字母开头，只能包含小写字母、数字、_ 和 -', trigger: 'blur' }
  ],
  name: [{ required: true, message: '请输入分类名称', trigger: 'blur' }]
}

const formatDateTime = (value) => value ? (formatDate(value) || '-') : '-'

const buildSearchParams = () => {
  const params = { page: page.value, pageSize: pageSize.value }
  if (searchInfo.keyword) params.keyword = searchInfo.keyword
  if (searchInfo.parentId !== null && searchInfo.parentId !== undefined) params.parentId = searchInfo.parentId
  if (searchInfo.status !== null && searchInfo.status !== undefined) params.status = searchInfo.status
  return params
}

const getTableData = async () => {
  const res = await getCategoryList(buildSearchParams())
  if (res.code === 0) {
    tableData.value = res.data.list || []
    total.value = Number(res.data.total || 0)
    page.value = Number(res.data.page || page.value)
    pageSize.value = Number(res.data.pageSize || pageSize.value)
  }
}

const getTreeData = async () => {
  const res = await getCategoryTree()
  if (res.code === 0) categoryTree.value = res.data || []
}

const refreshData = async () => {
  loading.value = true
  try {
    await Promise.all([getTableData(), getTreeData()])
  } finally {
    loading.value = false
  }
}

const onSubmit = () => {
  page.value = 1
  refreshData()
}
const onReset = () => {
  Object.assign(searchInfo, defaultSearchInfo())
  page.value = 1
  refreshData()
}
const handleSizeChange = () => {
  page.value = 1
  getTableData()
}

const resetForm = () => {
  Object.assign(form, defaultForm())
  formRef.value?.clearValidate()
}
const openCreate = (parent) => {
  editingId.value = 0
  resetForm()
  form.parentId = parent?.id || null
  form.status = parent?.status === 0 ? 0 : 1
  dialogVisible.value = true
}
const openEdit = (row) => {
  editingId.value = row.id
  Object.assign(form, {
    id: row.id,
    parentId: row.parentId || null,
    code: row.code,
    name: row.name,
    icon: row.icon || '',
    sort: Number(row.sort || 0),
    status: row.status
  })
  dialogVisible.value = true
}

const submitForm = async () => {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  submitLoading.value = true
  try {
    const payload = {
      parentId: Number(form.parentId || 0),
      name: form.name,
      icon: form.icon || '',
      sort: Number(form.sort || 0)
    }
    const res = editingId.value
      ? await updateCategory({ id: editingId.value, ...payload })
      : await createCategory({ code: form.code.toLowerCase(), status: form.status, ...payload })
    if (res.code === 0) {
      ElMessage.success(res.msg || '保存成功')
      dialogVisible.value = false
      await refreshData()
    }
  } finally {
    submitLoading.value = false
  }
}

const changeStatus = async (row) => {
  const status = row.status === 1 ? 0 : 1
  if (status === 0) {
    try {
      await ElMessageBox.confirm(
        `确认停用“${row.name}”吗？其全部子分类也会被级联停用。`,
        '停用分类',
        { type: 'warning', confirmButtonText: '确认停用', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
  }
  const res = await updateCategoryStatus({ id: row.id, status })
  if (res.code === 0) {
    ElMessage.success(status === 1 ? '分类已启用' : '分类已停用')
    await refreshData()
  }
}

const removeCategory = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确认删除“${row.name}”吗？仅已停用、无子分类且未被主播使用的分类可以删除。`,
      '删除分类',
      { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  const res = await deleteCategory({ id: row.id })
  if (res.code === 0) {
    ElMessage.success('删除成功')
    if (tableData.value.length === 1 && page.value > 1) page.value--
    await refreshData()
  }
}

onMounted(refreshData)
</script>

<style scoped lang="scss">
.category-page {
  .category-overview { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 22px 24px; color: #fff; background: linear-gradient(120deg, #064e3b 0%, #059669 58%, #34d399 100%); border-radius: 10px; }
  .overview-title { font-size: 22px; font-weight: 700; }
  .overview-desc { margin-top: 7px; font-size: 13px; color: rgb(255 255 255 / 76%); }
  .overview-counts { display: flex; gap: 10px; }
  .overview-counts > div { display: flex; flex-direction: column; min-width: 92px; padding: 10px 14px; background: rgb(255 255 255 / 12%); border: 1px solid rgb(255 255 255 / 16%); border-radius: 8px; }
  .overview-counts strong { font-size: 21px; }
  .overview-counts span { margin-top: 2px; color: rgb(255 255 255 / 72%); font-size: 12px; }
  .category-toolbar { align-items: center; justify-content: space-between; }
  .toolbar-tip, .muted-text, .form-tip { color: var(--el-text-color-secondary); font-size: 12px; }
  .category-cell { display: flex; align-items: center; gap: 10px; }
  .category-icon { width: 42px; height: 42px; overflow: hidden; background: var(--el-fill-color-light); border-radius: 8px; }
  .image-error { display: flex; align-items: center; justify-content: center; color: var(--el-text-color-placeholder); font-size: 11px; }
  .category-main { min-width: 0; }
  .category-name { overflow: hidden; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
  .form-alert { margin-bottom: 18px; }
  .form-tip { margin-top: 5px; line-height: 1.4; }
}

@media (max-width: 900px) {
  .category-page {
    .category-overview { align-items: flex-start; flex-direction: column; }
    .overview-counts { width: 100%; overflow-x: auto; }
    .overview-counts > div { flex: 1; }
    .category-toolbar { align-items: flex-start; flex-direction: column; gap: 10px; }
  }
}
</style>
