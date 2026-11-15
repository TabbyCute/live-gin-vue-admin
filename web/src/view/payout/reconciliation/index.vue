<template>
  <div class="gva-table-box">
    <h2>银行账单核对</h2>
    <el-alert title="导入只是外部账单声明，不会自动付款、修改 paid 或替代银行回单。差异须调查；真实出款仍在出款订单中完成独立复核。" type="warning" :closable="false" />
    <el-alert v-if="error" :title="error" type="error" :closable="false" class="my-4" />
    <el-form label-width="160px" :disabled="busy || !!pending" class="my-4">
      <el-form-item label="公司出款账户编号"><el-input v-model="form.channelAccountNo" placeholder="pca_...（账单完整账号须与此账户一致）" /></el-form-item>
      <el-form-item label="账期（左含右不含）"><el-date-picker v-model="period" type="datetimerange" value-format="x" start-placeholder="开始时间" end-placeholder="结束时间" /></el-form-item>
      <el-form-item label="完整性声明"><el-checkbox v-model="form.declaredComplete">已核验为该公司账户在此账期的完整出款账单</el-checkbox></el-form-item>
      <el-form-item label="标准CSV"><input ref="fileInput" type="file" accept=".csv,text/csv" @change="selectFile"><span class="ml-4">{{ csvReady ? '文件已加载，仅保留在本页内存' : '≤512KiB，≤1000行' }}</span></el-form-item>
      <el-form-item><el-button type="primary" :disabled="!csvReady" @click="importFile">导入核对声明</el-button><el-button @click="load">刷新批次</el-button></el-form-item>
    </el-form>
    <p>CSV列：bank_reference,company_account,recipient_account,currency,amount_minor,status,booked_at,direction</p>
    <p>金额使用最小单位正整数；状态为 settled / pending / failed / reversed；时间使用带时区的 RFC3339；direction 固定 debit。公司及收款完整账号只用于校验，服务端仅保存脱敏值或摘要。</p>
    <el-alert v-if="pending" title="提交结果未确认。参数和请求键已固定，可按原请求重试；离开页面会清除内存文件，请先查批次再决定是否重新导入。" type="info" :closable="false" />
    <el-button v-if="pending" :loading="busy" @click="submitPending">按原请求重试</el-button>
    <el-table :data="rows" v-loading="loading">
      <el-table-column prop="batchNo" label="批次" min-width="220" show-overflow-tooltip />
      <el-table-column prop="companyMasked" label="公司账户" />
      <el-table-column prop="rowCount" label="行数" />
      <el-table-column label="完整性"><template #default="{ row }">{{ row.declaredComplete ? '声明完整' : '部分账单' }}</template></el-table-column>
      <el-table-column label="操作" width="210"><template #default="{ row }"><el-button @click="open(row.batchNo)">查看</el-button><el-button :disabled="busy || !!pending" @click="run(row.batchNo)">运行核对</el-button></template></el-table-column>
    </el-table>
    <el-pagination v-model:current-page="page" :page-size="20" :total="total" @current-change="load" />
    <el-dialog v-model="visible" title="账单核对结果" width="85%">
      <p>{{ detail?.batch?.batchNo }} · {{ detail?.run ? `检查 ${detail.run.checkCount} 项，差异 ${detail.run.issueCount} 项` : '尚未运行核对' }}</p>
      <p>missing_in_statement 表示完整声明账期内未见记录；not_seen_partial 表示部分文件未覆盖。跨日记账、手续费、撤销及重复付款均须回银行调查，不能据此取消或补付。</p>
      <el-table :data="detail?.checks || []">
        <el-table-column prop="payoutNo" label="平台出款单" min-width="190" show-overflow-tooltip />
        <el-table-column prop="attemptNo" label="出款尝试" min-width="190" show-overflow-tooltip />
        <el-table-column label="银行流水尾号"><template #default="{ row }">{{ lineFor(row)?.referenceMasked || '未见记录' }}</template></el-table-column>
        <el-table-column prop="result" label="核对结果" width="200" />
        <el-table-column label="独立处理" width="180"><template #default="{ row }"><span v-if="reviewFor(row)">{{ reviewFor(row).decision === 'acknowledge' ? '已核对一致' : '已标记调查' }}</span><el-button v-else :disabled="busy || !!pending" @click="review(row)">{{ row.result === 'matched' ? '确认核对一致' : '标记调查' }}</el-button></template></el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>
