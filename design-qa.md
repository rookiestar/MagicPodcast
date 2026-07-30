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

## Required fidelity surfaces

- Fonts and typography: Songti-style editorial display hierarchy and monospace metadata match the source direction; fallback stacks remain legible in Chinese.
- Spacing and layout rhythm: desktop uses one merged knowledge-library/recent-update header, aligned `3:2` list/preview columns, hard rules, and a dense preview rail. Mobile gives content the first viewport and keeps the complete decision path reachable above the persistent navigation after normal scroll.
- Colors and tokens: warm paper, near-black ink, orange accents, blue evidence links, and brown personal relevance are consistent and contain no decorative gradients.
- Image quality and asset fidelity: the background uses the generated `1254 × 1254` warm paper grid raster; representative episode covers come from the public RSS feeds rather than invented artwork.
- Copy and content: public episode content comes from the current RSS feeds. Personal tags/notes are simulated preview metadata and only drive the existing relevance contract. No recommendation score, editorial claim, external candidate, or invented capability appears.

## Interaction and browser verification

- Desktop: five-candidate selection, four independent pre-read tabs, Show Notes disclosure, discard/restore/shortlist, and today's shortlist navigation.
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

## Follow-up polish

- P3: test a larger personal library with several episodes per show before treating five-item density as a performance or long-list benchmark.

final result: passed
