<template>
  <div class="gva-table-box">
    <el-alert title="核验报告仅反映开始时的账务快照；提现与出款仍会重新核验。每次后台执行最多30秒，失败重试从头核验。" type="info" :closable="false" />
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-button v-if="pending" :loading="busy" @click="submitPending">按原请求重试排队</el-button>
    <el-form inline class="audit-form" @submit.prevent="start()">
      <el-form-item label="主播编号"><el-input v-model.trim="anchorNo" placeholder="主播编号" /></el-form-item>
      <el-form-item label="币种"><el-input v-model.trim="currency" maxlength="8" /></el-form-item>
      <el-form-item><el-button type="primary" :loading="busy" :disabled="!!pending" @click="start()">启动深度核验</el-button></el-form-item>
      <el-form-item><el-button :loading="loading" @click="refresh">刷新</el-button></el-form-item>
    </el-form>
    <el-table v-loading="loading" :data="rows" row-key="jobNo">
      <el-table-column prop="jobNo" label="作业号" min-width="220" />
      <el-table-column prop="anchorNo" label="主播" min-width="120" />
      <el-table-column prop="currency" label="币种" width="80" />
      <el-table-column label="状态" width="110"><template #default="{row}">{{ states[row.status] || row.status }}</template></el-table-column>
      <el-table-column label="进度" min-width="200"><template #default="{row}">流水 {{ snapshot(row.progress).ledgerVersion || '0' }} / {{ snapshot(row.accountSnapshot).version || '—' }}；收益 {{ snapshot(row.progress).incomeFacts || '0' }} 笔</template></el-table-column>
      <el-table-column label="结果" min-width="160"><template #default="{row}">{{ issues[row.issueCode] || row.issueCode || '—' }}</template></el-table-column>
      <el-table-column label="创建时间" min-width="170"><template #default="{row}">{{ formatDate(row.createdAt) }}</template></el-table-column>
      <el-table-column label="操作" width="190"><template #default="{row}">
        <el-button link @click="selected = row">快照</el-button>
        <el-button v-if="['queued','running'].includes(row.status)" link type="danger" :disabled="busy" @click="cancel(row)">取消</el-button>
        <el-button v-if="['failed','cancelled'].includes(row.status)" link type="primary" :disabled="busy" @click="start(row)">重试</el-button>
      </template></el-table-column>
    </el-table>
    <el-pagination v-model:current-page="page" :page-size="20" :total="total" layout="prev, pager, next, total" @current-change="refresh" />
    <el-dialog :model-value="!!selected" title="核验历史快照" width="680px" @close="selected = null">
      <template v-if="selected">
        <p>作业：{{ selected.jobNo }}；校验摘要：{{ selected.snapshotHash || '尚未开始' }}</p>
        <pre>{{ JSON.stringify(snapshot(selected.accountSnapshot), null, 2) }}</pre>
        <pre>{{ JSON.stringify(snapshot(selected.progress), null, 2) }}</pre>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, onDeactivated, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/pinia/modules/user'
import { earningQuery, earningWrite } from '@/api/earning'
import { CreateUUID, formatDate } from '@/utils/format'
import { snapshot } from '../shared'

defineOptions({ name: 'EarningAudit' })
const anchorNo = ref(''), currency = ref('CNY'), rows = ref([]), page = ref(1), total = ref(0)
const busy = ref(false), loading = ref(false), selected = ref(null)
const pending = ref(null), error = ref(''), user = useUserStore()
let epoch = 0, listSeq = 0
function clear() { epoch++; listSeq++; rows.value = []; selected.value = null; pending.value = null; busy.value = false; loading.value = false; error.value = '' }
function failed() { error.value = '请求未完成，请刷新核实；排队结果未知时请使用原请求重试。' }
const states = { queued: '排队中', running: '核验中', passed: '快照通过', failed: '失败', cancelled: '已取消' }
const issues = { audit_timeout: '执行超时', invariant_violation: '账务不一致', untrusted_source: '来源不可信', proof_batch_limit: '单批证明超限', worker_interrupted: '执行中断', audit_unavailable: '核验暂不可用', cancelled: '已取消' }
async function refresh() {
  const generation = epoch, request = ++listSeq
  loading.value = true
  try {
    const r = await earningQuery('audit/list', { page: page.value, pageSize: 20, anchorNo: anchorNo.value })
    if (generation !== epoch || request !== listSeq) return
    if (r.code !== 0) throw r
    rows.value = r.data.list; total.value = r.data.total
  } catch { if (generation === epoch && request === listSeq) { rows.value = []; failed() } }
  finally { if (generation === epoch && request === listSeq) loading.value = false }
}
async function start(old) {
  if (busy.value || pending.value) return
  const anchor = old?.anchorNo || anchorNo.value, unit = old?.currency || currency.value
  if (!/^[A-Za-z0-9_-]{1,40}$/.test(anchor) || !/^[A-Z]{3,8}$/.test(unit)) return ElMessage.warning('请填写有效主播编号与币种')
  pending.value = { requestId: CreateUUID(), anchorNo: anchor, currency: unit, retryOf: old?.jobNo || '' }
  await submitPending()
}
async function submitPending() {
  if (busy.value || !pending.value) return
  const generation = epoch
  busy.value = true; error.value = ''
  try {
    const r = await earningWrite('audit/start', pending.value)
    if (generation !== epoch) return
    if (r.code !== 0) { pending.value = null; throw r }
    pending.value = null; ElMessage.success('已加入核验队列'); await refresh()
  } catch { if (generation === epoch) failed() }
  finally { if (generation === epoch) busy.value = false }
}
async function cancel(row) {
  if (busy.value) return
  const generation = epoch
  busy.value = true
  try { const r = await earningWrite('audit/cancel', { jobNo: row.jobNo }); if (generation !== epoch) return; if (r.code !== 0) throw r; await refresh() }
  catch { if (generation === epoch) failed() }
  finally { if (generation === epoch) busy.value = false }
}
watch([() => user.token, () => user.userInfo?.ID, () => user.userInfo?.authorityId], clear)
onBeforeUnmount(clear); onDeactivated(clear)
onMounted(refresh)
</script>

<style scoped>
.audit-form { margin-top: 18px; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
