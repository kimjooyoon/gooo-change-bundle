#!/usr/bin/env bash
set -euo pipefail

bin=${1:?path to the built binary is required}
run=${2:?caller-owned CI run directory is required}
repo_root=$(pwd)
source="$run/source"
bundle_one="$run/bundle-one"
bundle_two="$run/bundle-two"
mkdir -p "$source"
cp -R fixtures/source-tree/. "$source/"

now_ms() { date +%s%3N; }
canonical_digest() {
  local path=$1
  local field=$2
  jq -cS --arg field "$field" '.[ $field ] = ""' "$path" | sha256sum | awk '{print "sha256:" $1}'
}

source_digest=$("$bin" digest --source-root "$source")
intent="examples/change-bundle/change-intent.gooo"
intent_digest="sha256:$(sha256sum "$intent" | awk '{print $1}')"
preimage_digest="sha256:$(sha256sum "$source/app.txt" | awk '{print $1}')"
printf 'after\n' > "$run/after.txt"
postimage_digest="sha256:$(sha256sum "$run/after.txt" | awk '{print $1}')"
postimage_base64=$(base64 -w0 "$run/after.txt")

proposal="$run/proposal.json"
jq -n \
  --arg source "$source_digest" \
  --arg intent "$intent_digest" \
  --arg pre "$preimage_digest" \
  --arg post "$postimage_digest" \
  --arg content "$postimage_base64" \
  '{schema:"gooo/change-bundle/approved-proposal/v1",proposal_id:"proposal-ci-1",status:"APPROVED",source_tree_digest:$source,intent_digest:$intent,authority_receipt_id:"authority-ci-1",authority_receipt_digest:"",approved_by:"human-ci-1",changes:[{path:"app.txt",operation:"MODIFY",preimage_digest:$pre,postimage_digest:$post,postimage_base64:$content,hunks:[{start_line:1,end_line:1}]}],proposal_digest:""}' > "$proposal"
proposal_digest=$(canonical_digest "$proposal" proposal_digest)
jq --arg digest "$proposal_digest" '.proposal_digest=$digest' "$proposal" > "$run/proposal-final.json"
mv "$run/proposal-final.json" "$proposal"

authority="$run/authority.json"
jq -n \
  --arg proposal "$proposal_digest" \
  --arg intent "$intent_digest" \
  '{schema:"gooo/change-bundle/authority-receipt/v1",receipt_id:"authority-ci-1",proposal_id:"proposal-ci-1",proposal_digest:$proposal,intent_digest:$intent,approved:true,approved_by:"human-ci-1",repository_writes:0,local_test_executions:0,cross_project_required_gates:0,apply_authorized:false,commit_authorized:false,push_authorized:false,pull_request_authorized:false,merge_authorized:false,receipt_digest:""}' > "$authority"
authority_digest=$(canonical_digest "$authority" receipt_digest)
jq --arg digest "$authority_digest" '.receipt_digest=$digest' "$authority" > "$run/authority-final.json"
mv "$run/authority-final.json" "$authority"

mkdir -p "$bundle_one" "$bundle_two"
start=$(now_ms)
/usr/bin/time -f '%M' -o "$run/conformance-rss" "$bin" materialize \
  --source-root "$source" --source-digest "$source_digest" --proposal "$proposal" \
  --authority "$authority" --intent "$intent" \
  --contract contracts/change-bundle-denominator-v1.json --out "$bundle_one" > "$run/materialize-one.json"
end=$(now_ms)
conformance_wall_ms=$((end-start))

"$bin" materialize \
  --source-root "$source" --source-digest "$source_digest" --proposal "$proposal" \
  --authority "$authority" --intent "$intent" \
  --contract contracts/change-bundle-denominator-v1.json --out "$bundle_two" > "$run/materialize-two.json"

jq '{decision,proposal_digest,proposal_self_digest_observed,authority_receipt_digest,authority_self_digest_observed,metrics,findings,unknowns}' "$bundle_one/bundle-manifest.json"
jq -e '.decision == "CLOSED" and .metrics.repository_writes == 0 and .metrics.local_test_executions == 0 and .metrics.cross_project_required_gates == 0 and .metrics.replay_mismatches == 0 and .metrics.rollback_mismatches == 0' "$bundle_one/bundle-manifest.json" >/dev/null
diff -ru "$bundle_one" "$bundle_two" >/dev/null
jq -e '[.cases[] | select(.class == "NORMAL")] | length >= 3' fixtures/scenarios.json >/dev/null
jq -e '[.cases[] | select(.class == "UNKNOWN")] | length >= 3' fixtures/scenarios.json >/dev/null
jq -e '[.cases[] | select(.class == "REFUTED")] | length >= 3' fixtures/scenarios.json >/dev/null

