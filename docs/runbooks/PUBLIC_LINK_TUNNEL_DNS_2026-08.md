# 公网链路 Tunnel/DNS 稳定化 Runbook

最后更新：2026-08-06
关联：issue #63（父 PRD）、#64（[Infra] 稳定 Tunnel 与 DNS 公网链路）、[部署运维](../DEPLOYMENT.md)、[发布检查](../RELEASE_CHECKLIST.md)

本 Runbook 把"公网播客列表偶发长等待 / 15 秒超时 / 封面缺失、刷新后又恢复"组织为**可回退、可测量**的 Tunnel/DNS 稳定化步骤，不靠延长 15 秒超时掩盖故障。所有**生产写操作（cloudflared 配置、DNS、launchd 重启）执行前必须单独取得明确授权**；本文件仅发布方案与检查方法。

## 1. 当前态记录（AC#1：协议 / DNS / 发布版本 / 回退方式）

来源：`docs/DEPLOYMENT.md`「Cloudflare Tunnel」段 + 本仓库 HEAD + 2026-08-06 Mac mini 只读侦察。

| 项 | 当前态 | 证据 / 复核命令 |
| --- | --- | --- |
| 拓扑 | 命名 Tunnel `magicpodcast-prod` → `http://127.0.0.1:8088`（Nginx loopback）→ 前端 `127.0.0.1:3000`、后端 `127.0.0.1:8080`；`rookiestar.cn` 经 Cloudflare Access + HTTPS 跳转 + HSTS | `docs/DEPLOYMENT.md` |
| Tunnel 协议（origin connector） | **QUIC**（配置无 `protocol:` 行）；2026-08-06 A/B 后因 QUIC 延迟更低而恢复 | 配置校验通过；日志注册 4 条 `protocol=quic` 连接，precheck `suggested_protocol=quic` |
| Tunnel 进程管理 | `launchctl` `com.cloudflare.cloudflared`，`cloudflared tunnel run magicpodcast-prod` | `docs/DEPLOYMENT.md`「默认拒绝后的受控恢复」 |
| Mac mini DNS 解析器 | **单解析器 `192.168.3.1`（家用路由器）**，连续 20/20 解析 `rookiestar.cn` 稳定为 Cloudflare anycast，无超时 / NXDOMAIN / 地址跳变 → AC#3 不需调整 DNS | 已确认：`scutil --dns \| grep nameserver`；`for i in $(seq 1 20); do dig +short rookiestar.cn @192.168.3.1; done`（20/20 命中 `104.21.16.117 172.67.212.10`） |
| 发布版本（仓库） | `origin/main` = 当前分支基线 = `584d0f0 refine global typography`（2026-08-06 核对） | `git rev-parse --short origin/main` |
| 运行版本（生产） | **release_id `20260804T002237Z-b44117f-60223`**、`build_mode=release`、健康状态 `ok` | Mac mini 本机 `/health`，2026-08-06 复核 |
| 回退方式（协议） | 还原配置快照中 `protocol:` 原值 → `launchctl kickstart -k gui/$(id -u)/com.cloudflare.cloudflared` → 复测连通 | 见 §3 |
| 回退方式（DNS） | 用 `networksetup -setdnsservers <networkservice>` 或系统设置→网络还原原解析器（macOS 下 `/etc/resolv.conf` 为生成物，不直接改）；记录原值 | 见 §5 |
| 回退方式（发布） | `./scripts/release.sh --rollback`（停止服务前校验配对与 schema） | `docs/RELEASE_CHECKLIST.md` |

> 本 Runbook 不记录 Cloudflare 账号、Tunnel 凭据、Access JWT 密钥、Google 身份或恢复码；这些只能存放在所有者受控的密码管理器或本机受限目录。

## 2. 切换前只读基线（2026-08-05，Mac mini 本机 + 认证公网入口对照）

以下为切换 QUIC→HTTP/2 **之前**的只读基线，用于 §4 的 A/B 对照与回退判定。所有命令为只读复制 / 只读采样，未触碰任何生产配置。

### 2.1 源站（nginx loopback 8088 → 后端 8080）—— 非 #64 瓶颈
- Mac mini 本机直连列表 API 50 次：**50/50 HTTP 200，P95 ≈ 7.7 ms，最大 ≈ 9.0 ms**。
- 命令：`for i in $(seq 1 50); do curl -sS -o /dev/null -w '%{http_code} %{time_total}\n' 'http://127.0.0.1:8088/api/v1/podcasts?page=1&page_size=15&view=summary'; done`
- 结论：源站毫秒级稳定 → 公网偶发长等待 / 15 秒超时**不在源站**，落在 cloudflared 隧道传输路径。

