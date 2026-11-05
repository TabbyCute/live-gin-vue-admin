<template>
  <div>
    <div class="gva-search-box">
      <el-form inline @submit.prevent="search">
        <el-form-item label="名称"><el-input v-model="query.name" clearable /></el-form-item>
        <el-form-item v-if="kind === 'item'" label="分类编号"><el-input v-model="query.categoryNo" clearable /></el-form-item>
        <el-form-item label="状态"><el-select v-model="query.status" clearable style="width:130px"><el-option v-for="s in statuses" :key="s" :value="s" :label="labels[s]" /></el-select></el-form-item>
        <template v-if="kind === 'item'"><el-form-item label="价格范围"><el-input v-model="query.minAmount" placeholder="最低 COIN" /><el-input v-model="query.maxAmount" placeholder="最高 COIN" /></el-form-item></template>
        <el-button native-type="submit" :loading="loading">查询</el-button><el-button type="primary" @click="open()">新增</el-button>
        <el-button @click="openSort">批量排序</el-button>
      </el-form>
    </div>
    <div class="gva-table-box">
      <el-alert v-if="error" :title="error" type="error" :closable="false" />
      <el-table v-loading="loading" :data="rows" @sort-change="sortChanged">
        <el-table-column :prop="kind === 'item' ? 'giftNo' : 'categoryNo'" label="编号" min-width="150" />
        <el-table-column prop="name" label="名称" /><el-table-column prop="status" label="状态"><template #default="{ row }">{{ labels[row.status] }}</template></el-table-column>
        <el-table-column v-if="kind === 'item'" prop="categoryNo" label="分类" />
        <el-table-column v-if="kind === 'item'" prop="coinPrice" label="单价 COIN" sortable="custom" />
        <el-table-column prop="sortOrder" label="排序" />
        <el-table-column label="操作" width="180"><template #default="{row}"><el-button link type="primary" @click="open(row)">编辑 / 启停</el-button><el-button link type="danger" @click="remove(row)">删除</el-button></template></el-table-column>
      </el-table>
      <el-pagination v-model:current-page="query.page" v-model:page-size="query.pageSize" :total="total" layout="total, prev, pager, next, sizes" @change="load" />
    </div>
    <el-dialog v-model="dialog" :title="form.version ? '编辑配置' : '新增配置'" width="680px" :close-on-click-modal="false">
      <el-form label-width="155px" @submit.prevent="save">
        <el-form-item v-for="[key,label] in fields[kind].filter(([key]) => !['guardPlanNo','guardDurationDays'].includes(key))" :key="key" :label="label"><el-input v-model="form[key]" :disabled="!!form.version && key === (kind === 'item' ? 'giftNo' : 'categoryNo')" :maxlength="key === 'name' ? 64 : 500" /></el-form-item>
        <el-form-item label="状态"><el-select v-model="form.status"><el-option v-for="s in statuses" :key="s" :value="s" :label="labels[s]" /></el-select></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="form.sortOrder" :min="0" :max="2147483647" /></el-form-item>
        <template v-if="kind === 'item'">
          <el-form-item label="动画类型"><el-select v-model="form.animationType"><el-option v-for="s in ['none','svga','mp4','lottie']" :key="s" :value="s" /></el-select></el-form-item>
          <el-form-item label="贵重礼物"><el-switch v-model="form.isPremium" /></el-form-item>
          <el-form-item label="附带权益"><el-select v-model="form.benefitType"><el-option label="无" value="none" /><el-option label="主播守护" value="anchor_guard" /></el-select></el-form-item>
          <template v-if="form.benefitType === 'anchor_guard'">
            <el-alert :title="guardEnabled ? '每件天数 × 件数 = 本次守护时长；同等级顺延，退款仅撤回未使用部分。' : '守护售卖尚未开启，可保存关闭的配置。启用需完成服务发布与就绪核验。'" :type="guardEnabled ? 'info' : 'warning'" :closable="false" />
            <el-form-item label="守护方案"><el-select v-model="form.guardPlanNo" filterable><el-option v-for="plan in guardPlans" :key="plan.planNo" :value="plan.planNo" :label="`${plan.name}（${plan.planNo}）`" /></el-select><el-button link @click="loadGuardPlans">刷新方案</el-button></el-form-item>
            <el-form-item label="每件守护天数"><el-input v-model="form.guardDurationDays" placeholder="1 至 3650 的整数" /></el-form-item>
            <el-form-item label="启用守护附赠"><el-switch v-model="form.benefitStatus" active-value="enabled" inactive-value="disabled" :disabled="!guardEnabled && form.benefitStatus !== 'enabled'" /></el-form-item>
            <el-form-item label="权益有效期"><el-date-picker v-model="benefitRange" type="datetimerange" value-format="x" /></el-form-item>
          </template>
        </template>
        <el-alert v-if="writeError" :title="writeError" type="error" :closable="false" />
        <el-button type="primary" native-type="submit" :loading="saving">保存</el-button>
      </el-form>
    </el-dialog>
    <el-dialog v-model="sortDialog" title="完整集合批量排序" width="560px">
      <el-alert title="按编号排列，每行一个。必须包含当前分类下全部礼物（或全部分类），不可重复或遗漏。" :closable="false" />
      <el-input v-model="sortText" type="textarea" :rows="12" />
      <el-button :loading="saving" @click="saveSort">提交排序</el-button>
    </el-dialog>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { giftQuery, giftWrite } from '@/api/gift'
