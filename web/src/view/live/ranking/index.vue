<template>
  <div class="gva-table-box">
    <el-alert title="财富榜按用户、魅力榜按主播统计礼物原始净消费 COIN；不包含充值或提现，不按主播分成计算。退款冲回原送礼日期。日、周、月按马来西亚时区，周一开始。" :closable="false" />
    <el-form inline @submit.prevent="search">
      <el-form-item label="榜单"><el-select v-model="query.rankType" @change="changeType"><el-option value="wealth" label="财富榜" /><el-option value="charm" label="魅力榜" /><el-option value="gift" label="礼物榜" /></el-select></el-form-item>
      <el-form-item label="范围"><el-select v-model="query.scopeType"><el-option v-for="scope in scopes" :key="scope.value" v-bind="scope" /></el-select></el-form-item>
      <el-form-item v-if="query.scopeType !== 'platform'" label="范围编号"><el-input v-model="query.scopeNo" placeholder="主播/房间/场次/礼物分类编号" /></el-form-item>
      <el-form-item v-if="query.rankType === 'gift'" label="礼物编号"><el-input v-model="query.giftNo" clearable placeholder="留空表示全部礼物" /></el-form-item>
      <el-form-item label="周期"><el-select v-model="query.period" @change="query.periodKey = ''"><el-option value="day" label="日榜" /><el-option value="week" label="周榜" /><el-option value="month" label="月榜" /><el-option value="all" label="总榜" /></el-select></el-form-item>
      <el-form-item v-if="query.period !== 'all'" label="日期"><el-input v-model="query.periodKey" :placeholder="query.period === 'month' ? 'YYYY-MM；留空本月' : 'YYYY-MM-DD；周榜填周一'" /></el-form-item>
      <el-button native-type="submit" :loading="loading">查询</el-button>
    </el-form>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <p>统计周期：{{ board.periodKey || '—' }}　数据核验时间：{{ board.dataAsOf ? formatDate(board.dataAsOf) : '等待首次同步' }}　待同步流水：{{ board.lagCount ?? status.lagCount ?? '—' }}</p>
    <el-table :data="board.list || []" v-loading="loading"><el-table-column prop="rank" label="名次" /><el-table-column prop="subjectNo" label="公开编号" /><el-table-column prop="user.nickname" label="昵称" /><el-table-column prop="score" label="净价值 COIN" /><el-table-column prop="quantity" label="净礼物数" /></el-table>
    <p>共 {{ board.total || '0' }} 条，最多浏览 10000 页。</p>
    <el-pagination v-model:current-page="query.page" :page-size="query.pageSize" :total="pageTotal(board.total || '0', query.pageSize)" layout="prev, pager, next" @current-change="load" />
    <el-divider />
    <p>同步状态：{{ status.checkpoint?.lastError || '正常' }}　最近任务检查：{{ status.checkpoint?.updatedAt ? formatDate(status.checkpoint.updatedAt) : '—' }}</p>
    <el-button @click="loadStatus">刷新任务</el-button><el-button v-if="auth.reconcile || status.actions?.reconcile" :disabled="working" @click="act('reconcile')">核验榜单</el-button><el-button v-if="auth.rebuild || status.actions?.rebuild" :disabled="working" @click="act('rebuild')">从历史重建</el-button><el-button v-if="attempt" :loading="working" @click="act(attempt.path)">按原请求重试</el-button>
    <el-alert v-if="result" :title="result" :closable="false" />
    <el-table :data="jobs"><el-table-column prop="jobNo" label="任务编号" /><el-table-column label="状态"><template #default="{ row }">{{ jobStatus[row.status] || row.status }}</template></el-table-column><el-table-column prop="processedCount" label="已处理流水" /><el-table-column prop="errorCount" label="错误数" /><el-table-column prop="lastError" label="错误摘要" /></el-table>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessageBox } from 'element-plus'
import { rankingQuery, rankingWrite } from '@/api/ranking'
import { CreateUUID, formatDate } from '@/utils/format'
import { useBtnAuth } from '@/utils/btnAuth'
import { requestSequence } from '../gift/shared'
import { actionAttempt, pageTotal } from '../guard/shared'
defineOptions({ name: 'LiveRanking' })
const auth = useBtnAuth(), seq = requestSequence(), statusSeq = requestSequence()
const query = reactive({ rankType: 'wealth', period: 'day', periodKey: '', scopeType: 'platform', scopeNo: '', giftNo: '', page: 1, pageSize: 20 })
const board = ref({}), status = ref({}), jobs = ref([]), error = ref(''), result = ref(''), loading = ref(false), working = ref(false), attempt = ref(null)
const jobStatus = { running: '重建中', succeeded: '已完成', failed: '失败' }
const scopes = computed(() => (query.rankType === 'gift' ? [['anchor','主播'],['room','房间'],['session','场次']] : query.rankType === 'charm' ? [['platform','全平台'],['category','礼物分类']] : [['platform','全平台']]).map(([value,label]) => ({ value,label })))
function changeType() { query.scopeType = query.rankType === 'gift' ? 'anchor' : 'platform'; query.scopeNo = ''; query.giftNo = ''; query.page = 1 }
async function load() { const id = seq.next(); loading.value = true; try { const data = { ...query, scopeNo: query.scopeType === 'platform' ? '' : query.scopeNo }; const r = await rankingQuery('board', data); if (!seq.valid(id)) return; if(r.code !== 0) throw new Error(r.msg); board.value = r.data; error.value = '' } catch(e) { if(seq.valid(id)) error.value = e.message || '查询失败' } finally { if(seq.valid(id)) loading.value = false } }
function search() { query.page = 1; load() }
async function loadStatus() { const id = statusSeq.next(); try { const [s,j] = await Promise.all([rankingQuery('status'), rankingQuery('job/list',{ page: 1, pageSize: 20 })]); if(!statusSeq.valid(id)) return; if(s.code !== 0 || j.code !== 0) throw new Error(s.code ? s.msg : j.msg); status.value = s.data; jobs.value = j.data.list } catch(e) { if(statusSeq.valid(id)) error.value = e.message } }
async function act(path) { if(working.value) return; working.value = true; try { if(!attempt.value) { const frozen = actionAttempt(CreateUUID(), { scope: 'all' }); if(path === 'rebuild') await ElMessageBox.confirm('重建期间保留旧榜展示，完成后整体切换。确认继续？','重建榜单'); attempt.value = { path, data: frozen } } const r = await rankingWrite(attempt.value.path, attempt.value.data); if(r.code !== 0) { if([49001,49002,49003].includes(r.code)) attempt.value = null; throw new Error(r.msg) } result.value = r.data.consistent === false ? '榜单与来源存在差异，请核查后重建。' : r.data.jobNo ? `重建任务已创建：${r.data.jobNo}` : '核验通过'; attempt.value = null; await loadStatus() } catch(e) { if(e instanceof Error) error.value = e.message } finally { working.value = false } }
onMounted(() => { load(); loadStatus() }); onBeforeUnmount(() => { seq.invalidate(); statusSeq.invalidate() })
</script>
