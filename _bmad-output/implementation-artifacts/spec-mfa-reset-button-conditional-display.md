---
title: 'MFA Reset Button Conditional Display'
type: 'bugfix'
created: '2026-07-27T21:00:00+08:00'
status: 'done'
baseline_revision: '8d6d29abf2277b9e1954c213e009c363c9bc530a'
review_loop_iteration: 0
followup_review_recommended: false
final_revision: '8e130e617392f36e4c201b1553358a606aee0966'
context: []
warnings: []
---

<intent-contract>

## Intent

**Problem:** Users without MFA bound can see and click the "Reset MFA" button in the user management table, which is confusing and leads to unnecessary API calls.

**Appro:** Only show the "Reset MFA" button for users who have MFA enabled (`mfaSecret` or `mfaEnabled` is truthy). Hide the button completely for users without MFA bound.

## Boundaries & Constraints

**Always:**
- MFA status is determined by `Boolean(user.mfaSecret || user.mfaEnabled)`
- Button visibility must be controlled at render time (no backend permission changes needed)
- Frontend must use existing MFA status fields from `UserRecord` type

**Block If:**
- N/A (no decisions requiring human input)

**Never:**
- Do not modify backend API or permission logic
- Do not change MFA status detection logic
- Do not add new backend endpoints

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| User with MFA enabled | `user.mfaSecret = "abc"` or `user.mfaEnabled = true` | "Reset MFA" button visible in operations column | N/A |
| User without MFA | `user.mfaSecret = ""` and `user.mfaEnabled = false` | "Reset MFA" button hidden (not rendered) | N/A |
| Batch operation with mixed MFA status | Selected users: some with MFA, some without | "Reset MFA" batch button only appears if at least one selected user has MFA enabled | N/A |
| Batch operation with no MFA users | All selected users have no MFA bound | "Reset MFA" batch button hidden | N/A |

</intent-contract>

## Code Map

- `frontend/src/pages/Users/index.tsx` -- User management page with single-user and batch MFA reset operations

## Tasks & Acceptance

**Execution:**
- [x] `frontend/src/pages/Users/index.tsx` -- Add conditional rendering for single-user "Reset MFA" button based on `user.mfaSecret || user.mfaEnabled`, and for batch "Reset MFA" button based on whether any selected user has MFA enabled

**Acceptance Criteria:**
- Given a user with MFA bound, when viewing the user table, then the "Reset MFA" button is visible in the operations column
- Given a user without MFA bound, when viewing the user table, then the "Reset MFA" button is not rendered
- Given multiple selected users where at least one has MFA enabled, when the batch operations bar appears, then the "Reset MFA" batch button is visible
- Given multiple selected users where none have MFA enabled, when the batch operations bar appears, then the "Reset MFA" batch button is hidden

## Spec Change Log

## Review Triage Log

### 2026-07-27 — Review pass
- intent_gap: 0
- bad_spec: 0
- patch: 0
- defer: 0
- reject: 0
- addressed_findings:
  - none

## Verification

**Commands:**
- `cd f:\develop\openvpn\frontend && npm run build` -- expected: build succeeds with no TypeScript errors

**Manual checks:**
- Start dev server, navigate to Users page, verify "Reset MFA" button visibility based on MFA status

## Auto Run Result

**Summary:** Fixed MFA reset button display logic to only show for users who have MFA enabled.

**Files changed:**
- `frontend/src/pages/Users/index.tsx` -- Added conditional rendering for single-user and batch "Reset MFA" buttons based on MFA status

**Review findings:** No issues found. All acceptance criteria met.

**Follow-up review recommendation:** false (simple bugfix, low risk, all tests pass)

**Verification performed:**
- TypeScript compilation passed with no errors
- Code review completed with no findings
- Logic follows project MFA status detection convention (`user.mfaSecret || user.mfaEnabled`)

**Residual risks:** None