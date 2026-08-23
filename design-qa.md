[[31mERROR[0m] - (starship::print): Under a 'dumb' terminal (TERM=dumb).
/Users/bytedance/.zshrc:source:103: no such file or directory: /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh
/Users/bytedance/.zshrc:source:110: no such file or directory: /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh
# Issue #53 / #59 Design QA

## Comparison target

- Source visual truth:
  - `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/reference/A-web-discovery-desk-final.png`
  - `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/reference/A-web-personal-context-final.png`
  - `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/reference/A-mobile-triage-final-v2.png`
- Implementation:
  - `http://127.0.0.1:3100/`
  - `http://127.0.0.1:3100/discovery/today`
- Scope normalization: only personal-library recent episodes, four pre-reads, discard/shortlist, and today's shortlist were compared. Reference-only scores, filters, external candidates, editorial picks, and processing features are intentionally excluded by Issue #53.

## Evidence

### 2026-08-02 phase 1: library, reading detail, and global search

- Scope: `/podcasts`, `/podcasts/1`, and the global search dialog only. Tags, workflows, and import remain outside this phase.
- Same-viewport comparison:
  - Desktop `1434 × 1100`: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/21-desktop-comparison.png`
  - Mobile `390 × 844`: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/16-mobile-comparison.png`
- Final focused evidence:
  - Desktop library: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/18-podcasts-real-covers-desktop.png`
  - Desktop reading detail: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/19-podcast-detail-real-cover-desktop.png`
  - Desktop search: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/20-search-results-after-desktop.png`
  - Mobile library: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/23-podcasts-final-mobile.png`
  - Mobile reading detail: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/24-podcast-detail-final-mobile.png`
  - Mobile search: `/Users/bytedance/.codex/visualizations/2026/08/02/magicpodcast-secondary-surfaces/audit/25-search-final-mobile.png`
- Desktop keeps the existing podcast grid and turns detail into one reading surface with adjacent tags and notes. Search is an `840` CSS px right-side workbench. Mobile keeps the existing bottom navigation, collapsible detail, and full-screen search.
- Mock mode now supplies four distinct editorial cover photographs and four meaningful podcast records. This affects local preview only; no production data or schema changed.
- Real browser journeys covered list tag filtering, desktop and mobile sorting, opening detail, returning to the preserved list state, expanding mobile detail, opening tag and note management, searching `组织`, switching `全部 / 节目 / 单集`, closing the dialog, and restoring focus to the search trigger.
- P0 fixed: a versioned local image URL violated the Next image contract and crashed the page; the final paths are query-free.
- P1 fixed: URL history was mutated from a React state-updater callback; subsequent filter and sort interaction produced no render-time router error.
- P1 fixed: search snippets exposed raw Show Notes markup; final visible content is plain text.
- P2 fixed: the inherited rounded SaaS sorting drawer and hand-written SVGs were replaced by the same paper, hard-rule, ink/orange system and library icon set.
- Desktop and mobile document width equals scroll width. Every visible action target in the three final surfaces is at least `44 × 44` CSS px; mobile sort options are at least `50` CSS px high.
- Fresh post-fix browser journey produced no warning or error. Combined review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-08-02 annotation: simplify and align the decision controls

- Before: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/35-decision-buttons-before.png`.
- Desktop after: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/36-decision-buttons-icons-desktop.png`.
- Mobile after: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/38-decision-buttons-icons-mobile.png`.
- Same-viewport combined comparison, annotated source left / revised implementation right: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/compare-decision-before-left-icons-right.png`.
- Replaced the visible `略过 / 加入今日备选` labels with eye and bookmark icons. The controls retain dynamic accessible names, native hover titles, and visible keyboard-focus tooltips; focus evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/39-decision-button-focus-tooltip.png`.
- Desktop viewport `1434 × 1354`: the left footer and right decision area both begin at `y = 713.78125` and measure `54` CSS px high. Both icon controls are `44 × 44` CSS px.
- Mobile viewport `390 × 844`: both decision controls remain `157 × 48` CSS px and end at `y = 773.77`, before the persistent navigation at `y = 784`; document width and scroll width are both `390`.
- Real browser interaction exercised `加入 / 移出今日备选` and `略过 / 恢复显示`, then restored the first episode to pending. Console warnings/errors: none.
- Same-state desktop/mobile review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-08-02 annotation: clarify pre-reads and unify the two-column frame

- Source visual truth: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/29-right-panel-summary-before.png`.
- Implementation, desktop: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/33-right-panel-semantic-desktop.png`.
- Full-view combined comparison, annotated source left / revised implementation right: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/compare-right-panel-before-left-after-right.png`.
- Desktop viewport and pixels: source and implementation are both `1434 × 1354`, CSS viewport `1434 × 1354`, device scale factor `1`; summary selected, first episode pending.
- Focused mobile evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/34-right-panel-semantic-mobile.png`; pixels and CSS viewport `390 × 844`, device scale factor `1`.
- Replaced the competing left/right outer rules with one continuous workspace frame. The list and preview both begin at `y = 143`, their headings are both `54` CSS px high, and both columns end at `y = 767.78`.
- Secondary row rules use one warm gray `1px` token; black is reserved for the workspace frame, first-level headings, selected tabs, and the decision boundary.
- Removed the collapsed “节目原文” region and redundant footer. A single `44` CSS px “打开节目页面” link now sits with the episode identity; missing links state “节目链接暂缺”.
- Kept four independent #56 pre-reads, but renamed their visible scopes to `摘要 / 核心观点 / 与我相关 / 证据边界`. Their selected panels explicitly explain `这一集讲了什么 / 节目提出的核心主张 / 与你的标签和备注有何关联 / 证据缺口、适用边界与待核问题`.
- Real browser interaction switched all four scopes. “与我相关” retained its independent deep-clay selected treatment, personal-signal content, and relation strength. Add/remove today's shortlist was exercised and restored to pending.
- Mobile viewport `390 × 844`: document width and scroll width are both `390`; all four scope controls are `80.5 × 44` CSS px; decision controls are `157 × 48` CSS px and end at `y = 773.77`, before the persistent navigation at `y = 784`.
- Desktop keyboard resizing changed the split from `60` to `57`; list and preview measured `754.68 / 569.32` CSS px and still shared the same bottom edge.
- Console warnings/errors: none. The combined full-view and focused mobile review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-08-02 annotation: refine the recent-update masthead and desktop columns

- Replaced the framed banner treatment with an open editorial masthead: title, library context, count, and today's shortlist now share one compact baseline and a single bottom rule.
- Desktop viewport `1434 × 1354`: list and preview both end at `y = 893` with a shared height of `702` CSS px. Four candidate rows expand evenly to `149` CSS px, leaving only a `54` CSS px explicit end marker instead of an unstructured blank area.
- Narrow desktop viewport `909 × 1044`: list and preview both end at `y = 886`; document width and scroll width are both `909`, so there is no horizontal overflow.
- The desktop separator is pointer-draggable and keyboard accessible. `ArrowLeft` changed the split from `60` to `57`, moving the measured columns from `796 / 530` to `756 / 570` CSS px.
- Mobile viewport `390 × 844`: the separator is hidden, the compact header is `57` CSS px high, both decision controls remain `48` CSS px high, and document overflow is `0`.
- Same-state browser review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-07-31 annotation: reduce preview chrome and clarify shortlist action

- Removed the repeated preview subtitle `摘要、观点、关联与质疑`; the four pre-read names remain in their dedicated tabs, while the black rail keeps only the section label `内容摘录` and `个人库` context.
- Replaced the ambiguous `留到今天` action with `加入今日备选`; the selected state now reads `移出今日备选`.
- Desktop viewport: `1013 × 1044`; pending decision and summary selected; implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/27-discovery-copy-1013x1044.png`.
- Desktop preview heading measures `55` CSS px; primary decision control measures `113.34 × 48` CSS px; document overflow is `0`.
- Mobile viewport: `390 × 844`; pending decision and summary selected; implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/28-discovery-copy-390x844.png`.
- Mobile primary decision control remains `157 × 48` CSS px; all four pre-read tabs remain `80.5 × 44` CSS px; document overflow is `0`.
- Real browser interaction confirmed `加入今日备选 → 移出今日备选`, then restored the pending state. No console errors or warnings; no new P0/P1/P2 issue observed.

### 2026-07-31 annotation: merge the shelf identity into recent updates

- The annotated low-density masthead was removed. “个人播客知识库 / 你的播客书架” now sits in the same `个人库最近更新` header as the recent-update title, count, and today's-shortlist link; the content ledger starts immediately below it.
- Desktop viewport: `1172 × 1044`; summary pre-read selected; implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/25-discovery-merged-header-1172x1044-summary.png`.
- The merged header measures `118.84` CSS px and the standalone intro count is `0`; document overflow is `0`.
- Mobile viewport: `390 × 844`; summary pre-read selected; implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/26-discovery-merged-header-390x844-summary.png`.
- Mobile merged header measures `57.34` CSS px. Decision controls remain `157 × 48` CSS px, all four pre-read controls remain `80.5 × 44` CSS px, and document overflow is `0`.
- The real browser check switched “与我相关” and confirmed different evidence content plus an active pressed state, then restored the summary state. No new P0/P1/P2 issue was observed.

### 2026-07-30 content-first correction with representative real data

- The desktop banner no longer repeats global navigation or presents triage as the product identity. It is a `124.55` CSS px knowledge-library masthead followed by an `89.40` CSS px recent-update ledger.
- The controlled preview database `/tmp/magicpodcast-issue53-preview.db` contains five current public RSS episodes from 故事FM、文化有限、商业就是这样、东腔西调、声东击西. Public titles, covers, dates, durations, links, and Show Notes are real; personal tags and notes are explicitly simulated preview metadata.
- Desktop viewport: `1440 × 1100`; personal relevance selected; implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/23-home-content-first-real-data-1440x1100-relevant.png`.
- Desktop combined comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/compare-desktop-content-first-reference-left-implementation-right.png`.
- Mobile viewport: `390 × 844`; summary selected and temporary shortlist state visible; implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/24-home-content-first-real-data-390x844-summary-shortlisted.png`.
- Mobile combined comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/compare-mobile-content-first-reference-left-implementation-right.png`.
- Full-view desktop evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/20-home-content-first-real-data-1440x1080-final.png`.
- Focused mobile evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/22-home-content-first-real-data-390x844.png`.
- Mobile decision controls measure `157 × 48` CSS px. Previous/next and all four pre-read controls are `44` CSS px high. Minimum visible interactive height is `44` CSS px and document overflow is `0`.
- Browser journeys covered candidate switching, all pre-read types, distinct personal relevance, discard/restore, shortlist/remove, and mobile previous/next. The final console contained no warnings or errors.

### 2026-07-29 homepage information-architecture correction

- User feedback re-scoped the desktop homepage from a discovery-led page to a podcast knowledge desk with discovery as one daily workspace.
- Desktop viewport: `1440 × 1080`; personal relevance selected; screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/16-home-knowledge-desk-1440x1080-relevant.png`.
- Mobile viewport: `390 × 844`; same personal-relevance state; screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/17-home-mobile-390x844-relevant.png`.
- Desktop knowledge header is `155` CSS px high. The daily-workspace header is `99.59` CSS px high. List and preview begin at the same `y = 297.59` and share the same `714.90` CSS px height.
- Mobile actions remain `157 × 48` CSS px; the today's-shortlist target is `64 × 45.34` CSS px; document overflow is `0`.
- Real browser interaction exposed and fixed a P0 null-source crash in “与我相关”. Unavailable remote covers now switch to the honest fallback instead of rendering broken images.

### Desktop full view

- Viewport and CSS size: `1440 × 1100`.
- Source pixels: `1440 × 1100`.
- Implementation pixels: `1440 × 1100`.
- Device scale factor: `1`; no density conversion.
- State: first recent episode selected and shortlisted; summary pre-read visible.
- Implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/15-issue-59-home-1440x1100-shortlisted.png`
- Combined comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/compare-desktop-reference-left-implementation-right-final.png`

### Mobile full view

- Viewport and CSS size: `390 × 844`.
- Source pixels: `390 × 844`.
- Implementation pixels: `390 × 844`.
- Device scale factor: `1`; no density conversion.
- State: one selected candidate, summary visible, shortlisted state visible.
- Implementation screenshot: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/12-issue-59-mobile-390-final.png`
- Combined comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/compare-mobile-reference-left-implementation-right.png`

### Focused regions

- Personal relevance selected state: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/09-issue-59-personal-context-1440.png`
  - “与我相关” has a non-color pressed state, a distinct brown treatment, relation strength, different content, and a real note source.
- Mobile actions: both controls measure `157 × 48` CSS px; all navigation and pre-read controls are at least `44` CSS px high.
- Mobile document width: `390` CSS px with `scrollWidth = 390`; no horizontal overflow.
- Today's shortlist mobile journey: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/13-issue-59-today-mobile-390.png`

### 2026-08-02 annotation: replace the navigation wordmark

- Replaced the two-line `MAGIC / PODCAST · 01` label with a real raster mark plus deterministic `MagicPodcast` typography.
- The mark combines a radio tuning window and an open page; its second refinement restores five dial ticks around the orange needle. Its source was produced and refined through Creative Production board `447a9f24-456b-4e7e-b9a3-b57c0488e735`. It contains no generated text.
- User feedback removed the split `Magic`/`Podcast` hierarchy: the complete name now uses one `18` px face, one weight, and no logo-local underline. The only orange navigation underline is the consistent `3` px active-section indicator.
- Desktop viewport `909 × 1044`: brand target is `165.34 × 44` CSS px, mark is `32 × 32` CSS px, and document overflow is `0`.
- Mobile viewport `390 × 844`: desktop navigation remains hidden by the existing responsive contract and document overflow is `0`; the discovery journey is unchanged.
- Same-state browser review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-08-02 annotation: simplify the desktop search entrance

- Replaced the boxed `搜索` label with a borderless `20 × 20` magnifier while preserving the accessible name and click behavior.
- Desktop viewport `909 × 1044`: the visual glyph is centered exactly inside a `44 × 44` CSS px interaction target; document overflow is `0`.
- Real browser interaction opened the search field and close control successfully. Mobile navigation remains unchanged.
- The first visual pass exposed a P2 left-alignment error; centering was corrected and remeasured at `0 × 0` CSS px center delta. `final result: passed`.

### 2026-08-02 annotation: compress the recent-update header

- Removed `个人播客知识库` and `你的播客书架`; replaced two explanatory sentences with `订阅单集，按发布时间排序。`.
- Aligned the section with the podcast-library typographic system: the desktop title now uses the same Iowan/Baskerville/Songti stack, `22` px size, `800` weight, and editorial tracking instead of the previous `32–42` px display treatment. Mobile remains `20` px.
- Desktop viewport `1434 × 1354`: the header starts at the `64` CSS px navigation edge, measures `60` CSS px high, and places the title `9.85` CSS px below the navigation; document overflow is `0`.
- Mobile viewport `390 × 844`: the compact header remains `57.34` CSS px high, primary decision controls remain `48` CSS px high, and document overflow is `0`.
- Same-state browser review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-08-02 annotation: refine podcast sorting and New marker

- Replaced the boxed desktop select with a restrained `128 × 44` CSS px control: one explicit sort icon, the current native option, and a chevron. It has no visible “排序方式” copy and no independent underline; the native select still owns the full interaction area and accessible name `排序方式`.
- Restored the established English `New` marker with a compact translucent green treatment instead of the orange Chinese `新` badge.
- Desktop viewport `909 × 1044`: all four options remain selectable, the URL and selected value update together, the marker renders green, document overflow is `0`, and the fresh console has no warnings or errors.
- Mobile viewport `390 × 844`: the existing sort drawer opens with all four options, the sort target is `44 × 44` CSS px, the New marker is `35.5 × 18` CSS px without cover collision, document overflow is `0`, and the fresh console has no warnings or errors.
- Same-state browser review found no remaining P0/P1/P2 issue. `final result: passed`.

### 2026-08-02 annotation: align the podcast-library header

- Unified the podcast toolbar with the homepage recent-update header: the same warm paper grid texture, opaque paper color, `2` px ink divider, editorial title stack, and horizontal title/description rhythm.
- Desktop viewport `1434 × 1354`: the inner header is `60` CSS px high; title is `22` px / `800`, description is `13` px / `500`, and the sort control stays vertically centered. The divider is inset to the content grid at `x = 43`, width `1348`, instead of spanning the viewport. Document overflow is `0`.
- Mobile viewport `390 × 844`: the header is `62` CSS px including its divider, title is `20` px, and all actions retain their `44` px targets. The divider is inset to `x = 16`, width `358`. Document overflow is `0`.
- Fresh browser console contained no warnings or errors. Same-state review found no remaining P0/P1/P2 issue. `final result: passed`.

## Required fidelity surfaces

- Fonts and typography: Songti-style editorial display hierarchy and monospace metadata match the source direction; fallback stacks remain legible in Chinese.
- Spacing and layout rhythm: desktop uses one merged knowledge-library/recent-update header, aligned `3:2` list/preview columns, hard rules, and a dense preview rail. Mobile gives content the first viewport and keeps the complete decision path reachable above the persistent navigation after normal scroll.
- Colors and tokens: warm paper, near-black ink, orange accents, blue evidence links, and brown personal relevance are consistent and contain no decorative gradients.
- Image quality and asset fidelity: the background uses the generated `1254 × 1254` warm paper grid raster; representative episode covers come from the public RSS feeds rather than invented artwork.
- Copy and content: public episode content comes from the current RSS feeds. Personal tags/notes are simulated preview metadata and only drive the existing relevance contract. No recommendation score, editorial claim, external candidate, or invented capability appears.

## Interaction and browser verification

- Desktop: candidate selection, four independent pre-read scopes, source-link state, discard/restore/shortlist, and today's shortlist navigation.
- Mobile: one-candidate flow, previous/next controls, `3 / 5` progress, discard/restore/shortlist, and no horizontal overflow.
- Active navigation uses `aria-current="page"` plus border/weight, not color alone.
- Fresh browser console check after server restart: no warnings or errors.

## Comparison history

1. Initial mobile evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/10-issue-59-mobile-390.png`
   - [P1] The introductory block pushed the 48 px decision controls below the first viewport.
   - Fix: removed the hidden desktop-navbar spacer on mobile, compacted the intro, kept the today's-shortlist link tappable, changed four pre-reads to one row, and reduced unused panel minimum height.
2. Intermediate evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/11-issue-59-mobile-390-compact.png`
   - [P2] Decision controls still ended below the persistent bottom navigation.
   - Fix: removed the remaining mobile spacer and compressed only mobile pre-read layout.
3. Post-fix evidence: `/Users/bytedance/.codex/visualizations/2026/07/28/019faacf-bcc3-7861-b80c-fa87aba9fac3/issue-53/audit/12-issue-59-mobile-390-final.png`
   - Decision controls end at `776.14` CSS px; bottom navigation starts at `784` CSS px.
   - No actionable P0/P1/P2 findings remain.
4. Homepage correction:
   - [P1] Desktop positioned candidate triage as the product identity, used an oversized low-information banner, and fragmented supporting facts into equal-weight color blocks.
   - Fix: made the podcast library, tags/notes, and workflows first-class knowledge-management entrances; reduced the header; replaced the source strip with a compact daily-workspace boundary; aligned the list and preview ledger.
   - [P1] Invalid remote cover URLs rendered broken image chrome.
   - Fix: added a stable honest fallback.
   - [P0] A runtime `null` pre-read source list crashed the personal-relevance journey.
   - Fix: normalized absent sources to an empty evidence state and covered it with a regression test.
   - No actionable P0/P1/P2 findings remain.
5. Content-first correction with real-feed density:
   - [P1] The desktop masthead still repeated recent-update language and consumed space without establishing the knowledge-management identity.
   - Fix: changed it to a compact “你的播客书架” masthead, removed all repeated function entrances, and reduced its visual height.
   - [P1] One fictional episode could not expose long-title, cover, summary-density, or mixed-duration problems.
   - Fix: replaced the temporary fixture with five current public RSS episodes and simulated only the personal tags/notes required to exercise “与我相关”.
   - Post-fix evidence: the five-item list remains aligned, real covers render, long titles truncate without collision, mobile actions remain `48` px, all interactive targets remain at least `44` px, and horizontal overflow is `0`.
   - The mobile action area intentionally follows the content instead of dominating the first viewport; it remains reachable and unobscured after normal scroll.
   - No actionable P0/P1/P2 findings remain.

## 2026-08-10 annotation: simplify and align the selected-report header

- Source visual truth: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-header-before.png`, plus the user annotations to remove the four-card report strip, align the header with `最近更新`, and rename it `精选报告`.
- Desktop implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-selected-report-after.png`.
- Same-viewport comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-before-after.png`.
- Desktop source and implementation pixels and CSS viewport: `1280 × 720`; browser device pixel ratio `2`, with captures normalized to CSS pixels. State: first of four real reports selected.
- Mobile implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-selected-report-mobile.png`; pixels and CSS iframe viewport `390 × 844`, scale `1`, first report selected.
- Fonts and typography: `精选报告` and `最近更新` both compute to the same display stack, `22px`, weight `650`, line-height `24.2px`, and normal tracking on desktop; mobile uses the shared `20px` section-title scale.
- Spacing and layout rhythm: the redundant card strip is absent; the report header now uses the same `60px` editorial row, `16px` inset, and `2px` ink divider as the recent-update header. Mobile uses the same inset divider and has no horizontal overflow (`scrollWidth = clientWidth = 390`).
- Colors and visual tokens: existing paper, ink, orange type badge, and control colors are unchanged.
- Image quality and assets: report content and existing covers are unchanged; no new asset or placeholder was introduced.
- Copy and content: visible and accessible section, loading, and error labels now use `精选报告`; report content is unchanged.
- Interaction: desktop and mobile next-report controls changed `1 / 4 → 2 / 4`; mobile DOM touch activation also changed `2 / 4 → 3 / 4`. Single-report state has no switch group or `1 / 1`.
- Console: no warnings or errors in the fresh desktop or mobile runs.
- Comparison history: the initial source had one P1 density issue (four duplicate report cards) and one P2 consistency issue (small orange kicker without the shared section divider). Both were removed in the revised capture; no actionable P0/P1/P2 finding remains.

## 2026-08-10 annotation: distinguish report cadence and surface quick-add episodes

- Source visual truth: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-selected-report-after.png`, plus the user annotations to distinguish weekly/daily cadence with restrained colors and move the quick-add episode list below the report title.
- Desktop implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-order-color-desktop.png`.
- Full-view comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-order-color-compare.png`.
- Focused header/report comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-order-color-focus-compare.png`.
- Desktop source and implementation pixels and CSS viewport: `1280 × 720`, compared directly at the same normalized CSS-pixel size. State: first weekly report selected with two real episodes.
- Daily cadence state: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-daily-color-desktop.png`; second of four real reports selected.
- Mobile implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-order-color-mobile.png`; pixels and CSS viewport `390 × 844`, device pixel ratio `1`, first weekly report selected.
- Fonts and typography: the report title and body retain the shared reading typography; the two compact episode rows now sit immediately after the H1 without changing their title or metadata scales.
- Spacing and layout rhythm: visible order is `workflow-report-title → workflow-report-episodes → workflow-report-body`; the episode block uses the existing warm-gray separators, and one/two-item reports keep natural height. Mobile has no horizontal overflow (`scrollWidth = clientWidth = 390`).
- Colors and visual tokens: weekly uses restrained gray-blue (`#53686f` text, `#65777b` border, `rgba(224, 231, 229, 0.88)` background); daily retains the existing terracotta treatment (`#c4552a`, `rgba(249, 236, 224, 0.9)`). Both remain secondary to ink and paper.
- Image quality and assets: existing real episode covers, crops, and fallbacks are unchanged; no new image asset or approximation was introduced.
- Copy and content: the Markdown H1 and body remain verbatim; only their visual placement around the existing episode controls changes. Zero-episode reports remain one complete Markdown document.
- Interaction: next-report changed weekly `1 / 4` to daily `2 / 4` and exposed the correct cadence token; episode expand/collapse passed on mobile. Automated coverage retains shortlist behavior without writing production state during visual QA.
- Console: no warnings or errors in the desktop/mobile run.
- Comparison history: the source exposed a P2 action-discovery issue because quick-add episodes appeared only after the long report body, and a P2 cadence issue because weekly reused the daily orange token. The revised capture moves the episodes above the body and gives weekly a low-saturation gray-blue token; no actionable P0/P1/P2 finding remains.

## 2026-08-10 annotation: distinguish the embedded episode block

- Source visual truth: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-daily-color-desktop.png`, plus the user annotation requesting a refined background distinction between embedded episodes and the report.
- Desktop implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-episode-sage-desktop-collapsed.png`.
- Full-view comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-episode-sage-compare.png`.
- Focused report comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-episode-sage-focus-compare.png`.
- Desktop source and implementation pixels and CSS viewport: `1280 × 720`, compared directly at the same normalized CSS-pixel size. State: second daily report selected with six real episodes, all collapsed.
- Expanded-state implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-episode-sage-desktop.png`.
- Mobile implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-episode-sage-mobile.png`; pixels and CSS viewport `390 × 844`, device pixel ratio `1`, same report and collapsed state.
- Fonts and typography: title, show name, episode title, metadata, and controls are unchanged; the tint does not reduce text hierarchy or legibility.
- Spacing and layout rhythm: component size, row height, separators, cover crops, and surrounding report spacing are unchanged. Mobile has no horizontal overflow (`scrollWidth = clientWidth = 390`).
- Colors and visual tokens: the embedded episode block now uses low-saturation sage mist `rgba(224, 229, 220, 0.78)`, distinct from the warm report paper while remaining quieter than the daily terracotta and weekly gray-blue cadence tokens.
- Image quality and assets: real episode covers and their borders/crops are unchanged; no new image asset or approximation was introduced.
- Copy and content: report and episode copy are unchanged.
- Interaction: desktop and mobile expand/collapse remain functional; shortlist controls are unchanged. No production decision write was made during visual QA.
- Console: no warnings or errors in the desktop/mobile run.
- Comparison history: the source had one P2 grouping issue because the embedded episode rows shared the report-paper background and visually merged with the body. The revised capture adds one restrained shared block tint without changing structure or semantics; no actionable P0/P1/P2 finding remains.

## 2026-08-10 annotation: replace recommendation copy with Show Notes

- Source visual truth: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-episode-sage-desktop.png`, plus the user annotation to remove the unimplemented recommendation rationale and show only the episode Show Notes preview.
- Desktop implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-shownotes-only-desktop.png`.
- Focused comparison, source left / implementation right: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-shownotes-only-focus-compare.png`.
- Mobile implementation: `/Users/bytedance/.codex/visualizations/2026/08/10/019fea29-2595-7f93-9509-3aaaf59d9614/magicpodcast-report-shownotes-only-mobile.png`; viewport `390 × 844`.
- Copy and content: the expanded episode no longer renders recommendation data or the fallback recommendation sentence. `context`, then `excerpt`, supplies the `Show Notes` preview; when both are empty, the label is omitted and a valid source link remains available.
- Interaction and layout: expand/collapse and shortlist behavior are unchanged; the sage episode grouping remains intact. Mobile has no horizontal overflow.
- Console: no warnings or errors in the desktop/mobile run.
- Comparison history: the source exposed a P1 truthfulness issue by presenting an unimplemented recommendation rationale and an invented fallback. The revised capture shows only source-backed Show Notes; no actionable P0/P1/P2 finding remains.

## 2026-08-11 annotation: refactor the Actions reading and editing surface

- Source visual truth: the approved marginalia direction with three refinements—Show Notes as the primary surface, existing Episode/Podcast tags and notes only, and a collapsible editor.
- Desktop viewport `1440 × 1024`, using 30 real recent episodes. Episodes and Actions share an `862` CSS px workspace; the episode list and Show Notes scroll independently. Document width and scroll width are both `1440`.
- Collapsed state gives Show Notes the full `531` CSS px Actions width. Expanded state uses a restrained `400 / 290` CSS px reading/editor split; collapsing restores the full reading width.
- Copy and hierarchy: `Episodes` and `Actions` use the same editorial heading treatment. The duplicate cover and AI pre-read tabs are absent; source-backed Show Notes begin immediately after the compact episode identity.
- Interaction: the pencil opens and closes metadata editing; switching episodes preserves the open editor; `Episode / Podcast` changes the existing tag-and-note target and loads metadata on demand. Decision icons remain beside the podcast name.
- Mobile viewport `390 × 844`: Episodes is replaced by the existing previous/next flow, the editor becomes a fixed `390 × 784` CSS px layer above bottom navigation, and its close target is `44 × 44` CSS px. Closing preserves the underlying page position. Document width and scroll width are both `390`.
- Runtime: the homepage responds `200`; Show Notes render after client hydration without invoking the browser-only sanitizer during SSR.
- Console: no warnings or errors in fresh desktop and mobile runs. No actionable P0/P1/P2 finding remains.

## 2026-08-11 annotation: collapse discovery identity into Quick Actions

- Copy and hierarchy: the preview heading is now `Quick Actions`; the selected episode identity block and visible `Show Notes` heading are removed so the source-backed reading surface starts immediately below the toolbar.
- Interaction: open episode page, ignore/restore, and shortlist/unshortlist are icon-only controls with accessible labels and hover titles in the heading row. The existing pencil remains a separate entry point for the approved collapsible Episode/Podcast metadata editor.
- Responsive check: at `390 × 844`, `Quick Actions` remains on one line, the three triage/source controls retain `44 × 44` px touch targets, and document width equals client width with no horizontal overflow.

## Follow-up polish

- P3: test a larger personal library with several episodes per show before treating five-item density as a performance or long-list benchmark.

## 2026-08-12 search overlay and modal unification

- Visual source truth: `/Users/bytedance/.codex/generated_images/019fee58-2c9b-70e0-9dbd-768a3762ffdc/call_LkLwMv9tFYKrD6GSu1YpJU0P.png`.
- Browser-rendered implementation: `/Users/bytedance/.codex/visualizations/2026/08/11/019fee58-2c9b-70e0-9dbd-768a3762ffdc/search-overlay-production-qa/search-implementation-1487x1058.png`.
- Full-view comparison: `/Users/bytedance/.codex/visualizations/2026/08/11/019fee58-2c9b-70e0-9dbd-768a3762ffdc/search-overlay-production-qa/search-side-by-side.png`.
- Focused drawer comparison: `/Users/bytedance/.codex/visualizations/2026/08/11/019fee58-2c9b-70e0-9dbd-768a3762ffdc/search-overlay-production-qa/search-drawer-focused-comparison.png`.
- Source and implementation are both `1487 × 1058` pixels. Implementation CSS viewport is `1487 × 1058`, device pixel ratio `1`; the focused comparison normalizes the source drawer to the approved `640` CSS px width.
- State: `/podcasts` with global search open, query `人工智能`, all results selected, `29` results (`9` podcasts and `20` episodes).
- Fonts and typography: existing self-hosted display, body, and mono stacks are retained. The black kicker, search query, section headings, metadata, and snippets preserve the approved hierarchy.
- Spacing and layout rhythm: desktop search is a `640 × 1058` right-side sheet with one readable result column. At `390 × 844`, search is full-screen and sorting is a bottom drawer. Tag, workflow, and report dialog internals have no horizontal overflow or overlapping persistent controls.
- Colors and visual tokens: black/white contrast, warm paper, orange emphasis, blue focus ring, hard rules, square controls, and restrained shadows are shared across search, tag, workflow, report, and mobile sort overlays.
- Image quality and assets: real podcast covers remain sharp. All revised controls use the existing Tabler icon library; no placeholder art, custom SVG, or new image asset was introduced.
- Copy and content: search scope, counts, history, workflow steps, and report actions preserve existing semantics. No recommendation, shortcut, AI-search, or new sorting capability was added.
- Interactions tested in the real browser: search input and scope filters, Escape close with focus restoration, tag-create dialog, workflow-create dialog, report dialog, report links/images/download action, and mobile sort drawer.
- Responsive evidence: search is `390 × 844`; sort drawer is `390` wide; tag, workflow, and report dialogs have no internal horizontal overflow at `390 × 844`. Primary mobile controls and close controls meet the `44px` target.
- Report evidence: the current real report renders `10` clickable links, `2` images, and the existing Markdown download action.
- Browser console warnings/errors: none.
- Automated evidence: `.agents/skills/code-change-verification/scripts/verify.sh` passed TypeScript and all `103` test files / `535` tests. `git diff --check` passed.
- Comparison history: the earlier implementation retained a wider two-column podcast grid and inconsistent surrounding modal chrome. The revision uses the approved `640px` single-column search sheet and one editorial shell across related overlays. Post-fix same-size and focused comparisons have no actionable P0/P1/P2.
- P3: the concept image and production use different dynamic timestamps and result records; structure, density, typography, and interaction hierarchy remain equivalent.

final result: passed

## 2026-08-23 Issue #173: reversible homepage Inbox bookmark

- Source visual truth: `/Users/bytedance/.codex/generated_images/01a02eea-5e0f-7f41-a15f-af8a56ec2564/exec-e7c76356-badc-4836-aff3-3fc4e8698338.png` (`1905 × 825` pixels).
- Browser-rendered implementation:
  - Desktop: `/Users/bytedance/.codex/visualizations/2026/08/23/01a02eea-5e0f-7f41-a15f-af8a56ec2564/issue-173/implementation-desktop-1440-inbox.png` (`1440 × 1000` pixels and CSS viewport, device scale factor `1`).
  - Mobile: `/Users/bytedance/.codex/visualizations/2026/08/23/01a02eea-5e0f-7f41-a15f-af8a56ec2564/issue-173/implementation-mobile-390-inbox.png` (`390 × 844` pixels and CSS viewport, device scale factor `1`).
- Full-view comparison, source left and implementation right: `/Users/bytedance/.codex/visualizations/2026/08/23/01a02eea-5e0f-7f41-a15f-af8a56ec2564/issue-173/comparison-source-left-desktop-right.png`.
- Focused comparison, source left and implementation right: `/Users/bytedance/.codex/visualizations/2026/08/23/01a02eea-5e0f-7f41-a15f-af8a56ec2564/issue-173/comparison-focused-icons-normalized.png`. The crop was normalized to `220` pixels high; no density conversion was needed.
- State: the same Fixture episode is in Inbox in the report, recent list, and mobile preview. The implementation uses the selected blue solid bookmark without the former persistent blue rectangular frame.
- Full-view evidence: existing editorial hierarchy, report/list layout, typography, paper texture, covers, copy, and responsive structure are unchanged. Mobile document width and scroll width are both `390`.
- Focused evidence: the orange outlined bookmark-plus becomes the existing Tabler solid bookmark, colored `rgb(23, 104, 208)`. Report, list, and preview controls are `44 × 44` CSS px. Inbox controls have transparent background and transparent/zero-width visible border; keyboard focus remains a `3px` blue outline.
- Interaction evidence: collection and removal were exercised from report and preview, with synchronized report/recent/preview states, Inbox count, and the `未收集` filter; the Fixture was restored afterward. Focus/Someday/Done protection and failed-write rollback are covered by the Discovery integration tests. Browser console warnings/errors: none.
- Required fidelity surfaces:
  - Fonts and typography: unchanged.
  - Spacing and layout rhythm: unchanged; the icon remains optically centered in the existing `44 × 44` target.
  - Colors and visual tokens: existing orange action and blue Inbox tokens match the approved source.
  - Image quality and assets: existing covers are unchanged; the solid bookmark comes from the existing Tabler icon library, with no new image or custom SVG.
  - Copy and content: visible content is unchanged; accessible action names switch between `收集到 Inbox` and `从 Inbox 移除`.
- Findings: no actionable P0/P1/P2 difference. The source is a focused state concept rather than a complete page mock, so unchanged surrounding layout and content are expected.
- Comparison history: the first combined full-view and focused comparisons passed; no visual fix iteration was required.

final result: passed
