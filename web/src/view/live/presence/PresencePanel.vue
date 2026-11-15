<template>
  <section v-if="presenceAllowed" class="presence-panel">
    <h3>观众与在线状态</h3>
    <el-alert v-if="!data || data.stale || data.dataLost" :closable="false" type="warning"
      :title="data?.dataLost ? '在线状态曾丢失，已记录数据缺口；历史统计可能不完整' : '实时统计暂不可用；— 表示未知，不代表零人'" />
    <el-descriptions v-if="data" :column="2" border>
      <el-descriptions-item label="游客在线">{{ count('guestCount') }}</el-descriptions-item>
      <el-descriptions-item label="会员在线（已登录）">{{ count('memberCount') }}</el-descriptions-item>
      <el-descriptions-item label="当前连接">{{ count('connectionCount') }}</el-descriptions-item>
      <el-descriptions-item label="合计在线">{{ count('totalOnlineCount') }}</el-descriptions-item>
      <el-descriptions-item label="峰值在线（含游客）">{{ data.peakOnlineCount }}</el-descriptions-item>
      <el-descriptions-item label="累计进房次数">{{ data.viewCount }}</el-descriptions-item>
      <el-descriptions-item label="累计去重观众">{{ data.viewerCount }}</el-descriptions-item>
      <el-descriptions-item label="有效点赞">{{ data.likeCount }}</el-descriptions-item>
      <el-descriptions-item label="合计观看时长">{{ duration(data.totalWatchDurationMs) }}</el-descriptions-item>
      <el-descriptions-item label="MySQL检查点">{{ timestamp(data.checkpointAt) }}</el-descriptions-item>
      <el-descriptions-item label="最终汇总">{{ data.finalizedAt ? timestamp(data.finalizedAt) : '尚未完成' }}</el-descriptions-item>
      <el-descriptions-item label="Worker">{{ data.finalizedAt ? '场次已汇总' : data.workerHealthy ? '检查点正常' : '延迟或不可用' }}</el-descriptions-item>
      <el-descriptions-item label="待落库记录">{{ data.pendingCount || '—' }}</el-descriptions-item>
      <el-descriptions-item label="过期清理延迟">{{ data.cleanupLagMs ? `${data.cleanupLagMs} 毫秒` : '—' }}</el-descriptions-item>
    </el-descriptions>
    <p class="presence-note">在线值依赖心跳和TTL，约每10秒刷新。游客按签名会话去重；同一会员多页面只计一人。钱款请在礼物订单及资金流水中核查。</p>
  </section>
</template>
<script setup>
import { computed } from 'vue'
import { formatDate } from '@/utils/format'
import { usePresence, presenceCount } from './use-presence'
const props = defineProps({ sessionNo: { type: String, default: '' } })
const rows = computed(() => props.sessionNo ? [{ sessionNo: props.sessionNo }] : [])
const { presenceStats, presenceAllowed } = usePresence(rows)
const data = computed(() => presenceStats.value[props.sessionNo])
const count = (field) => presenceCount(data.value, field)
const timestamp = (value) => value ? formatDate(Number(value)) : '—'
const duration = (value) => { try { const seconds = BigInt(value || '0') / BigInt(1000); return `${seconds / BigInt(3600)}小时 ${seconds % BigInt(3600) / BigInt(60)}分 ${seconds % BigInt(60)}秒` } catch { return '—' } }
</script>
<style scoped>
.presence-panel { margin: 16px 0; }
.presence-note { color: var(--el-text-color-secondary); font-size: 12px; line-height: 1.6; }
</style>
