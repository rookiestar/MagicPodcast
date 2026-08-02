import type {
  DiscoveryCandidate,
  DiscoveryPreRead,
  TriageDecisionState,
} from "@/types/discovery";
import type { Episode, Podcast, Tag } from "@/types";

export interface MockRequest {
  method: string;
  pathname: string;
  search?: string;
  body?: unknown;
}

export interface MockResponseBody {
  success: boolean;
  data?: any;
  pagination?: any;
  error?: {
    code: string;
    message: string;
  };
  [key: string]: any;
}

export interface MockResponse {
  status: number;
  body: MockResponseBody;
}

const MOCK_COVERS = [
  "/api/mock-cover/1",
  "/api/mock-cover/2",
  "/api/mock-cover/3",
  "/api/mock-cover/5",
] as const;
const MOCK_NOW = "2026-08-02T09:00:00+08:00";
const MOCK_TODAY = "2026-08-02";

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

let mockTags: Tag[] = [
  { id: 1, name: "技术与产品", color: "#A85432", podcast_count: 3 },
  { id: 2, name: "商业观察", color: "#526B58", podcast_count: 2 },
  { id: 3, name: "个人成长", color: "#B47C42", podcast_count: 2 },
  { id: 4, name: "文化与社会", color: "#6C6673", podcast_count: 1 },
];

const mockPodcasts: Podcast[] = [
  {
    id: 1,
    xyz_id: "mock-podcast-001",
    title: "复杂世界观察",
    description: "从技术、组织与日常经验出发，拆解正在变化的世界。",
    author: "编辑部",
    cover_url: MOCK_COVERS[0],
    feed_url: "https://example.com/mock/complex-world.xml",
    episode_count: 42,
    newest_episode_date: "2026-08-01T18:30:00+08:00",
    created_at: "2026-05-12T10:00:00+08:00",
    added_date: "2026-05-12T10:00:00+08:00",
    is_subscribed: true,
    is_dead: false,
    my_rate: 5,
    notes: "关注技术如何改变组织和人的工作方式。",
    tags: [mockTags[0], mockTags[1]],
  },
  {
    id: 2,
    xyz_id: "mock-podcast-002",
    title: "未来编辑部",
    description: "把新技术放回真实生活，讨论它如何进入我们的选择。",
    author: "未来编辑部",
    cover_url: MOCK_COVERS[1],
    feed_url: "https://example.com/mock/future-editors.xml",
    episode_count: 28,
    newest_episode_date: "2026-07-31T09:15:00+08:00",
    created_at: "2026-06-03T09:30:00+08:00",
    added_date: "2026-06-03T09:30:00+08:00",
    is_subscribed: true,
    is_dead: false,
    my_rate: 4,
    notes: "适合在周末集中浏览。",
    tags: [mockTags[0], mockTags[2]],
  },
  {
    id: 3,
    xyz_id: "mock-podcast-003",
    title: "慢速商业评论",
    description: "不追热点，回到行业结构、现金流和长期判断。",
    author: "商业观察室",
    cover_url: MOCK_COVERS[2],
    feed_url: "https://example.com/mock/slow-business.xml",
    episode_count: 19,
    newest_episode_date: "2026-07-29T20:00:00+08:00",
    created_at: "2026-06-18T15:00:00+08:00",
    added_date: "2026-06-18T15:00:00+08:00",
    is_subscribed: true,
    is_dead: false,
    my_rate: 4,
    notes: "重点看公司治理和产品策略。",
    tags: [mockTags[1]],
  },
  {
    id: 4,
    xyz_id: "mock-podcast-004",
    title: "日常的研究",
    description: "从身边的制度、关系和习惯，重新理解个人经验。",
    author: "研究小组",
    cover_url: MOCK_COVERS[3],
    feed_url: "https://example.com/mock/everyday-research.xml",
    episode_count: 33,
    newest_episode_date: "2026-07-26T11:45:00+08:00",
    created_at: "2026-07-01T11:00:00+08:00",
    added_date: "2026-07-01T11:00:00+08:00",
    is_subscribed: true,
    is_dead: false,
    my_rate: 5,
    notes: "适合作为知识库里的长期参考。",
    tags: [mockTags[2], mockTags[3]],
  },
];