### 2.2 Mac mini 本地 DNS —— 稳定，AC#3 无需调整
- 解析器：单 `192.168.3.1`（家用路由器），连续 20 次解析 `rookiestar.cn` 均 `104.21.16.117 172.67.212.10`，无超时 / NXDOMAIN / 地址跳变。
- 结论：DNS 不是公网链路抖动来源 → **AC#3 触发条件不满足，本次不调整 DNS**。

### 2.3 Tunnel 传输（QUIC）—— 历史失败证据
- 配置无 `protocol:` 行 → 默认 QUIC；日志确认 `Registered tunnel connection ... protocol=quic`，4 连接分布 `1xlax01/1xlax08/1xlax10/1xsjc08`（LAX/SJC colo），connector 在线、版本 `2026.7.3`。
- cloudflared 日志关键失败（与 nginx 层 499 精确对齐）：
  - `datagram manager encountered a failure while serving`（QUIC datagram 路径抖动）
  - `Incoming request ended abruptly: context canceled`，命中 `/api/v1/podcasts`、`/api/v1/discovery/candidates`、`/api/v1/tags` —— 正是首屏列表加载链路。
  - 最近失败时间 `2026-08-04T03:39:11Z`（= 北京 11:39:11）；`2026-08-05`（今日）tunnel 零错误（间歇性，印证 #63「刷新后又恢复」）。
- nginx access log 近 5000 行状态分布：`3861×200 / 988×404 / 63×308 / 46×502 / 14×400 / 13×409 / 5×202 / 4×499 / 3×415 / 3×413`。
  - **4×499**（client closed connection）全部命中首屏列表 API，时间戳与 cloudflared `context canceled` 一致 → 即「长等待后用户/前端放弃」。
  - **46×502** 多为 7 月旧 `/images/proxy?url=...&_retry=1/2` 封面代理失败（含 `_retry` 生成新缓存键，属 #66 范畴，不在 #64 处理）。

### 2.4 开发机边缘只读探测（补充，非主证据）
- 2026-08-05 从开发机（本地无 cloudflared）对 `rookiestar.cn` 边缘：`dig +short` 连续 5 次 Cloudflare anycast 稳定；`https://` 与 `/api/podcasts` 均 `HTTP/2 302` 跳 Access 登录，`strict-transport-security: max-age=31536000`；`http://`→`301→https`。
- 边缘 `alt-svc: h3=":443"` 仅是边缘到客户端的 HTTP/3 通告，**与 cloudflared 隧道传输（QUIC ↔ HTTP/2）无关**。
- 仅证明边缘可达 + Access/HSTS 在位，不能代替 Mac mini 本机与认证公网入口的 §4 A/B。

### 2.5 切换假设
公网偶发 `context canceled` / 超时若源于 UDP/QUIC 路径抖动，则切 HTTP/2 over TCP 是更稳健假设；本基线提供 §4 A/B 用同请求集验证而非凭感觉切换的依据。

### 2.6 执行记录：2026-08-05 已授权 QUIC→HTTP/2 切换

经用户单独授权后执行 §3 流程，结果如下（UTC，= 北京时间 +0800）：

| 步骤 | 结果 |
| --- | --- |
| ① 快照 | `$HOME/.cloudflared-snap-20260805-005637/`（含 `magicpodcast-prod.yml` 原 302B 无协议行 + plist + STATE.txt：`protocol_before=default-quic`、`dns=192.168.3.1`、回滚命令） |
| ② 配置 | yml 顶层新增 `protocol: http2`，未触碰 `tunnel`/`credentials-file`/`ingress`/主机名 |
| ③ 重启 | `launchctl kickstart -k gui/501/com.cloudflare.cloudflared`；PID 15488→29100；16:56:56Z 旧 QUIC 连接优雅退出（`Connection terminated`/`no more connections active and exiting`） |
| ④ 验证 | 16:56:59–16:57:04Z 注册 **4 条 `protocol=http2` 连接**（lax10/lax01）；precheck 全 PASS（DNS/UDP/TCP/API），SUMMARY `Environment is healthy. cloudflared will use 'http2' as primary protocol`；本机 8088 仍 200/4.7ms |
| 切换后即时 | 切换窗口内未出现 `context canceled`/`datagram`；后续 HTTP/2 仍出现控制流断开、TLS handshake timeout 与连接重注册，不能扩大为“切换后全程零错误” |

### 2.7 首轮巡检（2026-08-05，**未通过 / 复核驳回**）

