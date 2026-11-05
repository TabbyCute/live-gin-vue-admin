<template>
  <div>
    <div class="gva-search-box"><el-alert title="榜单按 UTC 统计送礼订单，扣除这些订单截至查询时的全部退款。主播收益为待结算 COIN，不是法币或可提现余额。自定义范围最多93天。" :closable="false" />
      <el-form inline @submit.prevent="search"><el-form-item label="维度"><el-select v-model="query.dimension" style="width:150px"><el-option v-for="[value,label] in dimensions" :key="value" :value="value" :label="label" /></el-select></el-form-item><el-form-item label="周期"><el-select v-model="query.period" style="width:130px"><el-option v-for="[value,label] in periods" :key="value" :value="value" :label="label" /></el-select></el-form-item><el-date-picker v-if="query.period==='custom'" v-model="range" type="datetimerange" value-format="x" />
        <el-form-item v-for="[key,label] in filters" :key="key" :label="label"><el-input v-model="query[key]" clearable /></el-form-item><el-button native-type="submit" :loading="loading">查询</el-button>
      </el-form>
    </div>
    <div class="gva-table-box"><el-alert v-if="error" :title="error" type="error" /><el-table :data="rows" v-loading="loading" @sort-change="sortChanged"><el-table-column prop="no" label="编号" min-width="160" /><el-table-column v-for="[prop,label] in columns" :key="prop" :prop="prop" :label="label" sortable="custom" min-width="130" /></el-table><el-pagination v-model:current-page="query.page" :page-size="20" :total="total" layout="total,prev,pager,next" @change="load" /></div>
  </div>
</template>
<script setup>
import { ref,reactive,onMounted,onBeforeUnmount } from 'vue'
import { giftQuery } from '@/api/gift'
import { requestSequence } from '../shared'
defineOptions({name:'GiftRanking'})
const dimensions=[['sender','送礼用户'],['anchor','主播'],['room','直播间'],['session','直播场次'],['gift','礼物'],['category','礼物分类'],['platform','全平台收入']]
const periods=[['day','今日'],['week','本周'],['month','本月'],['custom','自定义']]
const columns=[['gross','原始 COIN'],['refund','退款 COIN'],['net','有效 COIN'],['earning','主播收益 COIN'],['quantity','有效件数'],['orders','订单数'],['users','有效送礼人数']]
const filters=[['anchorNo','主播'],['roomNo','房间'],['sessionNo','场次'],['categoryNo','分类'],['giftNo','礼物']]
const query=reactive({dimension:'sender',period:'day',page:1,pageSize:20,sort:'net',desc:true,...Object.fromEntries(filters.map(([k])=>[k,'']))}),range=ref([]),rows=ref([]),total=ref(0),loading=ref(false),error=ref(''),seq=requestSequence()
async function load(){const id=seq.next();loading.value=true;error.value='';try{const r=await giftQuery('ranking',{...query,from:Number(range.value?.[0]||0),to:Number(range.value?.[1]||0)});if(!seq.valid(id))return;if(r.code!==0)throw new Error(r.msg);rows.value=r.data.list;total.value=Number(r.data.total)}catch(e){if(seq.valid(id))error.value=e.message||'查询失败'}finally{if(seq.valid(id))loading.value=false}}
function search(){query.page=1;load()}
function sortChanged({prop,order}){query.sort=order?prop:'net';query.desc=!order||order==='descending';search()}
onMounted(load);onBeforeUnmount(()=>seq.invalidate())
</script>
