# Public Repository Foundation Design

## Goal

Make Meeseek Collective safe and understandable to publish as an experimental
open-source repository, without implying production readiness, hosted support,
or a stability guarantee.

## Positioning

Meeseek Collective is an experimental, local-first agent orchestration runtime.
The public documentation must consistently describe the current one-Cube MVC,
its local SQLite authority boundary, and the difference between implemented
controls and future/distributed work. It must not claim that the project is
production-ready or suitable for unmanaged consequential workloads.

## Public Documentation Set

`README.md` is the entry point. It will explain the project, current scope,
prerequisites, a verified local quick start, security/enforcement limits,
development validation, and links to the contributor and security policies.

The root will also contain:

- `CONTRIBUTING.md`: development setup, focused change expectations, required
  validation, pull-request expectations, and the Apache-2.0 contribution
  license.
- `SECURITY.md`: scope, a private reporting route using GitHub Private
  Vulnerability Reporting, the information needed for a report, a coordinated
  disclosure process, and an explicit request not to file public issues for
  suspected vulnerabilities.
- `CODE_OF_CONDUCT.md`: a compact Contributor Covenant 2.1 policy with a
  confidential maintainer contact, two designated recipients where feasible,
  a conflict-of-interest escalation path, and a prohibition on reporting
  community incidents through Issues or Security Advisories.
- `SUPPORT.md`: distinction between GitHub Discussions/questions, Issues for
  reproducible bugs, and Security Advisories for vulnerabilities; no SLA.
- `NOTICE`: project attribution suitable for the existing Apache-2.0 license.
- GitHub issue and pull-request templates that collect reproducible reports,
  security routing, test evidence, and a scope checklist.

## Out of Scope

This change will not add release automation, a roadmap, a governance model,
telemetry, badges whose status cannot be verified, or an implied support
commitment. It will not alter runtime behavior or licensing terms.

## Release-Blocking Operational Prerequisites

At design time the repository is private, GitHub Discussions are disabled, and
the GitHub API does not expose Private Vulnerability Reporting. The repository
must remain private until every item below is complete and independently
checked by a maintainer:

1. Enable GitHub Private Vulnerability Reporting and verify that the Security
   tab presents a private reporting flow. The SECURITY policy must name this
   route as the only security-reporting channel. If the feature cannot be
   enabled, do not make the repository public until a maintainer-owned private
   email alias with at least two recipients is available and documented.
2. Configure a confidential Code of Conduct contact that is distinct from
   Security Advisories, identify its recipients and a conflict-of-interest
   escalation path, and verify it can receive a private report. The public
   repository must not be opened before this contact exists.
3. Enable GitHub Discussions and create a visible question category, then
   verify a user can open a question. If Discussions are intentionally not
   enabled, the SUPPORT policy must say that general support is unavailable
   rather than redirecting people to Issues.
4. Enable the Issue and pull-request templates and verify their rendered forms
   in GitHub. Public bug reports must not solicit credentials, bearer tokens,
   private keys, local database files, or unredacted evidence.

## Pre-Publication Audit

Before the visibility change, record a dated audit that covers:

- the complete Git history and reachable refs for secrets, tokens, private
  keys, credentials, PII, generated local state, and unredacted artifacts;
- current source, test fixtures, documentation, examples, releases, and issue
  templates for the same classes of data;
- `.gitignore` coverage for local SQLite databases, runtime homes, keys,
  configuration, coverage, and build outputs;
- Go module and any vendored or copied asset provenance, including their
  license and required attribution/notice obligations; and
- the final repository visibility, branch-protection, maintainer access, and
  GitHub feature settings.

Any finding must be removed from both the current tree and all reachable
history before publication, or publication must be postponed. A scan of only
the default branch is insufficient.

## Acceptance Criteria

1. A newcomer can build, initialize, run, and query the local MVC by following
   the README without guessing commands.
2. Every public-facing document consistently states the experimental,
   local-first scope and avoids production-readiness claims.
3. Contributors, support seekers, and vulnerability reporters each have one
   clear, verified route; community conduct reports use a separate confidential
   route.
4. The existing Apache-2.0 license remains authoritative and the new NOTICE
   does not add extra license terms.
5. Documentation links resolve locally, Markdown is structurally valid, and
   all GitHub templates render correctly.
6. Public visibility is blocked until the operational prerequisites and
   pre-publication audit have been completed and recorded.
