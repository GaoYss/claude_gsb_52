<template>
  <div class="page">
    <PageHeader title="维修质量回访" description="完工后自动生成回访任务, 记录联系情况与满意度, 不合格触发返修并重新回访">
      <el-button :icon="Refresh" @click="load">刷新</el-button>
    </PageHeader>

    <div class="stat-strip">
      <StatCard label="回访任务" :value="stats.total" suffix="条" icon="Phone" color="#409eff" />
      <StatCard label="待回访" :value="stats.pending_total" suffix="条" icon="Bell" color="#f56c6c" />
      <StatCard label="已联系待判定" :value="stats.contacted_total" suffix="条" icon="ChatDotRound" color="#e6a23c" />
      <StatCard label="回访合格" :value="stats.qualified_total" suffix="条" icon="CircleCheck" color="#67c23a" />
      <StatCard label="返修次数" :value="stats.rework_total" suffix="次" icon="RefreshRight" color="#909399" />
      <StatCard label="回访超期" :value="stats.overdue_total" suffix="条" icon="AlarmClock" color="#f56c6c" />
    </div>

    <el-card shadow="never">
      <div class="filter-bar">
        <el-input v-model="query.keyword" placeholder="回访单号 / 故障单号 / 路灯编号 / 维修单号" clearable @keyup.enter="handleSearch" />
        <el-select v-model="query.status" placeholder="回访状态" clearable style="width: 150px" @change="handleSearch">
          <el-option v-for="(item, key) in CALLBACK_STATUS" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-select v-model="quickFilter" placeholder="快捷筛选" style="width: 150px" @change="applyQuickFilter">
          <el-option label="全部任务" value="all" />
          <el-option label="未完成" value="open" />
          <el-option label="仅超期" value="overdue" />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="daterange"
          value-format="YYYY-MM-DD"
          range-separator="至"
          start-placeholder="生成开始日期"
          end-placeholder="生成结束日期"
          @change="handleSearch"
        />
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="handleReset">重置</el-button>
      </div>
    </el-card>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="rows" stripe>
        <el-table-column prop="task_no" label="回访单号" width="150" fixed="left" />
        <el-table-column label="轮次" width="70">
          <template #default="{ row }">
            <el-tag size="small" :type="row.round > 1 ? 'warning' : 'info'" effect="plain">第{{ row.round }}轮</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="fault_no" label="故障单号" width="140" />
        <el-table-column prop="repair_no" label="维修单号" width="140" />
        <el-table-column prop="lamp_code" label="路灯编号" width="110" />
        <el-table-column label="回访状态" width="120">
          <template #default="{ row }">
            <StatusTag :dict="CALLBACK_STATUS" :value="row.status" />
            <el-tag v-if="row.overdue" type="danger" size="small" effect="plain" class="overdue-tag">超期</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="满意度" width="120">
          <template #default="{ row }">
            <el-rate v-if="row.satisfaction" :model-value="row.satisfaction" disabled size="small" />
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="回访时限" width="150">
          <template #default="{ row }">{{ formatDateTime(row.due_at) }}</template>
        </el-table-column>
        <el-table-column label="末次联系" width="150">
          <template #default="{ row }">{{ formatDateTime(row.contacted_at) }}</template>
        </el-table-column>
        <el-table-column label="返修单" width="140">
          <template #default="{ row }">
            <span v-if="row.rework_repair_no" class="rework-no">{{ row.rework_repair_no }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="result_remark" label="判定说明" min-width="160" show-overflow-tooltip />
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openDetail(row)">详情</el-button>
            <el-button v-if="row.status !== 'qualified'" link type="primary" @click="openContact(row)">联系</el-button>
            <el-button
              v-if="row.status === 'pending' || row.status === 'contacted'"
              link
              type="success"
              @click="openJudge(row)"
            >
              判定
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <DataPagination
        :page="query.page"
        :page-size="query.page_size"
        :total="total"
        @page-change="changePage"
        @size-change="changePageSize"
      />
    </el-card>

    <CallbackDetailDrawer
      v-model="detailVisible"
      :task-id="activeTaskId"
      @contact="handleContactFromDrawer"
      @judge="handleJudgeFromDrawer"
    />
    <ContactDialog v-model="contactVisible" :model="activeTask" @saved="handleSaved" />
    <JudgeDialog v-model="judgeVisible" :model="activeTask" @saved="handleSaved" />
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { Refresh, RefreshLeft, Search } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatCard from '@/components/common/StatCard.vue'
import StatusTag from '@/components/common/StatusTag.vue'
import DataPagination from '@/components/common/DataPagination.vue'
import CallbackDetailDrawer from './components/CallbackDetailDrawer.vue'
import ContactDialog from './components/ContactDialog.vue'
import JudgeDialog from './components/JudgeDialog.vue'
import { callbackApi } from '@/api/callback'
import { CALLBACK_STATUS } from '@/constants/dict'
import { formatDateTime } from '@/utils/format'
import { useListPage } from '@/composables/useListPage'

const { loading, rows, total, query, load, search, reset, changePage, changePageSize } = useListPage(callbackApi.list, {
  keyword: '',
  status: '',
  only_open: false,
  only_overdue: false,
  start_date: '',
  end_date: '',
})

const dateRange = ref([])
const quickFilter = ref('all')
const stats = ref(emptyStats())

const detailVisible = ref(false)
const contactVisible = ref(false)
const judgeVisible = ref(false)
const activeTaskId = ref(null)
const activeTask = ref(null)

function emptyStats() {
  return {
    total: 0, pending_total: 0, contacted_total: 0,
    qualified_total: 0, unqualified_total: 0, open_total: 0,
    overdue_total: 0, rework_total: 0,
  }
}

function applyDateRange() {
  query.start_date = dateRange.value?.[0] ?? ''
  query.end_date = dateRange.value?.[1] ?? ''
}

function applyQuickFilter(value) {
  query.only_open = value === 'open'
  query.only_overdue = value === 'overdue'
  if (value !== 'all') query.status = ''
  search()
}

function handleSearch() {
  applyDateRange()
  // 精确状态优先于快捷筛选。
  if (query.status) {
    quickFilter.value = 'all'
    query.only_open = false
    query.only_overdue = false
  }
  search()
}

function handleReset() {
  dateRange.value = []
  quickFilter.value = 'all'
  reset()
  loadStats()
}

function openDetail(row) {
  activeTaskId.value = row.id
  detailVisible.value = true
}

function openContact(row) {
  activeTask.value = { ...row }
  contactVisible.value = true
}

function openJudge(row) {
  activeTask.value = { ...row }
  judgeVisible.value = true
}

function handleContactFromDrawer(task) {
  detailVisible.value = false
  activeTask.value = task
  contactVisible.value = true
}

function handleJudgeFromDrawer(task) {
  detailVisible.value = false
  activeTask.value = task
  judgeVisible.value = true
}

function handleSaved() {
  load()
  loadStats()
  // 详情抽屉数据已过期, 关闭以便重新打开刷新。
  detailVisible.value = false
}

async function loadStats() {
  try {
    stats.value = await callbackApi.statistics()
  } catch (error) {
    stats.value = emptyStats()
  }
}

onMounted(loadStats)
</script>

<style scoped>
.stat-strip {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 12px;
}

.overdue-tag {
  margin-left: 4px;
}

.rework-no {
  color: #e6a23c;
  font-weight: 600;
}

@media (max-width: 1200px) {
  .stat-strip {
    grid-template-columns: repeat(3, 1fr);
  }
}
</style>
