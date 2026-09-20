<template>
  <el-dialog
    :model-value="modelValue"
    title="登记回访结果"
    width="600px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-descriptions v-if="model" :column="2" border size="small" class="callback-summary">
      <el-descriptions-item label="回访单号">{{ model.callback_no }}</el-descriptions-item>
      <el-descriptions-item label="回访轮次">第 {{ model.round }} 轮</el-descriptions-item>
      <el-descriptions-item label="故障单号">{{ model.fault_no }}</el-descriptions-item>
      <el-descriptions-item label="维修单号">{{ model.repair_no }}</el-descriptions-item>
    </el-descriptions>

    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="联系情况" prop="contact_result">
        <el-radio-group v-model="form.contact_result">
          <el-radio v-for="(item, key) in CALLBACK_CONTACT" :key="key" :value="key">{{ item.label }}</el-radio>
        </el-radio-group>
        <div class="form-hint text-muted">未联系上时仅记录联系情况, 任务保持待回访, 可稍后再次登记。</div>
      </el-form-item>
      <template v-if="form.contact_result === 'reached'">
        <el-form-item label="满意度" prop="satisfaction">
          <el-radio-group v-model="form.satisfaction">
            <el-radio v-for="(item, key) in CALLBACK_SATISFACTION" :key="key" :value="key">{{ item.label }}</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="回访判定" prop="verdict">
          <el-radio-group v-model="form.verdict">
            <el-radio v-for="(item, key) in CALLBACK_VERDICT" :key="key" :value="key">{{ item.label }}</el-radio>
          </el-radio-group>
          <div class="form-hint text-muted">判定不合格将自动触发返修并关联原维修记录, 返修完工后需重新回访。</div>
        </el-form-item>
        <el-form-item v-if="form.verdict === 'unqualified'" label="返修人员" prop="rework_repairman">
          <el-input v-model="form.rework_repairman" placeholder="缺省沿用原维修人员" maxlength="64" />
        </el-form-item>
      </template>
      <el-form-item label="回访人" prop="visitor">
        <el-input v-model="form.visitor" placeholder="回访工作人员姓名" maxlength="64" />
      </el-form-item>
      <el-form-item label="回访时间" prop="visited_at">
        <el-date-picker
          v-model="form.visited_at"
          type="datetime"
          value-format="YYYY-MM-DD HH:mm:ss"
          placeholder="默认取当前时间"
          style="width: 100%"
        />
      </el-form-item>
      <el-form-item label="回访反馈" prop="feedback">
        <el-input v-model="form.feedback" type="textarea" :rows="3" maxlength="512" show-word-limit
          placeholder="记录市民反馈的照明恢复情况与诉求" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">提交回访</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { callbackApi } from '@/api/callback'
import { CALLBACK_CONTACT, CALLBACK_SATISFACTION, CALLBACK_VERDICT } from '@/constants/dict'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  model: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const createForm = () => ({
  contact_result: 'reached',
  satisfaction: 'satisfied',
  verdict: 'qualified',
  rework_repairman: '',
  visitor: '',
  visited_at: '',
  feedback: '',
})

const form = reactive(createForm())

const rules = {
  contact_result: [{ required: true, message: '请选择联系情况', trigger: 'change' }],
  satisfaction: [{ required: true, message: '请选择满意度', trigger: 'change' }],
  verdict: [{ required: true, message: '请选择回访判定', trigger: 'change' }],
  visitor: [{ required: true, message: '请填写回访人', trigger: 'blur' }],
}

function syncForm() {
  Object.assign(form, createForm())
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const payload = { ...form }
    if (!payload.visited_at) {
      delete payload.visited_at
    }
    if (payload.contact_result !== 'reached') {
      delete payload.satisfaction
      delete payload.verdict
      delete payload.rework_repairman
    }
    const saved = await callbackApi.record(props.model.id, payload)
    if (saved.status === 'pending') {
      ElMessage.success('已记录联系情况, 任务保持待回访')
    } else if (saved.verdict === 'unqualified') {
      ElMessage.success(`回访已登记, 系统已触发返修 ${saved.rework_repair_no}`)
    } else {
      ElMessage.success('回访已登记, 判定合格')
    }
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
