# 小宇宙 Feed 访问韧性：一手来源研究与建议

日期：2026-07-19
状态：只读研究；未修改代码、配置或生产状态

> 后续决策：#35/#36 已取代本文关于“小宇宙首次 403 整域硬断路”的建议。目标方案改为共享单队列、简单自适应软限速和 15 分钟批次内的有界分类重试；本文其余一手证据和固定出口边界仍可参考。

## 结论

1. 现有 `403` 现象不足以证明 `feed.xyzfm.space` 单纯按出口 IP 封锁。它也可能来自请求频率、同一时段并发、边缘节点策略或其他未公开规则。需要同一失败窗口、同一请求参数下的固定出口对照实验才能区分。
2. 第一优先级不是换 IP，而是减少不必要请求：全局去重、单域名排队、缓存、错峰调度，以及对 `429/503` 正确执行 `Retry-After`。条件请求也应支持，但本次真实 Feed 样本没有返回 `ETag` 或 `Last-Modified`，暂时不能把它当成立即可用的减流手段。
3. 如果实验确认生产直连失败、固定备用出口持续成功，可以使用只面向 `feed.xyzfm.space` 的受控固定中继或备用出口。它应有稳定身份、明确负责人、访问控制和独立日志，不能成为开放代理，也不能按请求频繁换 IP。
4. 定期轮换住宅代理或代理池的主要作用是躲避来源识别；它显著降低可审计性，并可能把 Feed URL 和访问记录交给不可信第三方。结合小宇宙公开协议对未经许可干扰、探测服务的限制，本项目不应采用。
5. 即使增加备用出口，也必须保留内容层 fallback：经过稳定身份校验的非小宇宙 Feed 或明确标记的缓存结果。网络出口只能提升可达性，不能保证上游长期允许访问。

## 生产现场证据

本轮对生产数据库和现有抓取实现进行了只读检查：

- 2026-05-28 至 2026-06-02，`feed.xyzfm.space` 记录到 350 次成功。
- 2026-06-03 至 2026-07-18，记录到 2562 次失败，涉及 146 个 Feed；失败平均约 115 ms，更像快速拒绝而非连接超时。
- 系统中的“访问被拒绝”对应 HTTP `401/403`；当前抓取客户端只发送 `User-Agent: MagicPodcast/1.0`，没有保存条件请求验证器，也没有对 `Retry-After` 的处理。
- 现有工作流会在 06:00、06:45、07:30 等时段集中启动，单工作流并发为 5；同一 Feed 还可能被多个工作流重复抓取。因此，请求节奏和短时并发是必须先排除的变量。

本轮还对两个已知 Feed 做了一次低频可达性检查：

- 两个 Feed 当前从生产机均返回 HTTP `200`。
- `feed.xyzfm.space` 当前解析到 `111.31.56.97`–`111.31.56.104` 八个 CDN 节点；生产机分别固定访问八个节点，全部返回 `200`。
- 响应显示 Tengine/CDN 缓存命中，但样本没有 `ETag`、`Last-Modified`、标准 `Cache-Control`、RSS `<ttl>` 或 WebSub hub。
- 本机和生产机当前都能访问样本，但本机的独立公网出口未成功确认，因此这不是严格的“双出口对照”。

这组证据说明：历史上的连续快速拒绝是真实的，但“当前生产 source IP 被永久封禁”并不成立。IP/ASN、时间窗口、请求节奏、CDN 策略和生产公网 IP 动态变化都仍是候选解释。

## 一手来源确认

### 小宇宙公开规则

截至本次访问：