- 巡检脚本：Mac mini `$HOME/mp-tunnel-patrol.sh`（detached），日志 `$HOME/mp-tunnel-patrol-20260805.log`。
- 周期与覆盖：12 个采样点，每个间隔 600s；`PATROL START 16:59:48Z` → `PATROL COMPLETE 18:49:59Z`，实际仅约 **110 分钟（不足 AC 要求的 120 分钟）**。
- **采样路径错误（P1）**：巡检请求命中**本机 `127.0.0.1:8088`（nginx loopback → 后端）**，**未经过 Cloudflare 边缘 + cloudflared 隧道的公网链路**。因此「同请求集 QUIC↔HTTP/2 公网 A/B」与「公开列表时延 AC」**均未验证**；下表数值只代表源站侧，不代表公网链路。
- 源站侧数值（12 轮，仅供对照，非公网证据）：逐轮 `origin_200=12/12`、`non200=''`；全程最差 `p95=2.9ms`、`max=10.4ms`；巡检窗内 `nginx_new_499_list=0`、`nginx_new_5xx=0`。
- **后续错误（P2，巡检窗之外）**：`2026-08-04T20:10:40–20:13:23Z`（北京 04:10–04:13）出现 6 条 `Error shutting down control stream: context canceled`/`client disconnected`（connIndex 1/2 控制流），随后 4 条 `http2` 连接重注册——属 cloudflared 控制流重连，与 QUIC 期 list API 的请求级 `Incoming request ended abruptly` 机制不同；该窗口内无 list 流量，切换后（>87038 行）list/discovery/tags `499=0`、`5xx=0`。**故「切换后错误总计 0 / 连接稳定」仅在巡检 110min 窗内成立，不能扩大到全程**；需纳入 24 小时复查。
- 当时判定：**#64 门禁未满足，维持开放，不进入 #65。** 当时缺少公网 A/B 与完整
  120 分钟门禁；公网 A/B 后续结果见 §2.8。
- 快照 `~/.cloudflared-snap-20260805-005637/` 与巡检日志保留，必要时按 §3 第 5 步一键回退到 QUIC。
- **进展（详见 §2.8）**：公网认证 A/B 已完成；两种协议均未满足延迟门槛，因此未启动新的 120 分钟门禁。

### 2.8 公网认证 A/B（2026-08-06，未通过）

**方法**：在同一已登录 Cloudflare Access 的浏览器会话内，对
`/api/v1/podcasts?page=1&page_size=15&view=summary` 按顺序各请求 50 次。
请求使用 `no-store`/`no-cache`，响应 `cf-cache-status=DYNAMIC`，避免把边缘缓存命中当作链路性能。

| 协议 | 成功 | P50 | P95 | 最大值 | 499 / 5xx / 15s 超时 |
| --- | --- | --- | --- | --- | --- |
| HTTP/2 | 50/50 | 约 1.30s | 约 2.57s | 约 5.10s | 0 / 0 / 0 |
| QUIC | 50/50 | 约 1.31s | 约 2.17s | 约 2.59s | 0 / 0 / 0 |

- 本机源站同期为 200、约 3ms；公网长尾不在应用源站。
- QUIC 的 P95 和最大值优于 HTTP/2，故 A/B 后恢复 QUIC。HTTP/2 配置备份在
  `~/.cloudflared-ab-20260807-http2/`；原 QUIC 快照在
  `~/.cloudflared-snap-20260805-005637/`。
- 恢复后 4 条 QUIC 连接均注册成功，precheck 建议 QUIC；本次 50 次采样窗口内未见 Tunnel 传输错误。
- **判定**：两种协议的 P95 都未达到 `<1s`；HTTP/2 的最大值还超过 `<3s`。
  #64 不满足 AC，维持开放，不进入 #65。即时门槛未通过，不启动新的 120 分钟门禁。

## 3. 可回退切换流程：QUIC/auto → HTTP/2（执行前须授权）

前提：cloudflared QUIC 使用 UDP，HTTP/2 使用 TCP；当前 HTTP/2 日志显示 Edge
连接端口为 `:7844`，不能写成固定 `:443`。本步骤用同请求集 A/B 验证，不凭协议名称推断稳定性。

1. **快照原配置（只读复制，便于一键回退）**：
   ```bash
   SNAP="$HOME/.cloudflared-snap-$(date +%Y%m%d-%H%M%S)"
   mkdir -p "$SNAP"
   cp ~/.cloudflared/magicpodcast-prod.yml "$SNAP/"
   cp ~/Library/LaunchAgents/com.cloudflare.cloudflared.plist "$SNAP/" 2>/dev/null || true
   # 记录当前 protocol 值与解析器到快照目录的 README（不含凭据）
   { echo "protocol_before: $(grep -i '^protocol:' ~/.cloudflared/magicpodcast-prod.yml || echo default-quic)";
     echo "dns_before: $(scutil --dns | grep 'nameserver\[0\]' | head -3)"; } > "$SNAP/STATE.txt"
   ```
