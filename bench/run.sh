#!/usr/bin/env sh
# @gloss implements gmi:AgentBenchmark scope=file
#
# bench/run.sh: run a coding agent on each benchmark task with and without
# GlossMark context, and score what it leaves behind (docs/BENCHMARK.md).
#
#   bench/run.sh --examples ../glossmark-examples --agent claude [--tasks bench/tasks] \
#                [--conditions bare,glossmark] [--runs 1] [--out bench/results/<timestamp>] \
#                [--only <task-id>] [--timeout 1800] [--dry-run]
#
# Agents: claude, cursor-agent, codex, or cmd:'<shell>' which is run in the
# task's working copy with the prompt in $BENCH_PROMPT and $BENCH_PROMPT_FILE.
# The script builds gloss and glossbench from this tree; nothing is installed.
set -eu

EXAMPLES=""
AGENT=""
TASKS="bench/tasks"
CONDITIONS="bare,glossmark"
RUNS=1
OUT=""
ONLY=""
TIMEOUT=1800
DRY=0

while [ $# -gt 0 ]; do
  case "$1" in
    --examples) EXAMPLES="$2"; shift 2 ;;
    --agent) AGENT="$2"; shift 2 ;;
    --tasks) TASKS="$2"; shift 2 ;;
    --conditions) CONDITIONS="$2"; shift 2 ;;
    --runs) RUNS="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --only) ONLY="$2"; shift 2 ;;
    --timeout) TIMEOUT="$2"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) sed -n '3,15p' "$0"; exit 0 ;;
    *) echo "bench/run.sh: unknown flag $1" >&2; exit 2 ;;
  esac
done
[ -n "$EXAMPLES" ] || { echo "bench/run.sh: --examples <glossmark-examples checkout> is required" >&2; exit 2; }
[ -n "$AGENT" ] || { echo "bench/run.sh: --agent claude|cursor-agent|codex|cmd:'...' is required" >&2; exit 2; }
[ -d "$EXAMPLES/showcase" ] || { echo "bench/run.sh: $EXAMPLES has no showcase/ directory" >&2; exit 2; }

ROOT=$(cd "$(dirname "$0")/.." && pwd)
EXAMPLES=$(cd "$EXAMPLES" && pwd)
[ -n "$OUT" ] || OUT="$ROOT/bench/results/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$OUT/work" "$OUT/scores" "$OUT/logs"
BIN="$OUT/bin"
mkdir -p "$BIN"

echo "building gloss and glossbench into $BIN"
(cd "$ROOT" && go build -o "$BIN/gloss" ./cmd/gloss && go build -o "$BIN/glossbench" ./cmd/glossbench)

# The agent command. Every preset reads the prompt from $BENCH_PROMPT; the
# flags are the non-interactive, edit-allowing mode of each CLI as of the
# version this script was written against, so check them against yours.
case "$AGENT" in
  claude) AGENT_CMD='claude -p "$BENCH_PROMPT" --permission-mode acceptEdits' ;;
  cursor-agent) AGENT_CMD='cursor-agent -p --force "$BENCH_PROMPT"' ;;
  codex) AGENT_CMD='codex exec --full-auto "$BENCH_PROMPT"' ;;
  cmd:*) AGENT_CMD="${AGENT#cmd:}" ;;
  *) echo "bench/run.sh: unknown agent $AGENT (claude, cursor-agent, codex or cmd:'...')" >&2; exit 2 ;;
esac
AGENT_NAME="${AGENT%%:*}"

# yaml_field FILE KEY: the one-line scalar value of a top-level key.
yaml_field() { sed -n "s/^$2:[[:space:]]*//p" "$1" | head -1; }

# prompt_of FILE: the prompt block (the lines indented under "prompt: |").
prompt_of() { awk '/^prompt: \|/{p=1; next} p && /^[^ ]/{p=0} p{sub(/^  /, ""); print}' "$1"; }

