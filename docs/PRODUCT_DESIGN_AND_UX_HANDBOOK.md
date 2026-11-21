# Product Design and UX Handbook

> **Document status:** Portable draft in `TestDesk` checkout (`docs/PRODUCT_DESIGN_AND_UX_HANDBOOK.md`).  
> **Intended shared destination:** `Projects/repo-template/docs/product-design-and-ux-handbook.md` (reuse or promote there once workspace governance approves adding a shared design standard alongside `repo-standard.md`).  
> **Sources verified:** `2026-11-21`.

This handbook defines reusable product design, interaction, UI writing, and accessibility rules for desktop and web-backed applications (including Wails/WebView2 and native shells). Keep product-specific tokens, copy, and architecture out of this document and in each project's implementation handoff.

---

## Up-Front Review Checklist

Use this checklist before approving any new screen, refactor, or design handoff:

1. **Task clarity & entry:** Can a first-time user see what the product does, what scope is active, and what primary action comes next within 3 seconds without reading documentation?
2. **Repeat-user speed:** Can an experienced user reach frequent actions (filter, run, inspect, retry, switch context) in 1–2 steps with keyboard shortcuts or compact controls?
3. **Action & visual hierarchy:** Is there at most one primary filled CTA per focal region, with destructive actions clearly separated and disabled states visibly distinct?
4. **Density & whitespace:** Are data rows scannable without horizontal clipping, while avoiding hollow cards with empty voids or redundant stacked headings?
5. **State completeness:** Are `empty`, `loading`, `idle`, `active/running`, `paused`, `success`, `warning`, `error/recovery`, and `unavailable/stale` states explicitly designed?
6. **Status & error honesty:** Do status indicators combine text/icon + color (never color alone), avoid fabricating freshness when polling fails, and state *what happened* + *how to recover*?
7. **Accessibility baseline:** Do all text (`≥ 4.5:1` normal, `≥ 3:1` large) and UI borders/icons (`≥ 3:1`) meet WCAG 2.2 AA contrast, are interactive targets `≥ 24 × 24` CSS px without overlap, is every control keyboard-operable with visible focus (`:focus-visible`), and do all inputs/controls expose accurate accessible names?
8. **Platform & user settings:** Does the layout hold at 100%, 125%, and 150% DPI scaling, honor `prefers-reduced-motion: reduce`, and preserve semantic boundaries under Windows High Contrast (`forced-colors: active`)?

---

## Requirements vs. Defaults

- **Requirement (Must Not Violate):** Legal/accessibility baselines (WCAG 2.2 Level A & AA criteria as interpreted for non-web software via WCAG2ICT), truthful system state, non-destructive safety guards, and keyboard operability.
- **Default (Standard Practice):** House layout, spacing, typography, density, and component defaults defined below.
- **Overriding a Default:** Only override a default when a measured user workflow benefits (e.g., ultra-dense log tables or compact HUD windows) **and** the override documents its trade-off and preserves all Requirements (contrast, target size, keyboard access, and clarity).

---

## 1. Task Discovery, Information Architecture, and User Tiers

### 1.1 Progressive Orientation (Beginners vs. Repeat Users)
- **Decision rule:** Separate first-run setup/onboarding from daily execution, but always show active scope (which workspace/project/file is targeted) and the next valid action. Never expose navigation controls into unconfigured states.
- **Example:** When no workspace is configured, open directly into workspace selection/creation and hide or disable "Back to Workspace" until setup succeeds. Once configured, land directly in the primary work surface.
- **Common failure mode:** Showing an enabled "Back" button or shortcut (`Alt+M`, `F10`) before setup exists, dumping new users onto an empty shell or wrong mode.
- **Verification:** Walk through cold start with clean configuration (`TESTDESK_CONFIG_HOME` or empty profile); confirm zero dead-end buttons and clear next step.

### 1.2 Information Architecture & Grouping
- **Decision rule:** Group controls by *user intent and frequency* (`Scope & Context` → `Summary / Filter` → `Action Bar` → `Master / Detail Workspace`), not by backend data model. Use at most two levels of navigation (primary view switch + local sub-tabs).
- **Example:** Pair a left master/monitor pane (active task + recent history) with a main catalog/results pane, separated by a resizable splitter, rather than scattering run controls across modals.
- **Common failure mode:** Repeating the screen's purpose across three stacked headings (page eyebrow + H1 + subtitle + tab + section eyebrow + H2) before any interactive control appears.
- **Verification:** Count visual headings above the first interactive control; if a screen has >1 heading layer describing the same section, merge or remove the redundant layer.

