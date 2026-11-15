<template>
  <div>
    <el-alert title="仅 root 可操作。通道未配置时保持阻塞；送达仅表示下游已持久确认，不代表用户已阅读。" type="info" :closable="false" class="mb-4" />
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <div class="gva-search-box">
      <el-form inline @submit.prevent="search">
        <el-form-item label="状态"><el-select v-model="query.status" clearable class="w-40"><el-option v-for="(label, value) in statuses" :key="value" :label="label" :value="value" /></el-select></el-form-item>
        <el-form-item label="事件ID"><el-input v-model="query.eventId" clearable /></el-form-item>
        <el-form-item><el-button type="primary" :loading="loading" @click="search">查询</el-button></el-form-item>
      </el-form>
    </div>
    <div class="gva-table-box">
      <el-table v-loading="loading" :data="rows" row-key="id">
        <el-table-column label="事件ID" prop="eventId" min-width="220" show-overflow-tooltip />
        <el-table-column label="消费者" prop="consumer" width="130" />
        <el-table-column label="状态" width="130"><template #default="{ row }">{{ statuses[row.status] || row.status }}</template></el-table-column>
        <el-table-column label="累计尝试" prop="attempts" width="100" />
        <el-table-column label="最后结果" prop="lastCode" min-width="180" />
        <el-table-column label="更新时间" min-width="180"><template #default="{ row }">{{ time(row.updatedAt) }}</template></el-table-column>
        <el-table-column label="操作" width="100"><template #default="{ row }"><el-button type="primary" link @click="openDetail(row.id)">详情</el-button></template></el-table-column>
      </el-table>
      <el-pagination v-model:current-page="query.page" v-model:page-size="query.pageSize" :total="total" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next" @current-change="load" @size-change="search" />
    </div>
    <el-drawer v-model="visible" title="事件投递详情" size="640px">
      <div v-if="detail" v-loading="detailLoading">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="事件">{{ detail.event.eventType }}</el-descriptions-item>
          <el-descriptions-item label="原始来源">{{ detail.event.source }} / {{ detail.event.sourceId }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ statuses[detail.delivery.status] }}</el-descriptions-item>
          <el-descriptions-item label="结果代码">{{ detail.delivery.lastCode || '—' }}</el-descriptions-item>
          <el-descriptions-item label="下一次尝试">{{ time(detail.delivery.nextAttemptAt) }}</el-descriptions-item>
          <el-descriptions-item label="安全通知内容"><pre class="whitespace-pre-wrap break-all">{{ JSON.stringify(detail.payload, null, 2) }}</pre></el-descriptions-item>
        </el-descriptions>
        <el-alert v-if="detail.delivery.status === 'blocked'" class="my-4" type="warning" :closable="false" title="真实通道接入后才可重试；当前阻塞不会改写原业务或资金事实。" />
        <el-form v-if="['blocked', 'dead'].includes(detail.delivery.status)" class="my-4" label-position="top">
          <el-form-item label="重试原因（8—200字节，不填写账号或凭证）"><el-input v-model="reason" type="textarea" maxlength="200" /></el-form-item>
          <el-button type="primary" :loading="retrying" :disabled="reason.trim().length < 8" @click="retry">提交受控重试</el-button>
        </el-form>
        <h3>最近投递尝试</h3>
        <el-table :data="detail.attempts"><el-table-column label="次数" prop="number" width="70" /><el-table-column label="结果" prop="outcome" /><el-table-column label="开始时间"><template #default="{ row }">{{ time(row.startedAt) }}</template></el-table-column></el-table>
        <h3>管理员重试记录</h3>
        <el-table :data="detail.retries"><el-table-column label="代次" prop="generation" width="70" /><el-table-column label="原因" prop="reason" /><el-table-column label="操作人" prop="actorId" width="85" /></el-table>
      </div>
    </el-drawer>
  </div>
</template>
<script setup>
import { ref, reactive, onMounted, onDeactivated, onBeforeUnmount, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/pinia/modules/user'
import { listEventDeliveries, getEventDelivery, retryEventDelivery } from '@/api/event-delivery'
import { CreateUUID } from '@/utils/format'
import { formatTimeToStr } from '@/utils/date'
defineOptions({ name: 'EventDelivery' })
const statuses = { pending: '等待投递', leased: '投递中', blocked: '通道未配置', delivered: '下游已确认', dead: '停止重试' }
const query = reactive({ page: 1, pageSize: 20, status: '', eventId: '' })
const rows = ref([]), total = ref(0), loading = ref(false), visible = ref(false), detail = ref(null), detailLoading = ref(false), retrying = ref(false), reason = ref('')
const user = useUserStore(), error = ref('')
let retryCommand = null, epoch = 0, listSeq = 0, detailSeq = 0
function clear() { epoch++; listSeq++; detailSeq++; retryCommand = null; rows.value = []; detail.value = null; visible.value = false; loading.value = false; detailLoading.value = false; retrying.value = false; reason.value = ''; error.value = '' }
function failed() { error.value = '操作未完成，请核实权限和原请求结果；结果未知时请保留原参数重试。' }
const time = (value) => value ? formatTimeToStr(value) : '—'
const load = async () => { const generation = epoch, request = ++listSeq; loading.value = true; try { const res = await listEventDeliveries({ ...query }); if (generation !== epoch || request !== listSeq) return; if (res.code !== 0) throw res; rows.value = res.data.list; total.value = res.data.total } catch { if (generation === epoch && request === listSeq) { rows.value = []; failed() } } finally { if (generation === epoch && request === listSeq) loading.value = false } }
const search = () => { query.page = 1; return load() }
const openDetail = async (id) => { const generation = epoch, request = ++detailSeq; detailLoading.value = true; try { const res = await getEventDelivery(id); if (generation !== epoch || request !== detailSeq) return; if (res.code !== 0) throw res; detail.value = res.data; reason.value = ''; retryCommand = null; visible.value = true } catch { if (generation === epoch && request === detailSeq) { detail.value = null; failed() } } finally { if (generation === epoch && request === detailSeq) detailLoading.value = false } }
const retry = async () => {
  if (retrying.value || !detail.value) return
  const record = detail.value.delivery
  const input = { deliveryId: record.id, generation: record.retryGeneration, reason: reason.value.trim() }
  if (!retryCommand || retryCommand.reason !== input.reason || retryCommand.deliveryId !== input.deliveryId || retryCommand.generation !== input.generation) retryCommand = { ...input, requestId: CreateUUID() }
  retrying.value = true
  const generation = epoch
  try { const res = await retryEventDelivery(retryCommand); if (generation !== epoch) return; if (res.code !== 0) throw res; ElMessage.success('已登记重试，原事件ID保持不变'); await openDetail(record.id); await load() } catch { if (generation === epoch) failed() } finally { if (generation === epoch) retrying.value = false }
}
watch([() => user.token, () => user.userInfo?.ID, () => user.userInfo?.authorityId], clear)
onDeactivated(clear); onBeforeUnmount(clear)
onMounted(load)
</script>
