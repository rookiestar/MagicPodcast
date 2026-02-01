import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { workflowApi, podcastApi, tagApi } from "@/lib/api";
import type {
  WorkflowRequest,
  WorkflowScopeType,
  ScopeConfig,
  RulesConfig,
  Podcast,
  Workflow,
  Tag,
} from "@/types";

// 预设的Cron表达式（6位格式：秒 分 时 日 月 周）
const CRON_PRESETS = [
  { label: "每天凌晨2点", value: "0 0 2 * * *" },
  { label: "每天早上8点", value: "0 0 8 * * *" },
  { label: "每天晚上8点", value: "0 0 20 * * *" },
  { label: "每周日凌晨2点", value: "0 0 2 * * 0" },
  { label: "每周一早上6点", value: "0 0 6 * * 1" },
] as const;

// 默认User Prompt模板
const DEFAULT_USER_PROMPT = `# 工作流执行报告

工作流名称: \${"{{"}"}.WorkflowName\${"{{"}"}}
匹配的单集总数: \${"{{"}"}.TotalEpisodes\${"{{"}"}
节目数量: \${"{{"}"}.NumPodcasts\${"{{"}}}

## 数据来源

\${"{{"}"}range .Podcasts\${"{{"}"}
### \${"{{"}"}.PodcastTitle\${"{{"}"}}
单集数: \${"{{"}"}len .Episodes\${"{{"}"}
\${"{{"}"}range .Episodes\${"{{"}"}
- **\${"{{"}"}.Title\${"{{"}"}** (\${"{{"}"}.PublishedDate.Format "2006-01-02"\${"{{"}"}})
  \${"{{"}"}if ne .ShowNotes ""\${"{{"}"}\${"{{"}"}.ShowNotes\${"{{"}"}\${"{{"}"}end\${"{{"}"}
\${"{{"}"}end\${"{{"}"}
\${"{{"}"}end\${"{{"}"}

## 分析要求

请按照以下维度生成分析报告：

### 1. 总体概览
简要描述本次抓取的整体情况（1-2句话）

### 2. 核心内容
按节目分类列出重要单集的要点：
- 理解播客节目的主题和内容风格
- 提取每期单集的核心观点和关键信息
- 识别跨节目的主题关联和趋势

### 3. 关键洞察
提炼3-5个关键主题或趋势，指出值得关注的亮点

## 输出格式要求

1. 使用紧凑的列表格式，bullet point（-）或数字序号后直接跟内容，不要换行
2. 列表项之间保持一个换行即可
3. 避免连续的多个空行
4. 用简洁专业的语言生成摘要，避免过度解读
5. 客观准确，不添加原文没有的信息
6. 简洁明了，避免冗余表述`;

type Step = 1 | 2 | 3 | 4;

interface UseWorkflowFormProps {
  workflow?: Workflow | null;
  isOpen: boolean;
}

interface WorkflowFormData {
  // Step 1: 基本信息
  name: string;
  description: string;
  schedule: string;
  customCron: string;

  // Step 2: 范围配置
  scopeType: WorkflowScopeType;
  selectedPodcastIds: number[];
  customUrls: string[];
  selectedTagIds: number[];

  // Step 3: 规则配置
  timeRange: number;
  minDuration: number;
  maxResults: number;
  keywords: string;
  excludeWords: string;

  // LLM 配置
  llmEnabled: boolean;
  llmMaxEpisodes: number;
  llmModel: string;
  llmTemperature: number;
  llmMaxTokens: number;
  llmUserPrompt: string;
}

interface UseWorkflowFormReturn {
  // 状态
  step: Step;
  loading: boolean;
  formData: WorkflowFormData;

  // Step 1 相关
  cronError: string;
  cronPresets: typeof CRON_PRESETS;

  // Step 2 相关
  podcasts: Podcast[];
  podcastSearch: string;
  candidatePodcastIds: number[];
  isLoadingPodcasts: boolean;
  displayedCount: number;
  tags: Tag[];
  tagSearch: string;
  isTagFilterExpanded: boolean;
  isLoadingTags: boolean;
  newCustomUrl: string;

  // 操作方法
  nextStep: () => void;
  prevStep: () => void;
  updateField: <K extends keyof WorkflowFormData>(
    field: K,
    value: WorkflowFormData[K],
  ) => void;
  setPodcastSearch: (value: string) => void;
  setTagSearch: (value: string) => void;
  setIsTagFilterExpanded: (value: boolean) => void;
  setNewCustomUrl: (value: string) => void;
  loadMorePodcasts: () => void;
  addCustomUrl: () => void;
  removeCustomUrl: (index: number) => void;