### 1.3 Recognising Comparable Desktop Patterns
- **Pattern — Status Filtering & Targeted Rerun (e.g., VS Code Test Explorer):** Expose one-click status filter chips/dropdowns (`Failed`, `Passed`, `Skipped`) and a contextual **"Rerun Failed"** action wherever failures are summarized so users don't rebuild selections manually. *Trade-off:* Adds one conditional action in failure states; saves repeated multi-step filtering.
- **Pattern — Glanceable Companion HUD (e.g., task/build mini-players):** Keep only active target identity, live status + elapsed time, progress counts, and transport controls (`Pause/Resume`, `Skip`, `Stop`, `Expand`) in the compact surface; push logs and configuration to the full window.

---

## 2. Visual Hierarchy, Typography, Spacing, Density, Color, Iconography, Motion, and Whitespace

### 2.1 Typography (Default)
- **Decision rule:** Use the platform's native variable UI sans-serif (`"Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif` on Windows) for chrome and `ui-monospace, Consolas, monospace` for identifiers, paths, commands, and logs. Keep body text at `13–14px` (line-height `1.45`), metadata at `12px`, and avoid text below `11px` for readable labels. Use `tabular-nums` (`font-variant-numeric: tabular-nums`) for live timers, counters, and durations.
- **Example:** Live elapsed clock `0:23` and `5/10 items finished` use `tabular-nums` so numbers do not jitter horizontally during updates.
- **Common failure mode:** Unstyled browser defaults (`<a>` rendering `#0000ee` on dark surfaces) or proportional fonts in live clocks causing layout jitter.
- **Verification:** Inspect computed font-family, size, and `font-variant-numeric` in DevTools; watch a 10-second live timer for horizontal shift.

### 2.2 Spacing, Alignment, and Purposeful Whitespace (Default)
- **Decision rule:** Anchor spacing to a `4px` base unit (`4, 8, 12, 16, 20, 24px`). Size cards to their content (`height: auto`) unless filling a split-pane viewport (`flex: 1` for scrolling lists/logs). Never stretch status badges or pills across grid tracks unless explicitly intended as full-width banners.
- **Example:** A status pill uses `width: fit-content` (`justify-self: start` or inline-flex) so `FAILED` or `INSTALLED` stays compact next to its title instead of stretching across 300px and crushing the item title to 6 characters.
- **Common failure mode:** Global `.pill { width: 100% }` styles stretching inside flex/grid headers, or `margin-top: auto` inside fixed-height cards creating large hollow voids above footer buttons.
- **Verification:** Render every view with both 1 item and 50 items; check for crushed titles (`text-overflow: ellipsis` before 12 chars) and >120px unintentional empty card gaps.

### 2.3 Density & Responsive Bounds
- **Decision rule:** Support minimum desktop work areas comfortably (e.g., `1280 × 720` logical pixels, matching `1920 × 1080` at `150%` Windows DPI scaling). Allow secondary views (Setup, Settings) to shrink below default dimensions with internal scrolling rather than locking `MinHeight` above `720px`.
- **Example:** Default Settings to `1160 × 760` with `MinWidth: 640, MinHeight: 520` and a scrollable body, rather than locking `MinHeight: 795`.
- **Common failure mode:** Setting `MinHeight == MaxHeight == 810` on native dialogs/views, clipping bottom action bars off-screen on 768p or 150%-scaled 1080p laptops.
- **Verification:** Resize window to `640 × 520` and `1280 × 720` (and test at 150% Windows scaling); verify all headers, tabs, and primary buttons remain reachable.

### 2.4 Color & Semantic Tokens (Requirement + Default)
- **Decision rule:** Define semantic tokens (`--bg`, `--surface`, `--surface-2`, `--text`, `--muted`, `--border-subtle`, `--border-control`, `--accent`, `--accent-hover`, `--good`, `--warn`, `--bad`) rather than hardcoded hex values in components. **Requirement:** Text contrast must be `≥ 4.5:1` (`≥ 3:1` for ≥18pt or ≥14pt bold), and interactive control borders/icons must be `≥ 3:1` against adjacent backgrounds (WCAG 2.2 SC `1.4.3`, `1.4.11`).
- **Example:** On dark surface `#11182a`, use control borders `≥ #546894` (`≥ 3.1:1`) and darken primary purple button fill from `#6b73ff` (`3.80:1` with `#fff`, failing AA) to `#5348d9` / `#4e46c8` (`≥ 6.0:1` with `#fff`) or pair `#7c76ff` with `#0b1020` dark ink (`≥ 5.3:1`).
- **Common failure mode:** Saturated pastel gradients with white text (`3.5:1`–`3.8:1`) failing normal-text AA contrast, or dark `#2d3a5b` borders failing 3:1 UI component contrast.
- **Verification:** Run an automated WCAG contrast ratio check over all foreground/background and border/background token pairs.

