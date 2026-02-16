"use client";

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
import PodcastCover from "@/components/podcasts/PodcastCover";
import { PodcastListItem } from "./PodcastListItem";

type Step = 1 | 2 | 3 | 4;

interface WorkflowFormModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  workflow?: Workflow | null; // 如果提供，则为编辑模式
}

// 预设的Cron表达式（6位格式：秒 分 时 日 月 周）
const CRON_PRESETS = [
  { label: "每天凌晨2点", value: "0 0 2 * * *" },
  { label: "每天早上8点", value: "0 0 8 * * *" },
  { label: "每天晚上8点", value: "0 0 20 * * *" },
  { label: "每周日凌晨2点", value: "0 0 2 * * 0" },
  { label: "每周一早上6点", value: "0 0 6 * * 1" },
];

// 默认User Prompt模板（包含任务要求和格式要求）
const DEFAULT_USER_PROMPT = `# 工作流执行报告

工作流名称: {{.WorkflowName}}
匹配的单集总数: {{.TotalEpisodes}}
节目数量: {{.NumPodcasts}}

## 数据来源

{{range .Podcasts}}
### {{.PodcastTitle}}
单集数: {{len .Episodes}}
{{range .Episodes}}
- **{{.Title}}** ({{.PublishedDate.Format "2006-01-02"}})
  {{if ne .ShowNotes ""}}{{.ShowNotes}}{{end}}
{{end}}
{{end}}

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

1. 统一使用bullet point（-），所有列表项不要使用数字序号
2. **核心内容部分必须使用嵌套bullet列表**，格式示例：
   - **AI前沿**（一级bullet）下面列出该节目的单集要点（二级bullet）
   - **高能量**（一级bullet）下面列出该节目的单集要点（二级bullet）
3. 列表项之间保持一个换行即可
4. 避免连续的多个空行
5. 用简洁专业的语言生成摘要，避免过度解读
6. 客观准确，不添加原文没有的信息
7. 简洁明了，避免冗余表述`;

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

