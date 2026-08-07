---
title: 'Mobile-responsive admin interface'
type: 'bugfix'
created: '2026-08-07'
status: 'done'
review_loop_iteration: 0
baseline_revision: '568c16dad13018bd0e673881b2c8bc250ced6b13'
final_revision: '2211ba3f055bfc0dfd78919a5572623b00790cee'
followup_review_recommended: true
context: []
warnings: []
---

<intent-contract>

## Intent

**Problem:** The OpenVPN administration interface is not reliably usable on phone and tablet viewports. The reported clients screen can retain a large blank sidebar gutter after navigation is collapsed, and shared layouts, dialogs, data displays, toolbars, and several page-specific forms can overflow, truncate controls, or require impractical horizontal scrolling.

**Approach:** Establish a single responsive layout contract for the application shell and shared components, then adapt the remaining high-risk page-specific interfaces so every route is usable from 320px through desktop widths without hidden-sidebar space or unintended document-level horizontal overflow.

## Boundaries & Constraints

**Always:** Preserve existing desktop behavior, routes, API contracts, permission logic, translations, and visual theme. Use the existing React, Tailwind, Radix, and Lucide stack; follow mobile-first styling; keep keyboard focus behavior accessible; retain local scrolling for intentionally wide technical content; provide at least 44px touch targets for mobile-only interactive controls; use `dvh`-safe sizing for the shell and modal surfaces.

**Block If:** A required responsive change would alter a product permission, API payload, security decision, or destructive-action confirmation policy with no existing behavior to preserve.

**Never:** Do not hide information or actions merely to fit a viewport, use negative margins to mask sidebar geometry, introduce a parallel CSS/layout system, or rely on global `overflow-x: hidden` to conceal an overflow defect.

</intent-contract>

## Code Map

- `frontend/src/layout/Layout.tsx` -- application shell, responsive sidebar drawer, backdrop, and primary content region.
- `frontend/src/layout/TopBar.tsx` and `frontend/src/layout/Sidebar.tsx` -- mobile navigation and compact top-level controls.
- `frontend/src/styles/index.css` -- legacy layout styles, global viewport/overflow behavior, and responsive conventions.
- `frontend/src/components/DataTable.tsx` and `frontend/src/ui/{dialog,table,tabs,select}.tsx` -- shared mobile data, dialog, navigation, and form-control behavior.
- `frontend/src/pages/{Permissions,Roles,Users,Settings,Overview,Firewall,ChannelProviders,Clients,Profile,Login}/index.tsx` -- page-specific compact layouts, forms, action groups, and controls requiring targeted adaptation.
- `frontend/src/pages/{Audit,Certs,History,Notifications,Download}/index.tsx` -- routes to validate and adjust only where shared fixes do not provide correct mobile behavior.
- `frontend/tests/` and `frontend/playwright.config.ts` -- browser-level responsive verification.

## Tasks & Acceptance

**Execution:**
- [x] `frontend/src/layout/{Layout,TopBar,Sidebar}.tsx` -- make the desktop sidebar opt in only at the common desktop breakpoint; ensure the drawer is fully out of flow below it, the main region is full-width/min-width-safe, and compact top-bar controls stay available at 320px.
- [x] `frontend/src/styles/index.css` -- remove or isolate obsolete shell rules that reserve a sidebar column, adopt dynamic-viewport-safe root sizing, define non-masking overflow behavior, and retain only intentional local scrolling.
- [x] `frontend/src/components/DataTable.tsx` and `frontend/src/ui/{dialog,table,tabs,select}.tsx` -- harden shared small-screen behaviors: readable key/value cards without desktop width leakage, wrapped actions, constrained/scrollable dialogs, accessible tab overflow, and viewport-bounded selects/tables.
- [x] `frontend/src/pages/Permissions/index.tsx` -- supply a practical mobile permission-tree/card presentation and touch-safe actions rather than a wide management table.
- [x] `frontend/src/pages/{Roles,Users,Settings,Overview,Firewall,ChannelProviders,Clients,Profile,Login}/index.tsx` -- convert fixed desktop grids, dual-list assignment, inline action rows, compact controls, editors, and visual content into usable phone/tablet flows.
- [x] `frontend/src/pages/{Audit,Certs,History,Notifications,Download}/index.tsx` -- audit the routes in the final responsive matrix and make minimal targeted corrections for any remaining overflow or inaccessible content.
- [x] `frontend/tests/` and `frontend/playwright.config.ts` -- expand Playwright responsive coverage for all routes and key shared interactions, including sidebar geometry, page overflow, dialog reachability, tables, and compact navigation.

**Acceptance Criteria:**
- Given any authenticated route at 320px, 375px, 768px, and 1023px wide, when the navigation drawer is closed, then main content begins at its mobile page padding and no desktop-sidebar-width blank gutter remains.
- Given any application route at phone, tablet, and desktop widths, when the page finishes loading, then the document has no unintended horizontal overflow; intentionally scrollable local technical/table areas remain contained within their own surfaces.
- Given a long form, editor, error message, or soft-keyboard-constrained viewport, when its dialog opens, then its title, content, and all available actions can be reached and operated without clipping.
- Given management data with long identifiers, Chinese labels, file names, timestamps, and several actions, when viewed on a phone, then values remain intelligible, actions remain tappable, and desktop-only column sizing does not distort mobile cards.
- Given Permissions, Roles, Users, Settings, Overview, Firewall, Channel Providers, Clients, Profile, Login, Audit, Certs, History, Notifications, and Download, when each is inspected at 375px and 768px, then forms, filters, tabs, content grids, QR/media, and action bars stack or scroll locally as needed without overlap.
- Given mobile touch interfaces, when a user activates navigation, dialogs, selectors, tabs, or row actions, then adjacent controls are separated and primary touch targets are at least 44 by 44 CSS pixels where the visual control is compact.

