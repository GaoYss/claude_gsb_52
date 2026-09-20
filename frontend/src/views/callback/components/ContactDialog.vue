<template>
  <el-dialog
    :model-value="modelValue"
    title="登记回访联系情况"
    width="520px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-descriptions v-if="model" :column="1" border size="small" class="callback-summary">
      <el-descriptions-item label="回访单号">{{ model.task_no }}（第 {{ model.round }} 轮）</el-descriptions-item>
      <el-descriptions-item label="故障 / 维修">{{ model.fault_no }} / {{ model.repair_no }}</el-descriptions-item>
    </el-descriptions>

    <el-form ref="formRef" :model="form" :rules="rules" label-width="92px">
      <el-form-item label="联系渠道" prop="channel">
        <el-radio-group v-model="form.channel">
          <el-radio v-for="(item, key) in CONTACT_CHANNEL" :key="key" :value="key">{{ item.label }}</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="联系结果" prop="result">
        <el-select v-model="form.result" style="width: 100%">
          <el-option v-for="(item, key) in CONTACT_RESULT" :key="key" :label="item.label" :value="key" />
        </el-select>
        <div class="form-hint text-muted">未接通或无法联系时可再次回访, 任务保持待回访。</div>
      </el-form-item>
      <el-form-item label="回访人" prop="contact_person">
        <el-input v-model="form.contact_person" maxlength="64" />
      </el-form-item>
      <el-form-item label="受访人" prop="contact_name">
        <el-input v-model="form.contact_name" placeholder="报修人 / 现场负责人" maxlength="64" />
      </el-form-item>
      <el-form-item v-if="canRate" label="满意度" prop="satisfaction">
        <el-rate v-model="form.satisfaction" :max="5" show-text :texts="rateTexts" />
      </el-form-item>
      <el-form-item label="联系时间" prop="contacted_at">
        <el-date-picker
          v-model="form.contacted_at"
          type="datetime"
          value-format="YYYY-MM-DD HH:mm:ss"
          placeholder="默认取当前时间"
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="联系情况" prop="content">
        <el-input v-model="form.content" type="textarea" :rows="3" maxlength="512" show-word-limit
          placeholder="联系结果、现场反馈、未接通原因等" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">保存联系情况</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { callbackApi } from '@/api/callback'
import { CONTACT_CHANNEL, CONTACT_RESULT } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  model: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const rateTexts = ['非常不满意', '不满意', '一般', '满意', '非常满意']

const createForm = () => ({
  channel: 'phone',
  result: 'connected',
  contact_person: '',
  contact_name: '',
  satisfaction: 0,
  contacted_at: '',
  content: '',
})

const form = reactive(createForm())

// 仅"已联系"取得实质反馈时记录满意度(与后端校验保持一致)。
const canRate = computed(() => form.result === 'connected')

const rules = {
  result: [{ required: true, message: '请选择联系结果', trigger: 'change' }],
  contact_person: [{ required: true, message: '请填写回访人', trigger: 'blur' }],
}

function syncForm() {
  Object.assign(form, createForm())
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  const payload = { ...form }
  if (!payload.contacted_at) delete payload.contacted_at
  // 满意度为 0 或当前结果不允许评分时不下发。
  if (!payload.satisfaction || !canRate.value) delete payload.satisfaction

  submitting.value = true
  try {
    await callbackApi.addContact(props.model.id, payload)
    ElMessage.success('联系情况已记录')
    emit('update:modelValue', false)
    emit('saved')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.callback-summary {
  margin-bottom: 16px;
}

.form-hint {
  font-size: 12px;
  line-height: 1.6;
}
</style>
