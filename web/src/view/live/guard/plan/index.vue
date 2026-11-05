<template>
  <div class="gva-table-box">
    <el-alert title="守护方案定义等级与权益，不设置价格。用户赠送已配置的礼物后获得守护；每件天数 × 礼物件数 = 本次发放时长。修改方案不会改变已发放权益。" :closable="false" />
    <el-button :loading="loading" @click="load">刷新</el-button><el-button v-if="auth.save || actions.save" type="primary" @click="open()">新增方案</el-button>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <el-table :data="rows" v-loading="loading">
      <el-table-column prop="planNo" label="方案编号" /><el-table-column prop="name" label="名称" /><el-table-column prop="level" label="等级" /><el-table-column prop="priority" label="展示优先级" /><el-table-column prop="defaultDurationDays" label="建议天数" /><el-table-column prop="sortOrder" label="排序" />
      <el-table-column label="状态"><template #default="{ row }">{{ labels[row.status] }}</template></el-table-column>
      <el-table-column label="操作"><template #default="{ row }"><el-button v-if="auth.save || actions.save" link @click="open(row)">编辑 / 启停 / 排序</el-button></template></el-table-column>
    </el-table>
    <p>共 {{ total }} 条，最多浏览 10000 页。</p>
    <el-pagination v-model:current-page="query.page" :page-size="query.pageSize" :total="pageTotal(total, query.pageSize)" layout="prev, pager, next" @current-change="load" />
    <el-dialog v-model="dialog" title="守护方案" width="620px" :close-on-click-modal="false">
      <el-form label-width="130px" :disabled="saving || !!attempt">
        <el-form-item label="方案编号"><el-input v-model="form.planNo" :disabled="!!form.version" maxlength="40" /></el-form-item>
        <el-form-item label="名称"><el-input v-model="form.name" maxlength="64" /></el-form-item>
        <el-form-item label="等级"><el-input-number v-model="form.level" :min="1" :max="1000" /></el-form-item>
        <el-form-item label="展示优先级"><el-input-number v-model="form.priority" :min="0" :max="100000" /></el-form-item>
        <el-form-item label="建议天数"><el-input v-model="form.defaultDurationDays" /></el-form-item>
        <el-form-item label="权益配置 JSON"><el-input v-model="form.benefitConfig" type="textarea" :rows="4" maxlength="4096" /></el-form-item>
        <el-form-item label="状态"><el-select v-model="form.status"><el-option value="disabled" label="停用" /><el-option value="enabled" label="启用" /></el-select></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="form.sortOrder" :min="0" :max="100000" /></el-form-item>
      </el-form>
      <el-alert v-if="writeError" :title="writeError" type="error" :closable="false" />
      <el-button type="primary" :loading="saving" @click="save">{{ attempt ? '按原请求重试' : '保存' }}</el-button>
    </el-dialog>
  </div>
</template>
<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { guardQuery, guardWrite } from '@/api/guard'
import { CreateUUID } from '@/utils/format'
import { useBtnAuth } from '@/utils/btnAuth'
import { requestSequence } from '../../gift/shared'
import { labels, emptyPlan, planForm, actionAttempt, pageTotal } from '../shared'
defineOptions({ name: 'GuardPlan' })
const auth = useBtnAuth(), seq = requestSequence(), actions = ref({})
const query = reactive({ page: 1, pageSize: 20 })
const rows = ref([]), total = ref('0'), loading = ref(false), error = ref(''), dialog = ref(false), saving = ref(false), writeError = ref(''), form = ref(emptyPlan()), attempt = ref(null)
async function load() { const id = seq.next(); loading.value = true; try { const r = await guardQuery('plan/list', { ...query }); if (!seq.valid(id)) return; if (r.code !== 0) throw new Error(r.msg); rows.value = r.data.list; actions.value = r.data.actions || {}; total.value = r.data.total; error.value = '' } catch (e) { if (seq.valid(id)) error.value = e.message || '查询失败' } finally { if (seq.valid(id)) loading.value = false } }
function open(row) { if (attempt.value) { dialog.value = true; return } form.value = planForm(row); writeError.value = ''; dialog.value = true }
async function save() { if (saving.value) return; saving.value = true; try { if (!attempt.value) { JSON.parse(form.value.benefitConfig); attempt.value = actionAttempt(CreateUUID(), { ...form.value }) } const r = await guardWrite('plan/save', attempt.value); if (r.code !== 0) { if ([49001,49002,49003].includes(r.code)) attempt.value = null; throw new Error(r.msg) } attempt.value = null; dialog.value = false; await load() } catch (e) { writeError.value = e.message || '结果未确认，请按原请求重试' } finally { saving.value = false } }
onMounted(load); onBeforeUnmount(() => seq.invalidate())
</script>