## Design Notes

Use one breakpoint contract throughout: phone below 640px, compact/tablet from 640px through 1023px, and desktop from 1024px upward. The sidebar exists as a fixed overlay below desktop and as the document-flow desktop rail only at desktop widths. Data should reflow into labelled cards when its actions or columns no longer fit; technical content that must remain tabular may scroll only inside a clearly bounded container. Preserve the existing dark administration visual language and use state/color only as a supplement to text and icons.

## Implementation Verification

- `npm run build` (frontend) passed on August 7, 2026.
- `pnpm exec playwright test --list` (frontend) passed and discovered 207 tests.
- A real Chromium check passed for `/login` and `/download` at 320px, 375px, and 768px with no document overflow. The full authenticated Playwright suite could not run because the repository requires Playwright Chromium revision 1234, which is not installed locally, and the local API service at `127.0.0.1:8888` was unavailable. Browser installation was attempted for ten minutes without completing; test coverage is present for execution in a provisioned environment.
## Verification

**Commands:**
- `npm run build` (in `frontend`) -- expected: TypeScript and Vite production build complete without errors.
- `npm run test:mobile` (in `frontend`) -- expected: responsive Playwright assertions pass across the specified routes and shared interactions.

**Manual checks (if no CLI):**
- Inspect `/clients` at 375px and confirm it matches the reported scenario without a left-side hidden-menu gutter.
- Inspect all routes at 320px, 375px, 768px, 1024px, and a 667px-wide landscape viewport, including opened dialogs, side navigation, tabs, filters, and long data values.

## Review Triage Log

### 2026-08-07 - Review pass
- intent_gap: 0
- bad_spec: 0
- patch: 10 (high 3, medium 5, low 2)
- defer: 0
- reject: 2 (low 2)
- addressed_findings:
  - `[high] [patch]` Restored `100vh` fallbacks for dialog and select viewport caps, then progressively applies `dvh` caps with safe-area subtraction for capable browsers.
  - `[high] [patch]` Reflowed the Overview rate-limit value/unit controls into a phone-safe vertical flow and kept the compact horizontal presentation from `sm` upward.
  - `[medium] [patch]` Added the mobile DataTable card test hook and made the Clients assertion accept the valid empty-client state.
  - `[medium] [patch]` Prevented the Input clear affordance from covering native numeric-stepper controls, and fixed the public `/download` test path so it is not treated as an authenticated application shell.
  - `[medium] [patch]` Added safe-area-aware dialog bounds and retained a scrollable, reachable close control.
  - `[low] [patch]` Enlarged remaining sub-36px interactive controls in the compact shell/system monitor below the desktop breakpoint.
  - `[low] [reject]` The retained 320px body minimum does not violate the captured requirement, whose supported range begins at 320px.
  - `[low] [reject]` The claimed caller-side `h-9` clear-button regression was not present in the affected Input call sites.

## Auto Run Result

### Summary
Implemented the shared responsive layout contract across the OpenVPN administration interface. The mobile sidebar is now a fixed, out-of-flow drawer below `lg`, so closing it no longer leaves the reported 256px blank gutter. Shared controls and high-risk pages now reflow safely for phone/tablet widths while preserving the desktop rail at 1024px and above.

### Files Changed
- `frontend/src/layout/{Layout,Sidebar,TopBar}.tsx` - fixed mobile drawer geometry, main-content sizing, and compact shell controls.
- `frontend/src/styles/index.css` - removed obsolete shell-reservation behavior and retained non-masking viewport rules.
- `frontend/src/components/{DataTable,ManagementStatus,SystemMonitor}.tsx` - added readable mobile records, wrapped actions, semantic pagination labels, and touch-safe compact controls.
- `frontend/src/ui/{dialog,input,select,table,tabs}.tsx` - added safe, scrollable responsive shared surfaces and clear-control handling.
- `frontend/src/pages/{Overview,Permissions,Roles,Users,Settings,Firewall,ChannelProviders,Clients,Profile,Login,Certs,Notifications}/index.tsx` - adapted page-specific forms, trees, action bars, controls, and visual content.
- `frontend/playwright.config.ts` and `frontend/tests/mobile-adaptation.spec.ts` - expanded the viewport matrix and assertions for gutters, overflow, cards, dialogs, public routes, and compact navigation.

### Verification
- `pnpm exec tsc --noEmit` - passed.
- `npm run build` - passed; Vite emitted only its existing large-chunk advisory.
- `pnpm exec playwright test --list` - passed; discovered 299 tests.
- Authenticated system-Chrome checks against the local Vite build passed for all 14 routes at 320x568, 768x1024, and 1024x768 (42 route/viewport checks): no document-level horizontal overflow; no hidden-drawer gutter below `lg`; and correct desktop sidebar geometry at `lg`.
- An additional authenticated 667x375 landscape pass covered all 14 routes with no horizontal overflow or drawer gutter.
- A compact-interaction audit across 320x568, 768x1024, and 667x375 covered 42 route/viewport checks with no clipped primary surface and no visible button/link below 36px. Client-creation dialogs at phone/tablet widths remained fully inside the viewport and could be closed.
- The bundled Playwright browser executable is absent locally (`chromium_headless_shell` revision 1234), so a direct Playwright execution cannot start in this machine state. The responsive assertions were nevertheless executed manually in the installed system Chrome using the authenticated local service.

### Residual Risk
The source, production build, and real authenticated system-Chrome matrix are verified. A provisioned CI/local environment with the Playwright browser bundle installed should still execute the full 299-test suite, including WebKit projects, as a final cross-engine confirmation.