const mockEpisodes: Episode[] = [
  {
    id: 1001,
    guid: "mock-episode-1001",
    podcast_id: 1,
    episode_no: "E42",
    title: "AI 代理进入组织之后，谁在重新定义工作",
    medium_url: "https://example.com/mock/audio-1001.mp3",
    show_notes: "<p>从任务分配、反馈循环和责任边界，讨论 AI 代理进入组织后的变化。</p>",
    published_date: "2026-08-01T18:30:00+08:00",
    duration: 3120,
    link: "https://example.com/mock/episode-1001",
    image_url: MOCK_COVERS[0],
    enclosure_type: "audio/mpeg",
    enclosure_length: 0,
    my_rate: 5,
    notes: "记录组织边界和责任转移的例子。",
  },
  {
    id: 2001,
    guid: "mock-episode-2001",
    podcast_id: 2,
    episode_no: "E28",
    title: "从工具到同事：我们如何适应新的协作关系",
    medium_url: "https://example.com/mock/audio-2001.mp3",
    show_notes: "<p>工具开始拥有判断和记忆之后，人类的协作方式会如何变化？</p>",
    published_date: "2026-07-31T09:15:00+08:00",
    duration: 2640,
    link: "https://example.com/mock/episode-2001",
    image_url: MOCK_COVERS[1],
    enclosure_type: "audio/mpeg",
    enclosure_length: 0,
    my_rate: 4,
    notes: "关注人机协作的具体场景。",
  },
  {
    id: 3001,
    guid: "mock-episode-3001",
    podcast_id: 3,
    episode_no: "E19",
    title: "增长叙事之外：一家公司的第二曲线",
    medium_url: "https://example.com/mock/audio-3001.mp3",
    show_notes: "<p>讨论成熟公司如何在增长放缓后寻找新的价值来源。</p>",
    published_date: "2026-07-29T20:00:00+08:00",
    duration: 2880,
    link: "https://example.com/mock/episode-3001",
    image_url: MOCK_COVERS[2],
    enclosure_type: "audio/mpeg",
    enclosure_length: 0,
    my_rate: 4,
    notes: "补充第二曲线和组织惯性的案例。",
  },
  {
    id: 4001,
    guid: "mock-episode-4001",
    podcast_id: 4,
    episode_no: "E33",
    title: "为什么我们总是高估效率，低估注意力",
    medium_url: "https://example.com/mock/audio-4001.mp3",
    show_notes: "<p>从日常安排、注意力和环境设计，重新审视效率感。</p>",
    published_date: "2026-07-26T11:45:00+08:00",
    duration: 2280,
    link: "https://example.com/mock/episode-4001",
    image_url: MOCK_COVERS[3],
    enclosure_type: "audio/mpeg",
    enclosure_length: 0,
    my_rate: 5,
    notes: "和个人知识管理的节奏有关。",
  },
];

function makePreReads(
  summary: string,
  viewpoints: string,
  relevant: string,
  challenge: string,
): DiscoveryPreRead[] {
  return [
    {
      kind: "summary",
      label: "摘要",
      status: "available",
      content: summary,
      sources: [{ kind: "episode", label: "单集原文" }],
      generated_at: MOCK_NOW,
      version: "mock-v1",
    },
    {
      kind: "viewpoints",
      label: "观点",
      status: "available",
      content: viewpoints,
      sources: [{ kind: "episode", label: "单集原文" }],
      generated_at: MOCK_NOW,
      version: "mock-v1",
    },
    {
      kind: "relevant",
      label: "与我相关",
      status: "available",
      content: relevant,
      relation_strength: "明确相关",
      sources: [{ kind: "library", label: "我的播客库" }],
      generated_at: MOCK_NOW,
      version: "mock-v1",
    },
    {
      kind: "challenge",
      label: "质疑",
      status: "insufficient",
      content: challenge,
      sources: [{ kind: "episode", label: "单集原文" }],
      generated_at: MOCK_NOW,
      version: "mock-v1",
    },
  ];
}