### 2.5 Iconography & Motion
- **Decision rule:** Use consistent stroke weight (`1.75–2px`) and bounding boxes for vector icons, always paired with accessible labels (`aria-label` or visible text). Use motion strictly to clarify state transitions (`120–200ms ease-out`). **Requirement:** Honor `@media (prefers-reduced-motion: reduce)` by disabling all non-essential animations, pulse loops, and transitions globally.
- **Common failure mode:** Only disabling one modal's animation while leaving spinners, pulse dots, progress stripes, and scale-in keyframes active under reduced motion.
- **Verification:** Enable Windows *Animation effects = Off* (or DevTools `prefers-reduced-motion: reduce`) and verify zero continuous or jarring animations remain.

---

## 3. Components, Action Hierarchy, UI Writing, Progressive Disclosure, and States

### 3.1 Action Hierarchy
- **Decision rule:** One primary filled action per toolbar/region; secondary actions use neutral outlined/surface buttons; tertiary utilities use ghost buttons; destructive/stop actions use semantic danger styling and confirm before irreversible or queue-clearing loss. Visually distinguish disabled buttons (reduced opacity + desaturated fill, never looking active).
- **Example:** In a batch toolbar, keep `Run Selected` disabled until `selected > 0` and visually flat (`background: var(--surface-2); border-color: var(--border-subtle); color: var(--muted)`), instead of showing a glowing purple gradient at 48% opacity.
- **Common failure mode:** Multiple bright primary buttons competing inside the same card or scroll list, or disabled primary buttons still looking clickable.
- **Verification:** Inspect toolbar with 0 items selected; confirm the user can immediately distinguish enabled vs. disabled actions.

### 3.2 UI Writing & Copy
- **Decision rule:** Write concise, action-oriented, plain-language UI copy. Name the object and outcome (`"Rerun Failed"`, `"Refresh Tests"`, `"Switch"`, `"Active"` as a non-clickable badge). Never repeat surrounding context (`"Settings → General → Application and widget"`). For errors, follow Nielsen's 3-part rule: **what happened**, **why/scope**, and **next recovery action**.
- **Example:** Instead of stuffing an API error inside a button (`button.textContent = error.message` for 1.8s), keep the button stable (`"Retry Install"`) and render the full message in a persistent `role="status"` region with a copyable path/hint.
- **Common failure mode:** Transient error text inside buttons/toasts that disappears before the user can read or act on it, or ambiguous labels (`"Active"` on clickable switch buttons for non-active rows).
- **Verification:** Trigger invalid input, missing runtime, and stalled status; verify every error stays visible until resolved and states a concrete next step.

### 3.3 Progressive Disclosure & Interaction States
- **Decision rule:** Keep list rows scannable (status, identity, scope/layer, timing); reveal command templates, file paths, selectors, and full failure stacks inside expandable row details or a dedicated inspector pane. Clearly attribute *which* run's log is visible in an output pane (e.g., `Output: run-failed (18m ago)` with a `"Return to Active Run"` link if viewing history while another run executes).
- **Required States Matrix:** Every interactive component and panel must specify: `default`, `hover`, `focus-visible`, `active/pressed`, `selected`, `disabled`, `loading/busy`, `empty`, `error/warning`, and `unavailable/stale`.
- **Verification:** Click a historical run while idle and while another run is active; confirm the output header unambiguously states which run ID is shown.

---

## 4. Accessibility From Design Through Verification

### 4.1 Standards Baseline & Applicability
- **Formal Criteria (Requirements):**
  - **W3C WCAG 2.2 (Recommendation, Oct 2023)** Levels A and AA.
  - **W3C WCAG2ICT (Group Note)** for applying WCAG 2.0/2.1/2.2 concepts to non-web desktop software and hybrid WebView2/Wails applications.
  - **W3C WAI-ARIA Authoring Practices Guide (APG)** for custom widgets:
    - *Tabs:* `role="tablist"`, `role="tab"`, `role="tabpanel"`, `aria-selected`, `aria-controls`, Left/Right arrow navigation.
    - *Modal Dialogs:* `role="dialog"`, `aria-modal="true"`, `aria-labelledby`, focus trap (or `inert` background), `Escape` to close, focus restored to trigger.
    - *Accordion / Disclosures:* Native `<button>` inside heading with `aria-expanded` and `aria-controls`.
    - *Window Splitter:* `role="separator"`, `tabindex="0"`, `aria-orientation`, `aria-valuemin`/`max`/`now`, `ArrowLeft`/`ArrowRight` (or Up/Down) keyboard resizing.