<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, onDeactivated, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useUserStore } from '@/pinia/modules/user'
import { CreateUUID } from '@/utils/format'
import { listBankBatches, getBankBatch, bankAction } from '@/api/bank-reconciliation'
const user = useUserStore(), rows = ref([]), total = ref(0), page = ref(1), loading = ref(false), busy = ref(false), error = ref(''), visible = ref(false), detail = ref(null), pending = ref(null), period = ref([]), csvReady = ref(false), fileInput = ref(null)
const form = reactive({ channelAccountNo: '', declaredComplete: false })
let csv = '', epoch = 0, listSeq = 0, detailSeq = 0, fileSeq = 0
const lineFor = row => detail.value?.lines?.find(line => line.lineNo === row.lineNo)
const reviewFor = row => detail.value?.reviews?.find(review => review.checkNo === row.checkNo)
function clearFile() { fileSeq++; csv = ''; csvReady.value = false; if (fileInput.value) fileInput.value.value = '' }
function clear() { epoch++; listSeq++; detailSeq++; clearFile(); rows.value = []; detail.value = null; pending.value = null; visible.value = false; busy.value = false; loading.value = false; error.value = '' }
function failed(e) { error.value = `操作未完成（${Number(e?.code) || '网络异常'}），请核对权限、文件格式或查询原请求结果。` }
async function selectFile(event) { const file = event.target.files?.[0], id = epoch; clearFile(); const selection = fileSeq; if (!file) return; if (file.size > 512 * 1024) { error.value = '文件超过512KiB'; return } try { const value = await file.text(); if (id !== epoch || selection !== fileSeq) return; csv = value; csvReady.value = true } catch (e) { if (id === epoch && selection === fileSeq) failed(e) } }
async function load() { const id = ++listSeq, generation = epoch; loading.value = true; try { const r = await listBankBatches({ page: page.value, pageSize: 20 }); if (id !== listSeq || generation !== epoch) return; if (r.code !== 0) throw r; rows.value = r.data.list; total.value = r.data.total } catch (e) { if (id === listSeq && generation === epoch) failed(e) } finally { if (id === listSeq && generation === epoch) loading.value = false } }
async function open(no) { const id = ++detailSeq, generation = epoch; try { const r = await getBankBatch(no); if (id !== detailSeq || generation !== epoch) return; if (r.code !== 0) throw r; detail.value = r.data; visible.value = true } catch (e) { if (id === detailSeq && generation === epoch) failed(e) } }
function enqueue(action, data) { if (busy.value || pending.value) return; pending.value = { action, data: { ...data, requestId: CreateUUID() } }; return submitPending() }
function importFile() { if (!csvReady.value || period.value?.length !== 2) { error.value = '请选择文件与账期'; return } return enqueue('import', { ...form, periodFrom: Number(period.value[0]), periodTo: Number(period.value[1]), csv }) }
function run(batchNo) { return enqueue('run', { batchNo }) }
async function review(row) { const generation = epoch; try { const answer = await ElMessageBox.prompt('记录核对或调查原因，不要输入完整账号或联系方式。', '独立复核', { inputValidator: value => value?.trim().length >= 3 && value.length <= 500 }); if (generation !== epoch) return; return enqueue('review', { checkNo: row.checkNo, decision: row.result === 'matched' ? 'acknowledge' : 'investigate', reason: answer.value }) } catch { /* User cancelled. */ } }
async function submitPending() { if (busy.value || !pending.value) return; const generation = epoch, command = pending.value; busy.value = true; error.value = ''; try { const r = await bankAction(command.action, command.data); if (generation !== epoch) return; if (r.code !== 0) { if (r.code === 49301) pending.value = null; throw r } pending.value = null; if (command.action === 'import') clearFile(); await load(); if (command.action === 'run') await open(command.data.batchNo); if (command.action === 'review' && detail.value?.batch) await open(detail.value.batch.batchNo) } catch (e) { if (generation === epoch) failed(e) } finally { if (generation === epoch) busy.value = false } }
watch([() => user.token, () => user.userInfo?.ID, () => user.userInfo?.authorityId], clear)
onMounted(load); onBeforeUnmount(clear); onDeactivated(clear)
</script>