const mockCandidates: DiscoveryCandidate[] = [
  {
    episode_id: 101,
    podcast_id: 1,
    podcast_title: "复杂世界观察",
    podcast_author: "编辑部",
    podcast_cover_url: MOCK_COVERS[0],
    episode_title: "AI 代理进入组织之后，谁在重新定义工作",
    episode_no: "E42",
    duration: 3120,
    candidate_time: "2026-08-01T18:30:00+08:00",
    time_basis: "published_date",
    source: "最近更新",
    show_notes: mockEpisodes[0].show_notes,
    show_notes_status: "available",
    original_url: mockEpisodes[0].link,
    image_url: MOCK_COVERS[0],
    decision_state: "pending",
    pre_reads: makePreReads(
      "这一集把 AI 代理放进真实组织，而不是单独讨论模型能力。重点是任务如何被拆分、反馈如何回流，以及责任边界如何重新落位。",
      "当工具能够持续记忆并主动推进任务时，协作关系会从一次性交付转为长期配合；管理者需要重新设计检查点，而不是只增加审批。",
      "你最近在整理播客知识库和工作流，这一集可以直接对应到“哪些判断应该留在人的手里”这一问题。",
      "节目主要基于早期组织案例，尚不足以证明这些协作方式能迁移到所有团队。",
    ),
  },
  {
    episode_id: 102,
    podcast_id: 2,
    podcast_title: "未来编辑部",
    podcast_author: "未来编辑部",
    podcast_cover_url: MOCK_COVERS[1],
    episode_title: "从工具到同事：我们如何适应新的协作关系",
    episode_no: "E28",
    duration: 2640,
    candidate_time: "2026-07-31T09:15:00+08:00",
    time_basis: "published_date",
    source: "最近更新",
    show_notes: mockEpisodes[1].show_notes,
    show_notes_status: "available",
    original_url: mockEpisodes[1].link,
    image_url: MOCK_COVERS[1],
    decision_state: "pending",
    pre_reads: makePreReads(
      "节目从几个具体协作场景出发，讨论 AI 从被动工具变成可交接的协作者之后，人的工作边界会发生什么变化。",
      "协作的关键不只是让 AI 做更多，而是让交接、复核和撤回都变得可见。",
      "对你的播客知识库而言，最有价值的是把“可交接的上下文”留下，而不是只留下最后的答案。",
      "节目对失败案例的讨论较少，关于人类如何保留否决权仍然不够充分。",
    ),
  },
  {
    episode_id: 103,
    podcast_id: 3,
    podcast_title: "慢速商业评论",
    podcast_author: "商业观察室",
    podcast_cover_url: MOCK_COVERS[2],
    episode_title: "增长叙事之外：一家公司的第二曲线",
    episode_no: "E19",
    duration: 2880,
    candidate_time: "2026-07-29T20:00:00+08:00",
    time_basis: "published_date",
    source: "最近更新",
    show_notes: mockEpisodes[2].show_notes,
    show_notes_status: "available",
    original_url: mockEpisodes[2].link,
    image_url: MOCK_COVERS[2],
    decision_state: "pending",
    pre_reads: makePreReads(
      "这一集通过一家成熟公司的转型，讨论增长放缓后如何识别新的能力和新的价值来源。",
      "第二曲线不是另起炉灶，而是把已有能力迁移到新的问题上。",
      "可以和你库里的产品策略、组织变化类内容互相对照，形成一个长期主题。",
      "单个公司案例不能代表行业普遍规律，财务结果也需要更多外部材料验证。",
    ),
  },
  {
    episode_id: 104,
    podcast_id: 4,
    podcast_title: "日常的研究",
    podcast_author: "研究小组",
    podcast_cover_url: MOCK_COVERS[3],
    episode_title: "为什么我们总是高估效率，低估注意力",
    episode_no: "E33",
    duration: 2280,
    candidate_time: "2026-07-26T11:45:00+08:00",
    time_basis: "published_date",
    source: "最近更新",
    show_notes: mockEpisodes[3].show_notes,
    show_notes_status: "available",
    original_url: mockEpisodes[3].link,
    image_url: MOCK_COVERS[3],
    decision_state: "shortlisted",
    pre_reads: makePreReads(
      "节目从日常安排和注意力分配出发，重新讨论我们为何会把效率感误认为真正的进展。",
      "效率提升只有在目标稳定时才有意义；当目标本身不断变化，注意力的质量更值得被记录。",
      "它适合和你的知识库整理习惯放在一起看：整理不是增加更多动作，而是减少重复寻找。",
      "节目较少触及现实中的资源约束，建议结合自己的工作节奏判断。",
    ),
  },
];

const mockDecisionStates = new Map<number, TriageDecisionState>([
  [104, "shortlisted"],
]);

type MockWorkflow = {
  id: number;
  name: string;
  description: string;
  schedule: string;
  scope_type: string;
  scope_config: Record<string, any>;
  rules_config: Record<string, any>;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
  stats: {
    total_jobs: number;
    total_episodes: number;
    podcast_count: number;
    last_execution?: string;
    next_execution?: string;
  };
  last_job?: Record<string, any>;
};

