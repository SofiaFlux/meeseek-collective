# Public Repository Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a truthful, contributor-ready documentation surface for the experimental Meeseek Collective MVC while retaining explicit gates before the repository is made public.

**Architecture:** Root Markdown documents define expectations for readers, contributors, security researchers, and conduct reporters. GitHub issue forms and the pull-request template collect only public-safe information and direct sensitive reports to confidential channels. Release-gate evidence stays in a dated audit record, separating repository content from GitHub settings that must be verified by maintainers.

**Architecture Diagram:**

```mermaid
graph TD
    R[README] --> C[CONTRIBUTING]
    R --> S[SECURITY]
    R --> U[SUPPORT]
    C --> P[Pull request template]
    U --> I[Public bug issue form]
    S --> E[SofiaFlux@outlook.com]
    COC[Code of Conduct] --> E
    A[Pre-publication audit record] --> G[Visibility decision]
    G --> V[GitHub Private Vulnerability Reporting]
```

**Tech Stack:** Markdown, YAML GitHub issue forms, GitHub repository settings, Go validation commands.

---

### Task 1: Write the public-facing root documents

**Files:**

- Modify: `README.md`
- Create: `CONTRIBUTING.md`
- Create: `SECURITY.md`
- Create: `SUPPORT.md`
- Create: `NOTICE`

- [ ] **Step 1: Rewrite README as an experimental-MVC entry point**

  Keep the existing factual architecture content, then add a one-paragraph experimental-status notice, a `Prerequisites` section (`Go 1.27`, Linux/Docker only for the OCI gate), build and local-start commands, a small command table, `Contributing`, `Security`, `Support`, and `License` links. State plainly that the project is local-first, single-Cube, not production-ready, and must not be used for unmanaged consequential workloads.

- [ ] **Step 2: Add the contribution guide**

  Create `CONTRIBUTING.md` with these concrete sections: scope and behavioral expectations; local setup using `go mod download`; focused-change and test expectations; the full documented validation matrix; an instruction not to commit `MEESEEK_HOME`, keys, bearer tokens, SQLite databases, raw evidence, or credentials; pull-request checklist; and the Apache-2.0 inbound-contribution statement. Link `CODE_OF_CONDUCT.md`, `SECURITY.md`, and `SUPPORT.md`.

- [ ] **Step 3: Add the security policy with the transitional private channel**

  Create `SECURITY.md` that asks reporters not to create public Issues for potential vulnerabilities and instead to use `SofiaFlux@outlook.com` while the repository is private and during the publication transition. Ask for affected revision, reproduction steps, impact, and safe proof-of-concept; prohibit credentials, tokens, private keys, raw local databases, and unredacted personal data. Define acknowledgment, triage, remediation, and coordinated-publication phases without an SLA. State that GitHub Private Vulnerability Reporting becomes the primary path only after post-publication verification, with the mailbox retained as fallback.

- [ ] **Step 4: Add the support policy and NOTICE**

  Create `SUPPORT.md` to route reproducible defects to the bug form, security reports to `SECURITY.md`, and conduct concerns to `CODE_OF_CONDUCT.md`. State that general support is unavailable until GitHub Discussions has been enabled and verified; do not direct questions to Issues. Create `NOTICE` with the copyright attribution `Copyright 2026 SofiaFlux contributors` and a statement that it accompanies the Apache-2.0-licensed project without modifying the license.

- [ ] **Step 5: Verify root-document links and public claims**

  Run:

  ```bash
  rg -n 'TODO|TBD|production-ready|SLA|INSERT CONTACT' README.md CONTRIBUTING.md SECURITY.md SUPPORT.md NOTICE
  rg -n '\]\([^)]+' README.md CONTRIBUTING.md SECURITY.md SUPPORT.md CODE_OF_CONDUCT.md
  ```

  Expected: no placeholders; every non-production claim is deliberate; all referenced local files exist.

- [ ] **Step 6: Commit**

  ```bash
  git add README.md CONTRIBUTING.md SECURITY.md SUPPORT.md NOTICE
  git commit -m "docs: add public repository guidance"
  ```

### Task 2: Add conduct policy and GitHub contribution templates

**Files:**

- Create: `CODE_OF_CONDUCT.md`
- Create: `.github/ISSUE_TEMPLATE/bug_report.yml`
- Create: `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/pull_request_template.md`

- [ ] **Step 1: Add Contributor Covenant 2.1 with the configured confidential contact**

  Add the complete Contributor Covenant 2.1 policy, preserving its attribution, and replace its contact placeholder with `SofiaFlux@outlook.com`. Add a short local supplement: do not use public Issues, Discussions, or Security Advisories for conduct reports; a reporter who has a conflict with the recipient may request escalation to a repository administrator via the same mailbox; reports are handled confidentially to the extent reasonably possible.

- [ ] **Step 2: Add a safe bug-report issue form**

  Create `.github/ISSUE_TEMPLATE/bug_report.yml` using valid GitHub issue-form keys (`name`, `description`, `title`, `body`). Collect reproduction steps, expected/actual behavior, Meeseek revision, operating system, and safe/redacted logs. Add a required checkbox confirming the report is not a vulnerability and contains no secrets, credentials, private keys, raw databases, or personal data. Do not reference labels or assignees that may not exist.

