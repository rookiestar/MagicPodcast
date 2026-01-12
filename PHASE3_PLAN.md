# Phase 3: 架构重构方案

## 目标
将大文件拆分为多个小文件，每个文件职责单一，便于维护。

## 后端拆分方案

### service.go (1835行) → 5个文件

#### 1. service.go (主入口，~200行)
保留内容:
- Service结构体定义
- NewService构造函数
- Close方法
- 基础方法（ImportOPML等简单包装）

#### 2. opml_import.go (~400行)
提取内容:
- ImportOPMLWithProgressAndConfig
- ImportOPMLFromPodcastIndexOnly
- syncPodcastFromFeedWithRetry
- syncPodcastFromPodcastIndexOnly
- fetchPodcastOnline
- 相关的辅助方法

#### 3. metadata_sync.go (~500行)
提取内容:
- SyncPodcastsMetadataSSE
- syncPodcastMetadataWithUpdateCheck
- syncPodcastMetadata
- updatePodcastMetadataOnline
- 相关的辅助方法

#### 4. episode_sync.go (~500行)
提取内容:
- SyncPodcastEpisodes
- SyncAllPodcastEpisodes
- 单集同步的核心逻辑
- EpisodeSyncResult相关

#### 5. podcast_helpers.go (~300行)
提取内容:
- convertGofeedToModel
- convertGofeedItemToEpisode
- convertPodcastIndexToModel
- createEnhancedPodcastFromOPML
- saveOrUpdatePodcast
- saveEpisode

## 前端拆分方案

### api.ts (786行) → 8个文件

#### 1. lib/api/index.ts (主入口，~50行)
- 统一导出所有API
- API客户端配置

#### 2. lib/api/client.ts (~100行)
- HTTP客户端封装
- SSE连接处理
- 错误处理

#### 3. lib/api/types.ts (~100行)
- API类型定义
- Request/Response类型

#### 4. lib/api/podcast.ts (~150行)
- 播客相关API
- GET /api/v1/podcasts
- GET /api/v1/podcasts/:id
- POST /api/v1/podcasts/:id/episodes/sync

#### 5. lib/api/episode.ts (~100行)
- 单集相关API
- GET /api/v1/episodes
- PATCH /api/v1/episodes/:id

#### 6. lib/api/workflow.ts (~150行)
- 工作流相关API
- GET /api/v1/workflows
- POST /api/v1/workflows
- PUT /api/v1/workflows/:id

#### 7. lib/api/tag.ts (~80行)
- 标签相关API
- GET /api/v1/tags
- POST /api/v1/tags
- PUT /api/v1/tags/:id

#### 8. lib/api/sync.ts (~100行)
- 同步相关API
- POST /api/v1/sync/import-sse
- POST /api/v1/sync/podcasts/metadata-sse
- POST /api/v1/sync/episodes

## 实施步骤

### Step 1: 后端拆分
1. 创建新文件框架
2. 移动相关方法到新文件
3. 更新import路径
4. 编译验证

### Step 2: 前端拆分
1. 创建新文件框架
2. 拆分API调用
3. 更新import语句
4. TypeScript编译验证

### Step 3: 验证
1. 后端编译测试
2. 前端编译测试
3. 运行单元测试
4. 手动功能测试

## 预期成果
- 每个文件 <500行
- 职责清晰
- 更容易测试
- 更容易维护

## 风险评估
- 低风险：主要是代码移动，不改变逻辑
- 需要仔细处理import依赖
- 建议逐步拆分，每步验证
