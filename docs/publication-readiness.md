# Publication readiness

**Status as of 2026-09-22: not ready for a visibility change.** This record
is deliberately public-safe: it records gate status and command names, not
restricted audit evidence, reporting recipients, access configuration, or
security findings.

## Release decision

Do not make the repository public until every required gate below is checked
by authorized maintainers. An unchecked item is a stop condition, not a
waiver.

## Maintainer and community gates

- [ ] Two maintainers have tested access to the designated security mailbox.
- [ ] The confidential primary conduct-reporting route has been tested.
- [ ] The separate confidential conduct-escalation route has been tested.
- [ ] A security-mailbox receipt and acknowledgment test has been completed.
- [x] The documented no-general-support policy remains in effect; general
  support is unavailable until Discussions and a question category are enabled
  and tested.
- [ ] Issue-form and pull-request-template rendering has been verified in the
  GitHub UI after these files reach the default branch.
- [ ] Branch protection and maintainer access have been reviewed by authorized
  maintainers.

## Repository and release audit

- [x] Local object-integrity command recorded: `git fsck --no-reflogs
  --unreachable` completed successfully on 2026-09-22. Restricted evidence is
  retained outside this public record.
- [ ] Full reachable-history, fixture, generated-artifact, documentation, and
  release-asset review remains incomplete. The required scanner run and manual
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

- [ ] Dependency and copied-asset inventory is incomplete. `go list -m -json
  all` was requested for the inventory, but every dependency license and every
  copied/generated asset still requires provenance review.
- [ ] `NOTICE` has not been added. It may be created only after the inventory
  establishes the exact third-party attribution text required for this source
  tree and release artifacts.

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

## Publication-transition gates

- [ ] Before any visibility change: authorized maintainers must verify the
  confidential reporting routes, repository access, templates, and the
  documented support policy.
- [ ] After a public transition: authorized triagers must configure their
  notification settings, enable and test the platform vulnerability-reporting
  path, and record acknowledgments in restricted evidence.
- [ ] Update `SECURITY.md` only after the post-publication reporting-path test
  succeeds; until then, its current transition guidance remains authoritative.

## Evidence handling

Restricted command output, scan reports, access details, and any remediation
details belong only in the ignored `docs/publication-readiness-private.md`
record. Public remediation references may be added here only after a related
public pull request exists.
