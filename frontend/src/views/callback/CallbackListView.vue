<template>
  <div class="page">
    <PageHeader title="质量回访" description="维修完工后自动生成回访任务, 记录联系情况与满意度, 不合格自动触发返修">
      <el-button :icon="Refresh" @click="load">刷新</el-button>
    </PageHeader>

    <el-card shadow="never">
      <div class="filter-bar">
        <el-input v-model="query.keyword" placeholder="回访单号 / 故障单号 / 维修单号 / 路灯编号 / 回访人" clearable @keyup.enter="handleSearch" />
        <el-select v-model="query.status" placeholder="回访状态" clearable @change="handleSearch">
          <el-option v-for="(item, key) in CALLBACK_STATUS" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-select v-model="query.verdict" placeholder="回访判定" clearable @change="handleSearch">
          <el-option v-for="(item, key) in CALLBACK_VERDICT" :key="key" :label="item.label" :value="key" />
        </el-select>
        <el-select v-model="query.satisfaction" placeholder="满意度" clearable @change="handleSearch">
          <el-option v-for="(item, key) in CALLBACK_SATISFACTION" :key="key" :label="item.label" :value="key" />
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
        <el-table-column prop="callback_no" label="回访单号" width="140" fixed="left" />
        <el-table-column prop="fault_no" label="故障单号" width="140" />
        <el-table-column prop="repair_no" label="维修单号" width="140" />
        <el-table-column prop="lamp_code" label="路灯编号" width="110" />
        <el-table-column label="轮次" width="70" align="center">
          <template #default="{ row }">第 {{ row.round }} 轮</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }"><StatusTag :dict="CALLBACK_STATUS" :value="row.status" /></template>
        </el-table-column>
        <el-table-column label="联系情况" width="100">
          <template #default="{ row }">
            <StatusTag v-if="row.contact_result" :dict="CALLBACK_CONTACT" :value="row.contact_result" />
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="满意度" width="100">
          <template #default="{ row }">
            <StatusTag v-if="row.satisfaction" :dict="CALLBACK_SATISFACTION" :value="row.satisfaction" />
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="判定" width="90">
          <template #default="{ row }">
            <StatusTag v-if="row.verdict" :dict="CALLBACK_VERDICT" :value="row.verdict" />
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="visitor" label="回访人" width="90">
          <template #default="{ row }">{{ row.visitor || '-' }}</template>
        </el-table-column>
        <el-table-column label="回访时间" width="150">
          <template #default="{ row }">{{ formatDateTime(row.visited_at) }}</template>
        </el-table-column>
        <el-table-column label="关联返修" width="140">
          <template #default="{ row }">
            <el-link v-if="row.rework_repair_no" type="danger" @click="goRework">{{ row.rework_repair_no }}</el-link>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="feedback" label="回访反馈" min-width="180" show-overflow-tooltip />
        <el-table-column label="操作" width="170" fixed="right">
          <template #default="{ row }">
            <el-button v-if="row.status === 'pending'" link type="primary" @click="openRecord(row)">登记回访</el-button>
            <el-button link type="primary" @click="openDetail(row)">故障详情</el-button>
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

    <CallbackRecordDialog v-model="recordVisible" :model="recording" @saved="handleSaved" />
    <FaultDetailDrawer v-model="detailVisible" :fault-id="activeFaultId" />
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Refresh, RefreshLeft, Search } from '@element-plus/icons-vue'
import PageHeader from '@/components/common/PageHeader.vue'
import StatusTag from '@/components/common/StatusTag.vue'
import DataPagination from '@/components/common/DataPagination.vue'
import CallbackRecordDialog from './components/CallbackRecordDialog.vue'
import FaultDetailDrawer from '@/views/fault/components/FaultDetailDrawer.vue'
import { callbackApi } from '@/api/callback'
import { useDictStore } from '@/stores/dict'
import { CALLBACK_CONTACT, CALLBACK_SATISFACTION, CALLBACK_STATUS, CALLBACK_VERDICT } from '@/constants/dict'
import { formatDateTime } from '@/utils/format'
import { useListPage } from '@/composables/useListPage'

const router = useRouter()
const dictStore = useDictStore()

const { loading, rows, total, query, load, search, reset, changePage, changePageSize } = useListPage(callbackApi.list, {
  keyword: '',
  status: '',
  verdict: '',
  satisfaction: '',
  start_date: '',
  end_date: '',
})

const dateRange = ref([])
const recordVisible = ref(false)
const detailVisible = ref(false)
const recording = ref(null)
const activeFaultId = ref(null)

function applyDateRange() {
  query.start_date = dateRange.value?.[0] ?? ''
  query.end_date = dateRange.value?.[1] ?? ''
}

function handleSearch() {
  applyDateRange()
  search()
}

function handleReset() {
  dateRange.value = []
  reset()
}

function openRecord(row) {
  recording.value = { ...row }
  recordVisible.value = true
}

function openDetail(row) {
  activeFaultId.value = row.fault_id
  detailVisible.value = true
}

function goRework() {
  router.push({ path: '/repairs', query: { only_rework: 'true' } })
}

function handleSaved() {
  load()
  dictStore.loadRepairMeta().catch(() => {})
}

onMounted(() => {
  dictStore.ensureLoaded().catch(() => {})
})
</script>
