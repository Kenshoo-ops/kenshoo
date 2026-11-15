#!/usr/bin/env bash
# Asserts the chart's guarantees against the rendered manifests. Needs helm and yq (mikefarah).
set -uo pipefail
cd "$(dirname "$0")"
failures=0
render() { helm template kenshoo . -f ci/example-values.yaml "$@" 2>&1; }
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1"; shift; for l in "$@"; do echo "     $l"; done; failures=$((failures + 1)); }

# eq NAME KIND YQ_EXPR WANT [helm args]: the rendered object of this kind has WANT at YQ_EXPR
eq() {
  local name=$1 kind=$2 expr=$3 want=$4 out got; shift 4
  out=$(render "$@") || { fail "$name" "helm template failed: $out"; return; }
  got=$(printf '%s\n' "$out" | yq "select(.kind == \"$kind\") | $expr" 2>&1)
  if [ "$got" = "$want" ]; then pass "$name"; else fail "$name" "got:  $got" "want: $want"; fi
}
# eq_cfg NAME YQ_EXPR WANT [helm args]: the same for the YAML file inside the ConfigMap
eq_cfg() {
  local name=$1 expr=$2 want=$3 out got; shift 3
  out=$(render "$@") || { fail "$name" "helm template failed: $out"; return; }
  got=$(printf '%s\n' "$out" | yq 'select(.kind == "ConfigMap") | .data["kenshoo.yaml"]' | yq "$expr" 2>&1)
  if [ "$got" = "$want" ]; then pass "$name"; else fail "$name" "got:  $got" "want: $want"; fi
}
# refuses NAME MESSAGE [helm args]: rendering must fail, and the error must contain MESSAGE
refuses() {
  local name=$1 want=$2 out; shift 2
  if out=$(render "$@"); then fail "$name" "rendering succeeded, want an error containing: $want"; return; fi
  case "$out" in *"$want"*) pass "$name" ;; *) fail "$name" "error was: $out" ;; esac
}

P='.spec.jobTemplate.spec.template.spec'
eq "a running job is never overlapped" CronJob '.spec.concurrencyPolicy' Forbid
eq "a bad backup is not retried" CronJob '.spec.jobTemplate.spec.backoffLimit' 0
eq "the run has a hard deadline" CronJob '.spec.jobTemplate.spec.activeDeadlineSeconds' 1800
eq "postgres is a native sidecar" CronJob "$P"'.initContainers[0].restartPolicy' Always
eq "readiness is checked over TCP" CronJob "$P"'.initContainers[0].startupProbe.exec.command | join(" ")' "pg_isready -h 127.0.0.1 -U postgres"
eq "postgres listens on loopback only" CronJob "$P"'.initContainers[0].args | join(" ")' "-c listen_addresses=127.0.0.1"
eq "the admin DSN is the pod's loopback" CronJob "$P"'.containers[0].env[] | select(.name == "KENSHOO_ADMIN_DSN") | .value' "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable"
eq "the service account token is not mounted" CronJob "$P"'.automountServiceAccountToken' false
eq "the pod runs as non-root" CronJob "$P"'.securityContext.runAsNonRoot' true
eq "kenshoo has a read-only root filesystem" CronJob "$P"'.containers[0].securityContext.readOnlyRootFilesystem' true
eq "no credentials without a Secret" CronJob "$P"'.containers[0].env | length' 1
eq "credentials come from the named Secret" CronJob "$P"'.containers[0].env[] | select(.name == "AWS_ACCESS_KEY_ID") | .valueFrom.secretKeyRef.name' s3-creds --set credentialsSecret=s3-creds

img=$(render | yq 'select(.kind == "CronJob") | '"$P"'.containers[0].image')
if [[ $img =~ ^ghcr\.io/kenshoo-ops/kenshoo@sha256:[0-9a-f]{64}$ ]]; then pass "the image is pinned by digest"; else fail "the image is pinned by digest" "got: $img"; fi

eq_cfg "target.version defaults to postgres.major" '.target.version' 16
eq_cfg "target.version follows postgres.major" '.target.version' 15 --set postgres.major=15
eq_cfg "the user cannot override the generated target" '.target.version' 16 --set config.target.version=99
eq_cfg "the checks come from the values" '.checks[0].name' users-not-empty

refuses "a missing bucket is refused" "bucket is required" --set config.source.storage.bucket=
refuses "a missing region is refused" "region is required" --set config.source.storage.region=
refuses "no checks is refused" "at least one check" --set-json 'config.checks=[]'
refuses "a tag instead of a digest is refused" "must look like sha256" --set image.digest=latest
refuses "an empty digest is refused" "image.digest is required" --set image.digest=

echo
if [ "$failures" -ne 0 ]; then echo "$failures check(s) failed"; exit 1; fi
echo "all checks passed"