export default function WorkflowFormModal({
  isOpen,
  onClose,
  onSuccess,
  workflow,
}: WorkflowFormModalProps) {
  const [step, setStep] = useState<Step>(1);
  const [loading, setLoading] = useState(false);

  // Step 1: 基本信息
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [schedule, setSchedule] = useState("0 0 2 * *");
  const [customCron, setCustomCron] = useState("");
  const [lastCustomCron, setLastCustomCron] = useState(""); // 保存上一次的自定义Cron
  const [cronError, setCronError] = useState("");

  // Step 2: 范围配置
  const [scopeType, setScopeType] =
    useState<WorkflowScopeType>("all_subscribed");
  const [selectedPodcastIds, setSelectedPodcastIds] = useState<number[]>([]);
  const [customUrls, setCustomUrls] = useState<string[]>([]);
  const [newCustomUrl, setNewCustomUrl] = useState("");
  const [podcasts, setPodcasts] = useState<Podcast[]>([]);
  const [podcastSearch, setPodcastSearch] = useState("");
  const [candidatePodcastIds, setCandidatePodcastIds] = useState<number[]>([]); // 备选列表中的节目ID
  const [isLoadingPodcasts, setIsLoadingPodcasts] = useState(false); // 加载状态

  // 性能优化：渐进式渲染状态
  const [displayedCount, setDisplayedCount] = useState(50); // 初始显示50个
  const loadMoreTriggerRef = useRef<HTMLDivElement>(null); // 无限滚动触发器

  // 使用 ref 跟踪 displayedCount，避免在 Intersection Observer 依赖项中包含它
  const displayedCountRef = useRef(displayedCount);

  // 保持 ref 同步
  useEffect(() => {
    displayedCountRef.current = displayedCount;
  }, [displayedCount]);

  // 同步 podcasts ref（供 Intersection Observer 使用）
  const podcastsRef = useRef(podcasts);
  useEffect(() => {
    podcastsRef.current = podcasts;
  }, [podcasts]);

  // 同步 podcastSearch ref（供 Intersection Observer 使用）
  const podcastSearchRef = useRef(podcastSearch);
  useEffect(() => {
    podcastSearchRef.current = podcastSearch;
  }, [podcastSearch]);

  // 标签筛选相关状态
  const [tags, setTags] = useState<Tag[]>([]);
  const [selectedTagIds, setSelectedTagIds] = useState<number[]>([]);
  const [tagSearch, setTagSearch] = useState("");
  const [isTagFilterExpanded, setIsTagFilterExpanded] = useState(false);
  const [isLoadingTags, setIsLoadingTags] = useState(false);

  // 移动端标签切换状态（Step 2）
  const [mobileTab, setMobileTab] = useState<"search" | "selected">("search");

  // 同步 selectedTagIds ref（供 Intersection Observer 使用）
  // 注意：必须定义在 selectedTagIds state 之后
  const selectedTagIdsRef = useRef(selectedTagIds);
  useEffect(() => {
    selectedTagIdsRef.current = selectedTagIds;
  }, [selectedTagIds]);

  // Step 3: 规则配置
  const [timeRange, setTimeRange] = useState(0);
  const [minDuration, setMinDuration] = useState(0);
  const [maxResults, setMaxResults] = useState(0);
  const [keywords, setKeywords] = useState("");
  const [excludeWords, setExcludeWords] = useState("");

  // LLM智能摘要配置
  const [llmEnabled, setLlmEnabled] = useState(false);
  const [llmMaxEpisodes, setLlmMaxEpisodes] = useState(20);
  const [llmModel, setLlmModel] = useState("");
  const [llmTemperature, setLlmTemperature] = useState(0.7);
  const [llmMaxTokens, setLlmMaxTokens] = useState(1000);
  const [llmUserPrompt, setLlmUserPrompt] = useState(""); // User Prompt配置

  // 初始化表单数据（编辑模式）
  useEffect(() => {
    if (isOpen) {
      // 加载标签（仅在标签列表为空时加载一次）
      if (tags.length === 0) {
        loadTags();
      }

      if (workflow) {
        // 编辑模式：填充现有数据
        console.log("[WorkflowFormModal] Loading workflow for edit:", workflow);
        console.log(
          "[WorkflowFormModal] Workflow rules_config:",
          workflow.rules_config,
        );
        setName(workflow.name);
        setDescription(workflow.description || "");

        // 判断 schedule 是预设还是自定义
        const isPresetSchedule = CRON_PRESETS.some(
          (preset) => preset.value === workflow.schedule,
        );
        if (isPresetSchedule) {
          setSchedule(workflow.schedule);
          setCustomCron("");
        } else {
          // 自定义cron表达式
          setCustomCron(workflow.schedule);
          setSchedule("");
        }

        setScopeType(workflow.scope_type);

        if (workflow.scope_config) {
          if (workflow.scope_config.podcast_ids) {
            // 编辑模式：将已保存的节目ID设置到candidatePodcastIds（备选列表）
            setCandidatePodcastIds(workflow.scope_config.podcast_ids);
          }
          if (workflow.scope_config.custom_urls) {
            setCustomUrls(workflow.scope_config.custom_urls);
          }
        }

        if (workflow.rules_config) {
          setTimeRange(workflow.rules_config.time_range || 0);
          setMinDuration(workflow.rules_config.min_duration || 0);
          setMaxResults(workflow.rules_config.max_results || 0);
          setKeywords(workflow.rules_config.keywords || "");
          setExcludeWords(workflow.rules_config.exclude_words || "");

          // LLM配置 - 添加调试日志
          console.log("[WorkflowFormModal] Loading LLM config from workflow:", {
            llm_enabled: workflow.rules_config.llm_enabled,
            llm_max_episodes: workflow.rules_config.llm_max_episodes,
            llm_model: workflow.rules_config.llm_model,
            llm_temperature: workflow.rules_config.llm_temperature,
            llm_max_tokens: workflow.rules_config.llm_max_tokens,
          });

          setLlmEnabled(workflow.rules_config.llm_enabled || false);
          setLlmMaxEpisodes(workflow.rules_config.llm_max_episodes || 20);
          setLlmModel(workflow.rules_config.llm_model || "");
          setLlmTemperature(workflow.rules_config.llm_temperature ?? 0.7);
          setLlmMaxTokens(workflow.rules_config.llm_max_tokens || 1000);
          setLlmUserPrompt(workflow.rules_config.llm_user_prompt || "");
        }

        // 编辑模式下，如果是指定节目类型，立即加载podcasts
        // 这样可以在用户进入第2步时就已经准备好了数据
        if (workflow.scope_type === "specific_podcasts") {
          loadPodcasts();
        }
      } else {
        // 创建模式：重置为默认值
        resetForm();
      }
    }
  }, [isOpen, workflow]);

  // 重置表单
  const resetForm = () => {
    setName("");
    setDescription("");
    setSchedule("0 0 2 * * *");
    setCustomCron("");
    setCronError("");
    setScopeType("all_subscribed");
    setSelectedPodcastIds([]);
    setCandidatePodcastIds([]);
    setCustomUrls([]);
    setNewCustomUrl("");
    setPodcasts([]);
    setPodcastSearch("");
    setSelectedTagIds([]);
    setTagSearch("");
    setIsTagFilterExpanded(false);
    setTimeRange(0);
    setMinDuration(0);
    setMaxResults(0);
    setKeywords("");
    setExcludeWords("");
    setLlmEnabled(false);
    setLlmMaxEpisodes(20);
    setLlmModel("");
    setLlmTemperature(0.7);
    setLlmMaxTokens(1000);
    setStep(1);
  };

  // 加载播客列表 - 一次性加载所有播客
  const loadPodcasts = async () => {
    try {
      setIsLoadingPodcasts(true);

      let allPodcasts: Podcast[] = [];
      let page = 1;
      let hasMore = true;
      const maxPages = 20; // 最多加载20页（2000个节目）

      // 分页加载所有节目
      while (hasMore && page <= maxPages) {
        const response = await podcastApi.list({ page, page_size: 100 });
        const newPodcasts = response.data || [];

        allPodcasts = [...allPodcasts, ...newPodcasts];

        // 如果返回的数量少于 page_size，说明已经是最后一页
        if (newPodcasts.length < 100) {
          hasMore = false;
        } else {
          page++;
        }
      }

      if (page > maxPages && hasMore) {
        console.warn(
          "[CreateWorkflowModal] Reached max pages limit, some podcasts may not be loaded",
        );
      }

      setPodcasts(allPodcasts);

      // 初始显示50个
      setDisplayedCount(Math.min(50, allPodcasts.length));
    } catch (err) {
      console.error("[CreateWorkflowModal] Failed to load podcasts:", err);
      alert(
        "加载节目失败: " + (err instanceof Error ? err.message : "未知错误"),
      );
    } finally {
      setIsLoadingPodcasts(false);
    }
  };

  // 显示更多已加载的播客（滚动到底部时）
  const showMoreLoadedPodcasts = useCallback(() => {
    // 每次增加50个显示，但不超过已加载的总数
    setDisplayedCount((prev) => Math.min(prev + 50, podcasts.length));
  }, [podcasts.length]);

  // 计算是否有更多内容需要显示
  const hasMoreContent = useMemo(() => {
    const filteredCount = podcasts.filter((p) => {
      if (selectedTagIds.length > 0) {
        const podcastTagIds = p.tags?.map((t) => t.id) || [];
        if (!selectedTagIds.every((tagId) => podcastTagIds.includes(tagId))) {
          return false;
        }
      }
      if (!podcastSearch.trim()) return true;
      const searchLower = podcastSearch.toLowerCase().trim();
      return (
        (p.title || "").toLowerCase().includes(searchLower) ||
        (p.author || "").toLowerCase().includes(searchLower)
      );
    }).length;

    return filteredCount > displayedCount;
  }, [podcasts, selectedTagIds, podcastSearch, displayedCount]);

  // 无限滚动逻辑（Intersection Observer）
  useEffect(() => {
    // 如果还在加载中，等待加载完成后再设置 Observer
    if (isLoadingPodcasts) {
      return;
    }

    // 使用 ref 存储 observer
    const observerRef = { current: null as IntersectionObserver | null };

    // 设置 Observer 的函数
    const setupObserver = () => {
      const target = loadMoreTriggerRef.current;
      if (!target) {
        // 再次尝试
        setTimeout(setupObserver, 50);
        return;
      }

      const observer = new IntersectionObserver(
        (entries) => {
          if (entries[0].isIntersecting) {
            // 使用函数式更新，基于当前 state 计算
            setDisplayedCount((prevDisplayed) => {
              // 使用最新的 ref 值计算筛选结果数量
              const currentPodcasts = podcastsRef.current;
              const currentSelectedTagIds = selectedTagIdsRef.current;
              const currentPodcastSearch = podcastSearchRef.current;

              // 动态计算当前筛选结果数量
              const currentFilteredCount = currentPodcasts.filter((p) => {
                // 标签筛选
                if (currentSelectedTagIds.length > 0) {
                  const podcastTagIds = p.tags?.map((t) => t.id) || [];
                  if (
                    !currentSelectedTagIds.every((tagId) =>
                      podcastTagIds.includes(tagId),
                    )
                  ) {
                    return false;
                  }
                }
                // 搜索筛选
                if (!currentPodcastSearch.trim()) return true;
                const searchLower = currentPodcastSearch.toLowerCase().trim();
                return (
                  (p.title || "").toLowerCase().includes(searchLower) ||
                  (p.author || "").toLowerCase().includes(searchLower)
                );
              }).length;

              // 只有当真的有更多内容时才更新
              if (prevDisplayed < currentFilteredCount) {
                const newValue = Math.min(
                  prevDisplayed + 50,
                  currentFilteredCount,
                );
                // 同步更新 ref
                displayedCountRef.current = newValue;
                return newValue;
              }
              return prevDisplayed;
            });
          }
        },
        { rootMargin: "200px" },
      );

      observer.observe(target);
      observerRef.current = observer;
    };

    // 使用 requestAnimationFrame 确保 DOM 渲染完成
    requestAnimationFrame(() => {
      setTimeout(setupObserver, 0);
    });

    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect();
      }
    };
  }, [isLoadingPodcasts, selectedTagIds, podcastSearch]); // ✅ 添加筛选条件依赖，确保它们变化时重建 Observer

  // 当搜索或筛选条件变化时，重置 displayedCount
  useEffect(() => {
    setDisplayedCount(50);
    // 同步更新 ref
    displayedCountRef.current = 50;
  }, [selectedTagIds, podcastSearch]);

  // 加载标签列表
  const loadTags = async () => {
    try {
      setIsLoadingTags(true);
      const allTags = await tagApi.list();
      setTags(allTags);
    } catch (err) {
      console.error("[CreateWorkflowModal] Failed to load tags:", err);
    } finally {
      setIsLoadingTags(false);
    }
  };

  // 处理自定义URL添加
  const handleAddCustomUrl = () => {
    if (newCustomUrl.trim() && !customUrls.includes(newCustomUrl.trim())) {
      setCustomUrls([...customUrls, newCustomUrl.trim()]);
      setNewCustomUrl("");
    }
  };

  // 添加节目到备选列表 - 使用useCallback优化
  const handleAddToCandidate = useCallback(
    (podcastId: number) => {
      if (!candidatePodcastIds.includes(podcastId)) {
        setCandidatePodcastIds((prev) => [...prev, podcastId]);
      }
    },
    [candidatePodcastIds],
  );

  // 从备选列表移除节目 - 使用useCallback优化
  const handleRemoveFromCandidate = useCallback((podcastId: number) => {
    setCandidatePodcastIds((prev) => prev.filter((id) => id !== podcastId));
  }, []);

  // 批量添加搜索结果到备选列表 - 优化为异步分批处理
  const handleAddAllFiltered = useCallback(async () => {
    // 在函数内部计算过滤结果，避免依赖顺序问题
    const filtered = podcasts.filter((p) => {
      // 标签筛选
      if (selectedTagIds.length > 0) {
        const podcastTagIds = p.tags?.map((t) => t.id) || [];
        const hasAllTags = selectedTagIds.every((tagId) =>
          podcastTagIds.includes(tagId),
        );
        if (!hasAllTags) return false;
      }

      // 搜索筛选
      if (!podcastSearch.trim()) return true;
      const searchLower = podcastSearch.toLowerCase().trim();
      return (
        (p.title || "").toLowerCase().includes(searchLower) ||
        (p.author || "").toLowerCase().includes(searchLower)
      );
    });

    const filteredIds = filtered.slice(0, displayedCount).map((p) => p.id);

    // 分批添加（每批50个），避免阻塞UI
    for (let i = 0; i < filteredIds.length; i += 50) {
      const batch = filteredIds.slice(i, i + 50);
      setCandidatePodcastIds((prev) => [...new Set([...prev, ...batch])]);
      // 让出主线程，避免UI卡顿
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  }, [podcasts, selectedTagIds, podcastSearch, displayedCount]);

  // 处理自定义URL删除
  const handleRemoveCustomUrl = (url: string) => {
    setCustomUrls(customUrls.filter((u) => u !== url));
  };

  // 验证当前步骤
  const validateStep = (): boolean => {
    if (step === 1) {
      if (!name.trim()) {
        alert("请输入工作流名称");
        return false;
      }

      // 检查实际使用的cron表达式（自定义输入优先，否则用预设）
      const actualCron = customCron.trim() || schedule.trim();
      if (!actualCron) {
        setCronError("请选择或输入定时规则");
        return false;
      }

      // 校验 cron 格式
      const validation = validateCronExpression(actualCron);
      if (!validation.valid) {
        setCronError(validation.error || "Cron表达式格式错误");
        return false;
      }

      // 清除错误信息（校验通过时）
      setCronError("");
    }
    if (step === 2) {
      if (
        scopeType === "specific_podcasts" &&
        candidatePodcastIds.length === 0
      ) {
        alert("请至少添加一个节目到备选列表");
        return false;
      }
      if (scopeType === "custom_sources" && customUrls.length === 0) {
        alert("请至少添加一个RSS源");
        return false;
      }
    }
    return true;
  };

  // 处理下一步
  const handleNext = async () => {
    if (!validateStep()) return;

    if (
      step === 2 &&
      scopeType === "specific_podcasts" &&
      podcasts.length === 0
    ) {
      await loadPodcasts();
    }

    if (step < 4) {
      setStep((step + 1) as Step);
    } else {
      await handleSubmit();
    }
  };

  // 处理上一步
  const handlePrev = () => {
    if (step > 1) {
      setStep((step - 1) as Step);
    }
  };

  // 将5位cron格式转换为6位格式（添加秒字段）
  const convertToSixDigitCron = (cronExpr: string): string => {
    const trimmed = cronExpr.trim();
    const parts = trimmed.split(/\s+/);

    // 如果已经是6位格式，直接返回
    if (parts.length === 6) {
      return trimmed;
    }

    // 如果是5位格式（分 时 日 月 周），在前面添加 "0" 秒
    if (parts.length === 5) {
      return `0 ${trimmed}`;
    }

    // 其他情况（不合法的格式），原样返回让后端验证
    return trimmed;
  };

  // 提交创建
  const handleSubmit = async () => {
    try {
      setLoading(true);

      const scopeConfig: ScopeConfig = {};
      if (scopeType === "specific_podcasts") {
        scopeConfig.podcast_ids = candidatePodcastIds;
      } else if (scopeType === "custom_sources") {
        scopeConfig.custom_urls = customUrls;
      }

      const rulesConfig: RulesConfig = {
        time_range: timeRange || undefined,
        min_duration: minDuration || undefined,
        max_results: maxResults || undefined,
        keywords: keywords.trim() || undefined,
        exclude_words: excludeWords.trim() || undefined,
        // LLM配置 - 只在启用时发送值
        llm_enabled: llmEnabled,
        llm_max_episodes: llmEnabled ? llmMaxEpisodes : undefined,
        llm_model: llmEnabled && llmModel ? llmModel : undefined,
        llm_temperature: llmEnabled ? llmTemperature : undefined,
        llm_max_tokens: llmEnabled ? llmMaxTokens : undefined,
        // User Prompt配置 - 只在用户明确输入时才提交
        ...(llmEnabled && llmUserPrompt && { llm_user_prompt: llmUserPrompt }),
      };

      // 获取实际使用的cron表达式并转换为6位格式
      const actualCron = customCron.trim() || schedule.trim();
      const finalCron = convertToSixDigitCron(actualCron);

      console.log("[WorkflowFormModal] Cron conversion:", {
        original: actualCron,
        converted: finalCron,
        isCustom: !!customCron.trim(),
      });

      // 如果 cron 表达式有效,自动启用调度
      const shouldBeEnabled = actualCron.length > 0;

      const data: WorkflowRequest = {
        name: name.trim(),
        description: description.trim(),
        schedule: finalCron,
        scope_type: scopeType,
        scope_config: scopeConfig,
        rules_config: rulesConfig,
        is_enabled: shouldBeEnabled,
      };

      console.log("[WorkflowFormModal] Submitting workflow:", data);
      console.log(
        "[WorkflowFormModal] LLM Config in rules_config:",
        data.rules_config,
      );

      if (workflow) {
        // 编辑模式
        await workflowApi.update(workflow.id, data);
        console.log("[WorkflowFormModal] Workflow updated successfully");
      } else {
        // 创建模式
        await workflowApi.create(data);
        console.log("[WorkflowFormModal] Workflow created successfully");
      }

      onSuccess();
      handleClose();
    } catch (err) {
      console.error("[WorkflowFormModal] Submit failed:", err);
      const errorMessage = err instanceof Error ? err.message : "Unknown error";
      alert(`${workflow ? "更新" : "创建"}失败: ${errorMessage}`);
    } finally {
      setLoading(false);
    }
  };

  // 关闭Modal
  const handleClose = () => {
    setStep(1);
    setName("");
    setDescription("");
    setSchedule("0 0 2 * * *");
    setCustomCron("");
    setCronError("");
    setScopeType("all_subscribed");
    setSelectedPodcastIds([]);
    setCandidatePodcastIds([]);
    setCustomUrls([]);
    setNewCustomUrl("");
    setPodcasts([]);
    setPodcastSearch("");
    setSelectedTagIds([]);
    setTagSearch("");
    setIsTagFilterExpanded(false);
    setTimeRange(0);
    setMinDuration(0);
    setMaxResults(0);
    setKeywords("");
    setExcludeWords("");
    setLlmEnabled(false);
    setLlmMaxEpisodes(20);
    setLlmModel("");
    setLlmTemperature(0.7);
    setLlmMaxTokens(1000);
    onClose();
  };

  // 过滤播客列表（支持搜索和标签筛选） - 使用useMemo优化
  const filteredPodcasts = useMemo(() => {
    return podcasts.filter((p) => {
      // 标签筛选（AND逻辑：必须包含所有选中的标签）
      if (selectedTagIds.length > 0) {
        const podcastTagIds = p.tags?.map((t) => t.id) || [];
        const hasAllTags = selectedTagIds.every((tagId) =>
          podcastTagIds.includes(tagId),
        );
        if (!hasAllTags) return false;
      }

      // 搜索筛选
      if (!podcastSearch.trim()) return true;

      const searchLower = podcastSearch.toLowerCase().trim();
      const title = (p.title || "").toLowerCase();
      const author = (p.author || "").toLowerCase();

      const titleMatch = title.includes(searchLower);
      const authorMatch = author.includes(searchLower);

      return titleMatch || authorMatch;
    });
  }, [podcasts, selectedTagIds, podcastSearch]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-black/50 z-[60] flex items-start sm:items-center justify-center p-0 sm:p-4 overflow-y-auto">
      <div className="bg-white dark:bg-slate-800 rounded-none sm:rounded-lg shadow-2xl w-full max-w-3xl min-h-[100dvh] sm:min-h-0 sm:max-h-[85vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="border-b border-slate-200 dark:border-slate-700 p-4 sm:p-6 shrink-0">
          <div className="flex items-center justify-between">
            <h2 className="text-xl sm:text-2xl font-bold text-slate-900 dark:text-slate-50">
              {workflow ? "编辑工作流" : "创建工作流"} ({step}/4)
            </h2>
            <button
              onClick={handleClose}
              className="w-11 h-11 flex items-center justify-center text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 text-2xl rounded-lg hover:bg-slate-100 dark:hover:bg-slate-700 active:bg-slate-200 dark:active:bg-slate-600 transition-colors"
              aria-label="关闭"
            >
              ×
            </button>
          </div>
          {/* Progress Bar */}
          <div className="mt-4 flex gap-2">
            <div
              className={`flex-1 h-1 rounded ${step >= 1 ? "bg-blue-600" : "bg-slate-200 dark:bg-slate-700"}`}
            />
            <div
              className={`flex-1 h-1 rounded ${step >= 2 ? "bg-blue-600" : "bg-slate-200 dark:bg-slate-700"}`}
            />
            <div
              className={`flex-1 h-1 rounded ${step >= 3 ? "bg-blue-600" : "bg-slate-200 dark:bg-slate-700"}`}
            />
            <div
              className={`flex-1 h-1 rounded ${step >= 4 ? "bg-blue-600" : "bg-slate-200 dark:bg-slate-700"}`}
            />
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {step === 1 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  工作流名称 <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="例如: 每日科技播客抓取"
                  className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  描述
                </label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="简要描述这个工作流的用途..."
                  rows={3}
                  className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  定时规则 <span className="text-red-500">*</span>
                </label>
                <div className="space-y-3">
                  {/* 预设按钮区域 */}
                  <div className="grid grid-cols-1 gap-2 transition-all duration-200">
                    {CRON_PRESETS.map((preset) => (
                      <button
                        key={preset.value}
                        type="button"
                        onClick={() => {
                          setSchedule(preset.value);
                          // 保存上一次的自定义Cron（如果有值的话）
                          if (customCron.trim()) {
                            setLastCustomCron(customCron);
                          }
                          setCustomCron(""); // 清空自定义输入，切换到预设模式
                          setCronError(""); // 清除错误
                        }}
                        className={`
                          px-4 py-3 rounded-lg text-left transition-all border-2
                          flex items-center gap-3
                          ${
                            schedule === preset.value && !customCron.trim()
                              ? "border-blue-500 bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300"
                              : "border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:border-slate-300 dark:hover:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-700"
                          }
                        `}
                      >
                        <span className="text-2xl">
                          {preset.label.includes("凌晨") ||
                          preset.label.includes("早上")
                            ? "🌅"
                            : preset.label.includes("晚上")
                              ? "🌙"
                              : preset.label.includes("周")
                                ? "📅"
                                : "⏰"}
                        </span>
                        <div className="flex-1">
                          <div className="font-medium">{preset.label}</div>
                          <div className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                            {preset.value}
                          </div>
                        </div>
                        {schedule === preset.value && !customCron && (
                          <span className="text-blue-500 text-xl">✓</span>
                        )}
                      </button>
                    ))}
                  </div>

                  {/* 分隔线 */}
                  <div className="flex items-center gap-3">
                    <div className="flex-1 h-px bg-slate-200 dark:bg-slate-700"></div>
                    <span className="text-xs text-slate-400 dark:text-slate-500">
                      或
                    </span>
                    <div className="flex-1 h-px bg-slate-200 dark:bg-slate-700"></div>
                  </div>

                  {/* 自定义输入区域 */}
                  <div className="relative">
                    {/* 模式指示器 */}
                    {(customCron.trim() ||
                      (schedule && !customCron.trim())) && (
                      <div
                        className={`
                        absolute -top-2.5 left-3 z-10 px-2 py-0.5 text-xs font-medium rounded-full border transition-all
                        ${
                          customCron.trim()
                            ? "bg-purple-100 dark:bg-purple-900/30 border-purple-300 dark:border-purple-700 text-purple-700 dark:text-purple-300"
                            : "bg-blue-100 dark:bg-blue-900/30 border-blue-300 dark:border-blue-700 text-blue-700 dark:text-blue-300"
                        }
                      `}
                      >
                        {customCron.trim() ? "✏️ 自定义" : "📋 预设"}
                      </div>
                    )}

                    <div className="relative">
                      <input
                        type="text"
                        value={customCron}
                        onChange={(e) => {
                          setCustomCron(e.target.value);
                          if (e.target.value) {
                            setSchedule("");
                          }
                          // 输入时清除错误
                          if (cronError) setCronError("");
                        }}
                        onFocus={() => {
                          // 如果输入框为空，但有历史记录，则恢复历史输入
                          if (!customCron && lastCustomCron) {
                            setCustomCron(lastCustomCron);
                            setSchedule(""); // 清空预设选择
                          }
                          // 如果customCron为空（没有历史记录），则清空预设
                          if (!customCron && !lastCustomCron && schedule) {
                            setSchedule("");
                          }
                        }}
                        placeholder={
                          customCron
                            ? ""
                            : "自定义Cron表达式，如: 0 */6 * * * (每6小时)"
                        }
                        className={`
                          w-full px-4 py-3 border-2 rounded-lg transition-all
                          ${
                            customCron.trim()
                              ? "border-purple-500 bg-purple-50 dark:bg-purple-900/20 ring-2 ring-purple-200 dark:ring-purple-800 text-slate-900 dark:text-slate-100"
                              : cronError
                                ? "border-red-500 bg-white dark:bg-slate-700"
                                : "border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-700 hover:border-slate-400 dark:hover:border-slate-500 text-slate-900 dark:text-slate-100"
                          }
                        `}
                      />

                      {/* 清除按钮 */}
                      {customCron.trim() && (
                        <button
                          type="button"
                          onClick={() => {
                            setCustomCron("");
                            setCronError("");
                          }}
                          className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 transition-colors"
                          title="清除自定义表达式"
                        >
                          ✕
                        </button>
                      )}
                    </div>
                    {cronError && (
                      <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                        {cronError}
                      </p>
                    )}
                  </div>
                  {!customCron && lastCustomCron && (
                    <p className="mt-1 text-xs text-purple-600 dark:text-purple-400">
                      💡 点击输入框可恢复上次输入:{" "}
                      <code className="bg-purple-50 dark:bg-purple-900/30 px-1 py-0.5 rounded text-purple-700 dark:text-purple-300">
                        {lastCustomCron}
                      </code>
                    </p>
                  )}
                  <p className="text-xs text-slate-500 dark:text-slate-400">
                    支持5位格式（分 时 日 月 周）或6位格式（秒 分 时 日 月
                    周），系统会自动转换
                  </p>
                </div>
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                  选择要处理的节目范围 <span className="text-red-500">*</span>
                </label>
                <div className="space-y-3">
                  <label className="flex items-start gap-3 p-4 sm:p-3 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-900 active:bg-slate-100 dark:active:bg-slate-800 cursor-pointer transition-colors">
                    <input
                      type="radio"
                      name="scopeType"
                      checked={scopeType === "all_subscribed"}
                      onChange={() => setScopeType("all_subscribed")}
                      className="mt-1 w-5 h-5 flex-shrink-0"
                    />
                    <div>
                      <div className="font-medium text-slate-900 dark:text-slate-50">
                        全部已订阅节目
                      </div>
                      <div className="text-sm text-slate-600 dark:text-slate-400">
                        处理所有订阅的播客节目
                      </div>
                    </div>
                  </label>

                  <label className="flex items-start gap-3 p-4 sm:p-3 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-900 active:bg-slate-100 dark:active:bg-slate-800 cursor-pointer transition-colors">
                    <input
                      type="radio"
                      name="scopeType"
                      checked={scopeType === "specific_podcasts"}
                      onChange={() => {
                        setScopeType("specific_podcasts");
                        if (podcasts.length === 0) loadPodcasts();
                      }}
                      className="mt-1 w-5 h-5 flex-shrink-0"
                    />
                    <div className="flex-1">
                      <div className="font-medium text-slate-900 dark:text-slate-50">
                        指定节目
                      </div>
                      <div className="text-sm text-slate-600 dark:text-slate-400 mb-2">
                        从订阅中选择特定节目
                      </div>
                      {scopeType === "specific_podcasts" && (
                        <div className="space-y-3">
                          {/* 加载指示器 */}
                          {isLoadingPodcasts && (
                            <div className="flex items-center gap-2 text-sm text-blue-600 dark:text-blue-400">
                              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600"></div>
                              <span>正在加载节目数据...</span>
                            </div>
                          )}

                          {/* 可折叠标签栏 */}
                          <div className="border border-slate-200 dark:border-slate-700 rounded-lg overflow-hidden">
                            {/* 折叠状态的控制栏 */}
                            <div
                              className="flex items-center justify-between px-3 py-2 bg-slate-50 dark:bg-slate-900 cursor-pointer hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
                              onClick={() =>
                                setIsTagFilterExpanded(!isTagFilterExpanded)
                              }
                            >
                              <div className="flex items-center gap-2">
                                <span className="text-sm">🏷️</span>
                                <span className="text-sm font-medium text-slate-700 dark:text-slate-300">
                                  按标签筛选
                                </span>
                                {selectedTagIds.length > 0 && (
                                  <span className="text-xs bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 px-2 py-0.5 rounded-full">
                                    已选: {selectedTagIds.length}个
                                  </span>
                                )}
                              </div>
                              <div className="flex items-center gap-2">
                                {selectedTagIds.length > 0 && (
                                  <button
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      setSelectedTagIds([]);
                                    }}
                                    className="text-xs text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 px-2 py-1 rounded hover:bg-red-50 dark:hover:bg-red-900/20"
                                  >
                                    清除
                                  </button>
                                )}
                                <span className="text-slate-500 dark:text-slate-400">
                                  {isTagFilterExpanded ? "▲" : "▼"}
                                </span>
                              </div>
                            </div>

                            {/* 展开状态的内容 */}
                            {isTagFilterExpanded && (
                              <div className="p-3 border-t border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
                                {/* 标签搜索框 */}
                                <div className="relative mb-3">
                                  <input
                                    type="text"
                                    value={tagSearch}
                                    onChange={(e) =>
                                      setTagSearch(e.target.value)
                                    }
                                    placeholder="搜索标签..."
                                    disabled={isLoadingTags}
                                    className="w-full px-3 py-2 pr-8 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-700 text-sm disabled:opacity-50"
                                  />
                                  {tagSearch && !isLoadingTags && (
                                    <button
                                      onClick={() => setTagSearch("")}
                                      className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                                    >
                                      ✕
                                    </button>
                                  )}
                                </div>

                                {/* 标签列表 */}
                                <div className="max-h-60 overflow-y-auto">
                                  {isLoadingTags ? (
                                    <div className="text-center text-slate-500 dark:text-slate-400 py-4 text-sm">
                                      加载标签中...
                                    </div>
                                  ) : tags.length === 0 ? (
                                    <div className="text-center text-slate-500 dark:text-slate-400 py-4 text-sm">
                                      暂无标签
                                    </div>
                                  ) : (
                                    <div className="flex flex-wrap gap-2">
                                      {tags
                                        .filter((tag) => {
                                          if (!tagSearch.trim()) return true;
                                          return tag.name
                                            .toLowerCase()
                                            .includes(tagSearch.toLowerCase());
                                        })
                                        .sort(
                                          (a, b) =>
                                            (b.podcast_count || 0) -
                                            (a.podcast_count || 0),
                                        )
                                        .map((tag) => {
                                          const isSelected =
                                            selectedTagIds.includes(tag.id);
                                          return (
                                            <button
                                              key={tag.id}
                                              type="button"
                                              onClick={() => {
                                                if (isSelected) {
                                                  setSelectedTagIds(
                                                    selectedTagIds.filter(
                                                      (id) => id !== tag.id,
                                                    ),
                                                  );
                                                } else {
                                                  setSelectedTagIds([
                                                    ...selectedTagIds,
                                                    tag.id,
                                                  ]);
                                                }
                                              }}
                                              className={`
                                                flex items-center gap-1.5 px-3 py-1.5 rounded-full text-sm border transition-all
                                                ${
                                                  isSelected
                                                    ? "bg-blue-100 dark:bg-blue-900/30 border-blue-300 dark:border-blue-700 text-blue-700 dark:text-blue-300"
                                                    : "bg-white dark:bg-slate-700 border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-600"
                                                }
                                              `}
                                            >
                                              <span
                                                className="w-2.5 h-2.5 rounded-full border border-slate-300 dark:border-slate-500"
                                                style={{
                                                  backgroundColor: tag.color,
                                                }}
                                              />
                                              <span>{tag.name}</span>
                                              {tag.podcast_count !==
                                                undefined && (
                                                <span
                                                  className={`
                                                  text-xs
                                                  ${isSelected ? "text-blue-600 dark:text-blue-400" : "text-slate-500 dark:text-slate-400"}
                                                `}
                                                >
                                                  ({tag.podcast_count})
                                                </span>
                                              )}
                                              {isSelected && (
                                                <span className="ml-1 text-blue-600 dark:text-blue-400">
                                                  ✓
                                                </span>
                                              )}
                                            </button>
                                          );
                                        })}
                                    </div>
                                  )}
                                </div>
                              </div>
                            )}
                          </div>

                          {/* 搜索框 */}
                          <div className="relative">
                            <input
                              type="text"
                              value={podcastSearch}
                              onChange={(e) => setPodcastSearch(e.target.value)}
                              placeholder="搜索节目名称或作者..."
                              disabled={isLoadingPodcasts}
                              className="w-full px-3 py-2 pr-8 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-700 text-sm disabled:opacity-50"
                            />
                            {podcastSearch && !isLoadingPodcasts && (
                              <button
                                onClick={() => setPodcastSearch("")}
                                className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                              >
                                ✕
                              </button>
                            )}
                          </div>

                          {/* 使用提示 */}
                          {filteredPodcasts.length > 0 &&
                            !isLoadingPodcasts && (
                              <div className="text-xs text-slate-600 dark:text-slate-400 bg-slate-50 dark:bg-slate-900/30 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700">
                                💡 <strong>提示：</strong>
                                点击列表项选择节目，或点击中间的"全部添加"按钮批量加入
                              </div>
                            )}

                          {/* 移动端标签栏 */}
                          <div className="sm:hidden flex border-b border-slate-200 dark:border-slate-700 mb-3">
                            <button
                              type="button"
                              onClick={() => setMobileTab("search")}
                              className={`flex-1 py-3 text-sm font-medium transition-colors ${
                                mobileTab === "search"
                                  ? "text-blue-600 dark:text-blue-400 border-b-2 border-blue-600 dark:border-blue-400"
                                  : "text-slate-500 dark:text-slate-400"
                              }`}
                            >
                              📻 搜索 ({filteredPodcasts.length})
                            </button>
                            <button
                              type="button"
                              onClick={() => setMobileTab("selected")}
                              className={`flex-1 py-3 text-sm font-medium transition-colors ${
                                mobileTab === "selected"
                                  ? "text-blue-600 dark:text-blue-400 border-b-2 border-blue-600 dark:border-blue-400"
                                  : "text-slate-500 dark:text-slate-400"
                              }`}
                            >
                              ✅ 已选 ({candidatePodcastIds.length})
                            </button>
                          </div>

                          {/* 移动端底部操作栏 */}
                          <div className="sm:hidden sticky bottom-0 bg-white dark:bg-slate-800 border-t border-slate-200 dark:border-slate-700 p-3 flex gap-2 -mx-6 -mb-6 mt-4 z-10">
                            {filteredPodcasts.length > 0 && !isLoadingPodcasts && (
                              <button
                                type="button"
                                onClick={handleAddAllFiltered}
                                className="flex-1 min-h-[44px] bg-blue-600 text-white rounded-lg text-sm font-medium flex items-center justify-center gap-2"
                              >
                                <span>≫</span>
                                添加全部 ({filteredPodcasts.length})
                              </button>
                            )}
                            {candidatePodcastIds.length > 0 && (
                              <button
                                type="button"
                                onClick={() => setCandidatePodcastIds([])}
                                className="min-h-[44px] px-4 border border-red-300 dark:border-red-700 text-red-600 dark:text-red-400 rounded-lg text-sm font-medium"
                              >
                                清空
                              </button>
                            )}
                          </div>

                          {/* 三栏布局 - 桌面端固定高度，移动端根据tab显示 */}
                          <div className="grid grid-cols-1 sm:grid-cols-12 gap-3 transition-all duration-200">
                            {/* 左侧：搜索结果列表 */}
                            <div className={`col-span-1 sm:col-span-5 ${mobileTab !== "search" ? "hidden sm:block" : ""}`}>
                              <div className="hidden sm:flex items-center gap-1.5 text-xs font-medium text-slate-700 dark:text-slate-300 mb-2">
                                <span>📻</span>
                                <span>搜索结果</span>
                                <span className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-700 rounded-full text-xs">
                                  {filteredPodcasts.length}
                                </span>
                              </div>
                              <div className="h-[50vh] sm:h-80 overflow-y-auto border border-slate-200 dark:border-slate-700 rounded-lg p-2 bg-white dark:bg-slate-800 transition-all duration-200 hover:border-slate-300 dark:hover:border-slate-600">
                                {isLoadingPodcasts ? (
                                  <div className="text-center text-slate-500 dark:text-slate-400 py-4 text-xs">
                                    加载中...
                                  </div>
                                ) : filteredPodcasts.length === 0 ? (
                                  <div className="text-center text-slate-500 dark:text-slate-400 py-4 text-xs">
                                    {podcasts.length === 0 ? (
                                      <button
                                        onClick={() => loadPodcasts()}
                                        className="text-blue-600 dark:text-blue-400 hover:underline"
                                      >
                                        点击加载节目
                                      </button>
                                    ) : podcastSearch ? (
                                      `没有找到匹配"${podcastSearch}"的节目`
                                    ) : (
                                      "显示所有 " + podcasts.length + " 个节目"
                                    )}
                                  </div>
                                ) : (
                                  <>
                                    {/* 结果过多提示 */}
                                    {filteredPodcasts.length > 100 && (
                                      <div className="text-xs text-amber-600 dark:text-amber-400 px-2 py-1 bg-amber-50 dark:bg-amber-900/20 rounded mb-2">
                                        💡
                                        结果较多，建议使用标签或更精确的搜索词
                                      </div>
                                    )}

                                    {filteredPodcasts
                                      .slice(0, displayedCount)
                                      .map((podcast, index) => (
                                        <PodcastListItem
                                          key={podcast.id}
                                          podcast={podcast}
                                          isSelected={candidatePodcastIds.includes(
                                            podcast.id,
                                          )}
                                          onAdd={handleAddToCandidate}
                                          onRemove={handleRemoveFromCandidate}
                                          index={index}
                                        />
                                      ))}

                                    {/* 加载更多触发器 - 始终渲染，用 CSS 控制显示 */}
                                    <div
                                      ref={loadMoreTriggerRef}
                                      className="py-2 text-center"
                                      style={{
                                        display: hasMoreContent
                                          ? "block"
                                          : "none",
                                      }}
                                    >
                                      <div className="text-xs text-slate-400 dark:text-slate-500">
                                        向下滚动显示更多 ({displayedCount} /{" "}
                                        {filteredPodcasts.length})
                                      </div>
                                    </div>
                                  </>
                                )}
                              </div>
                            </div>

                            {/* 中间：批量操作按钮 - 仅桌面端显示 */}
                            <div className="hidden sm:flex col-span-2 flex-col justify-center gap-3">
                              {filteredPodcasts.length > 0 &&
                                !isLoadingPodcasts && (
                                  <button
                                    onClick={handleAddAllFiltered}
                                    className="group w-11 h-11 flex flex-col items-center justify-center text-xs bg-blue-50 dark:bg-blue-900/20 text-blue-700 dark:text-blue-300 rounded-lg hover:bg-blue-100 dark:hover:bg-blue-900/30 mx-auto border-2 border-blue-200 dark:border-blue-800 transition-all duration-200 hover:scale-105"
                                    title="添加所有搜索结果"
                                  >
                                    <span className="text-base font-bold group-hover:translate-x-0.5 transition-transform">
                                      ≫
                                    </span>
                                    <span className="text-[10px] leading-tight mt-0.5">
                                      全部添加
                                    </span>
                                  </button>
                                )}
                              {candidatePodcastIds.length > 0 && (
                                <button
                                  onClick={() => setCandidatePodcastIds([])}
                                  className="group w-11 h-11 flex flex-col items-center justify-center text-xs bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 rounded-lg hover:bg-red-100 dark:hover:bg-red-900/30 mx-auto border-2 border-red-200 dark:border-red-800 transition-all duration-200 hover:scale-105"
                                  title="清空备选列表"
                                >
                                  <span className="text-sm font-bold group-hover:rotate-90 transition-transform">
                                    ✕
                                  </span>
                                  <span className="text-[10px] leading-tight mt-0.5">
                                    清空
                                  </span>
                                </button>
                              )}
                            </div>

                            {/* 右侧：备选列表 */}
                            <div className={`col-span-1 sm:col-span-5 ${mobileTab !== "selected" ? "hidden sm:block" : ""}`}>
                              <div className="hidden sm:flex items-center gap-1.5 text-xs font-medium text-slate-700 dark:text-slate-300 mb-2">
                                <span>✅</span>
                                <span>已选</span>
                                <span className="px-1.5 py-0.5 bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 rounded-full text-xs">
                                  {candidatePodcastIds.length}
                                </span>
                              </div>
                              <div className="h-[50vh] sm:h-80 overflow-y-auto border-2 border-green-200 dark:border-green-800 rounded-lg p-2 bg-green-50/50 dark:bg-green-900/10 transition-all duration-200 hover:border-green-300 dark:hover:border-green-700">
                                {candidatePodcastIds.length === 0 ? (
                                  <div className="flex flex-col items-center justify-center h-full py-8 text-center">
                                    <div className="text-3xl mb-2 opacity-50">
                                      👈
                                    </div>
                                    <div className="text-sm text-slate-500 dark:text-slate-400">
                                      从搜索结果选择节目
                                    </div>
                                    <div className="text-xs text-slate-400 dark:text-slate-500 mt-1">
                                      切换到"搜索"标签
                                    </div>
                                  </div>
                                ) : (
                                  candidatePodcastIds.map((id) => {
                                    const podcast = podcasts.find(
                                      (p) => p.id === id,
                                    );
                                    if (!podcast) {
                                      console.warn(
                                        "[CreateWorkflowModal] Podcast not found for ID:",
                                        id,
                                      );
                                      return null;
                                    }
                                    return (
                                      <PodcastListItem
                                        key={podcast.id}
                                        podcast={podcast}
                                        isSelected={true}
                                        onAdd={handleAddToCandidate}
                                        onRemove={handleRemoveFromCandidate}
                                        index={0}
                                      />
                                    );
                                  })
                                )}
                              </div>
                            </div>
                          </div>
                        </div>
                      )}
                    </div>
                  </label>

                  <label className="flex items-start gap-3 p-4 sm:p-3 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-900 active:bg-slate-100 dark:active:bg-slate-800 cursor-pointer transition-colors">
                    <input
                      type="radio"
                      name="scopeType"
                      checked={scopeType === "custom_sources"}
                      onChange={() => setScopeType("custom_sources")}
                      className="mt-1 w-5 h-5 flex-shrink-0"
                    />
                    <div className="flex-1">
                      <div className="font-medium text-slate-900 dark:text-slate-50">
                        自定义RSS源
                      </div>
                      <div className="text-sm text-slate-600 dark:text-slate-400 mb-2">
                        添加自定义RSS源URL
                      </div>
                      {scopeType === "custom_sources" && (
                        <div className="space-y-2">
                          <div className="flex gap-2">
                            <input
                              type="url"
                              value={newCustomUrl}
                              onChange={(e) => setNewCustomUrl(e.target.value)}
                              onKeyPress={(e) =>
                                e.key === "Enter" && handleAddCustomUrl()
                              }
                              placeholder="输入RSS URL，按回车添加"
                              className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-700 text-sm"
                            />
                            <button
                              type="button"
                              onClick={handleAddCustomUrl}
                              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 text-sm"
                            >
                              添加
                            </button>
                          </div>
                          {customUrls.length > 0 && (
                            <div className="space-y-1">
                              {customUrls.map((url) => (
                                <div
                                  key={url}
                                  className="flex items-center gap-2 p-2 bg-slate-50 dark:bg-slate-900 rounded"
                                >
                                  <span className="text-xs flex-1 truncate">
                                    {url}
                                  </span>
                                  <button
                                    type="button"
                                    onClick={() => handleRemoveCustomUrl(url)}
                                    className="text-red-600 hover:text-red-700 text-sm"
                                  >
                                    删除
                                  </button>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  </label>
                </div>
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-4">
                  抓取规则配置 (可选)
                </label>

                {/* 基础规则 - 默认展开 */}
                <details open className="group mb-4">
                  <summary className="flex items-center justify-between cursor-pointer p-4 bg-slate-50 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors">
                    <div className="flex items-center gap-2">
                      <span className="text-lg">📋</span>
                      <span className="font-medium text-slate-700 dark:text-slate-300">
                        基础规则
                      </span>
                      <span className="text-xs text-slate-500 dark:text-slate-400">
                        (常用设置)
                      </span>
                    </div>
                    <span className="text-slate-400 group-open:rotate-180 transition-transform">
                      ▼
                    </span>
                  </summary>

                  <div className="mt-4 space-y-4 p-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
                    {/* 时间范围 */}
                    <div>
                      <label className="block text-sm text-slate-600 dark:text-slate-400 mb-2">
                        时间范围 (天)
                      </label>
                      <div className="flex gap-2 mb-2">
                        {[0, 1, 7, 30].map((days) => (
                          <button
                            key={days}
                            type="button"
                            onClick={() => setTimeRange(days)}
                            className={`px-3 py-1.5 rounded text-sm transition-colors ${
                              timeRange === days
                                ? "bg-blue-600 text-white"
                                : "bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600"
                            }`}
                          >
                            {days === 0 ? "不限" : `${days}天`}
                          </button>
                        ))}
                      </div>
                      <input
                        type="number"
                        inputMode="numeric"
                        pattern="[0-9]*"
                        min={0}
                        value={timeRange || ""}
                        onChange={(e) =>
                          setTimeRange(parseInt(e.target.value) || 0)
                        }
                        placeholder="0"
                        className="w-full px-4 py-2 text-base border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                      />
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        只抓取指定天数内发布的单集，0表示不限制
                      </p>
                    </div>

                    {/* 最小时长 */}
                    <div>
                      <label className="block text-sm text-slate-600 dark:text-slate-400 mb-2">
                        最小时长 (分钟)
                      </label>
                      <div className="flex gap-2 mb-2">
                        {[0, 600, 1800, 3600].map((seconds) => (
                          <button
                            key={seconds}
                            type="button"
                            onClick={() => setMinDuration(seconds)}
                            className={`px-3 py-1.5 rounded text-sm transition-colors ${
                              minDuration === seconds
                                ? "bg-blue-600 text-white"
                                : "bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600"
                            }`}
                          >
                            {seconds === 0
                              ? "不限"
                              : seconds === 600
                                ? "10分钟"
                                : seconds === 1800
                                  ? "30分钟"
                                  : "60分钟"}
                          </button>
                        ))}
                      </div>
                      <input
                        type="number"
                        inputMode="numeric"
                        pattern="[0-9]*"
                        min={0}
                        value={minDuration || ""}
                        onChange={(e) =>
                          setMinDuration(parseInt(e.target.value) || 0)
                        }
                        placeholder="0"
                        className="w-full px-4 py-2 text-base border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                      />
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        只抓取超过此时长的单集，0表示不限制（单位：秒）
                      </p>
                    </div>
                  </div>
                </details>

                {/* 高级规则 - 默认折叠 */}
                <details className="group">
                  <summary className="flex items-center justify-between cursor-pointer p-4 bg-slate-50 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors">
                    <div className="flex items-center gap-2">
                      <span className="text-lg">⚙️</span>
                      <span className="font-medium text-slate-700 dark:text-slate-300">
                        高级规则
                      </span>
                      <span className="text-xs text-slate-500 dark:text-slate-400">
                        (可选设置)
                      </span>
                    </div>
                    <span className="text-slate-400 group-open:rotate-180 transition-transform">
                      ▼
                    </span>
                  </summary>

                  <div className="mt-4 space-y-4 p-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
                    {/* 最大结果数 */}
                    <div>
                      <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                        最大结果数
                      </label>
                      <input
                        type="number"
                        inputMode="numeric"
                        pattern="[0-9]*"
                        min={0}
                        value={maxResults || ""}
                        onChange={(e) =>
                          setMaxResults(parseInt(e.target.value) || 0)
                        }
                        placeholder="0"
                        className="w-full px-4 py-2 text-base border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                      />
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        每个节目最多抓取的单集数量，0表示不限制
                      </p>
                    </div>

                    {/* 关键词过滤 */}
                    <div>
                      <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                        关键词过滤
                      </label>
                      <input
                        type="text"
                        value={keywords}
                        onChange={(e) => setKeywords(e.target.value)}
                        placeholder="例如: 技术,AI,机器学习"
                        className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                      />
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        只抓取标题或简介中包含这些关键词的单集，逗号分隔
                      </p>
                    </div>

                    {/* 排除词 */}
                    <div>
                      <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                        排除词
                      </label>
                      <input
                        type="text"
                        value={excludeWords}
                        onChange={(e) => setExcludeWords(e.target.value)}
                        placeholder="例如: 广告,推广"
                        className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                      />
                      <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                        排除标题或简介中包含这些词的单集，逗号分隔
                      </p>
                    </div>
                  </div>
                </details>
              </div>

              <div className="mt-6 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                <p className="text-sm text-blue-800 dark:text-blue-200">
                  ℹ️ 下一步可配置大模型智能摘要（可选功能）
                </p>
              </div>
            </div>
          )}

          {step === 4 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-4">
                  🤖 大模型智能摘要 (可选)
                </label>
                <p className="text-sm text-slate-600 dark:text-slate-400 mb-4">
                  启用后将使用大模型为抓取的单集生成智能摘要，帮助你快速了解内容要点
                </p>

                <div className="flex items-center gap-3 mb-6 p-4 bg-purple-50 dark:bg-purple-900/20 border border-purple-200 dark:border-purple-800 rounded-lg">
                  <input
                    type="checkbox"
                    id="llm-enable"
                    checked={llmEnabled}
                    onChange={(e) => {
                      setLlmEnabled(e.target.checked);
                    }}
                    className="w-5 h-5 text-purple-600 border-slate-300 rounded focus:ring-purple-500 focus:ring-2"
                  />
                  <label htmlFor="llm-enable" className="flex-1 cursor-pointer">
                    <div className="font-medium text-slate-900 dark:text-slate-50">
                      启用智能摘要
                    </div>
                    <div className="text-sm text-slate-600 dark:text-slate-400">
                      自动为抓取的单集生成AI摘要
                    </div>
                  </label>
                  <span className="text-xs text-purple-600 dark:text-purple-400 bg-purple-100 dark:bg-purple-900/30 px-2 py-1 rounded">
                    实验性功能
                  </span>
                </div>

                {llmEnabled && (
                  <div className="space-y-4">
                    {/* 常用配置 - 直接显示 */}
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div className="p-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
                        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                          单次最大单集数
                        </label>
                        <input
                          type="number"
                          inputMode="numeric"
                          pattern="[0-9]*"
                          min={1}
                          max={100}
                          value={llmMaxEpisodes || ""}
                          onChange={(e) =>
                            setLlmMaxEpisodes(parseInt(e.target.value) || 20)
                          }
                          placeholder="20"
                          className="w-full px-4 py-2 text-base border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                        />
                        <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                          采样部分单集生成摘要（1-100）
                        </p>
                      </div>

                      <div className="p-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
                        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                          最大生成Token数
                        </label>
                        <input
                          type="number"
                          inputMode="numeric"
                          pattern="[0-9]*"
                          min={100}
                          max={4000}
                          value={llmMaxTokens || ""}
                          onChange={(e) =>
                            setLlmMaxTokens(parseInt(e.target.value) || 1000)
                          }
                          placeholder="1000"
                          className="w-full px-4 py-2 text-base border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                        />
                        <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                          控制摘要的最大长度（100-4000）
                        </p>
                      </div>
                    </div>

                    {/* 高级设置 - 默认折叠 */}
                    <details className="group">
                      <summary className="flex items-center justify-between cursor-pointer p-4 bg-slate-50 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors">
                        <div className="flex items-center gap-2">
                          <span className="text-lg">⚙️</span>
                          <span className="font-medium text-slate-700 dark:text-slate-300">
                            高级设置
                          </span>
                          <span className="text-xs text-slate-500 dark:text-slate-400">
                            (可选)
                          </span>
                        </div>
                        <span className="text-slate-400 group-open:rotate-180 transition-transform">
                          ▼
                        </span>
                      </summary>

                      <div className="mt-4 space-y-4 p-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
                        {/* LLM模型 */}
                        <div>
                          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                            LLM模型
                          </label>
                          <input
                            type="text"
                            value={llmModel}
                            onChange={(e) => setLlmModel(e.target.value)}
                            placeholder="留空使用默认模型"
                            className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                          />
                          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                            覆盖默认模型（如
                            Qwen/Qwen2.5-7B-Instruct），留空使用系统默认
                          </p>
                        </div>

                        {/* 创造性参数 */}
                        <div>
                          <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                            创造性参数: {llmTemperature.toFixed(1)}
                          </label>
                          <input
                            type="range"
                            min={0}
                            max={1}
                            step={0.1}
                            value={llmTemperature}
                            onChange={(e) =>
                              setLlmTemperature(parseFloat(e.target.value))
                            }
                            className="w-full"
                          />
                          <div className="flex justify-between text-xs text-slate-500 dark:text-slate-400 mt-1">
                            <span>更确定</span>
                            <span>更创造</span>
                          </div>
                        </div>
                      </div>
                    </details>

                    {/* 自定义Prompt - 高级功能，默认折叠 */}
                    <details
                      className="group"
                      onToggle={(e) => {
                        const details = e.currentTarget;
                        // 首次展开时，如果textarea为空，自动填充默认值
                        if (details.open && !llmUserPrompt) {
                          setLlmUserPrompt(DEFAULT_USER_PROMPT);
                        }
                      }}
                    >
                      <summary className="flex items-center justify-between cursor-pointer p-4 bg-slate-50 dark:bg-slate-900/50 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors">
                        <div className="flex items-center gap-2">
                          <span className="text-lg">📝</span>
                          <span className="font-medium text-slate-700 dark:text-slate-300">
                            自定义Prompt模板
                          </span>
                          <span className="text-xs text-orange-600 dark:text-orange-400 bg-orange-100 dark:bg-orange-900/30 px-2 py-1 rounded">
                            高级
                          </span>
                        </div>
                        <span className="text-slate-400 group-open:rotate-180 transition-transform">
                          ▼
                        </span>
                      </summary>

                      <div className="mt-4 p-4 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 rounded-lg">
                        {/* User Prompt配置 */}
                        <div>
                          <div className="flex items-center justify-between mb-2">
                            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300">
                              用户提示词模板（User Prompt）
                            </label>
                            <button
                              type="button"
                              onClick={() =>
                                setLlmUserPrompt(DEFAULT_USER_PROMPT)
                              }
                              className="text-xs text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                            >
                              恢复默认值
                            </button>
                          </div>
                          <textarea
                            value={llmUserPrompt}
                            onChange={(e) => setLlmUserPrompt(e.target.value)}
                            placeholder="留空使用默认用户提示词模板：定义分析任务和输出格式"
                            className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-100 text-sm font-mono resize-y min-h-[120px] max-h-[40vh] sm:max-h-none"
                            rows={6}
                          />
                          <p className="text-xs text-slate-500 dark:text-slate-400 mt-2">
                            💡 支持Go template语法，可用变量：WorkflowName,
                            TotalEpisodes, NumPodcasts, Podcasts
                          </p>
                          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                            💡
                            系统提示词（角色定义+安全约束）已全局配置，此处只需定义分析任务和输出格式。
                          </p>
                        </div>
                      </div>
                    </details>
                  </div>
                )}
              </div>

              <div className="mt-6 p-4 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
                <p className="text-sm text-green-800 dark:text-green-200">
                  ✓ 点击"保存"后将自动启用调度（根据设置的定时规则运行）
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="hidden sm:flex border-t border-slate-200 dark:border-slate-700 p-4 sm:p-6 justify-between">
          <button
            onClick={handleClose}
            className="px-4 sm:px-6 py-2 min-h-[44px] text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200"
          >
            取消
          </button>
          <div className="flex gap-3">
            {step > 1 && (
              <button
                onClick={handlePrev}
                className="px-4 sm:px-6 py-2 min-h-[44px] border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors"
              >
                上一步
              </button>
            )}
            <button
              onClick={handleNext}
              disabled={loading}
              className="px-4 sm:px-6 py-2 min-h-[44px] bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {loading ? "处理中..." : step === 4 ? "保存" : "下一步"}
            </button>
          </div>
        </div>

        {/* 移动端底部固定操作栏 */}
        <div className="sm:hidden shrink-0 bg-white dark:bg-slate-800 border-t border-slate-200 dark:border-slate-700">
          <div className="flex gap-3 p-4">
            <button
              onClick={handleClose}
              className="flex-1 min-h-[44px] px-4 py-2 text-slate-600 dark:text-slate-400 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors"
            >
              取消
            </button>
            {step > 1 && (
              <button
                onClick={handlePrev}
                className="flex-1 min-h-[44px] px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors"
              >
                上一步
              </button>
            )}
            <button
              onClick={handleNext}
              disabled={loading}
              className="flex-1 min-h-[44px] px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors font-medium"
            >
              {loading ? "处理中..." : step === 4 ? "保存" : "下一步"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