  // 验证和提交
  validateCurrentStep: () => boolean;
  submit: () => Promise<void>;
}

// Cron表达式校验函数
const validateCronExpression = (
  cronExpr: string,
): { valid: boolean; error?: string } => {
  const trimmed = cronExpr.trim();
  const parts = trimmed.split(/\s+/);

  // 检查位数（支持5位或6位）
  if (parts.length !== 5 && parts.length !== 6) {
    return {
      valid: false,
      error: "Cron表达式必须包含5段或6段（秒 分 时 日 月 周）",
    };
  }

  // 检查每段是否为通配符、数字、范围、间隔或列表
  const validPatterns = [
    /^\*$/, // 通配符
    /^\d+$/, // 单个数字
    /^\d+-\d+$/, // 范围 (如 1-5)
    /^\*\/\d+$/, // 间隔 (如 */6)
    /^\d+(,\d+)+$/, // 列表 (如 1,3,5)
  ];

  for (let i = 0; i < parts.length; i++) {
    if (!validPatterns.some((pattern) => pattern.test(parts[i]))) {
      return {
        valid: false,
        error: `第 ${i + 1} 段 "${parts[i]}" 格式不正确（支持: * 数字 范围1-5 间隔*/6 列表1,3,5）`,
      };
    }
  }

  return { valid: true };
};

