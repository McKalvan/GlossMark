#!/usr/bin/env bash
# Registers every subject in subjects.yaml with the Schema Registry, references first.
# Usage: SCHEMA_REGISTRY_URL=http://localhost:8081 ./register.sh
set -euo pipefail
cd "$(dirname "$0")"
REGISTRY="${SCHEMA_REGISTRY_URL:-http://localhost:8081}"

register() {
  local subject=$1 file=$2 refs=$3
  local body
  body=$(jq -n --arg schema "$(cat "$file")" --argjson refs "$refs" \
    '{schemaType: "AVRO", schema: $schema, references: $refs}')
  curl -fsS -X POST -H 'Content-Type: application/vnd.schemaregistry.v1+json' \
    --data "$body" "$REGISTRY/subjects/$subject/versions" >/dev/null
  echo "registered $subject"
}

curl -fsS -X PUT -H 'Content-Type: application/json' \
  --data '{"compatibility": "BACKWARD_TRANSITIVE"}' "$REGISTRY/config" >/dev/null

register com.tidewater.claims.common.Money Money.avsc '[]'
register com.tidewater.claims.common.ClaimRef ClaimRef.avsc '[]'

ref() { jq -n --arg n "com.tidewater.claims.common.$1" '{name: $n, subject: $n, version: 1}'; }
REFS_BOTH=$(jq -s . <(ref Money) <(ref ClaimRef))
REFS_CLAIM=$(jq -s . <(ref ClaimRef))

yq -r '.subjects | to_entries[] | [.key, .value.file, (.value.refs | join(","))] | @tsv' subjects.yaml |
while IFS=$'\t' read -r subject file refs; do
  case "$refs" in
    *Money*) register "$subject" "$file" "$REFS_BOTH" ;;
    *) register "$subject" "$file" "$REFS_CLAIM" ;;
  esac
done