let mockWorkflows: MockWorkflow[] = [
  {
    id: 1,
    name: "订阅更新入库",
    description: "同步订阅节目，保留新单集和原始 show notes。",
    schedule: "0 2 * * *",
    scope_type: "all_subscribed",
    scope_config: { podcast_ids: [] },
    rules_config: { time_range: 14, max_results: 30, llm_enabled: false },
    is_enabled: true,
    created_at: "2026-06-10T10:00:00+08:00",
    updated_at: "2026-08-01T22:10:00+08:00",
    stats: {
      total_jobs: 18,
      total_episodes: 86,
      podcast_count: 4,
      last_execution: "2026-08-01T02:04:00+08:00",
      next_execution: "2026-08-02T02:00:00+08:00",
    },
    last_job: {
      id: 18,
      workflow_id: 1,
      status: "completed",
      podcasts_processed: 4,
      episodes_found: 6,
      episodes_created: 4,
      episodes_matched: 2,
      error_count: 0,
      triggered_by: "cron",
      created_at: "2026-08-01T02:00:00+08:00",
      duration: 124000,
    },
  },
  {
    id: 2,
    name: "重点节目整理",
    description: "每周回看重点节目，整理可以进入知识库的内容。",
    schedule: "0 9 * * 1",
    scope_type: "specific_podcasts",
    scope_config: { podcast_ids: [1, 2] },
    rules_config: {
      time_range: 30,
      min_duration: 1200,
      max_results: 12,
      keywords: "组织,产品,AI",
      llm_enabled: false,
    },
    is_enabled: false,
    created_at: "2026-06-22T14:00:00+08:00",
    updated_at: "2026-07-28T16:40:00+08:00",
    stats: {
      total_jobs: 7,
      total_episodes: 21,
      podcast_count: 2,
      last_execution: "2026-07-27T09:18:00+08:00",
      next_execution: "2026-08-03T09:00:00+08:00",
    },
    last_job: {
      id: 7,
      workflow_id: 2,
      status: "partial",
      podcasts_processed: 2,
      episodes_found: 4,
      episodes_created: 0,
      episodes_matched: 4,
      error_count: 1,
      triggered_by: "cron",
      created_at: "2026-07-27T09:00:00+08:00",
      duration: 98000,
    },
  },
];

function ok(data: any, extra: Record<string, any> = {}): MockResponse {
  return { status: 200, body: { success: true, data, ...extra } };
}

function fail(status: number, code: string, message: string): MockResponse {
  return { status, body: { success: false, error: { code, message } } };
}

function bodyAsRecord(body: unknown): Record<string, any> {
  return body && typeof body === "object" ? (body as Record<string, any>) : {};
}

function pageNumber(query: URLSearchParams, key: string, fallback: number) {
  const value = Number(query.get(key));
  return Number.isFinite(value) && value > 0 ? Math.floor(value) : fallback;
}

function pageSize(query: URLSearchParams, key: string, fallback: number) {
  const value = Number(query.get(key));
  if (!Number.isFinite(value) || value <= 0) return fallback;
  return Math.min(Math.floor(value), 100);
}

function paginate<T>(items: T[], page: number, size: number) {
  const total = items.length;
  const totalPages = Math.max(1, Math.ceil(total / size));
  const start = (page - 1) * size;
  return {
    items: items.slice(start, start + size),
    pagination: {
      page,
      page_size: size,
      total,
      total_pages: totalPages,
    },
  };
}

function currentCandidates() {
  return clone(
    mockCandidates.map((candidate) => ({
      ...candidate,
      decision_state:
        mockDecisionStates.get(candidate.episode_id) ?? candidate.decision_state,
    })),
  );
}

function podcastByID(id: number) {
  return mockPodcasts.find((podcast) => podcast.id === id);
}

function episodeByID(id: number) {
  return mockEpisodes.find((episode) => episode.id === id);
}

