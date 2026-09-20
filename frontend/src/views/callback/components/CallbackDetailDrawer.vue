<template>
  <el-drawer
    :model-value="modelValue"
    title="维修质量回访详情"
    size="620px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="load"
  >
    <div v-loading="loading">
      <template v-if="detail.id">
        <el-descriptions :column="2" border size="small" title="回访任务">
          <el-descriptions-item label="回访单号">{{ detail.task_no }}</el-descriptions-item>
          <el-descriptions-item label="回访轮次">第 {{ detail.round }} 轮</el-descriptions-item>
          <el-descriptions-item label="任务状态">
            <StatusTag :dict="CALLBACK_STATUS" :value="detail.status" />
          </el-descriptions-item>
          <el-descriptions-item label="回访时限">{{ formatDateTime(detail.due_at) }}</el-descriptions-item>
          <el-descriptions-item label="故障单号">{{ detail.fault_no }}</el-descriptions-item>
          <el-descriptions-item label="维修单号">{{ detail.repair_no }}</el-descriptions-item>
          <el-descriptions-item label="路灯编号">{{ detail.lamp_code }}</el-descriptions-item>
          <el-descriptions-item label="末次联系">
            {{ detail.contacted_at ? formatDateTime(detail.contacted_at) : '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="满意度" :span="2">
            <el-rate v-if="detail.satisfaction" :model-value="detail.satisfaction" disabled />
            <span v-else class="text-muted">未评价</span>
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.result_remark" label="判定说明" :span="2">
            {{ detail.result_remark }}
          </el-descriptions-item>
          <el-descriptions-item v-if="detail.rework_repair_no" label="返修维修单" :span="2">
            <el-link type="primary" :underline="false" @click="goRework">{{ detail.rework_repair_no }}</el-link>
          </el-descriptions-item>
        </el-descriptions>

        <div class="drawer-actions">
          <el-button
            v-if="detail.status !== 'qualified'"
            type="primary"
            :icon="Phone"
            @click="$emit('contact', detail)"
          >
            登记联系情况
          </el-button>
          <el-button
            v-if="canJudge"
            :type="detail.status === 'unqualified' ? 'info' : 'success'"
            :icon="CircleCheck"
            @click="$emit('judge', detail)"
          >
            回访质量判定
          </el-button>
          <el-button :icon="View" @click="goTrack">查看处置链路</el-button>
        </div>

        <div class="section-title drawer-block">联系情况记录</div>
        <el-timeline v-if="detail.contacts?.length">
          <el-timeline-item
            v-for="item in detail.contacts"
            :key="item.id"
            :timestamp="formatDateTime(item.contacted_at)"
            :type="contactTimelineType(item)"
          >
            <div class="contact-head">
              <StatusTag :dict="CONTACT_RESULT" :value="item.result" />
              <StatusTag :dict="CONTACT_CHANNEL" :value="item.channel" />
              <el-tag v-if="item.qualified === true" type="success" size="small" effect="light">判定合格</el-tag>
              <el-tag v-if="item.qualified === false" type="danger" size="small" effect="light">判定不合格</el-tag>
            </div>
            <div class="contact-line text-muted">
              回访人: {{ item.contact_person || '-' }}
              <span v-if="item.contact_name"> · 受访人: {{ item.contact_name }}</span>
              <span v-if="item.satisfaction"> · 满意度 {{ item.satisfaction }}/5</span>
            </div>
            <div v-if="item.content" class="contact-content">{{ item.content }}</div>
          </el-timeline-item>
        </el-timeline>
        <el-empty v-else description="暂无联系记录" :image-size="70" />
      </template>
      <el-empty v-else description="暂无回访任务数据" />
    </div>
  </el-drawer>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CircleCheck, Phone, View } from '@element-plus/icons-vue'
import StatusTag from '@/components/common/StatusTag.vue'
import { callbackApi } from '@/api/callback'
import { CALLBACK_STATUS, CONTACT_CHANNEL, CONTACT_RESULT } from '@/constants/dict'
import { formatDateTime } from '@/utils/format'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  taskId: { type: [Number, String], default: null },
})

const emit = defineEmits(['update:modelValue', 'contact', 'judge'])

const router = useRouter()
const loading = ref(false)
const detail = ref({})

// 待回访/已联系/已不合格但返修尚未闭环时, 仍允许登记联系; 合格/不合格已终判时禁止再次判定。
const canJudge = computed(
  () => detail.value.status === 'pending' || detail.value.status === 'contacted',
)

function contactTimelineType(item) {
  if (item.qualified === true) return 'success'
  if (item.qualified === false) return 'danger'
  return 'primary'
}

async function load() {
  if (!props.taskId) return
  loading.value = true
  try {
    detail.value = await callbackApi.detail(props.taskId)
  } catch (error) {
    detail.value = {}
  } finally {
    loading.value = false
  }
}

function goTrack() {
  emit('update:modelValue', false)
  router.push({ path: '/status/track', query: { fault_no: detail.value.fault_no } })
}

function goRework() {
  emit('update:modelValue', false)
  router.push({ path: '/repairs' })
}
</script>

<style scoped>
.drawer-block {
  margin-top: 20px;
}

.drawer-actions {
  display: flex;
  gap: 8px;
  margin-top: 16px;
  flex-wrap: wrap;
}

.contact-head {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
}

.contact-line {
  font-size: 13px;
  margin-top: 4px;
}

.contact-content {
  font-size: 13px;
  margin-top: 2px;
}
</style>