export function useWorkflowForm({
  workflow,
  isOpen,
}: UseWorkflowFormProps): UseWorkflowFormReturn {
  const [step, setStep] = useState<Step>(1);
  const [loading, setLoading] = useState(false);

  // 表单数据状态
  const [formData, setFormData] = useState<WorkflowFormData>({
    // Step 1
    name: "",
    description: "",
    schedule: "0 0 2 * * *",
    customCron: "",

    // Step 2
    scopeType: "all_subscribed",
    selectedPodcastIds: [],
    customUrls: [],
    selectedTagIds: [],

    // Step 3
    timeRange: 0,
    minDuration: 0,
    maxResults: 0,
    keywords: "",
    excludeWords: "",

    // LLM
    llmEnabled: false,
    llmMaxEpisodes: 20,
    llmModel: "",
    llmTemperature: 0.7,
    llmMaxTokens: 1000,
    llmUserPrompt: "",
  });

  // Step 1 相关状态
  const [cronError, setCronError] = useState("");

  // Step 2 相关状态
  const [podcasts, setPodcasts] = useState<Podcast[]>([]);
  const [podcastSearch, setPodcastSearchState] = useState("");
  const [candidatePodcastIds, setCandidatePodcastIds] = useState<number[]>([]);
  const [isLoadingPodcasts, setIsLoadingPodcasts] = useState(false);
  const [displayedCount, setDisplayedCount] = useState(50);
  const loadMoreTriggerRef = useRef<HTMLDivElement>(null);

  // Refs用于Intersection Observer
  const displayedCountRef = useRef(displayedCount);
  const podcastsRef = useRef(podcasts);
  const podcastSearchRef = useRef(podcastSearch);
  const selectedTagIdsRef = useRef(formData.selectedTagIds);

  // 同步refs
  useEffect(() => {
    displayedCountRef.current = displayedCount;
  }, [displayedCount]);
  useEffect(() => {
    podcastsRef.current = podcasts;
  }, [podcasts]);
  useEffect(() => {
    podcastSearchRef.current = podcastSearch;
  }, [podcastSearch]);
  useEffect(() => {
    selectedTagIdsRef.current = formData.selectedTagIds;
  }, [formData.selectedTagIds]);

  // 标签相关状态
  const [tags, setTags] = useState<Tag[]>([]);
  const [tagSearch, setTagSearchState] = useState("");
  const [isTagFilterExpanded, setIsTagFilterExpanded] = useState(false);
  const [isLoadingTags, setIsLoadingTags] = useState(false);
  const [newCustomUrl, setNewCustomUrlState] = useState("");

  // 初始化表单数据（编辑模式）
  useEffect(() => {
    if (isOpen) {
      if (tags.length === 0) {
        loadTags();
      }

      if (workflow) {
        // 编辑模式：填充现有数据
        setFormData({
          name: workflow.name,
          description: workflow.description || "",
          schedule: workflow.schedule,
          customCron: CRON_PRESETS.some((p) => p.value === workflow.schedule)
            ? ""
            : workflow.schedule,
          scopeType: workflow.scope_config.type,
          selectedPodcastIds: workflow.scope_config.podcast_ids || [],
          customUrls: workflow.scope_config.custom_urls || [],
          selectedTagIds: workflow.scope_config.tag_ids || [],
          timeRange: workflow.rules_config.time_range || 0,
          minDuration: workflow.rules_config.min_duration || 0,
          maxResults: workflow.rules_config.max_results || 0,
          keywords: workflow.rules_config.keywords || "",
          excludeWords: workflow.rules_config.exclude_words || "",
          llmEnabled: workflow.rules_config.llm_enabled || false,
          llmMaxEpisodes: workflow.rules_config.llm_max_episodes || 20,
          llmModel: workflow.rules_config.llm_model || "",
          llmTemperature: workflow.rules_config.llm_temperature || 0.7,
          llmMaxTokens: workflow.rules_config.llm_max_tokens || 1000,
          llmUserPrompt: workflow.rules_config.llm_user_prompt || "",
        });
      } else {
        // 新建模式：重置为默认值
        setFormData({
          name: "",
          description: "",
          schedule: "0 0 2 * * *",
          customCron: "",
          scopeType: "all_subscribed",
          selectedPodcastIds: [],
          customUrls: [],
          selectedTagIds: [],
          timeRange: 0,
          minDuration: 0,
          maxResults: 0,
          keywords: "",
          excludeWords: "",
          llmEnabled: false,
          llmMaxEpisodes: 20,
          llmModel: "",
          llmTemperature: 0.7,
          llmMaxTokens: 1000,
          llmUserPrompt: "",
        });
      }

      setStep(1);
      setCronError("");
      setPodcastSearchState("");
      setCandidatePodcastIds([]);
      setDisplayedCount(50);
    }
  }, [isOpen, workflow, tags.length]);

  // 加载播客列表
  useEffect(() => {
    if (isOpen && formData.scopeType !== "custom_urls") {
      loadPodcasts();
    }
  }, [isOpen, formData.scopeType]);

  // 性能优化：无限滚动加载更多播客
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        if (
          entries[0].isIntersecting &&
          displayedCountRef.current < podcastsRef.current.length
        ) {
          setDisplayedCount((prev) =>
            Math.min(prev + 50, podcastsRef.current.length),
          );
        }
      },
      { threshold: 0.1 },
    );

    const currentTrigger = loadMoreTriggerRef.current;
    if (currentTrigger) {
      observer.observe(currentTrigger);
    }

    return () => {
      if (currentTrigger) {
        observer.unobserve(currentTrigger);
      }
    };
  }, [podcasts]);

  // 加载标签
  const loadTags = async () => {
    setIsLoadingTags(true);
    try {
      const data = await tagApi.getTags();
      setTags(data);
    } catch (error) {
      console.error("[useWorkflowForm] Failed to load tags:", error);
    } finally {
      setIsLoadingTags(false);
    }
  };

  // 加载播客
  const loadPodcasts = async (searchTerm?: string) => {
    setIsLoadingPodcasts(true);
    try {
      const data = await podcastApi.getPodcasts({
        page: 1,
        page_size: 1000,
        search: searchTerm || undefined,
      });
      setPodcasts(data);
      setDisplayedCount(Math.min(50, data.length));
    } catch (error) {
      console.error("[useWorkflowForm] Failed to load podcasts:", error);
    } finally {
      setIsLoadingPodcasts(false);
    }
  };

  // 更新表单字段
  const updateField = useCallback(
    <K extends keyof WorkflowFormData>(
      field: K,
      value: WorkflowFormData[K],
    ) => {
      setFormData((prev) => ({ ...prev, [field]: value }));
    },
    [],
  );

  // 导出的搜索设置器（带防抖）
  const setPodcastSearch = useCallback((value: string) => {
    setPodcastSearchState(value);
    // 防抖加载
    const timer = setTimeout(() => {
      loadPodcasts(value);
    }, 300);
    return () => clearTimeout(timer);
  }, []);

  const setTagSearch = useCallback((value: string) => {
    setTagSearchState(value);
  }, []);

  const setIsTagFilterExpandedSetter = useCallback((value: boolean) => {
    setIsTagFilterExpanded(value);
  }, []);

  const setNewCustomUrl = useCallback((value: string) => {
    setNewCustomUrlState(value);
  }, []);

  // 加载更多播客
  const loadMorePodcasts = useCallback(() => {
    setDisplayedCount((prev) => Math.min(prev + 50, podcasts.length));
  }, [podcasts.length]);

  // 添加自定义URL
  const addCustomUrl = useCallback(() => {
    if (
      newCustomUrl.trim() &&
      !formData.customUrls.includes(newCustomUrl.trim())
    ) {
      updateField("customUrls", [...formData.customUrls, newCustomUrl.trim()]);
      setNewCustomUrlState("");
    }
  }, [newCustomUrl, formData.customUrls, updateField]);

  // 移除自定义URL
  const removeCustomUrl = useCallback(
    (index: number) => {
      const newUrls = formData.customUrls.filter((_, i) => i !== index);
      updateField("customUrls", newUrls);
    },
    [formData.customUrls, updateField],
  );

  // 验证当前步骤
  const validateCurrentStep = useCallback((): boolean => {
    switch (step) {
      case 1:
        // 验证基本信息
        if (!formData.name.trim()) {
          setCronError("请输入工作流名称");
          return false;
        }

        // 验证Cron表达式
        const scheduleToUse = formData.customCron || formData.schedule;
        const validation = validateCronExpression(scheduleToUse);
        if (!validation.valid) {
          setCronError(validation.error || "Cron表达式格式不正确");
          return false;
        }

        setCronError("");
        return true;

      case 2:
        // 验证范围配置
        if (
          formData.scopeType === "selected" &&
          formData.selectedPodcastIds.length === 0
        ) {
          alert("请至少选择一个节目");
          return false;
        }
        if (
          formData.scopeType === "custom_urls" &&
          formData.customUrls.length === 0
        ) {
          alert("请至少添加一个自定义RSS地址");
          return false;
        }
        return true;

      case 3:
        // 规则配置可选，无需验证
        return true;

      default:
        return true;
    }
  }, [
    step,
    formData,
    formData.customCron,
    formData.schedule,
    formData.selectedPodcastIds,
    formData.customUrls,
  ]);

  // 下一步
  const nextStep = useCallback(() => {
    if (validateCurrentStep()) {
      if (step < 4) {
        setStep((step + 1) as Step);
      }
    }
  }, [step, validateCurrentStep]);

  // 上一步
  const prevStep = useCallback(() => {
    if (step > 1) {
      setStep((step - 1) as Step);
    }
  }, [step]);

  // 提交表单
  const submit = useCallback(async () => {
    if (!validateCurrentStep()) {
      return;
    }

    setLoading(true);
    try {
      const scheduleToUse = formData.customCron || formData.schedule;

      const scopeConfig: ScopeConfig = {
        type: formData.scopeType,
        podcast_ids:
          formData.scopeType === "selected"
            ? formData.selectedPodcastIds
            : undefined,
        custom_urls:
          formData.scopeType === "custom_urls"
            ? formData.customUrls
            : undefined,
        tag_ids:
          formData.scopeType === "by_tags"
            ? formData.selectedTagIds
            : undefined,
      };

      const rulesConfig: RulesConfig = {
        time_range: formData.timeRange || undefined,
        min_duration: formData.minDuration || undefined,
        max_results: formData.maxResults || undefined,
        keywords: formData.keywords || undefined,
        exclude_words: formData.excludeWords || undefined,
        llm_enabled: formData.llmEnabled,
        llm_max_episodes: formData.llmMaxEpisodes,
        llm_model: formData.llmModel || undefined,
        llm_temperature: formData.llmTemperature,
        llm_max_tokens: formData.llmMaxTokens,
        llm_user_prompt: formData.llmUserPrompt || DEFAULT_USER_PROMPT,
      };

      const request: WorkflowRequest = {
        name: formData.name,
        description: formData.description,
        schedule: scheduleToUse,
        scope_config: scopeConfig,
        rules_config: rulesConfig,
      };

      if (workflow) {
        await workflowApi.updateWorkflow(workflow.id, request);
      } else {
        await workflowApi.createWorkflow(request);
      }

      setLoading(false);
      // 调用成功回调（由父组件处理）
      return true;
    } catch (error) {
      console.error("[useWorkflowForm] Failed to submit workflow:", error);
      alert(
        "提交失败：" + (error instanceof Error ? error.message : "未知错误"),
      );
      setLoading(false);
      return false;
    }
  }, [formData, workflow, validateCurrentStep]);

  return {
    // 状态
    step,
    loading,
    formData,

    // Step 1
    cronError,
    cronPresets: CRON_PRESETS,

    // Step 2
    podcasts,
    podcastSearch,
    candidatePodcastIds,
    isLoadingPodcasts,
    displayedCount,
    tags,
    tagSearch,
    isTagFilterExpanded,
    isLoadingTags,
    newCustomUrl,
    loadMoreTriggerRef,

    // 操作
    nextStep,
    prevStep,
    updateField,
    setPodcastSearch,
    setTagSearch,
    setIsTagFilterExpanded: setIsTagFilterExpandedSetter,
    setNewCustomUrl,
    loadMorePodcasts,
    addCustomUrl,
    removeCustomUrl,

    // 验证和提交
    validateCurrentStep,
    submit,
  };
}
