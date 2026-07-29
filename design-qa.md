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
- Spacing and layout rhythm: desktop preserves the source's editorial header, source strip, list/preview split, hard rules, and dense preview rail. Mobile keeps the complete decision path above the persistent navigation.
- Colors and tokens: warm paper, near-black ink, orange accents, blue evidence links, and brown personal relevance are consistent and contain no decorative gradients.
- Image quality and asset fidelity: the background uses the generated `1254 × 1254` warm paper grid raster. Missing podcast covers remain an honest data state instead of invented artwork.
- Copy and content: all visible claims come from fixture/API data or explicit scope explanations. No recommendation score, editorial claim, external candidate, or invented capability appears.

## Interaction and browser verification

- Desktop: candidate selection, four independent pre-read tabs, Show Notes disclosure, discard/restore/shortlist, and today's shortlist navigation.
- Mobile: one-candidate flow, previous/next controls, `3 / 3` progress, discard/undo/shortlist, today's shortlist navigation, and refresh restoration.
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

## Follow-up polish

- P3: real feed cover art will naturally replace the honest “暂无封面” state when the library provides it.

final result: passed
