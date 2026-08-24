# Gemini Notebook Enterprise 无人值守 API Spike（2026-08-24）

## 结论

**当前结论：生产 Adapter No-Go；具备条件后可 Go 做一次隔离、可删除的真实账号 Spike。**

已证实：

- Google 提供 Gemini Notebook Enterprise 的 Notebook/Source REST API，但当前是 `v1alpha` **Preview / Pre-GA**，按 “as is” 提供且支持可能有限。
- Notebook 可创建、回读、列最近访问项和批量删除；Source 可批量创建、上传文件、回读和批量删除。
- `.md` 文件可按 `text/markdown` 上传；上传仅返回 Source ID，必须继续回读 Source 状态。
- 当前 API 没有公开的幂等键、调用方指定资源 ID、Source 更新或原子替换接口。
- 产品需要区域一致的用户许可。当前公开价为每席每月 9 美元，付费订阅至少 15 席，即每个多区域至少 **135 美元/月**；14 天试用提供 5,000 席。
- 本机有可用的用户型 gcloud/ADC，但没有默认项目、WIF、服务账号凭据或 ADC quota project，无法验证 API、计费、IAM 和许可。

尚未证实：

- 服务账号能否成为 Notebook 所有者，以及是否需要、能否获得用户许可。
- 相同 Markdown 重复提交时是否去重。
- 请求超时后的安全重试、删除最终一致性、实际处理耗时、API 请求配额和试用资格。
- Preview API 是否满足 MagicPodcast 的稳定性要求。

本文只使用 Google 官方文档、官方 REST/API Discovery 和本机只读 gcloud/ADC 状态。未创建项目、未启用 API、未开试用、未改 IAM、未上传、未删除任何云资源。

## 证据标记

- **已证实**：官方文档、REST Reference、API Discovery 或本机只读命令直接支持。
- **未知**：官方材料没有给出足以确定的答案。
- **需真实账号验证**：只有在已批准的隔离项目、许可和凭据中才能确定。
- **工程判断**：基于已证实接口形状给出的 MagicPodcast 设计建议，不冒充 Google 承诺。

## 1. API 阶段、服务与端点

### 已证实

Notebook 和 Source 管理页面都标记为 **Preview**，受 Pre-GA 条款约束。API 版本为 `v1alpha`：