function handlePodcasts(
  segments: string[],
  method: string,
  query: URLSearchParams,
  body: unknown,
): MockResponse {
  if (segments.length === 1 && method === "GET") {
    let filtered = [...mockPodcasts];
    const search = query.get("search")?.trim().toLowerCase();
    if (search) {
      filtered = filtered.filter((podcast) =>
        [podcast.title, podcast.author, podcast.description]
          .join(" ")
          .toLowerCase()
          .includes(search),
      );
    }

    const tagIDs = query.getAll("tag_id").map(Number).filter(Number.isFinite);
    if (tagIDs.length > 0) {
      filtered = filtered.filter((podcast) =>
        tagIDs.every((tagID) => podcast.tags?.some((tag) => tag.id === tagID)),
      );
    }

    switch (query.get("sort_by")) {
      case "title":
        filtered.sort((a, b) => a.title.localeCompare(b.title));
        break;
      case "episode_count":
        filtered.sort((a, b) => b.episode_count - a.episode_count);
        break;
      case "added_date":
        filtered.sort((a, b) => (b.added_date ?? "").localeCompare(a.added_date ?? ""));
        break;
      default:
        filtered.sort((a, b) => b.newest_episode_date.localeCompare(a.newest_episode_date));
    }

    const page = pageNumber(query, "page", 1);
    const size = pageSize(query, "page_size", 15);
    const result = paginate(filtered, page, size);
    return ok(clone(result.items), { pagination: result.pagination });
  }

  if (segments[1] === "batch" && method === "POST") {
    const ids = Array.isArray(bodyAsRecord(body).ids)
      ? bodyAsRecord(body).ids.map(Number)
      : [];
    return ok(clone(mockPodcasts.filter((podcast) => ids.includes(podcast.id))));
  }

  const id = Number(segments[1]);
  const podcast = podcastByID(id);
  if (!podcast) return fail(404, "NOT_FOUND", "Mock podcast not found");

  if (segments.length === 2 && method === "GET") return ok(clone(podcast));

  if (segments[2] === "notes") {
    if (method === "GET") return ok({ id, notes: podcast.notes ?? "" });
    if (method === "PUT") {
      podcast.notes = String(bodyAsRecord(body).notes ?? "");
      return ok(undefined);
    }
  }

  if (segments[2] === "tags") {
    if (method === "GET") return ok({ tags: clone(podcast.tags ?? []) });
    if (method === "POST") {
      const tag = mockTags.find((item) => item.id === Number(bodyAsRecord(body).tag_id));
      if (tag && !podcast.tags?.some((item) => item.id === tag.id)) {
        podcast.tags = [...(podcast.tags ?? []), tag];
      }
      return ok(undefined);
    }
    if (segments[3] && method === "DELETE") {
      podcast.tags = (podcast.tags ?? []).filter(
        (tag) => tag.id !== Number(segments[3]),
      );
      return ok(undefined);
    }
  }

  if (segments[2] === "episodes" && method === "GET") {
    const episodes = mockEpisodes.filter((episode) => episode.podcast_id === id);
    const result = paginate(
      episodes,
      pageNumber(query, "page", 1),
      pageSize(query, "page_size", 20),
    );
    return ok(clone(result.items), {
      pagination: { ...result.pagination, has_more: result.pagination.page < result.pagination.total_pages },
    });
  }

  if (segments[2] === "custom-cover" && method === "PUT") {
    podcast.custom_cover_url = String(bodyAsRecord(body).custom_cover_url ?? "");
    return ok(undefined);
  }

  return fail(404, "NOT_FOUND", "Mock podcast endpoint not found");
}

function handleEpisodes(
  segments: string[],
  method: string,
  body: unknown,
): MockResponse {
  const id = Number(segments[1]);
  const episode = episodeByID(id);
  if (!episode) return fail(404, "NOT_FOUND", "Mock episode not found");

  if (segments[2] === "notes") {
    if (method === "GET") return ok({ id, notes: episode.notes });
    if (method === "PUT") {
      episode.notes = String(bodyAsRecord(body).notes ?? "");
      return ok(undefined);
    }
  }

  if (segments[2] === "tags") {
    if (method === "GET") return ok({ tags: [] });
    return ok(undefined);
  }

  return fail(404, "NOT_FOUND", "Mock episode endpoint not found");
}

