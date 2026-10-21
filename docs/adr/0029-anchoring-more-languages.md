<!-- @gloss documents gmi:DeclarationAnchor, gmi:MarkerGrammar scope=file -->
# ADR 0029: Anchoring more languages, and comment styles by file name or glob

Status: accepted, 2026-10-21. Closes issue #156 (E8, L15, L16, L17, FF6, K24).

## Context

Adoptions in real repositories found languages and file types that anchor wrongly or not at all, mostly without a diagnostic:

- Kotlin `suspend`, `inline` and `operator` functions and backtick test names fell back to `scope=block`; a data class with a multi-line primary constructor anchored to its first line only (E8).
- Perl had a comment style but no declaration rules (L15).
- A SQL `CREATE FUNCTION` ended at the first `;`, which sits inside its dollar-quoted body (L16).
- `comment-styles:` could not name a file without an extension by path, such as everything under `cron.d/` (L17).
- `.prisma` had no comment style, and `gloss suggest` never proposed in a schema (FF6).
- A `#` line inside a bash heredoc reads as a comment of the script (K24).

## Decision

**Kotlin functions.** A Kotlin rule finds `fun` after any modifiers (`suspend`, `inline`, `operator`, `infix`, `tailrec`, visibility, `override`, ...), type parameters and an extension receiver (`fun <T> List<T>.only()` is `only`). A backtick name is recorded without its backticks (`early claim plus prior claims scores high`), as a Gherkin scenario keeps its spaces: the symbol is what a reader searches for, and backticks are quoting, not part of the name. Kotlin functions are kind `function`, as in Python and TypeScript.

**Where a brace declaration ends.** A parameter list that spans lines is part of the signature, so the body may open after it, and a declaration ends at its closing brace. A declaration with no `{` on its signature's last line or the two lines after it, or with a blank line first, has no body and ends with its signature: a data class with a multi-line constructor ends at its `)`. The blank-line stop is new for every brace language; before, a one-line declaration followed by a blank line and a class within two lines took that class's range. A brace on its own line (Allman style) still opens the body. Fingerprints do not change, because a symbol fingerprint hashes the decorator stack and signature line only.

**Perl.** `sub name` is a function and `package A::B` a package. A statement-form `package A::B;` runs to the next `package` or the end of the file; a `sub` or a `package A::B { ... }` matches braces.

**SQL.** A statement ends at the first `;` outside a dollar quote (`$$ ... $$`, `$tag$ ... $tag$`), a quoted string and a `--` comment, so a PostgreSQL function or procedure ends at the line that closes its body (`$$ LANGUAGE plpgsql;`). Bodies of other dialects (T-SQL `BEGIN ... END` with `GO`, MySQL `DELIMITER`) are not handled; their range still ends at the first `;`.

**Comment styles by file name or glob.** A `comment-styles:` key is an extension (`.pkl`), an exact file name (`crontab`), or a glob over the repository-relative path (`**/cron.d/*`; a key with `/`, `*`, `?` or `[`), matched like `exclude:`. The most specific wins: a file name, then the longest matching glob, then an extension; all win over the builtin table. File names already worked; the order changed from extension-before-file-name, which only mattered for a file named like an extension key.

**Prisma.** `.prisma` takes `//` line comments (Prisma has no block comments). `model` and `view` are declarations of kind `model`, which suggest treats as a table (it proposes `represents`, and reads schemas before code, like SQL); `enum` and `type` are types; `datasource` and `generator` are blocks, anchored but never proposed. suggest never proposed in a path a manifest declares `generated:`, because the workspace walk drops those files; the migrations Prisma writes from the schema belong there, and a test now says so.

**Heredocs (K24).** Documented, not fixed. Telling a heredoc body from the script around it needs a shell parser, and a line scanner that guessed would drop real markers. GRAMMAR.md and MARKING.md say to assemble such a line (`printf '# @%s ...' gloss`) or move the embedded code to its own file.

## Consequences

- A marker that anchored to a block on one of these declarations now anchors to the symbol: its artifact gets a symbol, and its range is the declaration's. A marker id without `id=` derives from the symbol and scope, so it changes once: the next `gloss build` records the marker under its new id, accepted or proposed as its `?` says, and the old entry's reviewer does not carry over. Earlier detector fixes (#131) did the same. A marker that already anchored to a symbol keeps its id and fingerprint; only its range grows.
- `gloss impact` and the viewer show whole Kotlin classes, Perl subs and SQL functions.
- Prisma schemas are proposed on by `gloss suggest` without a `comment-styles:` entry.
