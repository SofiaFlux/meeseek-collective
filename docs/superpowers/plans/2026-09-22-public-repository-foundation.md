# Public Repository Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a truthful, contributor-ready documentation surface for the experimental Summa42 MVC while retaining explicit gates before the repository is made public.

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
    COC --> X[zofiastrumien101@gmail.com escalation]
    A[Private audit evidence] --> G[Public redacted readiness record]
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
- Create: `scripts/verify_docs.py`

- [ ] **Step 1: Rewrite README as an experimental-MVC entry point**

  Keep the existing factual architecture content, then add a one-paragraph experimental-status notice, a `Prerequisites` section (`Go 1.27`, Linux/Docker only for the OCI gate), build and local-start commands, a small command table, `Contributing`, `Security`, `Support`, and `License` links. State plainly that the project is local-first, single-Cube, not production-ready, and must not be used for unmanaged consequential workloads.

- [ ] **Step 2: Add the contribution guide**

  Create `CONTRIBUTING.md` with these concrete sections: scope and behavioral expectations; local setup using `go mod download`; focused-change and test expectations; the full documented validation matrix; an instruction not to commit `SUMMA42_HOME`, keys, bearer tokens, SQLite databases, raw evidence, or credentials; pull-request checklist; and the Apache-2.0 inbound-contribution statement. Link `CODE_OF_CONDUCT.md`, `SECURITY.md`, and `SUPPORT.md`.

- [ ] **Step 3: Add the security policy with the transitional private channel**

  Create `SECURITY.md` that asks reporters not to create public Issues for potential vulnerabilities and instead to use `SofiaFlux@outlook.com` while the repository is private and during the publication transition. Ask for affected revision, reproduction steps, impact, and safe proof-of-concept; prohibit credentials, tokens, private keys, raw local databases, and unredacted personal data. Define acknowledgment, triage, remediation, and coordinated-publication phases without an SLA. State that GitHub Private Vulnerability Reporting becomes the primary path only after post-publication verification, with the mailbox retained as fallback.

- [ ] **Step 4: Add the support policy and NOTICE**

  Create `SUPPORT.md` to route reproducible defects to the bug form, security reports to `SECURITY.md`, and conduct concerns to `CODE_OF_CONDUCT.md`. State that general support is unavailable until GitHub Discussions has been enabled and verified; do not direct questions to Issues. Do not create `NOTICE` in this task: add it only after the dependency and copied-asset provenance audit in Task 3 establishes its required attribution text.

- [ ] **Step 5: Add the local documentation verifier**

  Create `scripts/verify_docs.py` using only the Python standard library. It must accept file paths, reject unmatched fenced-code delimiters and duplicate heading anchors in each Markdown file, derive GitHub-style lowercase hyphenated anchors from headings, and extract Markdown links with the pattern `\[[^]]+\]\(([^)]+)\)`. Ignore `http`, `https`, and `mailto` links. For every local link, resolve the path relative to the source file (or use the source file for a fragment-only link), fail if that path does not exist, and fail if a `#fragment` is absent from the target Markdown file’s derived anchors. For YAML files, inspect `contact_links` and fail unless every entry has non-empty `name`, `about`, and an absolute `https://` URL. Exit zero only when every supplied file passes and print one `valid: <path>` line per file.

- [ ] **Step 6: Verify root-document links and public claims**

  Run:

  ```bash
  rg -n 'TODO|TBD|production-ready|SLA|INSERT CONTACT' README.md CONTRIBUTING.md SECURITY.md SUPPORT.md
  python3 scripts/verify_docs.py README.md CONTRIBUTING.md SECURITY.md SUPPORT.md
  ```

  Expected: no placeholders; every non-production claim is deliberate; every local Markdown link resolves; headings and fenced-code blocks are structurally balanced.

- [ ] **Step 7: Commit**

  ```bash
  git add README.md CONTRIBUTING.md SECURITY.md SUPPORT.md scripts/verify_docs.py
  git commit -m "docs: add public repository guidance"
  ```

### Task 2: Add conduct policy and GitHub contribution templates

**Files:**

