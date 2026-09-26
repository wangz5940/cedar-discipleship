# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

## 5. Required Compatibility Skill

For any change to existing code paths, routing, defaults, shared helpers, data contracts, migrations, frontend content/resource resolution, completion semantics, or cross-group compatibility, read and follow:

```text
.agents/skills/preserve-existing-behavior/SKILL.md
```

This skill is mandatory unless the task is provably isolated from existing behavior. Existing behavior is the default contract; change it only when the user explicitly requests that behavior change.

## 6. Upstream Before Changes

Before changing this project, check the latest `master` commit of `wangz5940/cedar-discipleship` and compare its relevant code with the current branch. Bring over upstream functionality while preserving the optimized frontend UI and existing local features. Do not treat a local upstream file copy as proof that it is current.

## 7. Exclude Local Environment Files Before Every Commit

Before committing or opening a PR, inspect staged file paths with `git diff --cached --name-only`. Never include environment backups (`.env*.bak*`, `*.env.bak*`) or local development configuration (`.env.development`, `.env.*.local`), even when Git allows them to be force-added. Remove any such paths from the index before committing. Keep `.env.example` as the committed template.
## 7. Changelog Required

Every non-merge commit must update `CHANGELOG.md`.

- Add one concise reader-facing entry at the beginning of the changelog content, ordered newest first.
- Write changelog entries in Chinese and include the date, related commit ID when already known, and the user-visible impact.
- For a product change commit whose hash is not yet known, temporarily write `待回填`; immediately follow it with a changelog-only commit that replaces the marker with the product change commit ID.
- A changelog-only hash backfill commit updates the existing entry and does not add another entry for itself.
- Before pushing, `CHANGELOG.md` must not contain `本次提交` or `待回填`.
- Use `新增`, `变更`, `修复`, `安全`, or `运维` as appropriate.
- Describe the behavior, compatibility, data, configuration, or deployment impact.
- Do not use the changelog as a raw commit log; state why the change matters.
- Include `CHANGELOG.md` in the same commit as the code, configuration, test, or documentation change.

## 8. Chinese Git History

- Write every new commit subject and description in Chinese.
- Keep the subject concise and explain behavior, compatibility, or operational impact in the description.
- Do not rewrite already-published history only to translate older commit messages.

## 9. Repository Publish Guard

Before every commit or push, read and follow:

```text
.trae/skills/repository-publish-guard/SKILL.md
```

- Remove local-only, private, generated, transient, and task-unrelated files from the staged list.
- Local deployment and verification scripts or Skills must not be committed unless they are generalized and required by other repository users.
- Review every ignored file that is intentionally tracked or force-added.
- Unstage local files without deleting the user's local copy.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.