- [ ] **Step 3: Configure the template chooser and pull-request checklist**

  Create `config.yml` with blank issues disabled and links to `SECURITY.md`, `SUPPORT.md`, and `CODE_OF_CONDUCT.md`. Create `.github/pull_request_template.md` with checkboxes for focused scope, tests, documentation, secret/PII review, and acknowledgement of Apache-2.0 inbound licensing. It must explicitly tell contributors not to put security-sensitive details in a PR.

- [ ] **Step 4: Validate YAML and template safety**

  Run:

  ```bash
  ruby -e 'require "yaml"; %w[.github/ISSUE_TEMPLATE/bug_report.yml .github/ISSUE_TEMPLATE/config.yml].each { |p| YAML.load_file(p); puts "valid: #{p}" }'
  rg -n -i 'token|password|private key|database|credential' .github/ISSUE_TEMPLATE .github/pull_request_template.md
  ```

  Expected: both YAML files parse; each sensitive-data term is part of a prohibition or redaction instruction.

- [ ] **Step 5: Commit**

  ```bash
  git add CODE_OF_CONDUCT.md .github/ISSUE_TEMPLATE .github/pull_request_template.md
  git commit -m "docs: add contributor reporting templates"
  ```

### Task 3: Record and execute the publication gates

**Files:**

- Create: `docs/publication-readiness.md`
- Modify: `.gitignore` only if the audit finds a missing local-artifact pattern

- [ ] **Step 1: Create the dated readiness checklist**

  Create `docs/publication-readiness.md` with unchecked, evidence-bearing entries for: two-maintainer access to `SofiaFlux@outlook.com`; tested confidential conduct route; security mailbox test; Discussions enabled plus a question-category test or an explicit decision to retain “general support unavailable”; Issue/PR template rendering; branch protection and maintainer-access review; full-history secret/PII audit; asset/license inventory; and required local validation results.

- [ ] **Step 2: Run non-destructive repository and history audit**

  Run:

  ```bash
  git fsck --no-reflogs --unreachable
  git log --all -p -- . ':!go.sum' | rg -n -i 'BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|api[_-]?key|secret|token|password' || true
  git ls-files | rg -n '(\.db(-wal|-shm)?|\.sqlite(-wal|-shm)?|config\.json|id_(rsa|ed25519)|\.pem|\.key)$' || true
  git check-ignore -v .meeseek/config.json .meeseek/state.db .meeseek/evidence/example || true
  go list -m -json all > /tmp/meeseek-modules.json
  ```

  Record commands, date, reviewer, findings, remediation references, and final disposition in the readiness checklist. If any secret, key, credential, or PII is found in reachable history, stop: remove it from every reachable ref/history before publication rather than merely adding it to `.gitignore`.

- [ ] **Step 3: Verify software and document checks**

  Run:

  ```bash
  go test ./... -count=1
  go vet ./...
  go build ./cmd/meeseek ./cmd/meeseek-box
  python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
  ```

  Record the exact outcome and environment in `docs/publication-readiness.md`. Run the Docker OCI gate when Docker is available; otherwise leave the checklist unchecked and do not claim full publication readiness.

- [ ] **Step 4: Perform GitHub settings checks at the publication transition**

  Before visibility changes, verify the mailbox and conduct route, templates, access, and either Discussions or the explicit no-support policy. After the repository becomes public, enable GitHub Private Vulnerability Reporting, configure security-alert notifications for the designated triagers, verify the external reporting UI, then update `SECURITY.md` so PVR is primary and `SofiaFlux@outlook.com` remains fallback. Record each result in the readiness checklist.

- [ ] **Step 5: Commit**

  ```bash
  git add docs/publication-readiness.md .gitignore
  git commit -m "docs: add publication readiness checklist"
  ```

### Task 4: Final documentation review

**Files:**

- Review: `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `SUPPORT.md`, `NOTICE`, `CODE_OF_CONDUCT.md`, `.github/ISSUE_TEMPLATE/*`, `.github/pull_request_template.md`, `docs/publication-readiness.md`

- [ ] **Step 1: Check scope and links**

  Run:

  ```bash
  rg -n 'production-ready|guarantee|SLA|TODO|TBD|INSERT CONTACT' README.md CONTRIBUTING.md SECURITY.md SUPPORT.md NOTICE CODE_OF_CONDUCT.md .github docs/publication-readiness.md
  git diff --check origin/main...HEAD
  ```

  Expected: no accidental production/SLA claims, no placeholders, and no whitespace errors.

- [ ] **Step 2: Request independent documentation review**

  Review for inaccurate claims, missing privacy routes, unsupported GitHub features, Apache-2.0 inconsistencies, dangerous reporting prompts, and mismatches with the current MVC behavior. Resolve all Important findings before publication.

- [ ] **Step 3: Publish only when gates are factual**

  Do not change repository visibility as part of documentation implementation. Visibility requires the completed checklist and explicit maintainer authorization.