- Create: `CODE_OF_CONDUCT.md`
- Create: `.github/ISSUE_TEMPLATE/bug_report.yml`
- Create: `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/pull_request_template.md`

- [ ] **Step 1: Add Contributor Covenant 2.1 with the configured confidential contact**

  Add the complete Contributor Covenant 2.1 policy, preserving its attribution, and replace its contact placeholder with `SofiaFlux@outlook.com`. Add a short local supplement: do not use public Issues, Discussions, or Security Advisories for conduct reports; a reporter who has a conflict with the primary recipient sends the report directly to `zofiastrumien101@gmail.com`; reports are handled confidentially to the extent reasonably possible. Do not link the escalation address from README or SUPPORT.

- [ ] **Step 2: Add a safe bug-report issue form**

  Create `.github/ISSUE_TEMPLATE/bug_report.yml` using valid GitHub issue-form keys (`name`, `description`, `title`, `body`). Collect reproduction steps, expected/actual behavior, Summa42 revision, operating system, and safe/redacted logs. Add a required checkbox confirming the report is not a vulnerability and contains no secrets, credentials, private keys, raw databases, or personal data. Do not reference labels or assignees that may not exist.

- [ ] **Step 3: Configure the template chooser and pull-request checklist**

  Create `config.yml` with blank issues disabled and complete `contact_links` entries, each with `name`, `about`, and an absolute GitHub URL for `SECURITY.md`, `SUPPORT.md`, and `CODE_OF_CONDUCT.md`. Create `.github/pull_request_template.md` with checkboxes for focused scope, tests, documentation, secret/PII review, and acknowledgement of Apache-2.0 inbound licensing. It must explicitly tell contributors not to put security-sensitive details in a PR.

- [ ] **Step 4: Validate YAML and template safety**

  Run:

  ```bash
  ruby -e 'require "yaml"; %w[.github/ISSUE_TEMPLATE/bug_report.yml .github/ISSUE_TEMPLATE/config.yml].each { |p| YAML.load_file(p); puts "valid: #{p}" }'
  python3 scripts/verify_docs.py .github/ISSUE_TEMPLATE/config.yml
  rg -n -i 'token|password|private key|database|credential' .github/ISSUE_TEMPLATE .github/pull_request_template.md
  ```

  Expected: both YAML files parse; each `contact_links` record has non-empty `name`, `about`, and absolute `url`; each sensitive-data term is part of a prohibition or redaction instruction. After merging to the default branch, open the bug form from the repository’s Issue chooser and submit no report; open a draft pull request and verify the pull-request checklist is prefilled. These GitHub UI checks are the authoritative render validation; `config.yml` itself is only chooser configuration.

- [ ] **Step 5: Commit**

  ```bash
  git add CODE_OF_CONDUCT.md .github/ISSUE_TEMPLATE .github/pull_request_template.md
  git commit -m "docs: add contributor reporting templates"
  ```

### Task 3: Record and execute the publication gates

**Files:**

- Create: `docs/publication-readiness.md`
- Create: `docs/publication-readiness-private.md` (never commit)
- Create: `NOTICE`
- Modify: `.gitignore` only if the audit finds a missing local-artifact pattern

- [ ] **Step 1: Create the dated readiness checklist**

  Create public `docs/publication-readiness.md` with dated pass/fail status, command names, and links to public remediation PRs only. It must never contain mailbox recipients, access configuration, secret/PII locations, raw scan output, report contents, or private remediation details. Create untracked `docs/publication-readiness-private.md` for restricted evidence and add it to `.gitignore`. The public checklist includes: two-maintainer access to `SofiaFlux@outlook.com`; tested confidential primary and escalation conduct routes; security mailbox test; Discussions enabled plus a question-category test or an explicit decision to retain “general support unavailable”; Issue/PR template rendering; branch protection and maintainer-access review; full-history/release asset audit; asset/license inventory; and required local validation results.