- [Create and manage notebooks (API)](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks)
- [Add and manage data sources in a notebook (API)](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks-sources)
- [Google Cloud product launch stages](https://cloud.google.com/products/#product-launch-stages)

服务为 `discoveryengine.googleapis.com`。Notebook 资源路径使用项目**编号**：

```text
projects/{PROJECT_NUMBER}/locations/{LOCATION}/notebooks/{NOTEBOOK_ID}
```

官方管理指南使用区域前缀端点：

```text
global-discoveryengine.googleapis.com / locations/global
us-discoveryengine.googleapis.com     / locations/us
eu-discoveryengine.googleapis.com     / locations/eu
```

`global` 也允许无前缀的 `discoveryengine.googleapis.com`。端点前缀、资源 `LOCATION` 和许可多区域必须一致。官方建议在没有合规/驻留要求时选择 `global`，以获得更低延迟、更新模型和更多功能。

来源：

- [Data residency and locations](https://docs.cloud.google.com/gemini/enterprise/docs/locations)
- [Notebook create method](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks/create)

`us`、`eu` 和 `global` 是本 Spike 可直接评估的多区域。加拿大、印度、日本、新加坡、英国等 in-country 区域需要 allowlist 和 Google 账号团队参与，不应作为首个 Spike 默认项。

### API Discovery 与官方 CLI

**已证实：**

- 2026-08-24 读取公开 Discovery 文档：
  `https://discoveryengine.googleapis.com/$discovery/rest?version=v1alpha`
- 返回 `discoveryengine / v1alpha / revision 20260815`，但未包含 `notebook`、`NotebookService`、`SourceService` 或 Source 状态模型。
- 本机 Google Cloud SDK 为 `569.0.0`；安装目录的正式 gcloud command surface 中没有 `notebooklm` 命令组。

因此，当前官方可复现路径是：

1. 用 gcloud 获取 OAuth access token；
2. 按官方文档直接调用 REST。

不能假设 Discovery 生成客户端或专用 gcloud 命令已覆盖 Preview API。官方指南中的 `sources:uploadFile` 可调用，但对应 REST Reference 方法页当前返回 404，这也是 Preview 文档/工具链未完全收口的证据。

复现：

```bash
curl -fsS \
  'https://discoveryengine.googleapis.com/$discovery/rest?version=v1alpha' |
  jq -r '[.name,.version,.revision,.baseUrl] | @tsv'

curl -fsS \
  'https://discoveryengine.googleapis.com/$discovery/rest?version=v1alpha' |
  jq -e '.. | strings | select(test("notebook|NotebookService|SourceService"; "i"))'

gcloud version
gcloud help -- notebooklm
```

第二条和第四条在本机没有匹配结果。

## 2. Notebook 与 Source 生命周期

### Notebook

| 操作 | 方法 | 已证实行为 |
| --- | --- | --- |
| 创建 | `notebooks.create` | 同步返回新 `Notebook`；调用方只提供可选 `title`，`notebookId` 为输出字段 |
| 回读 | `notebooks.get` | 按完整资源名回读 Notebook 与 Source 列表 |
| 列表 | `notebooks.listRecentlyViewed` | 按最近查看时间列出；默认/最大每页 500，支持 page token |
| 删除 | `notebooks.batchDelete` | 接收完整资源名数组；成功返回空 JSON |
| 分享 | `notebooks.share` | 可授予项目内用户 Viewer/Editor；本 Spike 不需要 |

来源：

- [Notebook REST resource](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks)
- [List recently viewed notebooks](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks/listRecentlyViewed)
- [Batch delete notebooks](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks/batchDelete)

Notebook 没有当前公开的 update 方法。删除 Notebook 会连同其数据一起删除；删除项目也会删除全部数据。产品没有公开恢复接口。

来源：[What is Gemini Notebook Enterprise?](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/overview#about_data_access_storage_and_lifecycle)

### Source

| 操作 | 方法 | 已证实行为 |
| --- | --- | --- |
| 批量创建 | `notebooks.sources.batchCreate` | 支持 Drive、纯文本、网页、YouTube、Gemini Enterprise 内容 |
| 文件上传 | `notebooks.sources.uploadFile` | 使用 `/upload/v1alpha/.../sources:uploadFile`；只返回 `sourceId` |
| 回读 | `notebooks.sources.get` | 返回 Source、元数据、状态和失败原因 |
| 删除 | `notebooks.sources.batchDelete` | 按完整资源名批量删除；成功返回空 JSON |

来源：

- [Source guide](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks-sources)
- [Source REST resource](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources)
- [Get source](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources/get)
- [Batch delete sources](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources/batchDelete)

Source 状态：

```text
SOURCE_STATUS_UNSPECIFIED
SOURCE_STATUS_PENDING
SOURCE_STATUS_COMPLETE
SOURCE_STATUS_ERROR
SOURCE_STATUS_PENDING_DELETION
SOURCE_STATUS_TENTATIVE
```

`COMPLETE` 是成功终态，`ERROR` 是永久失败。错误模型可区分内容过长、空内容、上传/摄取失败、不可达、Google Drive、YouTube、音频转写、超限、域名/MIME/策略阻断等。

### 生命周期缺口

**未知：**

- 官方没有规定轮询间隔、总超时、最大重试次数或服务端重试建议。
- 删除成功响应为空，但 Source 模型存在 `PENDING_DELETION`；官方没有说明应轮询到 404、Notebook 列表中消失，还是可把空响应视为最终完成。
- Source 没有 list/update/replace 方法；只能通过 `Notebook.get` 的 `sources[]` 回收列表。

**工程判断：**

- 上传后以指数退避轮询 `sources.get`，直到 `COMPLETE`、`ERROR` 或本地超时。
- 删除后同时检查 `sources.get` 和 `notebooks.get`，直到旧 Source 不再可见；真实 Spike 必须记录实际行为。
- 轮询初始建议 2 秒，封顶 30 秒，整体超时先设 15 分钟；这些值不是 Google 承诺，必须由真实耗时修正。

## 3. Markdown 上传

### 已证实

`.md` 的官方 MIME type 是 `text/markdown`。单 Source 上限为 500 MB 或 500,000 words。

官方上传形状：

```text
POST https://{ENDPOINT_LOCATION}-discoveryengine.googleapis.com/upload/v1alpha/
  projects/{PROJECT_NUMBER}/locations/{LOCATION}/notebooks/{NOTEBOOK_ID}/sources:uploadFile

Authorization: Bearer {ACCESS_TOKEN}
X-Goog-Upload-File-Name: {DISPLAY_NAME}
X-Goog-Upload-Protocol: raw
Content-Type: text/markdown

{RAW_MARKDOWN_BYTES}
```

成功响应只有：

```json
{
  "sourceId": {
    "id": "SOURCE_ID"
  }
}
```

随后必须调用 `sources.get` 取得完整资源名、标题、word/token count、状态和失败原因。

来源：[Add and manage data sources in a notebook (API)](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks-sources#upload-file)

产品使用上传内容的静态副本；原 Markdown 后续变化不会自动同步到 Notebook。

来源：[Gemini Notebook Enterprise source behavior](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/overview)

### 本期建议

真实 Spike 只上传合成、无隐私的标准 Markdown 知识包，不上传音频。文件名建议包含稳定的 episode ID 和包哈希短码，例如：

```text
episode-{EPISODE_ID}-{SHA256_12}.md
```

这是 MagicPodcast 的恢复标识，不是服务端幂等键。

## 4. 幂等、重复、删除与替换

### 官方已证实

- `notebooks.create` 没有 `requestId` 或调用方指定 `notebookId`。
- `sources.uploadFile` 由服务端返回 Source ID。
- `sources.batchCreate`、`uploadFile`、`batchDelete` 没有公开的幂等键。
- Source 资源只有 `batchCreate`、`batchDelete`、`get`；没有 update/replace。
- Notebook 标题和 Source 展示名没有文档化的唯一约束。

### 风险

**工程判断：**

- POST 成功但响应在客户端丢失时，盲目重试可能创建重复 Notebook/Source。
- 相同字节和相同展示名不能当作服务端去重承诺。
- “先删后传”可能在上传失败后丢失旧版本。
- “先传后删”会短暂产生重复 Source，并可能影响 Notebook 回答。
- 删除与创建不是事务，无法原子替换。

### MagicPodcast 必需的客户端恢复契约

至少保存：

```text
target
project_number
location
episode_id
package_sha256
notebook_resource_name
source_resource_name
remote_status
attempt
last_error_class
created_at / updated_at
```

建议状态机：

1. `(target, episode_id, package_sha256)` 已为 `COMPLETE`：no-op。
2. 已有 `source_resource_name` 且状态未终止：恢复轮询。
3. POST 响应已返回：先持久化资源 ID，再继续。
4. POST 结果不明：先通过 `Notebook.get` 查找带哈希的展示名；无法唯一匹配则进入人工/补偿态，禁止盲重试。
5. 替换采用“新 Source 完成 → 删除旧 Source → 验证旧 Source 消失”；若不能容忍短暂重复，则 No-Go。

### 真实账号必须回答

- 同包连续上传两次是否生成两个 Source。
- 相同 `title`/展示名是否允许重复。
- 客户端超时后服务端最终成功时如何恢复。
- `batchDelete` 后 `get` 的状态序列和最终错误码。
- 新 Source 完成后删除旧 Source 是否影响 Notebook 可用性。

## 5. 服务账号、WIF 与 ADC

### 官方认证事实

REST 方法接受 OAuth，常见 scope 包括：

```text
https://www.googleapis.com/auth/cloud-platform
https://www.googleapis.com/auth/discoveryengine.readwrite
https://www.googleapis.com/auth/discoveryengine.serving.readwrite
```

最直接的权限包括：

```text
discoveryengine.notebooks.create
discoveryengine.notebooks.get
discoveryengine.notebooks.list
discoveryengine.sources.create
discoveryengine.sources.get
discoveryengine.sources.delete
```

来源：

- [Notebook create authorization](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks/create)
- [Source batch create authorization](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources/batchCreate)
- [Source get authorization](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources/get)
- [Source delete authorization](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources/batchDelete)

产品预定义角色：

| 用途 | 角色 |
| --- | --- |
| 产品管理 | `roles/discoveryengine.notebookLmOwner`（Cloud NotebookLM Admin） |
| 用户进入产品、创建 Notebook | `roles/discoveryengine.notebookLmUser`（Cloud NotebookLM User） |
| Notebook 所有者 | `roles/discoveryengine.notebookOwner` |
| Notebook 编辑者 | `roles/discoveryengine.notebookEditor` |
| Notebook 查看者 | `roles/discoveryengine.notebookViewer` |

`Cloud NotebookLM User` 包含 Notebook create/list；Notebook Owner 包含 Notebook get/update/策略与 Source create/get/delete 等。Owner/Editor/Viewer 是 Notebook 级角色，由产品在创建/分享过程中分配，不应由管理员作为普通项目角色手动分配。

来源：

- [Discovery Engine IAM roles](https://docs.cloud.google.com/iam/docs/roles-permissions/discoveryengine#discoveryengine.notebookLmUser)
- [Gemini Notebook Enterprise user roles](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/overview#user_roles)

### ADC 与无人值守

ADC 查找顺序包括：

1. `GOOGLE_APPLICATION_CREDENTIALS` 指向的外部账号/WIF 配置或凭据文件；
2. `gcloud auth application-default login` 生成的本地 ADC；
3. Google Cloud 资源绑定的服务账号。

gcloud CLI 自身的登录凭据与 ADC 是两套独立状态。官方 API 示例中的 `gcloud auth print-access-token` 证明的是 gcloud 登录路径，不等同于应用使用 ADC。

来源：

- [How Application Default Credentials works](https://docs.cloud.google.com/docs/authentication/application-default-credentials)
- [Set up Application Default Credentials](https://docs.cloud.google.com/docs/authentication/provide-credentials-adc)

Mac mini 不在 Google Cloud 托管资源上，不能依赖 metadata server 的 attached service account。长期无人值守优先级：

1. **Workload Identity Federation + 短期凭据**；
2. WIF 后 impersonate 专用服务账号；
3. 仅在前两者不可行且另行接受风险时使用服务账号 key。

Google 官方说明，WIF 可让本地/多云工作负载不用服务账号 key，通过 OIDC、SAML、X.509、AWS/Azure 等外部身份换取短期 token；服务账号 impersonation 是官方备选。服务账号 key 是高风险长期凭据，不推荐。

来源：

- [Workload Identity Federation](https://docs.cloud.google.com/iam/docs/workload-identity-federation)
- [Service account key best practices](https://docs.cloud.google.com/iam/docs/best-practices-for-managing-service-account-keys)

### 关键未知

官方许可文档只说明“用户”必须拥有 Cloud NotebookLM User 角色和区域许可，未说明：

- 服务账号能否被分配许可；
- 服务账号没有许可时能否调用 Notebook API；
- WIF workload principal 是否能成为 Notebook Owner；
- 服务账号创建的 Notebook 能否由已许可用户在 UI 中访问；
- 每用户 500 Notebook 的限制如何作用于服务账号。

REST Reference 暴露 IAM permission 和 OAuth scope，不能据此推导许可一定对服务账号豁免。**这是 #183 最重要的真实账号门槛。**

## 6. 本机只读前置状态

检查时间：2026-08-24；只输出布尔值和哈希，不保存账号、项目、token 或凭据正文。

| 项目 | 脱敏结果 |
| --- | --- |
| gcloud | 已安装，Google Cloud SDK `569.0.0` |
| 活动 gcloud 账号 | 1 个；标识哈希前缀 `b98c6584dc65` |
| 默认项目 | 未配置 |
| ADC 文件 | 存在，模式 `600` |
| ADC 类型 | `authorized_user` |
| ADC access token 只读签发测试 | 成功；token 未输出 |
| ADC quota project | 未配置 |
| 服务账号私钥 | ADC 中不存在 |
| WIF `external_account` | 未配置 |
| 服务账号 impersonation | 未配置 |
| Discovery Engine API | 因无默认项目，未知 |
| Billing | 因无默认项目，未知 |
| NotebookLM IAM | 因无默认项目，未知 |

这只能证明本机用户 ADC 目前可换取 token，不能证明 Notebook API 可用，更不能作为生产无人值守凭据。

脱敏复现原则：

```bash
# 依次输出：账号查询成功布尔值、活动账号计数、活动账号集合 SHA-256
if active_accounts="$(gcloud auth list --filter='status:ACTIVE' --format='value(account)' 2>/dev/null)"; then
  printf 'true\n'
else
  active_accounts=''
  printf 'false\n'
fi
printf '%s\n' "$active_accounts" | awk 'NF { count++ } END { print count + 0 }'
printf '%s\n' "$active_accounts" | LC_ALL=C sort | shasum -a 256 | awk '{ print $1 }'

# 依次输出：项目查询成功布尔值、项目存在布尔值、项目 SHA-256（未配置时为 false）
if project_id="$(gcloud config get-value project 2>/dev/null)"; then
  printf 'true\n'
else
  project_id=''
  printf 'false\n'
fi
if [ -n "$project_id" ] && [ "$project_id" != '(unset)' ]; then
  printf 'true\n'
  printf '%s' "$project_id" | shasum -a 256 | awk '{ print $1 }'
else
  printf 'false\nfalse\n'
fi

# 仅输出 ADC 文件存在布尔值，不输出配置目录
gcloud_config_dir="$(gcloud info --format='value(config.paths.global_config_dir)' 2>/dev/null)"
if [ -n "$gcloud_config_dir" ] &&
  [ -f "$gcloud_config_dir/application_default_credentials.json" ]; then
  printf 'true\n'
else
  printf 'false\n'
fi

# token 丢弃，仅输出 ADC token 签发成功布尔值
if gcloud auth application-default print-access-token >/dev/null 2>&1; then
  printf 'true\n'
else
  printf 'false\n'
fi
```

命令自身只输出布尔值、计数或完整 SHA-256；不得打印账号、项目、配置目录、ADC JSON、refresh token 或 access token。

## 7. 许可、试用与费用

### 已证实

- 用户必须有许可才能登录 Gemini Notebook Enterprise。
- 许可与多区域绑定；用户若访问多个多区域，需要分别获得许可。
- 付费订阅最少 15 席，最多 5,000 席。
- 14 天 free-trial tier 提供完整功能和 5,000 席。
- 付费可按月或按年订阅。
- Google 当前产品页公开价为 **9 美元/席/月**，年度订阅有折扣但未公开折扣比例。

来源：

- [Get licenses for Gemini Notebook Enterprise](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-licensing)
- [Gemini Notebook for enterprise product page](https://cloud.google.com/resources/notebooklm-enterprise)

由公开价格计算：

```text
15 × 9 USD = 135 USD / 月 / 多区域
```

这是最低月度标价，不含税、汇率、年度折扣或合同价格。

### 需真实账号验证

- 当前组织/结算账号是否仍有 14 天试用资格。
- 试用是否要求先绑定有效 Billing Account。
- 服务账号/API-only 调用是否消耗或要求用户许可。
- 试用到期后资源保留期、API 错误形状和是否可导出。
- 最终含税价格及取消/续费行为。

设置文档要求项目已启用 Billing，再启用 Discovery Engine API：

- [Set up Gemini Notebook Enterprise](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-notebooklm)
- [Verify billing is enabled](https://docs.cloud.google.com/billing/docs/how-to/verify-billing-enabled#confirm_billing_is_enabled_on_a_project)

## 8. 审计与敏感信息

Gemini Notebook Enterprise 的 usage audit logging 是项目级可选配置，通过 `ObservabilityConfig` 开启。

官方明确警告：**敏感数据不会从 usage audit logs 中过滤。** 日志可包含请求/响应、prompt、grounding metadata、回答和 citation 文本。

开启需要：

```text
roles/discoveryengine.agentspaceAdmin
```

查看需要：

```text
roles/logging.viewer
```

Notebook 专用日志名：

```text
discoveryengine.googleapis.com/notebooklm_enterprise_user_activity
```

来源：[Configure usage audit logging](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-usage-audit-logs-for-nblme)

本轮未开启日志，因为该操作会 PATCH 项目配置。真实 Spike 只能使用合成 Markdown，并在开启前确认：

- 日志保留期；
- 谁可读取；
- 是否创建隔离 log bucket/sink；
- Spike 后是否关闭；
- 是否允许记录正文。

若生产审计必须保存完整 prompt/response，而又不能保证转写和 shownotes 不含隐私，则 No-Go。MagicPodcast 自身只应记录资源名、状态、耗时、错误分类、包哈希和费用观察，不记录 token 或私有正文。

## 9. 配额与容量

官方产品限制：

| 项目 | 限制 |
| --- | --- |
| Notebooks | 500 / user |
| Sources | 300 / notebook |
| 单 Source | 500 MB 或 500,000 words |
| Queries | 500 / user / day |
| Audio overviews | 20 / user / day |

来源：[Gemini Notebook Enterprise usage limits](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/overview#usage-limits)

与本期最相关的是 500 Notebook/用户、300 Source/Notebook 和单 Source 上限。

**未知：**

- Notebook/Source API 每分钟、每天或每项目请求配额。
- Preview API 是否有单独限流。
- 429 的 retry-after、突发限制和配额提升流程。
- 服务账号是否按 user、project 或 license 计数。

公开 Notebook 文档与当前 API Discovery 没有给出这些请求配额。真实 Spike 必须在隔离项目启用 API 后只读回查 “Quotas & System Limits” 或 Service Usage，并记录配额名称、默认值和可调性。

## 10. 对分组方案的约束

| 方案 | 已知优势 | 已知硬约束 | 当前判断 |
| --- | --- | --- | --- |
| 一集一个 Notebook | 删除/替换边界最清晰 | 500 Notebook/用户，很快耗尽 | 不宜直接作为长期默认 |
| 一个节目一个 Notebook | 语义自然，Notebook 数少 | 300 Source/Notebook；长寿节目需要分卷 | 候选，但须验证大 Notebook 性能 |
| 主题 Notebook | 便于跨节目研究 | 同一集可能重复上传，幂等和成本更难 | 首期不建议 |

**工程判断：** 首期更可能需要“节目 + 分卷”而不是“一集一个 Notebook”，但真实 Spike 未验证搜索体验、处理耗时和许可计数前不做最终选择。

## 11. 真实 Spike 尚缺前置

所有者已批准在本票范围内执行隔离、可删除的云端 Spike；当前仍缺以下外部前置，因此没有执行：

1. 独立 Google Cloud 项目与 Billing Account。
2. 明确 `global`、`us` 或 `eu`，并保持 API、资源、许可一致。
3. 启用 `discoveryengine.googleapis.com`。
4. 配置 Cloud Identity 或产品支持的用户 IdP。
5. 向人类管理员授予：
   - Cloud NotebookLM Admin；
   - Gemini Enterprise Admin（订阅/审计场景）。
6. 开通 14 天试用并向测试用户分配区域许可。
7. 为 API 主体准备两条独立路径：
   - 已许可测试用户，用于建立基线；
   - 专用服务账号或 WIF workload principal，用于验证无人值守。
8. 明确 WIF 的外部 IdP/短期凭据来源；禁止默认创建服务账号 key。
9. 准备无隐私、可公开的标准 Markdown 包和固定 SHA-256。
10. 设定预算、日志保留和试用到期清理责任人。
11. 在真实执行前锁定资源清理范围和回读证据；本票已授权创建、重复上传、删除和审计配置变更。

## 12. 建议的真实 Spike 顺序

1. 用已许可用户执行 Notebook create/get，证明产品与区域基线。
2. 上传合成 Markdown，保存 Source ID。
3. 轮询 Source 到 `COMPLETE` 或 `ERROR`，记录耗时和失败模型。
4. 重复上传完全相同的包，观察 Source 数量与 ID。
5. 模拟一次响应丢失/客户端中断，验证恢复策略。
6. 创建新版本 Source，验证“新完成 → 删旧 → 回读”的替换流程。
7. 用服务账号/WIF 重复 1–6，验证许可、所有权和 UI 可见性。
8. 让 Mac mini 在无交互 shell 中换取新 token，验证过期续期。
9. 用合成内容短时开启 usage audit logging，回读日志后关闭。
10. 读取项目 API 配额、试用/费用和到期行为。

每一步只保存脱敏请求形状、资源名哈希、状态、耗时、错误分类和费用；不保存 token、ADC 内容或私有正文。

## 13. Go / No-Go 门槛

### Go：允许进入生产 Adapter（#184）

必须全部满足：

1. Google 明确支持或真实账号稳定证明服务账号/WIF 可无人值守创建、回读、上传和删除。
2. 服务账号与用户许可关系明确，费用可接受且不会依赖个人账号续期。
3. Mac mini 可连续换取短期 token，不保存服务账号私钥。
4. 相同包重复提交可被安全识别、恢复和清理；不存在不可控重复。
5. Source 处理在约定超时内稳定完成，错误可分类重试。
6. 替换与删除可被回读证明，失败后不会静默丢数据。
7. API 请求配额覆盖峰值和补偿重试，并有 429 策略。
8. `global/us/eu` 的驻留、许可和端点一致性满足要求。
9. 审计不泄露 token/私有正文，日志保留和访问边界已批准。
10. 团队明确接受 Preview / Pre-GA 稳定性与支持风险。

### No-Go

任一成立即 No-Go，且禁止浏览器自动化或私有 API fallback：

- 服务账号/WIF 不受支持，或必须依赖个人用户 refresh token。
- 服务账号必须购买但无法分配许可，或许可关系无法得到支持性结论。
- 重试会产生无法唯一识别/清理的重复 Notebook/Source。
- API Discovery/Preview 变化导致无法建立可测试契约。
- 配额、费用、区域、审计或数据生命周期不满足生产要求。
- 必须记录私有正文才能获得可用审计证据。

## 14. 一手来源索引

- [Gemini Notebook Enterprise overview](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/overview)
- [Set up Gemini Notebook Enterprise](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-notebooklm)
- [Get licenses](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-licensing)
- [Notebook API guide](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks)
- [Source API guide](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks-sources)
- [Notebook REST resource](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks)
- [Source REST resource](https://docs.cloud.google.com/gemini/enterprise/docs/reference/rest/v1alpha/projects.locations.notebooks.sources)
- [Locations and data residency](https://docs.cloud.google.com/gemini/enterprise/docs/locations)
- [Discovery Engine IAM roles](https://docs.cloud.google.com/iam/docs/roles-permissions/discoveryengine)
- [Usage audit logging](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-usage-audit-logs-for-nblme)
- [Application Default Credentials](https://docs.cloud.google.com/docs/authentication/application-default-credentials)
- [Set up ADC](https://docs.cloud.google.com/docs/authentication/provide-credentials-adc)
- [Workload Identity Federation](https://docs.cloud.google.com/iam/docs/workload-identity-federation)
- [Service account key best practices](https://docs.cloud.google.com/iam/docs/best-practices-for-managing-service-account-keys)
- [Official product and current list price](https://cloud.google.com/resources/notebooklm-enterprise)
- [Public Discovery document](https://discoveryengine.googleapis.com/$discovery/rest?version=v1alpha)