import { guardQuery } from '@/api/guard'
import { fields, emptyForm, editForm, positiveCoin, requestSequence } from './shared'
const props = defineProps({ kind: { type: String, required: true } })
const kind = props.kind
const labels = { draft: '草稿', enabled: '启用', disabled: '停用' }
const statuses = computed(() => kind === 'item' ? ['draft','enabled','disabled'] : ['enabled','disabled'])
const query = reactive({ page: 1, pageSize: 20, name: '', status: '', categoryNo: '', minAmount: '', maxAmount: '' })
const rows = ref([]), total = ref(0), loading = ref(false), error = ref(''), saving = ref(false), dialog = ref(false), form = ref(emptyForm(kind)), writeError = ref(''), benefitRange = ref([])
const sortDialog = ref(false), sortText = ref(''), sortVersion = ref(''), sortCategory = ref('')
const seq = requestSequence()
const guardPlans = ref([]), guardEnabled = ref(false), guardSeq = requestSequence()
watch(() => form.value.benefitType, value => { if(value === 'anchor_guard') loadGuardPlans() })
async function loadGuardPlans() { if(kind !== 'item') return; const request = guardSeq.next(); try { const list=[]; let enabled=false; for(let page=1;page<=20;page++) { const r=await guardQuery('plan/list',{page,pageSize:100,status:'enabled'}); if(r.code!==0) throw new Error(r.msg); enabled=!!r.data.guardEnabled; list.push(...r.data.list); if(list.length>=Number(r.data.total)) break; if(page===20) throw new Error('方案超过选择上限，请联系管理员'); } if (!guardSeq.valid(request)) return; guardPlans.value=list;guardEnabled.value=enabled } catch(e) { if (!guardSeq.valid(request)) return; guardEnabled.value=false;writeError.value=e.message || '守护方案加载失败' } }
async function load() { const id = seq.next(); loading.value = true; error.value = ''; try { const r = await giftQuery(kind + '/list', { ...query }); if (!seq.valid(id)) return; if (r.code !== 0) throw new Error(r.msg); rows.value = r.data.list; total.value = Number(r.data.total) } catch(e) { if(seq.valid(id)) error.value = e.message || '查询失败' } finally { if(seq.valid(id)) loading.value = false } }
function search() { query.page = 1; load() }
function sortChanged({prop,order}) { query.sort = order ? ({coinPrice:'price'})[prop] : ''; query.desc = order === 'descending'; search() }
function open(row) { form.value = row ? editForm(kind,row) : emptyForm(kind); benefitRange.value = row?.effectiveTo ? [String(row.effectiveFrom),String(row.effectiveTo)] : []; writeError.value = ''; dialog.value = true }
async function save() { if(saving.value) return; saving.value = true; writeError.value = ''; try { const data = { ...form.value }; if(kind === 'item') { if(!positiveCoin(data.coinPrice) || !positiveCoin(data.maxQuantityPerOrder)) throw new Error('金额和件数须为正整数字符串'); if(data.benefitType === 'none') { data.guardPlanNo = ''; data.guardDurationDays = ''; data.benefitStatus = 'disabled' } data.effectiveFrom = Number(benefitRange.value?.[0] || 0); data.effectiveTo = Number(benefitRange.value?.[1] || 0) } const r = await giftWrite(kind + '/save',data); if(r.code !== 0) throw new Error(r.msg); dialog.value = false; await load() } catch(e) { writeError.value = e.message || '保存失败' } finally { saving.value = false } }
async function remove(row) { try { await ElMessageBox.confirm('已产生订单或被引用的配置不能删除。确认删除此配置？','确认删除'); const r = await giftWrite('delete',{ kind, no:row.giftNo || row.categoryNo, version:row.version }); if(r.code !== 0) throw new Error(r.msg); await load() } catch(e) { if(e instanceof Error) ElMessage.error(e.message) } }
async function openSort() { if(kind === 'item' && !query.categoryNo) { ElMessage.warning('请先输入要排序的分类编号'); return } try { sortCategory.value = query.categoryNo; let numbers = [], version; for(let page=1;page<=10;page++) { const r=await giftQuery(kind+'/list',{ page,pageSize:100,categoryNo:sortCategory.value }); if(r.code!==0) throw new Error(r.msg); if(version && version!==r.data.version) throw new Error('目录已变化，请重试'); version=r.data.version; numbers.push(...r.data.list.map(x => x.giftNo || x.categoryNo)); if(numbers.length>=Number(r.data.total)) break; if(page===10) throw new Error('集合超过1000条排序上限') } sortVersion.value=version; sortText.value=numbers.join('\n'); sortDialog.value=true } catch(e) { ElMessage.error(e.message) } }
async function saveSort() { if(saving.value)return; saving.value=true; try { const r=await giftWrite('sort',{kind,categoryNo:sortCategory.value,version:sortVersion.value,numbers:sortText.value.trim().split(/\s+/)}); if(r.code!==0)throw new Error(r.msg);sortDialog.value=false;await load() }catch(e){ElMessage.error(e.message)}finally{saving.value=false} }
onMounted(load); onBeforeUnmount(() => { seq.invalidate(); guardSeq.invalidate() })
</script>
