<template>
  <el-dialog v-model="visible" title="创建结算单" width="680px" :close-on-click-modal="false" :close-on-press-escape="!writing" :show-close="!writing">
    <el-form label-width="120px" :disabled="writing || submitted" @submit.prevent>
      <el-form-item label="主播编号" required><el-input v-model.trim="draft.anchorNo" :disabled="pinnedAnchor" maxlength="40" /></el-form-item>
      <el-form-item label="兑换规则" required>
        <el-select v-model="draft.ruleNo" filterable :loading="rulesLoading" placeholder="选择已审核且当前生效的规则" class="w-full" @change="selectRule">
          <el-option v-for="rule in options" :key="rule.ruleNo" :value="rule.ruleNo" :label="ruleLabel(rule)" :disabled="!usableRule(rule)" />
          <template #footer><el-pagination size="small" :current-page="rulePage" :page-size="20" :total="ruleTotal" layout="prev, pager, next" :disabled="rulesLoading" @current-change="loadRules" /></template>
        </el-select>
      </el-form-item>
      <el-alert v-if="rulesError" type="error" :closable="false" :title="rulesError" />
      <el-button v-if="rulesError" link type="primary" @click="loadRules(rulePage)">重新加载规则</el-button>
      <el-alert v-if="selectedRule" type="info" :closable="false" class="mb-4">
        <p>规则编号：{{ selectedRule.ruleNo }}</p>
        <p>{{ preview }}</p>
        <p>规则有效期：{{ dateText(selectedRule.effectiveFrom) }} 至 {{ dateText(selectedRule.effectiveTo) }}。提交时须仍生效。</p>
      </el-alert>
      <el-form-item v-for="[key, label] in timeFields" :key="key" :label="label" required>
        <el-date-picker v-model="draft[key]" type="datetime" value-format="x" format="YYYY-MM-DD HH:mm:ss" :disabled-date="disabledDate" :disabled-hours="() => futureTimes(draft[key]).hours()" :disabled-minutes="hour => futureTimes(draft[key]).minutes(hour)" :disabled-seconds="(hour, minute) => futureTimes(draft[key]).seconds(hour, minute)" @change="clampTime(key)" />
      </el-form-item>
      <el-button link type="primary" @click="useRecentPeriod">最近 7 天（截至当前秒）</el-button>
      <el-alert class="mt-4" type="info" :closable="false" title="按收益产生时间结算，包含开始时间、不包含结束时间；只纳入未结算收益，每单最多 1000 笔。金额由服务端计算，创建后仍需复核才入账。" />
    </el-form>
    <el-alert v-if="writeError" class="mt-4" type="error" :closable="false" :title="writeError" />
    <el-alert v-if="submitted" class="mt-4" type="warning" :closable="false" title="提交结果尚未确认，已保留原请求号及参数。请按原参数重试，避免重复创建。" />
    <template #footer><el-button :disabled="writing" @click="visible=false">关闭</el-button><el-button type="primary" :loading="writing" :disabled="!submitted && (rulesLoading || !selectedRule)" @click="submit">{{ submitted ? '按原参数重试' : '创建结算单' }}</el-button></template>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { earningQuery, earningWrite } from '@/api/earning'
import { CreateUUID, formatDate } from '@/utils/format'
import { exchangePreview, sequence } from './shared'
import { currentSecond, recentSettlementPeriod, usableRule, settlementPayload, futureTimes } from './settlement-form'

const emit = defineEmits(['created'])
const visible = ref(false), writing = ref(false), submitted = ref(false), writeError = ref('')
const draft = ref({}), pinnedAnchor = ref(false), selectedRule = ref(null)
const rules = ref([]), rulesLoading = ref(false), rulesError = ref(''), rulePage = ref(1), ruleTotal = ref(0)
const requestSeq = sequence()
let confirmed = null
const timeFields = [['periodStart', '期间开始'], ['periodEnd', '期间结束']]
const options = computed(() => selectedRule.value && !rules.value.some(r => r.ruleNo === selectedRule.value.ruleNo) ? [selectedRule.value, ...rules.value] : rules.value)
const preview = computed(() => selectedRule.value ? exchangePreview(selectedRule.value.coinUnits, selectedRule.value.minorUnits, selectedRule.value.currency, '1000') : '')
const dateText = value => formatDate(new Date(Number(value)), 'yyyy-MM-dd hh:mm:ss')
const ruleLabel = rule => `${rule.name} · ${rule.coinUnits} COIN / ${rule.minorUnits} ${rule.currency}最小单位 · ${rule.ruleNo}${usableRule(rule) ? '' : '（当前不可用）'}`
function disabledDate(date) { const today = new Date(); today.setHours(23, 59, 59, 999); return date.getTime() > today.getTime() }
function clampTime(key) { if (Number(draft.value[key]) > currentSecond()) { draft.value[key] = currentSecond(); ElMessage.warning('时间已调整为当前时分秒') } }
function useRecentPeriod() { if (!writing.value && !submitted.value) Object.assign(draft.value, recentSettlementPeriod()) }
function selectRule(no) { selectedRule.value = options.value.find(r => r.ruleNo === no) || null }
async function loadRules(page = 1) {
  const id = requestSeq.next(); rulesLoading.value = true; rulesError.value = ''; rulePage.value = page
  try {
    const r = await earningQuery('exchange/list', { page, pageSize: 20, status: 'active' })
    if (!requestSeq.valid(id)) return
    if (r.code !== 0) throw new Error(r.msg || '规则加载失败')
    rules.value = r.data.list || []; ruleTotal.value = Math.min(Number(r.data.total), 200000)
    if (!rules.value.length) rulesError.value = '暂无已审核的兑换规则，请先配置并审核规则。'
  } catch (e) { if (requestSeq.valid(id)) { rules.value = []; ruleTotal.value = 0; rulesError.value = e.message || '规则加载失败，请检查规则查询权限' } }
  finally { if (requestSeq.valid(id)) rulesLoading.value = false }
}
function open(anchorNo = '') {
  if (writing.value) return
  visible.value = true
  // An uncertain request remains recoverable even when the dialog is reopened from another row.
  if (submitted.value) return
  confirmed = null; writeError.value = ''; selectedRule.value = null; rules.value = []
  pinnedAnchor.value = Boolean(anchorNo)
  draft.value = { requestId: CreateUUID(), anchorNo: String(anchorNo), ruleNo: '', ...recentSettlementPeriod() }
  return loadRules(1)
}
async function submit() {
  if (writing.value) return
  writing.value = true; writeError.value = ''
  try {
    if (!confirmed) {
      const payload = settlementPayload(draft.value, selectedRule.value)
      await ElMessageBox.confirm(`为主播 ${payload.anchorNo} 创建结算单？期间 ${dateText(payload.periodStart)} 至 ${dateText(payload.periodEnd)}，兑换规则：${selectedRule.value.name}。`, '确认结算', { type: 'warning' })
      confirmed = payload
    }
    submitted.value = true
    const r = await earningWrite('settlement/create', { ...confirmed })
    if (r.code !== 0) {
      if (r.code !== 49003) { submitted.value = false; confirmed = null }
      throw new Error(r.msg || '创建失败')
    }
    submitted.value = false; confirmed = null; visible.value = false
    ElMessage.success(`结算单已创建：${r.data?.settlementNo || ''}，请到收益结算中复核`)
    emit('created', r.data)
  } catch (e) { if (e !== 'cancel' && e !== 'close') writeError.value = e.message || '提交失败，请按原参数重试' }
  finally { writing.value = false }
}
onBeforeUnmount(() => requestSeq.invalidate())
defineExpose({ open })
</script>
