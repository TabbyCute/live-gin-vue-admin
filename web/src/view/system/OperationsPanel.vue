<template>
  <el-card v-if="isRoot" class="m-2">
    <template #header>
      <div class="flex items-center justify-between"><span>业务服务运行状态</span><el-button :loading="loading" @click="refresh">刷新</el-button></div>
    </template>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <template v-if="snapshot">
      <p>数据库：{{ snapshot.database }}　Redis：{{ snapshot.redis }}　在途请求：{{ snapshot.metrics.inFlight }}</p>
      <p>数据库连接：{{ snapshot.databasePool.inUse }} 使用中 / {{ snapshot.databasePool.open }} 已打开；累计等待 {{ snapshot.databasePool.waitCount }} 次</p>
      <el-alert v-for="alert in snapshot.alerts" :key="alert.code" :title="alert.message" :type="alert.severity === 'critical' ? 'error' : 'warning'" :closable="false" class="my-2" />
      <el-table :data="snapshot.backlogs" empty-text="暂无积压数据">
        <el-table-column prop="name" label="出款队列" />
        <el-table-column label="数量"><template #default="{row}">{{ row.state === 'ready' ? `${row.count}${row.truncated ? '+' : ''}` : '未知' }}</template></el-table-column>
        <el-table-column label="最早创建时间"><template #default="{row}">{{ formatTime(row.oldestAt) }}</template></el-table-column>
      </el-table>
      <el-table :data="snapshot.metrics.workers" empty-text="尚无后台任务运行记录" class="my-3">
        <el-table-column prop="name" label="后台任务" min-width="180" />
        <el-table-column label="状态"><template #default="{row}">{{ row.stale ? '停滞' : row.running ? '执行中' : row.consecutiveFailures !== '0' ? '失败待重试' : '正常' }}</template></el-table-column>
        <el-table-column prop="runs" label="累计执行" />
        <el-table-column prop="failures" label="累计失败" />
        <el-table-column prop="lastDurationMs" label="最近耗时(ms)" />
        <el-table-column label="最近成功"><template #default="{row}">{{ formatTime(row.lastSuccessAt) }}</template></el-table-column>
      </el-table>
      <p class="text-gray-500">更新于 {{ formatTime(snapshot.at) }}。计数从本次进程启动累计，重启后清零；未知数据不代表正常。单个队列最多展示 1000 条的计数。</p>
    </template>
  </el-card>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useUserStore } from '@/pinia/modules/user'
import service from '@/utils/request'

const user = useUserStore()
const isRoot = computed(() => String(user.userInfo.ID) === '1')
const snapshot = ref(null)
const loading = ref(false)
const error = ref('')
let sequence = 0
let alive = true
const formatTime = value => value ? new Date(value).toLocaleString() : '—'

async function refresh() {
  if (!isRoot.value || loading.value) return
  const request = ++sequence
  loading.value = true
  error.value = ''
  try {
    const result = await service({ url: '/system/operations', method: 'get', donNotShowLoading: true })
    if (!alive || request !== sequence) return
    if (result.code !== 0) throw new Error(result.msg || '运行状态查询失败')
    snapshot.value = result.data
  } catch (e) {
    if (alive && request === sequence) { snapshot.value = null; error.value = e.message || '运行状态查询失败' }
  } finally {
    if (alive && request === sequence) loading.value = false
  }
}
watch(() => [user.userInfo.ID, user.token], () => {
  sequence++; snapshot.value = null; error.value = ''; loading.value = false
  if (isRoot.value) refresh()
}, { immediate: true })
onBeforeUnmount(() => { alive = false; sequence++ })
</script>