- [小宇宙官网 robots.txt](https://www.xiaoyuzhoufm.com/robots.txt) 对 `/www`、登录、充值和 hybrid 路径设置了通用禁止。
- [`feed.xyzfm.space/robots.txt`](https://feed.xyzfm.space/robots.txt) 返回 `200`，明确禁止一组 AI 搜索/训练和 SEO 分析机器人；没有提供 `User-agent: *` 的全站禁止规则。响应带有 `Cache-Control: public, max-age=86400`，说明该 robots 文件可以缓存一天。
- 这不等于自动化 Feed 抓取获得了授权。Robots Exclusion Protocol 的正式标准明确说明：robots 规则不是访问授权；服务端仍可以用 HTTP 认证、限速或其他方式控制访问。[RFC 9309 §1](https://www.rfc-editor.org/rfc/rfc9309.html#section-1)、[§3](https://www.rfc-editor.org/rfc/rfc9309.html#section-3)
- RFC 9309 还要求爬虫使用能对应到自身产品的 User-Agent，并建议在识别字符串中说明用途；robots 缓存通常不应超过 24 小时。[RFC 9309 §2.2.1](https://www.rfc-editor.org/rfc/rfc9309.html#section-2.2.1)、[§2.4](https://www.rfc-editor.org/rfc/rfc9309.html#section-2.4)

[小宇宙软件许可及服务协议](https://www.xiaoyuzhoufm.com/agreement) 当前公开版本标注更新于 2022-09-01，相关边界包括：

- 2.1.1：给予个人、不可转让、非排他的非商业使用许可。
- 3.9：不得使用未经授权的插件、外挂或第三方工具干扰、破坏、修改产品和服务的正常运行。
- 3.10：不得未经许可使用相关数据、进入服务器，探查/扫描/测试系统弱点，干涉网站正常运行或伪造 TCP/IP 数据包信息。
- 4.5：未经权利人同意，不得对网页、应用、软件进行反向工程等行为。

协议没有公开承诺无限量自动轮询 RSS，也没有公开列出 Feed 的请求配额。因此，普通、低频、诚实标识的 Feed 获取与“为绕过限制而轮换出口”应被视为不同风险等级。若后续需要长期运行中继，最稳妥的合规动作是先通过协议中的联系邮箱 `xyz@iftech.io` 询问自动化 Feed 获取和固定中继是否被允许。

[小宇宙隐私政策](https://www.xiaoyuzhoufm.com/privacy) 说明其可能记录服务日志，并可通过 IP 地址获取大致位置信息。这意味着出口 IP 和访问行为本身对平台可见；代理轮换不是不可见手段，反而可能形成更异常、难解释的访问轨迹。

### 本次 Feed 行为边界

本次没有对未知 Feed 路径做批量探测，也没有在失败窗口压测真实 Feed；只对两个已知样本做了低频请求，并对当前八个 CDN 节点做了各一次固定访问。因此当前能确认它们在这个观测时刻可访问，不能把结果外推为所有 Feed 或所有时间窗口都可访问。

同样，条件请求是客户端应支持的标准能力，但本次样本没有提供可用的 `ETag` 或 `Last-Modified`；只有上游实际返回验证器时，它才能减少响应传输成本。

正式实验应从一个已知合法 Feed 样本读取以下响应字段，并逐 Feed 保存：

- HTTP 状态码和响应时间
- `Retry-After`
- `ETag`、`Last-Modified`
- `Cache-Control`、`Expires`、`Age`
- 响应字节数和实际出口标识

只记录排障所需元数据；日志中不保存代理凭据、Cookie、令牌或完整响应正文。

## HTTP 与 RSS 标准给出的低负载手段

### 条件请求优先于重复下载

HTTP 标准规定：

- `If-None-Match` 配合服务器先前返回的 `ETag`，可以让未变化资源返回 `304 Not Modified`，以更小开销更新缓存。[RFC 9110 §13.1.2](https://www.rfc-editor.org/rfc/rfc9110.html#section-13.1.2)
- 没有 `ETag` 时，可用 `If-Modified-Since` 配合 `Last-Modified` 避免重复传输未变化内容；如果两者都有，应优先 `If-None-Match`。[RFC 9110 §13.1.3](https://www.rfc-editor.org/rfc/rfc9110.html#section-13.1.3)
- 新鲜缓存可以直接复用而不联系源站；缓存过期后再验证。源站给出的 `Cache-Control` 和 `Expires` 应优先于本地猜测。[RFC 9111 §4.2](https://www.rfc-editor.org/rfc/rfc9111.html#section-4.2)、[§4.3](https://www.rfc-editor.org/rfc/rfc9111.html#section-4.3)

对 MagicPodcast 的含义：同一个 Feed 在同一轮或相邻工作流中只能有一个共享抓取结果，不能让每个工作流独立重复下载；已保存的验证器应跨工作流复用。

### 尊重 Feed 自带刷新提示

RSS 2.0 规范定义：

- `<ttl>` 表示频道在再次刷新前可缓存的分钟数。
- `<skipHours>`、`<skipDays>` 是 Feed 给聚合器的跳过时段提示。

来源：[RSS 2.0 Specification — channel optional elements](https://www.rssboard.org/rss-specification#optionalChannelElements)、[`ttl`](https://www.rssboard.org/rss-specification#ltttlgtSubelementOfLtchannelgt)。

因此应以服务器缓存头和 Feed 的 `<ttl>` 为下限，避免早于上游建议再次请求；支持时也应尊重 `skipHours/skipDays`。如果 Feed 没给出任何提示，采用保守的本地刷新下限和随机错峰，比按固定整点集中请求更安全。

### 正确处理拒绝与限速

- `429 Too Many Requests` 明确表示一段时间内请求过多，响应可以包含 `Retry-After`。[RFC 6585 §4](https://www.rfc-editor.org/rfc/rfc6585.html#section-4)
- `Retry-After` 可以是 HTTP 日期或等待秒数，表示客户端在后续请求前应等待多久。[RFC 9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110.html#section-10.2.3)
- `403 Forbidden` 表示服务器理解请求但拒绝执行；标准明确不建议用相同凭据自动重复相同请求，而且拒绝原因可能与凭据无关。[RFC 9110 §15.5.4](https://www.rfc-editor.org/rfc/rfc9110.html#section-15.5.4)

建议语义：

- `429/503 + Retry-After`：按服务器时间等待，并停止该域名的新请求。
- `429/503` 无 `Retry-After`：有限次数指数退避并加入小幅随机抖动，不做密集重试。
- `403`：视为域名级断路信号，不立即重试、不自动换 IP；等待下一观察窗口或进入经过批准的固定出口实验。
- 连续失败：继续生成明确的部分结果，使用缓存或替代 Feed 时标明来源和新鲜度，不能把旧内容伪装成最新内容。

## 方案比较

| 方案 | 合规风险 | 稳定性 | 可观测性 | 维护成本 | 判断 |
| --- | --- | --- | --- | --- | --- |
| 降频、共享缓存、条件请求、调度分散 | 低 | 高；对频率/并发型限制最有效 | 高 | 低 | 首选，先做 |
| 单域名受控固定中继 | 中；应确认上游允许，且不能规避明确封禁 | 中到高；仅在直连出口确有问题时有效 | 高；出口和调用方固定 | 中 | 有证据后可选 |
| 两个固定备用出口，按健康状态切换 | 中；不能做逐请求轮换 | 高于单出口，可承受单点故障 | 高；必须记录明确 `egress_id` | 中到高 | 只做主备，不做轮询池 |
| 云厂商多 IP NAT 自动分摊 | 中 | 容量高，但单请求来源可能不够直观 | 中 | 中到高 | 不适合作为首选诊断模型 |
| Cloudflare Dedicated Egress IP | 中；需 Enterprise，且不支持 China Network | 高，官方支持主/备地址 | 高 | 高 | 现有 Access/Tunnel 不自动具备此能力 |
| 住宅代理或高频轮换代理池 | 高 | 低；共享/污染 IP 和供应商质量不可控 | 低 | 高 | 不采用 |

### 固定出口与多出口的技术事实

AWS 官方文档说明，Public NAT Gateway 会把公网方向的来源映射为关联的 Elastic IP；跨可用区共享一个 NAT 会形成单区故障风险，官方建议各可用区使用各自 NAT。NAT Gateway 也可以关联多个 Elastic IP，但该能力主要用于连接容量扩展，不应假定它天然提供可解释的逐请求主备语义。

来源：[AWS NAT gateway](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-nat-gateway.html)、[NAT gateway basics](https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-basics.html)、[NAT gateway use cases](https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-scenarios.html)。

AWS VPC Flow Logs 可以记录网络接口的 IP 流量元数据并发送到 CloudWatch Logs、S3 或 Firehose，适合核对实际出口和连接结果；它不替代应用层的状态码、缓存命中和响应时间日志。[AWS VPC Flow Logs](https://docs.aws.amazon.com/vpc/latest/userguide/flow-logs.html)

Cloudflare 官方提供 Dedicated Egress IP 和按目标域名/地址选择出口的 Egress Policy，并要求配置备用 IPv4 以提高韧性；但该功能仅面向 Zero Trust Enterprise 附加服务，且官方明确不支持 Cloudflare China Network。MagicPodcast 当前使用的 Cloudflare Access/Tunnel 是入站访问路径，不等于已经拥有固定出站 IP。

来源：[Cloudflare Egress policies](https://developers.cloudflare.com/cloudflare-one/traffic-policies/egress-policies/)、[Dedicated egress IPs](https://developers.cloudflare.com/cloudflare-one/traffic-policies/egress-policies/dedicated-egress-ips/)。

### 受控中继的安全边界

若实验最终支持中继，设计必须同时满足：

- 只接受 MagicPodcast 的已认证调用，不面向互联网匿名开放。
- 只允许 `HTTPS GET/HEAD` 到 `feed.xyzfm.space`，不接受调用方提供任意 scheme、host、端口或重定向目标。
- DNS 解析后拒绝私网、回环、链路本地和保留地址；跨域重定向默认拒绝，避免形成 SSRF 或开放代理。
- 主、备出口均为长期固定地址；只在明确健康阈值触发后切换，并记录出口 ID，不做每次请求轮换。
- 设定并发、速率、单响应大小、超时和总重试上限；中继自身不能放大原始请求量。
- 凭据放在密钥管理中；日志只保留哈希后的 Feed 标识、目标域名、状态码、延迟、字节数、缓存结果和出口 ID。
- 数据库升级、Feed fallback 和中继分别发布、分别回滚，避免无法判断哪个变化产生效果。

## 固定出口对照实验

这是一项低频可用性实验，不是封锁规避测试。

### 实验组

在下一次历史上易失败的窗口，使用少量已知合法 Feed，保持完全相同的 User-Agent、超时、条件请求和请求间隔：

1. 生产直连，当前节奏。
2. 生产直连，单线程并显著拉开请求间隔。
3. 一个固定、可信的备用出口。
4. 如确有必要，再加入第二个固定备用出口验证单点问题。

每组不并发轰击，测试顺序轮换，避免时间变化被误判为出口差异。

### 记录

- 窗口、Feed 样本哈希、出口 ID
- HTTP 状态、`Retry-After`、缓存验证器、响应字节和延迟
- DNS 结果、连接错误类别、是否命中缓存
- 同一出口该窗口总请求数和并发峰值

### 决策门槛

- 低频直连恢复：优先判定为请求节奏问题，不上线中继。
- 直连和备用出口都失败：更像上游统一策略、Feed 本身或公共故障，不上线中继。
- 直连稳定失败、同速率固定备用出口稳定成功，并在至少两个独立失败窗口复现：才进入域名级固定中继评审。
- 备用出口在增加请求量后也失败：优先判定为频率/容量问题，不增加更多 IP。
- 服务端明确要求停止、robots 改为通用禁止，或运营方拒绝授权：停止中继方案，转向缓存和经确认的替代 Feed。

## 建议优先级

### P0：立即作为设计约束

1. 为 `feed.xyzfm.space` 建立域名级共享队列，初始并发为 1；同一 Feed 的并发或相邻请求合并。
2. 在上游提供时保存 `ETag/Last-Modified`、支持 `304`，并遵守 HTTP 缓存头、RSS `<ttl>` 和跳过时段；当前样本没有这些提示时，使用保守的本地最小刷新间隔。
3. 将工作流整点集中执行打散；抖动用于避免同时请求，不用于逃避限制。
4. `429/503` 遵守 `Retry-After`；小宇宙 403 按 #35/#36 在 15 分钟批次内有界重试，并通过共享单队列和简单自适应软限速降低压力，不再触发整域硬断路，也不自动换出口。其他域名的域名级熔断仍需显式策略或多个不同 Feed 的共同失败证据。
5. 使用诚实、稳定、带联系信息的 MagicPodcast User-Agent，并每天刷新 robots 缓存。
6. 记录按域名、窗口、状态码、缓存命中和出口 ID 聚合的指标，避免记录凭据和完整正文。

### P1：用证据确认根因

执行上述固定出口对照实验；同时向小宇宙公开联系邮箱询问合理的 Feed 轮询频率和固定中继许可。未获得两个独立窗口的对照证据前，不引入代理。

### P2：有条件的韧性增强

若确认出口相关：只为 `feed.xyzfm.space` 增加一个受控固定中继和一个固定备用出口，按健康阈值主备切换。并行建设高置信的非小宇宙 Feed fallback；代理失败时回到缓存或替代源，不继续扩充 IP 池。

### P3：明确排除

不使用住宅代理、共享代理池、按请求轮换 IP、伪装成浏览器/其他客户端、绕过验证码、开放任意 URL 转发或无限重试。这些手段降低稳定性和可审计性，也更容易越过小宇宙公开协议的合理使用边界。

## 本报告没有证明的事项

- 没有证明 `403` 必然按 IP 触发。
- 没有证明当前生产出口和本机一定是两个独立 source IP。
- 没有证明所有小宇宙 Feed 支持 ETag、Last-Modified 或统一缓存头。
- 没有获得小宇宙对 MagicPodcast 自动轮询或中继的书面许可。
- 没有测试或部署任何云出口、中继、代理或生产配置。

因此，本文是合规、低负载的决策方案，不是对绕过上游访问控制的授权。

## 一手来源

1. [小宇宙软件许可及服务协议](https://www.xiaoyuzhoufm.com/agreement)
2. [小宇宙隐私政策](https://www.xiaoyuzhoufm.com/privacy)
3. [小宇宙官网 robots.txt](https://www.xiaoyuzhoufm.com/robots.txt)
4. [小宇宙 Feed robots.txt](https://feed.xyzfm.space/robots.txt)
5. [RFC 9309: Robots Exclusion Protocol](https://www.rfc-editor.org/rfc/rfc9309.html)
6. [RFC 9110: HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110.html)
7. [RFC 9111: HTTP Caching](https://www.rfc-editor.org/rfc/rfc9111.html)
8. [RFC 6585: 429 Too Many Requests](https://www.rfc-editor.org/rfc/rfc6585.html#section-4)
9. [RSS 2.0 Specification](https://www.rssboard.org/rss-specification)
10. [AWS NAT gateways](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-nat-gateway.html)
11. [AWS VPC Flow Logs](https://docs.aws.amazon.com/vpc/latest/userguide/flow-logs.html)
12. [Cloudflare Egress policies](https://developers.cloudflare.com/cloudflare-one/traffic-policies/egress-policies/)
13. [Cloudflare Dedicated egress IPs](https://developers.cloudflare.com/cloudflare-one/traffic-policies/egress-policies/dedicated-egress-ips/)