bundle_files=$(find "$bundle_one" -type f -print | wc -l | tr -d ' ')
bundle_bytes=$(find "$bundle_one" -type f -printf '%s\n' | awk '{sum += $1} END {print sum + 0}')
changed_paths=$(jq '.metrics.changed_paths' "$bundle_one/bundle-manifest.json")
changed_hunks=$(jq '.metrics.changed_hunks' "$bundle_one/bundle-manifest.json")
replay_comparisons=$(jq '.comparisons' "$bundle_one/replay-receipt.json")
replay_mismatches=$(jq '.mismatches' "$bundle_one/replay-receipt.json")
rollback_comparisons=$(jq '.rollback_comparisons' "$bundle_one/replay-receipt.json")
rollback_mismatches=$(jq '.rollback_mismatches' "$bundle_one/replay-receipt.json")
conformance_rss_kib=$(cat "$run/conformance-rss")
build_wall_ms=${BUILD_WALL_MS:-0}
test_wall_ms=${TEST_WALL_MS:-0}
build_rss_kib=${BUILD_RSS_KIB:-0}
test_rss_kib=${TEST_RSS_KIB:-0}
peak_rss_kib=$((build_rss_kib > test_rss_kib ? build_rss_kib : test_rss_kib))
peak_rss_kib=$((peak_rss_kib > conformance_rss_kib ? peak_rss_kib : conformance_rss_kib))

tests_executed=$(jq '[.[] | select(.Test != null and .Action == "pass" and (.Cached != true))] | length' "$run/test-events.json")
tests_reused=$(jq '[.[] | select(.Test != null and .Action == "pass" and .Cached == true)] | length' "$run/test-events.json")
tests_skipped=$(jq '[.[] | select(.Test != null and .Action == "skip")] | length' "$run/test-events.json")
tests_not_observed=0

count_lines() {
  local pattern=$1
  local total=0
  while IFS= read -r -d '' file; do
    lines=$(wc -l < "$file" | tr -d ' ')
    total=$((total + lines))
  done < <(find . -type f -name "$pattern" -not -path './.git/*' -print0)
  echo "$total"
}
go_physical_lines=$(count_lines '*.go')
gooo_physical_lines=$(count_lines '*.gooo')
repo_files=$(git ls-files | awk '$0 != "README.md" {n++} END {print n + 0}')
repo_directories=$(git ls-files | awk '$0 != "README.md" {n=split($0,a,"/"); for (i=1; i<n; i++) {d=""; for (j=1; j<=i; j++) d=d (j==1?"":"/") a[j]; seen[d]=1}} END {count=0; for (d in seen) count++; print count + 0}')

jq -n \
  --arg schema "gooo/change-bundle/ci-metrics/v1" \
  --argjson bundle_file_count "$bundle_files" \
  --argjson bundle_bytes "$bundle_bytes" \
  --argjson changed_paths "$changed_paths" \
  --argjson changed_hunks "$changed_hunks" \
  --argjson replay_comparisons "$replay_comparisons" \
  --argjson replay_mismatches "$replay_mismatches" \
  --argjson rollback_comparisons "$rollback_comparisons" \
  --argjson rollback_mismatches "$rollback_mismatches" \
  --argjson build_wall_ms "$build_wall_ms" --argjson test_wall_ms "$test_wall_ms" \
  --argjson conformance_wall_ms "$conformance_wall_ms" --argjson peak_rss_kib "$peak_rss_kib" \
  --argjson tests_executed "$tests_executed" --argjson tests_reused "$tests_reused" \
  --argjson tests_skipped "$tests_skipped" --argjson tests_not_observed "$tests_not_observed" \
  --argjson go_physical_lines "$go_physical_lines" --argjson gooo_physical_lines "$gooo_physical_lines" \
  --argjson files "$repo_files" --argjson directories "$repo_directories" \
  '{schema:$schema,bundle_file_count:$bundle_file_count,bundle_bytes:$bundle_bytes,changed_paths:$changed_paths,changed_hunks:$changed_hunks,replay_comparisons:$replay_comparisons,replay_mismatches:$replay_mismatches,rollback_comparisons:$rollback_comparisons,rollback_mismatches:$rollback_mismatches,build_wall_ms:$build_wall_ms,test_wall_ms:$test_wall_ms,conformance_wall_ms:$conformance_wall_ms,peak_rss_kib:$peak_rss_kib,tests_executed:$tests_executed,tests_reused:$tests_reused,tests_skipped:$tests_skipped,tests_not_observed:$tests_not_observed,go_physical_lines:$go_physical_lines,gooo_physical_lines:$gooo_physical_lines,files:$files,directories:$directories,repository_writes:0,local_test_executions:0,cross_project_required_gates:0}' > "$run/ci-metrics.json"
cp "$run/ci-metrics.json" "$bundle_one/ci-metrics.json"
echo "conformance: PASS"
