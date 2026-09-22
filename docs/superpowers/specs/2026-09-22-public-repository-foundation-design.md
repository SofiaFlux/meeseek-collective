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
- `SECURITY.md`: scope, a private reporting route using GitHub Security
  Advisories, the information needed for a report, and an explicit request not
  to file public issues for suspected vulnerabilities.
- `CODE_OF_CONDUCT.md`: a compact Contributor Covenant 2.1 policy with a
  repository-maintainer enforcement contact through GitHub.
- `SUPPORT.md`: distinction between GitHub Discussions/questions, Issues for
  reproducible bugs, and Security Advisories for vulnerabilities; no SLA.
- `NOTICE`: project attribution suitable for the existing Apache-2.0 license.
- GitHub issue and pull-request templates that collect reproducible reports,
  security routing, test evidence, and a scope checklist.

## Out of Scope

This change will not add release automation, a roadmap, a governance model,
telemetry, badges whose status cannot be verified, or an implied support
commitment. It will not alter runtime behavior or licensing terms.

## Acceptance Criteria

1. A newcomer can build, initialize, run, and query the local MVC by following
   the README without guessing commands.
2. Every public-facing document consistently states the experimental,
   local-first scope and avoids production-readiness claims.
3. Contributors, support seekers, and vulnerability reporters each have one
   clear route.
4. The existing Apache-2.0 license remains authoritative and the new NOTICE
   does not add extra license terms.
5. Documentation links resolve locally and Markdown is structurally valid.