- **Platform Guidance (Defaults):**
  - **Microsoft Windows 11 / Fluent 2 Design System:** Native window chrome where reliable, `Segoe UI Variable`, `4px` spatial grid, system Contrast Themes, and Windows Accessibility Insights / UI Automation verification.

### 4.2 Key Criteria Applied to Desktop UI
- **Keyboard Operability & Focus (WCAG 2.1.1, 2.4.3, 2.4.7, 2.4.11):** Every control (including collapsible group headers, splitters, and compact widget controls) must be reachable and operable via keyboard; focus must remain visible (`outline: 3px solid ...`) and must never be completely obscured by sticky headers, floating close buttons, or sheets (`2.4.11 Focus Not Obscured`). Do not hijack standard OS shortcuts (`F10`, `Alt+F4`, `Shift+F10`).
- **Target Size & Spacing (WCAG 2.5.8 Target Size Minimum, AA):** Interactive targets must be at least `24 × 24` CSS px (or meet the 24px spacing circle exception) and must never overlap neighboring buttons (e.g., floating close `×` buttons overlapping toolbar buttons).
- **Accessible Names, Roles & Status Announcements (WCAG 4.1.2, 4.1.3):** Every `<input>`, `<select>`, icon button, and checkbox needs a programmatic name (`<label>` or `aria-label`). Dynamic status updates (filter counts, run state changes, errors) use `role="status"` or `aria-live="polite"` without stealing focus.
- **Platform Accessibility Settings:** Support Windows DPI scaling (`100%`–`150%+`), Windows Contrast Themes (`@media (forced-colors: active)` using system colors like `ButtonText`, `Highlight`, `Canvas`, `GrayText` so custom checkboxes and pills remain legible), and Windows reduced motion (`prefers-reduced-motion: reduce`).

### 4.3 Reporting Limits (Requirement)
- **Never** claim full WCAG conformance from design mockups, HTML prototypes, or automated scanners alone.
- Always state which checks were **token/DOM measurements**, which were **live native UIA / keyboard / Narrator walkthroughs**, and which remain unverified (e.g., multi-monitor high-contrast testing).

---

## 5. Critique, Design Comparison, Task Testing, and Engineering Handoff

1. **Observe Before Redesigning:** Capture the running product across real states (empty, first-run, idle, active, failure, dense catalog, compact sizes, error/unavailable) before sketching changes.
2. **Compare Variants Deliberately:** Test at most 2–3 targeted structural variants where a trade-off matters (e.g., compact HUD button density or master/detail width allocation), document why one wins, and carry that single system across all surfaces.
3. **Validate Priority Tasks:** Re-run priority user tasks (orient & add project, filter & run selection, monitor compact HUD, diagnose & rerun failure, keyboard-only navigation) on both baseline and refined designs, separating **expert walkthroughs**, **direct measurements** (contrast, target px, click count), and **post-implementation native checks**.
4. **Engineering Handoff Contract:** Deliver an exact screen/state inventory, verified token table, long-text/truncation rules, keyboard & ARIA specifications, final UI copy, source file ownership, and a concrete native verification checklist so engineering never has to guess intent.

---

## References

- W3C, *Web Content Accessibility Guidelines (WCAG) 2.2* (W3C Recommendation, October 2023) — `https://www.w3.org/TR/WCAG22/`
- W3C, *Guidance on Applying WCAG 2 to Non-Web Information and Communications Technologies (WCAG2ICT)* (W3C Group Note) — `https://www.w3.org/TR/wcag2ict-22/`
- W3C, *WAI-ARIA Authoring Practices Guide (APG)* — `https://www.w3.org/WAI/ARIA/apg/`
- Microsoft Learn, *Design guidelines for Windows apps / Fluent 2 Design System* & *Accessibility testing (Accessibility Insights)* — `https://learn.microsoft.com/windows/apps/design/`
- Nielsen Norman Group (Jakob Nielsen), *10 Usability Heuristics for User Interface Design* — `https://www.nngroup.com/articles/ten-usability-heuristics/`
- Visual Studio Code, *Testing API & Test Explorer UX Patterns* — `https://code.visualstudio.com/api/extension-guides/testing`

**Review triggers:** Revisit this handbook when W3C publishes WCAG 2.3/3.0 updates, Microsoft updates Windows 11 Fluent desktop guidance, or a new target platform/framework (e.g., macOS AppKit / Linux GTK) introduces conflicting native conventions.