2. **切换为 HTTP/2**：在 `~/.cloudflared/magicpodcast-prod.yml` 顶层设置 `protocol: http2`（若已有 `protocol:` 行则改其值；不要碰 `credentials-file`、`ingress`、主机名）。
3. **重启 connector**：`launchctl kickstart -k "gui/$(id -u)/com.cloudflare.cloudflared"`。
4. **确认连通（只读）**：`cloudflared --config ~/.cloudflared/magicpodcast-prod.yml tunnel info magicpodcast-prod` 显示已连接；Mac mini 本机 `curl -sS -o /dev/null -w '%{http_code} %{time_total}\n' http://127.0.0.1:8088/`。
5. **回退（任一指标回退或连通异常）**：`cp "$SNAP/magicpodcast-prod.yml" ~/.cloudflared/magicpodcast-prod.yml` → 第 3 步重启 → 停止后续票（#65/#66/#67）并登记。

> 不改 Access、域名、TLS、端口、认证边界；不为前端或后端另开公网主机名；QUIC 与 HTTP/2 都仍是同一条命名 Tunnel、同一主机名。

## 4. A/B 与每 10 分钟巡检方法论（AC#2/#4/#5/#6）

主验收缝：**已认证公网入口**打开真实播客列表（首屏 API + 封面 + 分页 + 失败保留 + 重试）；Mac mini 本机直连仅作对照。两个时段用**同一请求集**分别采样。

- **对照基线（本机直连，无认证）**：Mac mini 上复用现有工具，不改代码：
  ```bash
  node scripts/performance-audit.mjs \
    --base-url http://127.0.0.1:3000 \
    --api-url  http://127.0.0.1:8080 \
    --runs 3 --json
  ```
- **公网认证采样（门禁）**：经已登录 Access 的会话对公网入口连续采样 50 次（列表 API：`/api/v1/podcasts?page=1&page_size=15&view=summary`），记录成功率、P50/P95、最大值、499、15 秒超时与 Tunnel/DNS 错误。`scripts/performance-audit.mjs` 当前无 Access 认证、且部署不创建服务令牌，故认证采样须在 Mac mini 上用所有者浏览器会话或同等认证上下文运行；不要为绕过 Access 而开服务令牌或公开路径。
- **2 小时发布门禁**：先用 50 次公网采样确认即时门槛；通过后才在 HTTP/2 下连续观察 2 小时，每 10 分钟巡检一次（13 个点，首尾覆盖 120 分钟）。任一窗口出现 499 / 15 秒超时 / 成功率或时延回退 → 立即按 §3 第 5 步回退并停止后续票。
- **通过门槛**：列表 API P95 < 1 秒、最大值 < 3 秒；观察窗口内无 499 或 15 秒超时。
- 24 小时复查为**非阻塞**上线后核对，不作为 #64 关闭条件，但结果须登记。

## 5. DNS 处理边界（AC#3）

1. 先做**重复解析与连通性对照**（只读）：对 Mac mini 当前解析器与公共解析器各 `dig +short cloudflare.com`、`dig +short rookiestar.cn` 连续多次，记录是否有超时、NXDOMAIN、地址跳变。
2. **只有重复解析证明确有异常**（如超时率 > 0 或地址反复跳变）才调整 DNS；不把单次成功当作修复证据。
3. 调整时在 macOS 上用**系统设置 → 网络（对应网络接口）** 或 `networksetup -setdnsservers <networkservice> <dns1> <dns2>`（不要直接改 `/etc/resolv.conf`——macOS 下它是生成物、非权威），**先记录原值**（`networksetup -getdnsservers <networkservice>`、`scutil --dns`），保留 §1 的恢复方式。
4. 调整后再用第 1 步重复解析验证；任一回退按 §3 第 5 步处理。

## 6. 完成标准与 AC 映射

| #64 AC | 落点 |
| --- | --- |
| 记录协议/DNS/发布版本/回退方式，不泄露凭据 | §1（已于 2026-08-06 复核） |
| 同请求集 QUIC/auto 与 HTTP/2 对照，记录成功率/P50/P95/最大/499/超时 | §2.8 |
| 仅在重复解析证明确有异常时调整 DNS 并保留恢复方式 | §5 |
| HTTP/2 下连续 2 小时、每 10 分钟巡检 | 未执行：§2.8 即时门槛未通过 |
| 列表 API P95 < 1s、最大 < 3s、无 499 或 15s 超时 | §4 通过门槛 |
| 任一指标回退恢复原配置并停止后续票 | §3 第 5 步 |
| 生产配置、重启等写操作执行前再次取得明确授权 | 本 Runbook 全程标注，授权另取 |

## 7. 明确不在本 Runbook 授权范围

- 不执行 `git commit/push`、PR、部署、`release.sh` 实写、数据库迁移或数据修复。
- 不改 Access 策略、域名、TLS、端口、认证或部署拓扑。
- 不创建 Access 服务令牌、旁路策略或公开路径以图绕过认证采样。
- 上述任一项需在执行前**单独**取得明确授权。
