# WorkflowFormModal 重构分析

## 当前状态
- **文件**: `WorkflowFormModal.tsx`
- **代码行数**: 1656行
- **复杂度**: 极高
  - 50个hooks调用
  - 25+个状态变量
  - 4个步骤的复杂表单

## 问题分析

### 1. 状态管理混乱
```typescript
// 25+个状态变量分散在组件中
const [step, setStep] = useState<Step>(1)
const [loading, setLoading] = useState(false)
const [name, setName] = useState('')
const [description, setDescription] = useState('')
const [schedule, setSchedule] = useState('0 0 2 * *')
// ... 20+ more states
```

### 2. 职责不清
- 步骤导航逻辑
- 表单验证逻辑
- API调用逻辑
- 数据加载逻辑
- UI渲染逻辑

全部混在一个组件中！

### 3. 性能问题
- 无限滚动的Intersection Observer实现复杂
- 大量useEffect导致不必要的重渲染
- 没有Memo优化

## 重构策略

### 阶段1: 提取自定义Hook
创建 `useWorkflowForm.ts` 管理所有状态和逻辑：
```typescript
// hooks/useWorkflowForm.ts
export function useWorkflowForm(workflow?: Workflow | null) {
  // 所有状态
  // 所有验证逻辑
  // 所有API调用
  // 返回简洁的API
  return {
    // 状态
    step, currentStep,
    formData,

    // 操作
    nextStep, prevStep, updateField,

    // 验证
    errors, isValid, validateCurrentStep,

    // 提交
    submit, isSubmitting,
  }
}
```

### 阶段2: 拆分Step组件
```
components/workflows/steps/
├── Step1BasicInfo.tsx         (150行)
├── Step2ScheduleConfig.tsx    (200行)
├── Step3ScopeConfig.tsx       (250行)
└── Step4RulesConfig.tsx       (200行)
```

每个Step组件：
- 接收 `formData` 和 `updateField`
- 负责自己的UI渲染
- 不包含业务逻辑

### 阶段3: 简化父组件
```typescript
// WorkflowFormModal.tsx (简化后约400行)
export default function WorkflowFormModal({ isOpen, onClose, onSuccess, workflow }) {
  const {
    step, nextStep, prevStep,
    formData, updateField,
    errors, isValid, submit, isSubmitting,
  } = useWorkflowForm(workflow)

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
        {step === 1 && <Step1BasicInfo {...} />}
        {step === 2 && <Step2ScheduleConfig {...} />}
        {step === 3 && <Step3ScopeConfig {...} />}
        {step === 4 && <Step4RulesConfig {...} />}

        {/* 共用的导航按钮 */}
        <StepNavigation
          step={step}
          onBack={prevStep}
          onNext={nextStep}
          onSubmit={submit}
          isValid={isValid}
          isSubmitting={isSubmitting}
        />
      </DialogContent>
    </Dialog>
  )
}
```

## 预期效果

### 代码量
- **原始**: 1656行
- **重构后**: ~800行
  - 父组件: 400行
  - useWorkflowForm: 250行
  - 4个Step组件: 100行

### 可维护性
- ✅ 职责清晰：Hook管理状态，Step组件负责UI
- ✅ 易于测试：Hook和Step组件可独立测试
- ✅ 代码复用：useWorkflowForm可用于其他地方

### 性能
- ✅ 减少不必要的重渲染
- ✅ 更好的Memo优化
- ✅ 更清晰的依赖关系

## 实施计划

1. **创建 useWorkflowForm Hook** (250行)
   - 提取所有状态
   - 提取所有验证逻辑
   - 提取所有API调用

2. **创建 Step 组件** (800行)
   - Step1BasicInfo
   - Step2ScheduleConfig
   - Step3ScopeConfig
   - Step4RulesConfig

3. **重构父组件** (400行)
   - 使用 useWorkflowForm
   - 渲染对应的 Step 组件
   - 处理导航和提交

4. **测试验证**
   - 单元测试
   - 集成测试
   - 手动测试

## 估计时间
- 创建 Hook: 2-3小时
- 创建 Step 组件: 4-5小时
- 重构父组件: 2小时
- 测试和调试: 2小时

**总计**: 10-12小时（约1.5个工作日）