function handleTags(segments: string[], method: string, body: unknown): MockResponse {
  if (segments.length === 1 && method === "GET") return ok(clone(mockTags));

  if (segments.length === 1 && method === "POST") {
    const input = bodyAsRecord(body);
    const tag: Tag = {
      id: Math.max(...mockTags.map((item) => item.id), 0) + 1,
      name: String(input.name ?? "新标签"),
      color: String(input.color ?? "#6C6673"),
      podcast_count: 0,
    };
    mockTags = [...mockTags, tag];
    return ok(clone(tag));
  }

  const id = Number(segments[1]);
  const index = mockTags.findIndex((tag) => tag.id === id);
  if (index < 0) return fail(404, "NOT_FOUND", "Mock tag not found");
  if (method === "GET") return ok(clone(mockTags[index]));
  if (method === "PUT") {
    mockTags[index] = {
      ...mockTags[index],
      ...(bodyAsRecord(body).name ? { name: String(bodyAsRecord(body).name) } : {}),
      ...(bodyAsRecord(body).color ? { color: String(bodyAsRecord(body).color) } : {}),
    };
    return ok(clone(mockTags[index]));
  }
  if (method === "DELETE") {
    mockTags = mockTags.filter((tag) => tag.id !== id);
    return ok(undefined);
  }
  return fail(404, "NOT_FOUND", "Mock tag endpoint not found");
}

function handleWorkflows(
  segments: string[],
  method: string,
  query: URLSearchParams,
  body: unknown,
): MockResponse {
  if (segments.length === 1 && method === "GET") {
    const result = paginate(
      mockWorkflows,
      pageNumber(query, "page", 1),
      pageSize(query, "page_size", 20),
    );
    return ok(
      { workflows: clone(result.items), pagination: result.pagination },
    );
  }

  if (segments.length === 1 && method === "POST") {
    const input = bodyAsRecord(body);
    const workflow = {
      id: Math.max(...mockWorkflows.map((item) => item.id), 0) + 1,
      name: String(input.name ?? "未命名工作流"),
      description: String(input.description ?? ""),
      schedule: String(input.schedule ?? "0 2 * * *"),
      scope_type: input.scope_type ?? "all_subscribed",
      scope_config: input.scope_config ?? { podcast_ids: [] },
      rules_config: input.rules_config ?? {},
      is_enabled: Boolean(input.is_enabled),
      created_at: MOCK_NOW,
      updated_at: MOCK_NOW,
      stats: {
        total_jobs: 0,
        total_episodes: 0,
        podcast_count: 0,
      },
    };
    mockWorkflows = [...mockWorkflows, workflow];
    return ok(clone(workflow));
  }

  const id = Number(segments[1]);
  const index = mockWorkflows.findIndex((workflow) => workflow.id === id);
  if (index < 0) return fail(404, "NOT_FOUND", "Mock workflow not found");
  const workflow = mockWorkflows[index];

  if (segments.length === 2 && method === "GET") return ok(clone(workflow));
  if (segments[2] === "toggle" && method === "POST") {
    workflow.is_enabled = !workflow.is_enabled;
    workflow.updated_at = MOCK_NOW;
    return ok(clone(workflow));
  }
  if (segments[2] === "jobs" && method === "GET") {
    return ok({
      jobs: workflow.last_job ? [clone(workflow.last_job)] : [],
      pagination: { page: 1, page_size: 10, total: workflow.last_job ? 1 : 0, total_pages: 1 },
    });
  }
  if (segments[2] === "trigger" && method === "POST") return ok(undefined);
  if (segments[2] === "report" && method === "GET") {
    return ok({
      id: 1,
      job_id: workflow.last_job?.id ?? 0,
      title: `${workflow.name}报告`,
      content: "这是用于前端调试的本地 Mock 报告。",
      summary: "Mock 报告摘要",
      episodes_count: 4,
      podcasts_count: 2,
      matched_count: 3,
      time_range_start: "2026-07-01T00:00:00+08:00",
      time_range_end: MOCK_NOW,
      time_range_mode: "manual",
      generated_at: MOCK_NOW,
      format: "markdown",
      file_size: 128,
    });
  }
  if (segments.length === 2 && method === "PUT") {
    Object.assign(workflow, bodyAsRecord(body), { updated_at: MOCK_NOW });
    return ok(clone(workflow));
  }
  if (segments.length === 2 && method === "DELETE") {
    mockWorkflows = mockWorkflows.filter((item) => item.id !== id);
    return ok(undefined);
  }

  return fail(404, "NOT_FOUND", "Mock workflow endpoint not found");
}