- [ ] **Step 2: Run non-destructive repository and history audit**

  Run:

  ```bash
  git fetch --all --tags --prune
  git fsck --no-reflogs --unreachable
  git rev-list --objects --all > /tmp/summa42-all-objects.txt
  gitleaks detect --source . --log-opts="--all" --redact --report-format json --report-path /tmp/summa42-gitleaks.json
  git log --all -p | rg -n -i 'BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|api[_-]?key|secret|token|password|@' || true
  git ls-files | rg -n '(\.db(-wal|-shm)?|\.sqlite(-wal|-shm)?|config\.json|id_(rsa|ed25519)|\.pem|\.key)$' || true
  git check-ignore -v .summa42/config.json .summa42/state.db .summa42/evidence/example || true
  gh release list --repo SofiaFlux/summa42 --limit 100
  go list -m -json all > /tmp/summa42-modules.json
  ```

  Before this step, download `gitleaks` v8.30.1 from its official release and verify the Linux x64 archive against SHA-256 `551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb`; record the version and checksum in the private evidence record. Do not treat its output as a complete PII review. Manually inspect every reachable ref/object class listed in `/tmp/summa42-all-objects.txt`, all GitHub release assets, test fixtures, generated artifacts, and documentation for PII and private-state material. Record sensitive evidence only in the ignored private record and publish only redacted status/remediation references. If any secret, key, credential, or PII is found in reachable history, stop: remove it from every reachable ref/history before publication rather than merely adding it to `.gitignore`.

- [ ] **Step 3: Verify software and document checks**

  Run:

  ```bash
  go test ./... -count=1
  go vet ./...
  go build ./cmd/summa42 ./cmd/summa42-box
  python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
  ```

  Complete the dependency and copied-asset license inventory before writing `NOTICE`. For every required third-party attribution or notice, add the exact required text to `NOTICE`, then record the module/asset, license, evidence source, and public-safe conclusion in the public checklist; keep detailed evidence private. Record the exact validation outcome and environment in the public checklist. Run the Docker OCI gate when Docker is available; otherwise leave the checklist unchecked and do not claim full publication readiness.

- [ ] **Step 4: Perform GitHub settings checks at the publication transition**

  Before visibility changes, verify that two maintainers can receive and acknowledge mail at `SofiaFlux@outlook.com`, test the primary conduct route and the separate escalation route at `zofiastrumien101@gmail.com`, verify templates/access, and either Discussions or the explicit no-support policy. After the repository becomes public, each designated triager enables repository `Security alerts` watching and email notifications in their personal GitHub settings; enable Private Vulnerability Reporting; submit a harmless external test report; and record receipt/acknowledgment by every triager. Only then update `SECURITY.md` so PVR is primary and `SofiaFlux@outlook.com` remains fallback. Record public-safe results in the public checklist and details privately.

- [ ] **Step 5: Commit**

  ```bash
  git add docs/publication-readiness.md .gitignore NOTICE scripts/verify_docs.py
  git commit -m "docs: add publication readiness checklist"
  ```

### Task 4: Final documentation review

**Files:**

- Review: `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `SUPPORT.md`, `NOTICE`, `CODE_OF_CONDUCT.md`, `.github/ISSUE_TEMPLATE/*`, `.github/pull_request_template.md`, `docs/publication-readiness.md`

- [ ] **Step 1: Check scope and links**

  Run:

  ```bash
  rg -n 'production-ready|guarantee|SLA|TODO|TBD|INSERT CONTACT' README.md CONTRIBUTING.md SECURITY.md SUPPORT.md NOTICE CODE_OF_CONDUCT.md .github docs/publication-readiness.md
  python3 scripts/verify_docs.py README.md CONTRIBUTING.md SECURITY.md SUPPORT.md CODE_OF_CONDUCT.md .github/ISSUE_TEMPLATE/config.yml .github/pull_request_template.md docs/publication-readiness.md
  git diff --check origin/main...HEAD
  ```

  Expected: no accidental production/SLA claims, no placeholders, and no whitespace errors.

- [ ] **Step 2: Request independent documentation review**

  Review for inaccurate claims, missing privacy routes, unsupported GitHub features, Apache-2.0 inconsistencies, dangerous reporting prompts, and mismatches with the current MVC behavior. Resolve all Important findings before publication.

- [ ] **Step 3: Publish only when gates are factual**

  Do not change repository visibility as part of documentation implementation. Visibility requires the completed checklist and explicit maintainer authorization.
