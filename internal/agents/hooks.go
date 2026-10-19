// @gloss implements gmi:HookGuard, gmi:AgentsSnippet scope=file

package agents

import "fmt"

// The Claude Code hook command lines. gloss hook install writes them into
// .claude/settings.json, and the GlossMark Claude Code plugin ships the same
// lines in hooks/hooks.json; the contract test compares the two.

// GuardMatcher is the tool matcher of the PreToolUse and PostToolUse guards.
const GuardMatcher = "Edit|Write"

// GuardEvents are the Claude Code events gloss hook install --guards
// registers, in order.
var GuardEvents = []string{"PreToolUse", "PostToolUse", "Stop"}

// SessionStartCommand is the SessionStart hook: the briefing for the
// session's directory, failing open when gloss is missing.
func SessionStartCommand(actor string) string {
	return fmt.Sprintf("GLOSS_ACTOR=%s gloss context --dir \"$PWD\" --changed --budget 2000 2>/dev/null || true", actor)
}

// GuardCommand is the hook command for one of GuardEvents. The edit guards
// fail open; the Stop guard's exit status and output are the decision.
func GuardCommand(event, actor string) string {
	switch event {
	case "PreToolUse":
		return fmt.Sprintf("GLOSS_ACTOR=%s gloss hook guard --event pre-edit 2>/dev/null || true", actor)
	case "PostToolUse":
		return fmt.Sprintf("GLOSS_ACTOR=%s gloss hook guard --event post-edit 2>/dev/null || true", actor)
	}
	return fmt.Sprintf("GLOSS_ACTOR=%s gloss hook guard --event stop", actor)
}