function handleDiscovery(
  segments: string[],
  method: string,
  query: URLSearchParams,
  body: unknown,
): MockResponse {
  if (segments[1] === "candidates" && segments.length === 2 && method === "GET") {
    const limit = Math.min(Math.max(Number(query.get("limit")) || 30, 1), 100);
    return ok(currentCandidates().slice(0, limit));
  }

  if (segments[1] === "shortlist" && segments[2] === "today" && method === "GET") {
    return ok({
      date: MOCK_TODAY,
      timezone: "Asia/Shanghai",
      candidates: currentCandidates().filter(
        (candidate) => candidate.decision_state === "shortlisted",
      ),
    });
  }

  if (segments[1] === "candidates" && segments[3] === "decision" && method === "PUT") {
    const episodeID = Number(segments[2]);
    const state = bodyAsRecord(body).state as TriageDecisionState;
    if (!["pending", "shortlisted", "discarded"].includes(state)) {
      return fail(400, "VALIDATION_ERROR", "Unsupported mock triage state");
    }
    if (!mockCandidates.some((candidate) => candidate.episode_id === episodeID)) {
      return fail(404, "NOT_FOUND", "Mock discovery candidate not found");
    }
    mockDecisionStates.set(episodeID, state);
    return ok({ state, decision_updated_at: MOCK_NOW });
  }

  return fail(404, "NOT_FOUND", "Mock discovery endpoint not found");
}

function handleSearch(query: URLSearchParams): MockResponse {
  const keyword = (query.get("q") ?? "").trim().toLowerCase();
  const matchedPodcasts = mockPodcasts.filter((podcast) =>
    !keyword || [podcast.title, podcast.author, podcast.description].join(" ").toLowerCase().includes(keyword),
  );
  const matchedEpisodes = mockEpisodes.filter((episode) =>
    !keyword || [episode.title, episode.show_notes].join(" ").toLowerCase().includes(keyword),
  );
  return ok({
    podcasts: clone(matchedPodcasts.map((podcast) => ({
      id: podcast.id,
      title: podcast.title,
      author: podcast.author,
      description: podcast.description,
      cover_url: podcast.cover_url,
      episode_count: podcast.episode_count,
      newest_episode_date: podcast.newest_episode_date,
      relevance_score: 1,
      matched_fields: [],
      tags: podcast.tags,
    }))),
    episodes: clone(matchedEpisodes.map((episode) => ({
      id: episode.id,
      podcast_id: episode.podcast_id,
      podcast_title: podcastByID(episode.podcast_id)?.title ?? "",
      podcast_cover_url:
        podcastByID(episode.podcast_id)?.cover_url ?? MOCK_COVERS[0],
      title: episode.title,
      show_notes: episode.show_notes,
      published_date: episode.published_date,
      duration: episode.duration,
      relevance_score: 1,
      matched_fields: [],
    }))),
    pagination: {
      podcasts: { page: 1, page_size: 20, total: matchedPodcasts.length, total_pages: 1 },
      episodes: { page: 1, page_size: 20, total: matchedEpisodes.length, total_pages: 1 },
    },
  });
}

export async function handleMockRequest(request: MockRequest): Promise<MockResponse> {
  const method = request.method.toUpperCase();
  const query = new URLSearchParams(request.search ?? "");
  const pathname = request.pathname.replace(/\/+$/, "") || "/";
  const apiPrefix = "/api/v1";

  if (pathname === "/health") {
    return ok({ status: "ok", mode: "mock", database: "not connected" });
  }

  if (!pathname.startsWith(apiPrefix)) {
    return fail(404, "NOT_FOUND", "Mock route not found");
  }

  const relative = pathname.slice(apiPrefix.length).replace(/^\/+/, "");
  const segments = relative ? relative.split("/") : [];
  if (segments.length === 0) return ok({ mode: "mock" });

  switch (segments[0]) {
    case "podcasts":
      return handlePodcasts(segments, method, query, request.body);
    case "episodes":
      return handleEpisodes(segments, method, request.body);
    case "tags":
      return handleTags(segments, method, request.body);
    case "workflows":
      return handleWorkflows(segments, method, query, request.body);
    case "discovery":
      return handleDiscovery(segments, method, query, request.body);
    case "search":
      return method === "GET"
        ? handleSearch(query)
        : fail(405, "METHOD_NOT_ALLOWED", "Mock search is read-only");
    case "scheduler":
      return ok({ enabled: false, running: false, mock: true });
    case "cache":
      return ok({ entries: 0, size_bytes: 0 });
    case "sync":
      return ok({ status: "mock", message: "同步在本机 Mock 模式下未执行" });
    default:
      return fail(404, "NOT_FOUND", "Mock endpoint not found");
  }
}
