<template>
  <div class="snapshot-panel">
    <el-alert :type="data.consistent ? 'success' : 'error'" :closable="false" :title="data.consistent ? '系统核验通过' : '存在异常，审核通过将被阻止'" />
    <el-descriptions :column="2" border class="mt-4">
      <el-descriptions-item v-for="[key,value] in entries" :key="key" :label="labels[key] || key">{{ typeof value === 'boolean' ? (value ? '是' : '否') : value }}</el-descriptions-item>
    </el-descriptions>
    <ul v-if="data.issues?.length"><li v-for="issue in data.issues" :key="issue">{{ issue }}</li></ul>
    <el-alert v-if="data.payee" type="info" :closable="false" :title="`收款信息：${data.payee.name} · ${data.payee.masked} · ${data.payee.method} · 最近变更：${data.payee.changedAt || '未变更'}`" />
  </div>
</template>
<script setup>
import { computed } from 'vue'
import { snapshot, labels } from './shared'
const props=defineProps({ value: { type: String, default: '' } })
const data=computed(()=>snapshot(props.value))
const entries=computed(()=>Object.entries(data.value).filter(([key,value])=>key!=='issues'&&key!=='payee'&&value!==null&&typeof value!=='object'))
</script>
