# Contributing to Meeseek Collective

## Scope and behavioral expectations

Meeseek Collective is an experimental, local-first, single-Cube MVC. Keep contributions consistent with that scope: do not present local controls as production guarantees, and do not expand authority, egress, or containment claims without matching implementation and evidence. Treat contributors and reviewers respectfully; the conduct policy is available at [CODE_OF_CONDUCT.md](https://github.com/SofiaFlux/meeseek-collective/blob/main/CODE_OF_CONDUCT.md).

## Local setup

Use Go 1.27, then download dependencies and build the local binaries:

```bash
go mod download
mkdir -p ./bin
go build -o ./bin/meeseek ./cmd/meeseek
go build -o ./bin/meeseek-box ./cmd/meeseek-box
```

For a local smoke path, initialize a disposable home, start the Box in one terminal, and query it from another:

```bash
MEESEEK_HOME="$(mktemp -d)"
export MEESEEK_HOME
printf 'Reuse this MEESEEK_HOME in both terminals: %s\n' "$MEESEEK_HOME"
./bin/meeseek init
```

In terminal 1, export the printed path and start the Box:

```bash
export MEESEEK_HOME=/the/path/printed/above
./bin/meeseek-box
```

In terminal 2, export that same path and query the Box:

```bash
export MEESEEK_HOME=/the/path/printed/above
./bin/meeseek status
```

## Focused changes and tests

Keep each change focused, explain its behavior and boundaries, and add or update tests for changed behavior. Run the relevant focused tests while working, then run the documented validation matrix before requesting review.

Do not commit `MEESEEK_HOME`, keys, bearer tokens, SQLite databases, raw evidence, or credentials. Redact logs and examples before sharing them.

## Validation matrix

Run these checks from the repository root:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./cmd/meeseek ./cmd/meeseek-box
python3 docs/superpowers/research/spikes/2026-09-14-sqlite-persistence-spike.py
```

On Linux with Docker available, also run the OCI bypass/capability acceptance profile. That gate must not be replaced with a unit-test-only approximation.

## Pull-request checklist

- [ ] The change is focused and its scope is described.
- [ ] Relevant tests were added or updated and the validation matrix was run as applicable.
- [ ] Documentation reflects user-visible behavior and limits.
- [ ] The change contains no secrets, personal data, raw local state, or unredacted evidence.
- [ ] Suspected vulnerabilities are reported through [SECURITY.md](SECURITY.md), not a pull request.

## Inbound license

By contributing, you agree that your contributions are licensed under the Apache License 2.0, consistent with the repository [LICENSE](LICENSE).

For security reports, use [SECURITY.md](SECURITY.md). For support routing, use [SUPPORT.md](SUPPORT.md).