# prepare WORK CONDITION: a fresh copy of the example repository as one git
# commit, then the condition applied.
prepare() {
  work="$1"; cond="$2"; src="$3"
  rm -rf "$work"
  mkdir -p "$work"
  (cd "$src" && tar --exclude=.git -cf - .) | (cd "$work" && tar -xf -)
  (cd "$work" && git init -q && git add -A && git -c user.name=bench -c user.email=bench@example.invalid commit -q -m "base")
  case "$cond" in
    bare)
      # No agent surface: no MCP server, no hook, no vocabulary block in the
      # agent files, no gloss on PATH. The markers and ontology stay, as they
      # would in any repository that adopted GlossMark.
      rm -f "$work/.mcp.json" "$work/.cursor/mcp.json" "$work/.cursor/rules/glossmark.mdc"
      rm -rf "$work/.claude/skills/adopt-glossmark" "$work/.claude/skills/derive-ontology" "$work/.claude/skills/review-glossmark" "$work/.claude/skills/curate-increment"
      if [ -f "$work/.claude/settings.json" ] && grep -q "gloss context" "$work/.claude/settings.json"; then rm -f "$work/.claude/settings.json"; fi
      for f in "$work/AGENTS.md" "$work/CLAUDE.md"; do
        [ -f "$f" ] && awk '/<!-- gloss:begin -->/{skip=1} !skip{print} /<!-- gloss:end -->/{skip=0}' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
      done
      ;;
    glossmark)
      # The full agent surface: the SessionStart hook and the MCP server
      # (Claude Code), the rule and MCP config (Cursor), the vocabulary block
      # in AGENTS.md, and gloss on PATH so the gate can be run.
      (cd "$work" && PATH="$BIN:$PATH" GLOSS_ACTOR="agent:$AGENT_NAME" gloss hook install --actor "agent:$AGENT_NAME" --cursor >/dev/null)
      [ -f "$work/AGENTS.md" ] || printf '# Agents\n\n' > "$work/AGENTS.md"
      (cd "$work" && PATH="$BIN:$PATH" gloss agents --write AGENTS.md >/dev/null)
      ;;
    *) echo "bench/run.sh: unknown condition $cond (bare, glossmark)" >&2; exit 2 ;;
  esac
  (cd "$work" && git add -A && git -c user.name=bench -c user.email=bench@example.invalid commit -q --allow-empty -m "condition: $cond")
}

for task in "$TASKS"/*.yaml; do
  id=$(yaml_field "$task" id)
  [ -n "$id" ] || id=$(basename "$task" .yaml)
  [ -z "$ONLY" ] || [ "$ONLY" = "$id" ] || continue
  repo=$(yaml_field "$task" repo)
  src="$EXAMPLES/$repo"
  [ -d "$src" ] || { echo "$id: $src is missing" >&2; exit 1; }
  base_prompt=$(prompt_of "$task")
  for cond in $(echo "$CONDITIONS" | tr ',' ' '); do
    run=1
    while [ "$run" -le "$RUNS" ]; do
      name="$id.$cond.$run"
      work="$OUT/work/$name"
      echo "== $name"
      prepare "$work" "$cond" "$src"
      # The base the run is scored against is the tree the agent saw, with
      # the condition's files in it, so hook and MCP files never count as
      # the agent's change.
      base="$OUT/work/$name.base"
      rm -rf "$base"
      mkdir -p "$base"
      (cd "$work" && tar --exclude=.git -cf - .) | (cd "$base" && tar -xf -)
      prompt="$base_prompt"
      if [ "$cond" = glossmark ]; then
        prompt="$prompt
$(cat "$ROOT/bench/prompts/glossmark.md")"
      fi
      printf '%s\n' "$prompt" > "$OUT/logs/$name.prompt.md"
      if [ "$DRY" -eq 1 ]; then
        echo "   (dry run) would run: $AGENT_CMD"
      else
        path="$PATH"
        [ "$cond" = glossmark ] && path="$BIN:$PATH"
        start=$(date +%s)
        (cd "$work" && BENCH_PROMPT="$prompt" BENCH_PROMPT_FILE="$OUT/logs/$name.prompt.md" PATH="$path" GLOSS_ACTOR="agent:$AGENT_NAME" \
          timeout "$TIMEOUT" sh -c "$AGENT_CMD") > "$OUT/logs/$name.agent.log" 2>&1 || echo "   agent exited $? (see logs/$name.agent.log)"
        echo "   agent took $(( $(date +%s) - start ))s"
      fi
      (cd "$work" && git add -A && git diff --cached > "$OUT/logs/$name.patch" && git reset -q)
      "$BIN/glossbench" score --task "$task" --base "$base" --head "$work" --condition "$cond" --run "$run" --agent "$AGENT_NAME" \
        -o "$OUT/scores/$name.json" | sed 's/^/   /'
      run=$((run + 1))
    done
  done
done

"$BIN/glossbench" report "$OUT/scores" > "$OUT/REPORT.md"
echo
echo "report: $OUT/REPORT.md"
cat "$OUT/REPORT.md"
