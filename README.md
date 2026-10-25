# kenshoo

**Prove your backups actually restore.**

kenshoo (from the Japanese 検証, *kenshō*, "verification") restores your latest database backup into a throwaway database, runs checks against the restored data, and reports whether the backup is really usable. A backup job that finishes with "success" only proves a file was written. kenshoo proves the file works.

> **Status: early development.** The design is settled, but the code is not written yet. Nothing in this repository is usable today. See the [roadmap](#roadmap).

## Why

Most teams take backups. Far fewer ever restore them, and the usual way to find out a backup is broken is during an incident. Backups fail silently: a truncated file, a schema change, a missing extension, a full disk, an expired key. kenshoo turns "can we recover?" into something you measure continuously.

## How it works

Each run is one short-lived job:

1. Load the configuration (what to verify, where backups live, which checks to run).
2. Find the newest backup in storage and download it.
3. Start an empty temporary PostgreSQL database.
4. Restore the backup into it.
5. Run checks: restore succeeded, schema version, row counts, data freshness, custom SQL, restore duration.
6. Report the result as Prometheus metrics, logs and alerts.
7. Throw everything away.

The main signal is **time since the last verified restore**: the number that tells you whether you could recover right now.

## Planned configuration

*This is the intended shape of the config. It may change before the first release.*

```yaml
source:
  type: pg_dump
  storage:
    type: s3
    endpoint: https://<s3-compatible-endpoint>
    bucket: db-backups
    prefix: my-app/
target:
  engine: postgres
  version: "16"
checks:
  - name: users-not-empty
    sql: "SELECT count(*) FROM users"
    expect: "> 0"
  - name: backup-freshness
    sql: "SELECT now() - max(created_at) FROM transactions"
    expect: "< 26h"
limits:
  restore_timeout: 15m
```

## Scope

**Version 1:**
- PostgreSQL backups made with `pg_dump`, stored in S3-compatible object storage
- Config-driven checks
- Prometheus metrics and Telegram alerts
- Runs as a Kubernetes CronJob

**Later:** point-in-time recovery (WAL-G, pgBackRest), other database engines, other storage backends, a Kubernetes operator.

kenshoo only *verifies* backups. It does not take or manage them.

## Part of a larger project

kenshoo is one of three repositories in the [Kenshoo-ops](https://github.com/Kenshoo-ops) organization:

| Repository | Purpose |
|---|---|
| **kenshoo** (this repo) | The verifier, written in Go |
| [kenshoo-platform](https://github.com/Kenshoo-ops/kenshoo-platform) | Infrastructure: Terraform, k3s, Argo CD, Kyverno policies, observability |
| [kenshoo-workflows](https://github.com/Kenshoo-ops/kenshoo-workflows) | Reusable CI pipeline that builds, scans, signs and attests the images |

The pipeline signs the image, the cluster refuses anything unsigned, and the verifier runs on that cluster. A tool that reads production backups should itself be deployed safely.

## Roadmap

- [ ] Verifier skeleton: configuration, logging, `Restorer` and `Check` interfaces
- [ ] `pg_dump` restorer reading from S3-compatible storage
- [ ] Built-in checks and custom SQL checks
- [ ] Prometheus metrics and Telegram alerts
- [ ] Container image and Helm chart
- [ ] Additional restorers (WAL-G, pgBackRest)

## Contributing

The project is just getting started. Issues and ideas are welcome. Please open an issue before starting a large change, so we can agree on the approach first. All changes go through pull requests.

## Security

Please report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
