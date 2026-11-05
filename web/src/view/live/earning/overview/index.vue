<template><div>
  <div class="gva-search-box"><h2>直播收益</h2><el-form :inline="true" @submit.prevent="search"><el-form-item label="主播编号"><el-input v-model.trim="anchorNo" clearable placeholder="留空查看平台汇总" /></el-form-item><el-button type="primary" :loading="loading" @click="search">查询</el-button></el-form>
    <el-alert v-if="error" type="error" :closable="false" :title="error" />
    <template v-if="overview"><el-descriptions border :column="2" class="mt-4"><el-descriptions-item label="待结算收益 COIN">{{ overview.pendingCoin }}</el-descriptions-item><el-descriptions-item label="已结算收益 COIN">{{ overview.settledCoin }}</el-descriptions-item></el-descriptions>
    <el-table :data="overview.balances"><el-table-column v-for="[key,label] in [['currency','币种'],['available','可提现余额'],['frozen','冻结'],['debt','待抵扣'],['paid','已成功出款']]" :key="key" :prop="key" :label="label" /></el-table><p>余额按币种最小单位展示，各币种分别统计。COIN 为收益计量单位。</p><el-alert v-if="!overview.withdrawalEnabled" type="error" :closable="false" title="提现服务初始化异常，请联系技术人员处理。" /></template>
  </div>
  <div class="gva-table-box"><h3>统一收益事实</h3><el-form :inline="true"><el-form-item label="收益来源"><el-select v-model="query.sourceType" clearable style="width:160px"><el-option v-for="v in ['gift','bounty','donation','guard','fan_club','charge']" :key="v" :value="v" :label="sourceLabel(v)" /></el-select></el-form-item><el-form-item label="结算状态"><el-select v-model="query.status" clearable style="width:140px"><el-option label="待结算" value="pending" /><el-option label="已结算" value="settled" /></el-select></el-form-item><el-button @click="search">筛选</el-button></el-form>
    <el-table :data="rows"><el-table-column v-for="[key,label] in columns.income" :key="key" :prop="key" :label="label" min-width="160" show-overflow-tooltip><template #default="{row}">{{ key==='sourceType'?sourceLabel(row[key]):key==='entryType'?statusLabel(row[key]):key==='createdAt'?formatDate(new Date(row[key]),'yyyy-MM-dd hh:mm:ss'):row[key] }}</template></el-table-column></el-table>
    <el-pagination v-model:current-page="query.page" :total="Math.min(Number(total),1000000)" :page-size="20" layout="prev,pager,next" @current-change="load" />
  </div>
</div></template>
<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { earningQuery } from '@/api/earning'
import { formatDate } from '@/utils/format'
import { columns, sequence, sourceLabel, statusLabel } from '../shared'
defineOptions({name:'EarningOverview'})
const anchorNo=ref(''),overview=ref(null),rows=ref([]),total=ref('0'),loading=ref(false),error=ref(''),query=reactive({page:1,pageSize:20,sourceType:'',status:''}),seq=sequence()
async function load(){const id=seq.next();loading.value=true;error.value='';const filter={...query,anchorNo:anchorNo.value};try{const [summary,list]=await Promise.all([earningQuery('overview',{anchorNo:filter.anchorNo}),earningQuery('income/list',filter)]);if(!seq.valid(id))return;if(summary.code!==0||list.code!==0)throw new Error(summary.code?summary.msg:list.msg);overview.value=summary.data;rows.value=list.data.list;total.value=list.data.total}catch(e){if(seq.valid(id))error.value=e.message}finally{if(seq.valid(id))loading.value=false}}
function search(){query.page=1;return load()}
onMounted(load);onBeforeUnmount(()=>seq.invalidate())
</script>
