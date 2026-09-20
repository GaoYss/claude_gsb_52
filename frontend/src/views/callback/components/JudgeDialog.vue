<template>
  <el-dialog
    :model-value="modelValue"
    title="回访质量判定"
    width="560px"
    @update:model-value="$emit('update:modelValue', $event)"
    @open="syncForm"
  >
    <el-descriptions v-if="model" :column="1" border size="small" class="callback-summary">
      <el-descriptions-item label="回访单号">{{ model.task_no }}（第 {{ model.round }} 轮）</el-descriptions-item>
      <el-descriptions-item label="故障 / 维修">{{ model.fault_no }} / {{ model.repair_no }}</el-descriptions-item>
      <el-descriptions-item label="路灯">{{ model.lamp_code }}</el-descriptions-item>
    </el-descriptions>

    <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
      <el-form-item label="回访结论" prop="qualified">
        <el-radio-group v-model="form.qualified">
          <el-radio-button :value="true">合格, 可结算</el-radio-button>
          <el-radio-button :value="false">不合格, 触发返修</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="满意度" prop="satisfaction">
        <el-rate v-model="form.satisfaction" :max="5" show-text :texts="rateTexts" />
      </el-form-item>
      <el-form-item label="判定说明" prop="remark">
        <el-input
          v-model="form.remark"
          type="textarea"
          :rows="2"
          maxlength="512"
          show-word-limit
          :placeholder="form.qualified ? '回访合格说明' : '不合格原因, 将随返修单一并派工'"
        />
      </el-form-item>

      <template v-if="!form.qualified">
        <el-divider content-position="left">返修派工</el-divider>
        <el-form-item label="返修维修人" prop="rework_repairman">
          <el-input v-model="form.rework_repairman" maxlength="64" />
        </el-form-item>
        <el-form-item label="返修班组" prop="rework_repair_team">
          <el-input v-model="form.rework_repair_team" maxlength="64" />
        </el-form-item>
        <el-form-item label="返修内容" prop="rework_content">
          <el-input v-model="form.rework_content" type="textarea" :rows="2" maxlength="512" show-word-limit
            placeholder="留空则按不合格原因自动生成" />
        </el-form-item>
        <el-form-item label="预计费用" prop="rework_cost">
          <el-input-number v-model="form.rework_cost" :min="0" :precision="2" :step="10" style="width: 100%" />
        </el-form-item>
        <div class="form-hint text-muted">
          提交后将在原故障上创建返修维修单并关联原维修记录({{ model?.repair_no }}),
          故障回到维修中; 返修完工后自动发起重新回访。
        </div>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button :type="form.qualified ? 'success' : 'danger'" :loading="submitting" @click="handleSubmit">
        {{ form.qualified ? '确认合格' : '确认不合格并返修' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { callbackApi } from '@/api/callback'

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  model: { type: Object, default: null },
})

const emit = defineEmits(['update:modelValue', 'saved'])

const formRef = ref(null)
const submitting = ref(false)

const rateTexts = ['非常不满意', '不满意', '一般', '满意', '非常满意']

const createForm = () => ({
  qualified: true,
  satisfaction: 5,
  remark: '',
  rework_repairman: '',
  rework_repair_team: '',
  rework_content: '',
  rework_cost: 0,
})

const form = reactive(createForm())

const rules = {
  rework_repairman: [{ required: true, message: '不合格返修必须填写返修维修人', trigger: 'blur' }],
}

function syncForm() {
  Object.assign(form, createForm())
}

async function handleSubmit() {
  if (!form.qualified) {
    const valid = await formRef.value.validate().catch(() => false)
    if (!valid) return
  }

  if (!form.qualified) {
    try {
      await ElMessageBox.confirm('判定不合格将立即创建返修单并重新走维修流程, 确认继续?', '返修确认', {
        type: 'warning',
        confirmButtonText: '确认触发返修',
        cancelButtonText: '取消',
      })
    } catch (error) {
      return
    }
  }

  const payload = { ...form }
  if (!payload.satisfaction) delete payload.satisfaction
  if (payload.qualified) {
    delete payload.rework_repairman
    delete payload.rework_repair_team
    delete payload.rework_content
    delete payload.rework_cost
  }
  if (!payload.rework_cost) delete payload.rework_cost

  submitting.value = true
  try {
    await callbackApi.judge(props.model.id, payload)
    ElMessage.success(form.qualified ? '回访已判定合格' : '已判定不合格并触发返修')
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
