# Publication readiness

**Status as of 2026-09-22: not ready for a visibility change.** This record
is deliberately public-safe: it records gate status and command names, not
restricted audit evidence, reporting recipients, access configuration, or
security findings.

## Release decision

Do not make the repository public until every **pre-publication** gate below
is checked by authorized maintainers. An unchecked pre-publication item is a
stop condition, not a waiver. The mandatory post-publication PVR transition is
not a precondition for visibility: it must instead be completed and recorded
immediately after the public transition.

## Pre-publication maintainer and community gates

- [ ] The designated sole maintainer has tested access to the security mailbox.
- [ ] The confidential primary conduct-reporting route has been tested.
- [ ] The separate confidential conduct-escalation route has been tested.
- [ ] A security-mailbox receipt and acknowledgment test has been completed.
- [x] The documented no-general-support policy remains in effect; general
  support is unavailable until Discussions and a question category are enabled
  and tested.
- [ ] Issue-form and pull-request-template rendering has been verified in the
  GitHub UI after these files reach the default branch.
- [ ] Branch protection and the sole maintainer's access have been reviewed by
  the authorized maintainer.

## Repository and release audit

- [x] Local object-integrity command recorded: `git fsck --no-reflogs
  --unreachable` completed successfully on 2026-09-22. Restricted evidence is
  retained outside this public record.
- [x] The automated redacted `gitleaks detect` scan completed on 2026-09-22
  without a reported finding. Its report and other restricted evidence are not
  published here.
- [ ] Manual reachable-history, fixture, generated-artifact, documentation,
  and release-asset review remains incomplete. Manual PII and private-state
  adjudication must be completed and recorded in restricted evidence before
  publication.
- [x] Tracked-path check recorded: `git ls-files | rg -n
  '(\\.db(-wal|-shm)?|\\.sqlite(-wal|-shm)?|config\\.json|id_(rsa|ed25519)|\\.pem|\\.key)$'` found no
  matching tracked path on 2026-09-22.
- [x] Local-artifact ignore coverage was corrected for `.meeseek/`; re-run
  `git check-ignore -v .meeseek/config.json .meeseek/state.db
  .meeseek/evidence/example` before publication.
- [ ] Release-asset audit requires an authorized GitHub check at the
  publication transition; no public remediation PR is recorded.

## License and provenance

- [x] Dependency inventory completed on 2026-09-22 from the actual build
  graph (`go list -deps -json ./...`): 46 third-party modules were found.
  Each had a reviewed root license. The resulting set is Apache-2.0, BSD, or
  MIT; no copyleft dependency was identified in this source-build inventory.
- [x] Tracked source, fixtures, generated artifacts, and documentation were
  reviewed for copied third-party material. No additional attribution-bearing
  copied asset was identified. Release assets remain subject to the separate
  release-asset audit below.
- [x] `NOTICE` records the two upstream Apache notices found in the used
  dependency graph (`go.yaml.in/yaml/v2` and `go.yaml.in/yaml/v3`).
- [x] The authorized owner designated the copyright notice for
  project-authored source on 2026-09-22; it is recorded in `COPYRIGHT`.

## Local validation

The 2026-09-22 audit ran the following commands in a Linux sandbox. Repeat
them in the intended release environment and retain the full output only in
restricted evidence:

```text
go test ./... -count=1
go vet ./...
go build ./cmd/meeseek ./cmd/meeseek-box
python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
```

- [ ] `go test ./... -count=1` is not green in this sandbox: tests requiring
  IPv6 loopback listeners cannot bind (`socket operation not permitted`). This
  is an environment-limited result, not a release-environment pass.
- [x] `go vet ./...` completed with exit 0 in the Linux sandbox.
- [x] `go build ./cmd/meeseek ./cmd/meeseek-box` completed with exit 0 in the
  Linux sandbox.
- [x] The SQLite persistence spike completed with exit 0 in the Linux sandbox.
- [x] `python3 scripts/verify_docs.py docs/publication-readiness.md` completed
  successfully in the Linux sandbox.
- [ ] Docker was unavailable in the 2026-09-22 audit environment, so the OCI
  gate remains unchecked.

## Mandatory post-publication PVR transition

The following actions are mandatory immediately after the public transition.
They are intentionally separate from pre-publication gates so that the
checklist does not require a public-only capability before visibility changes.

- [ ] After a public transition: authorized triagers must configure their
  notification settings, enable and test Private Vulnerability Reporting
  (PVR), and record acknowledgments in restricted evidence.
- [ ] Update `SECURITY.md` only after the post-publication reporting-path test
  succeeds; until then, its current transition guidance remains authoritative.

## Evidence handling

Restricted command output, scan reports, access details, and any remediation
details belong only in the ignored `docs/publication-readiness-private.md`
record. Public remediation references may be added here only after a related
public pull request exists.
