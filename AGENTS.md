# AGENTS.md

本文件是仓库内所有 Agent 的公共开发与发布规则。Codex 直接读取本文件；Claude Code 通过根目录 `CLAUDE.md` 引入；Trae 通过 `.trae/rules/project_rules.md` 加载。维护规则时以本文件为准。

## 每次修改前必读

每次修改、优化或修复前，必须先阅读根目录 `CHANGE-CONSTRAINTS.md`；涉及前端时还必须阅读 `ui.md`。先说明本次允许改变的范围和需要保留的行为，再修改代码。后续任务和恢复任务同样适用，不得凭旧记忆跳过阅读。用户明确提出的新要求优先，并同步更新受影响的约束。

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

### 已有 PR 冲突须人工确认

- 本次改动若与已有 PR 的行为、实现或明确约定冲突，必须先明确提出相关 PR 编号或链接、冲突内容、受影响入口和预期行为变化，等待用户人工确认处理方向后才能继续处理该冲突。
- 此要求同时适用于已合入和未合入的 PR，以及产品行为冲突和代码合并冲突；不得自行覆盖、撤销或重新解释已有 PR 的改动，也不得仅凭笼统的开发、合入或部署授权视为已确认冲突。
- 用户已针对同一冲突明确确认处理方向时，按该确认执行，无需重复询问；确认前可继续不依赖冲突处理的只读排查和独立工作。

### 兼容验证证据

- 修改前列出受影响入口、旧行为和本次明确允许改变的行为；共享辅助函数的调用方也必须纳入。
- 涉及阅读、导航、下载、打卡或共享请求逻辑时，直接调用实际业务入口验证结果，例如打开的 URL、页码、下载状态、原始错误或完成记录；仅验证辅助函数参数不足以证明兼容。
- 日志、诊断上下文采集和自动反馈必须隔离自身异常；其失败不得阻断成功操作、替换原始业务错误或改变完成状态。
- JavaScript 必须通过 `npm run lint` 的未定义变量检查，TypeScript/Vue 必须通过 `npm run typecheck`；不得把未声明的业务变量加入 globals 来绕过检查。
- PR 按入口记录验证方式、实际结果和未覆盖项。健康检查或测试总数不能代替场景证据；模拟浏览器测试不能宣称为真机验收，未实测浏览器和设备必须明确标注。

## 6. Upstream Before Changes

修改前端界面、交互或样式时，必须先阅读并遵守根目录 `ui.md`，复用同一功能的组件与语义样式。

Before changing this project, check the latest `master` commit of `wangz5940/cedar-discipleship` and compare its relevant code with the current branch. Bring over upstream functionality while preserving the optimized frontend UI and existing local features. Do not treat a local upstream file copy as proof that it is current.

## 7. Exclude Local Environment Files Before Every Commit

Before committing or opening a PR, inspect staged file paths with `git diff --cached --name-only`. Never include environment backups (`.env*.bak*`, `*.env.bak*`) or local development configuration (`.env.development`, `.env.*.local`), even when Git allows them to be force-added. Remove any such paths from the index before committing. Keep `.env.example` as the committed template.
## 7. Changelog Required

Every PR must update `CHANGELOG.md`.

- Add one concise reader-facing entry at the beginning of the changelog content, ordered newest first.
- Write changelog entries in Chinese and include the date, related PR number, and user-visible impact.
- Use `关联 PR：#<PR 编号>` for the change reference. Commit IDs are optional and are not required.
- Do not use placeholders such as `本次提交` or `待回填`.
- Use `新增`, `变更`, `修复`, `安全`, or `运维` as appropriate.
- Describe the behavior, compatibility, data, configuration, or deployment impact.
- Do not use the changelog as a raw commit log; state why the change matters.
- Include `CHANGELOG.md` in the same PR as the code, configuration, test, or documentation change.

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

## 10. 分支、PR、合入与部署

- 每个任务开始前先获取最新主分支，并从 `origin/master` 创建一个新的功能分支；一个分支只承载一个任务，已合入的分支不得继续开发或复用。

```bash
git fetch origin master
git switch -c <feature-branch> origin/master
```

使用独立工作树时执行：

```bash
git fetch origin master
git worktree add -b <feature-branch> <worktree-path> origin/master
```

- 所有变更经 Pull Request 合入 `master`；禁止直接推送 `master`、强推主干或绕过分支保护，包括仓库所有者和管理员。
- 变更必须先在隔离的本地环境实际部署，并通过受影响业务入口验证，才能合入 `master`；测试、构建和 CI 通过不能代替本地部署验收。PR 中记录候选版本、部署方式、入口验证结果与未覆盖项；验证失败时修复后重新部署和验证。用户明确要求暂不合入时，即使验证通过也保持 PR 未合入。
- 每次本地部署前，先向用户说明本次变更涉及的页面、内容与功能，并列出需要重点验证的要点。部署完成后必须输出简明验收摘要：本地地址与候选版本、本次变更、界面／内容显示检查、功能正常性检查、实际验证结果及未覆盖／待用户确认项。验证要点写明操作入口、操作步骤和预期结果；显示检查关注标题、正文、布局与状态，功能检查关注阅读／导航、保存／切换及完成记录等本次受影响操作。只列与本次变更相关的项目，不用测试数量、构建成功或健康检查代替业务验收，不把未验证项写成通过。
- 合入前必须同步最新 `master`，并通过仓库配置的全部必需 CI 检查；不得为了合入而关闭检查或降低保护要求。
- 默认使用 **Squash and merge**。除非用户明确要求保留分支提交历史，不使用 merge commit 或 rebase merge。
- PR 标题和正文同时作为最终 Squash 提交信息：标题使用中文准确概括用户可感知的改动；正文说明主要改动、兼容性或数据/配置/部署影响，以及验证结果。禁止使用“合并分支”“修复问题”等无法说明实际变化的笼统提交信息。
- 仓库设置应启用 Squash merging，并将默认提交信息设为 PR 标题和描述；启用 Automatically delete head branches。执行合入时优先使用：

```bash
gh pr merge --squash --delete-branch
```

- 合入后先确认 PR 状态为 merged 且 `origin/master` 已更新，再删除本地功能分支或工作树并执行 `git fetch --prune`。Squash 后原分支提交不会成为 `master` 的祖先；只有在确认 PR 已合入、工作区干净且没有未推送提交后，才可强制删除本地分支。
- 后续任务必须重新从当时最新的 `origin/master` 创建新分支，不得在已合入分支上追加提交。
- **线上部署版本必须先通过本地部署验收并合入 `master`。** 发布前重新获取远端状态，并确认目标提交已包含在 `origin/master` 历史中。此要求适用于 NAS 及其他线上部署环境、完整发布和单个服务发布。未合入功能分支允许部署到隔离的本地环境进行验证；本地验证不能连接或写入生产数据库。

```bash
git fetch origin master
deploy_commit="$(git rev-parse --verify "${DEPLOY_REF:-origin/master}^{commit}")"
git merge-base --is-ancestor "$deploy_commit" origin/master
```

- 上述命令全部成功后，才可从目标提交的干净检出构建并部署；记录该提交及对应制品或镜像，禁止混入未提交代码、未合入分支或服务器工作区中的额外修改。
- 其他贡献者的未合入修复由原作者提交。未经用户明确授权，不得代为提交、挑选或整合进自己的分支。若部署会覆盖这些修复，暂停受影响的发布并说明差异，等待原作者完成合入。
- 回滚代码同样通过功能分支、PR 和必需 CI 合入后发布，不得绕过上述流程。

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.
