<template>
  <div class="gva-table-box">
    <el-alert title="权益按来源保存；退款仅撤回未使用时长。已使用历史不会被改写。核验发现差异后可从原始来源重建，不能手工修改到期时间。" :closable="false" />
    <el-form inline @submit.prevent="search">
      <template v-if="kind === 'member'"><el-form-item label="主播编号"><el-input v-model="query.anchorNo" /></el-form-item><el-form-item label="用户编号"><el-input v-model="query.accountNo" /></el-form-item></template>
      <el-form-item v-if="kind !== 'task'" label="会员编号"><el-input v-model="query.membershipNo" /></el-form-item>
      <el-form-item v-if="kind === 'ledger'" label="记录类型"><el-select v-model="source" @change="search"><el-option value="ledger" label="权益流水" /><el-option value="grant" label="发放来源" /><el-option value="compensation" label="退款补偿" /></el-select></el-form-item>
      <el-button native-type="submit" :loading="loading">查询</el-button>
    </el-form>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-table :data="rows" v-loading="loading">
      <el-table-column v-for="[key,label] in columns" :key="key" :prop="key" :label="label" min-width="150"><template #default="{ row }">{{ display(row, key) }}</template></el-table-column>
      <el-table-column label="操作" min-width="220"><template #default="{ row }">
        <el-button v-if="kind === 'member'" link @click="showDetail(row)">详情</el-button>
        <el-button v-if="kind === 'member' && (auth.reconcile || actions.reconcile)" link :disabled="working" @click="act('reconcile', row.membershipNo)">核验</el-button>
        <el-button v-if="kind === 'member' && (auth.rebuild || actions.rebuild)" link :disabled="working" @click="act('rebuild', row.membershipNo)">重建</el-button>
        <el-button v-if="kind === 'task' && row.status === 'failed' && (auth.retry || actions.retry)" link :disabled="working" @click="act('task/retry', row.taskNo)">重试任务</el-button>
      </template></el-table-column>
    </el-table>
    <p>共 {{ total }} 条，最多浏览 10000 页。</p>
    <el-pagination v-model:current-page="query.page" :page-size="query.pageSize" :total="pageTotal(total, query.pageSize)" layout="prev, pager, next" @current-change="load" />
    <el-alert v-if="result" :title="result" :closable="false" />
    <el-button v-if="attempt" :loading="working" @click="act(attempt.path, attempt.data.number)">按原请求重试</el-button>
    <el-dialog v-model="detailVisible" title="守护会员详情" width="800px"><el-descriptions :column="2" border><el-descriptions-item v-for="[key,label] in memberColumns" :key="key" :label="label">{{ display(detail, key) }}</el-descriptions-item></el-descriptions><el-alert title="复制会员编号，可在“守护权益流水”按会员编号查询完整发放、补偿与到期记录。" :closable="false" /></el-dialog>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessageBox } from 'element-plus'
import { guardQuery, guardWrite } from '@/api/guard'
import { CreateUUID, formatDate } from '@/utils/format'
import { useBtnAuth } from '@/utils/btnAuth'
import { requestSequence } from '../gift/shared'
import { labels, actionAttempt, pageTotal } from './shared'
const props = defineProps({ kind: { type: String, required: true } }), kind = props.kind, auth = useBtnAuth(), seq = requestSequence()
const actions = ref({})
const source = ref('ledger'), query = reactive({ page: 1, pageSize: 20, membershipNo: '', anchorNo: '', accountNo: '' })
const rows = ref([]), total = ref('0'), loading = ref(false), error = ref(''), working = ref(false), attempt = ref(null), result = ref(''), detail = ref({}), detailVisible = ref(false)
const memberColumns = [['membershipNo','会员编号'],['user.accountNo','用户编号'],['user.nickname','用户昵称'],['anchorNo','主播编号'],['activePlanNo','当前方案'],['status','状态'],['expiresAt','当前方案到期'],['remainingDays','剩余天数（不足1天按1天）']]
const sets = { member: memberColumns, task: [['taskNo','任务编号'],['kind','任务类型'],['sourceNo','来源'],['status','状态'],['retryCount','尝试次数'],['lastError','失败原因'],['nextRetryAt','下次重试']], ledger: [['ledgerNo','流水编号'],['membershipNo','会员编号'],['entryType','类型'],['sourceNo','来源'],['planNo','方案'],['durationDelta','时长变更（毫秒）'],['createdAt','发生时间']], grant: [['grantNo','发放编号'],['membershipNo','会员编号'],['sourceNo','来源订单'],['planNo','方案'],['durationDays','原始天数'],['compensatedDuration','撤回毫秒'],['status','状态'],['expiresAt','到期时间']], compensation: [['compensationNo','补偿编号'],['grantNo','发放编号'],['sourceNo','退款单'],['duration','申请补偿毫秒'],['status','处理状态'],['completedAt','完成时间']] }
const columns = computed(() => sets[kind === 'ledger' ? source.value : kind])
function display(row,key) { const value = key.split('.').reduce((o,k) => o?.[k],row); return key.endsWith('At') && value ? formatDate(value) : labels[value] || value || '—' }
async function load() { const id = seq.next(); loading.value = true; try { const r = await guardQuery((kind === 'ledger' ? source.value : kind) + '/list', { ...query }); if (!seq.valid(id)) return; if (r.code !== 0) throw new Error(r.msg); rows.value = r.data.list; actions.value = r.data.actions || {}; total.value = r.data.total; error.value = '' } catch(e) { if (seq.valid(id)) error.value = e.message || '查询失败' } finally { if (seq.valid(id)) loading.value = false } }
function search() { query.page = 1; load() }
async function showDetail(row) { try { const r = await guardQuery('member/detail', { membershipNo: row.membershipNo, page: 1, pageSize: 1 }); if(r.code !== 0) throw new Error(r.msg); detail.value = r.data.list[0] || {}; detailVisible.value = true } catch(e) { error.value = e.message } }
async function act(path,number) { if(working.value) return; working.value = true; try { if (!attempt.value) { const frozen = actionAttempt(CreateUUID(), { number }); if(path !== 'reconcile') await ElMessageBox.confirm('确认按原始记录执行此操作？', '确认'); attempt.value = { path, data: frozen } } const r = await guardWrite(attempt.value.path, attempt.value.data); if(r.code !== 0) { if([49001,49002,49003].includes(r.code)) attempt.value = null; throw new Error(r.msg) } result.value = r.data.consistent === false ? '核验存在差异，请检查来源流水并重建。' : '操作完成'; attempt.value = null; await load() } catch(e) { if(e instanceof Error) error.value = e.message } finally { working.value = false } }
onMounted(load); onBeforeUnmount(() => seq.invalidate())
</script>
