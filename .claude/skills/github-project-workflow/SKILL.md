---
name: github-project-workflow
description: Enforces the GitHub project board workflow. Manages issue lifecycle (Backlog > Ready > In Progress > In Review > Done), ensures draft PRs, assignments, and status transitions happen at the right time. Posts concise issue comments at milestones.
disable-model-invocation: true
allowed-tools:
  - Bash
  - Read
  - Write
  - Edit
  - Grep
  - Glob
  - Agent
  - "gh:*"
---

# GitHub Project Board Workflow

This skill enforces the GitHub project board workflow. Every status transition (Backlog, Ready, In Progress, In Review, Done) generates GitHub events that feed into project metrics. **Skipping a step corrupts the data.**

## When to Use

Use this workflow for ANY work that touches the codebase. Common triggers:

- **"fix this bug: #32"** or **"fix https://github.com/.../issues/32"** — existing issue, start from Phase 1 (verify issue)
- **"create an issue to..."** or **"track this bug..."** — create a new issue on the board (Phase 1), stop there unless told to continue
- **"build feature X"** or **"implement..."** — create or find an issue, then proceed through the full workflow
- **"create an issue to track @docs/brainstorms/..."** — create issue, link the referenced doc, keep in Backlog (Phase 1 + 2)
- **Creating plans** — move issue to Ready, link plan doc

When the user provides an issue URL or `#number`, always start by viewing it — don't create a duplicate.

**This workflow is always active.** When other skills run (`/ce:plan`, `/ce:work`, `/ce:review`, etc.), the board transitions described here still apply. Those skills handle *what* to build; this skill handles *where the work lives* on the board.

## Setup: Resolve Repo and Board at Runtime

All commands use `gh` to discover the repo and board. **Never hardcode repo names or internal IDs.**

### Discover the current repo

```bash
# Returns "owner/repo" from the git remote
REPO=$(gh repo view --json nameWithOwner --jq '.nameWithOwner')
```

Use `$REPO` with `--repo "$REPO"` on all `gh issue` and `gh pr` commands.

### Discover the project board

The project board URL is in AGENTS.md (look for a `[project board]` link). The URL format is `https://github.com/users/{BOARD_OWNER}/projects/{BOARD_NUMBER}`. Read the URL and extract the owner and number directly — no shell parsing needed.

For example, `https://github.com/users/dvhthomas/projects/1` gives `BOARD_OWNER=dvhthomas` and `BOARD_NUMBER=1`.

### Resolve board IDs from human-readable names

The `gh project item-edit` command requires internal IDs. Resolve them from the board URL and status names:

```bash
# Project ID
PROJECT_ID=$(gh project list --owner "$BOARD_OWNER" --format json \
  --jq ".projects[] | select(.number == $BOARD_NUMBER) | .id")

# Status field ID
FIELD_ID=$(gh project field-list "$BOARD_NUMBER" --owner "$BOARD_OWNER" --format json \
  --jq '.fields[] | select(.name == "Status") | .id')

# Status option ID — replace STATUS_NAME with: Backlog, Ready, In progress, In review, Done
OPTION_ID=$(gh project field-list "$BOARD_NUMBER" --owner "$BOARD_OWNER" --format json \
  --jq '.fields[] | select(.name == "Status") | .options[] | select(.name == "STATUS_NAME") | .id')

# Project item ID for an issue
ITEM_ID=$(gh project item-list "$BOARD_NUMBER" --owner "$BOARD_OWNER" --format json \
  --jq '.items[] | select(.content.number == ISSUE_NUMBER) | .id')
```

### Move an issue to a status

```bash
gh project item-edit \
  --project-id "$PROJECT_ID" \
  --id "$ITEM_ID" \
  --field-id "$FIELD_ID" \
  --single-select-option-id "$OPTION_ID"
```

### Sub-issues

For large features with multiple phases, create a parent issue and link child issues as sub-issues using the REST API:

```bash
# Get the child issue's internal ID
CHILD_ID=$(gh api repos/{owner}/{repo}/issues/{CHILD_NUMBER} --jq '.id')

# Add it as a sub-issue of the parent
gh api repos/{owner}/{repo}/issues/{PARENT_NUMBER}/sub_issues \
  -X POST \
  -F sub_issue_id="$CHILD_ID"
```

Note: `-F` (not `-f`) is required so the ID is sent as an integer.

To verify sub-issues are linked:

```bash
gh api repos/{owner}/{repo}/issues/{PARENT_NUMBER}/sub_issues \
  --jq '.[] | "#\(.number): \(.title)"'
```

Each sub-issue should also be added to the project board individually so it has its own status column tracking.

## Board Statuses

| Status | When |
|--------|------|
| Backlog | Issue created, brainstorm linked |
| Ready | Plan complete, ready for implementation |
| In progress | Draft PR created, code being written |
| In review | PR marked ready for review |
| Done | PR merged, issue closed |

## Issue Naming Conventions

Issue titles use a `type: description` format. The type is a lowercase conventional-commit prefix; the description is a concise, specific summary of the *problem or outcome* — not implementation details.

**Format**: `<type>: <imperative description>`

| Type | When |
|------|------|
| `feat` | New user-facing capability |
| `fix` | Bug fix |
| `refactor` | Internal restructuring, no behavior change |
| `docs` | Documentation only |
| `ci` | CI/CD pipeline changes |
| `test` | Test-only changes |
| `chore` | Maintenance (deps, config, tooling) |

**Rules**:
- Start the description with an imperative verb: "add", "fix", "remove", not "added" or "fixes"
- Be specific about what's wrong or what's being built, not how
- Keep titles under 72 characters when possible
- No trailing period

**Good**:
- `fix: repo default is misleading without better user output`
- `feat: add cycle-time histogram to weekly report`
- `refactor: extract scope assembly into internal/scope`

**Bad**:
- `fix bug` (too vague)
- `Fix: Updated the resolveRepo function to emit a warning` (past tense, implementation detail, wrong case)
- `feat: new feature for the thing` (says nothing)

PR titles follow the same convention. When creating a PR for issue #N, the PR title should match or closely paraphrase the issue title.

## The Workflow

### Phase 1: Issue Exists

Every piece of work needs a GitHub issue. No exceptions.

**If an issue already exists** (e.g., user says "fix #32"):
```bash
gh issue view <NUMBER> --repo "$REPO" --json number,title,state,assignees,projectItems
```

**If no issue exists** (e.g., user describes a feature or bug):
```bash
# Create the issue — follow naming conventions above
gh issue create --repo "$REPO" \
  --title "<type>: <description>" \
  --body "<brief description>"

# Add to project board (lands in Backlog by default)
gh project item-add "$BOARD_NUMBER" --owner "$BOARD_OWNER" --url <ISSUE_URL>
```

**Issue comment** (when creating):
> No comment needed on creation -- the issue body speaks for itself.

### Phase 2: Brainstorm (if applicable)

If a brainstorm doc was produced (by `/ce:brainstorm` or manually), link it to the issue.

```bash
gh issue comment <NUMBER> --repo "$REPO" \
  --body "Brainstorm: [docs/brainstorms/<filename>](<permalink-url>)"
```

The issue stays in **Backlog**. Do not assign yet.

**Important**: Never create, move, or delete files in `docs/`. Other skills manage that directory. Only *read* those files and *link* to them by URL.

### Phase 3: Plan Complete → Ready

When planning is done (by `/ce:plan` or manually), transition the issue:

```bash
# 1. Move to Ready (resolve IDs at runtime — see "Resolve board IDs" above)
#    Use STATUS_NAME = "Ready"

# 2. Assign to active user
CURRENT_USER=$(gh api user --jq '.login')
gh issue edit <NUMBER> --repo "$REPO" --add-assignee "$CURRENT_USER"

# 3. Link the plan
gh issue comment <NUMBER> --repo "$REPO" \
  --body "Plan: [docs/plans/<filename>](<permalink-url>)"
```

### Phase 4: Work Begins → In Progress

When code is being written (by `/ce:work` or manually), create a draft PR and transition.

**Use a worktree** (required by project hard rules in AGENTS.md):

```bash
# 1. Create worktree with feature branch from main
git worktree add ../<repo>-<feature> -b <branch-name> main

# 2. Work in the worktree directory
cd ../<repo>-<feature>
```

**Create an opening commit and draft PR.** GitHub requires at least one commit to create a PR:

```bash
# 3. Create an empty opening commit and push
git commit --allow-empty -m "<type>: begin <description>"
git push -u origin <branch-name>

# 4. Create draft PR
CURRENT_USER=$(gh api user --jq '.login')
gh pr create --draft \
  --title "<type>: <description>" \
  --assignee "$CURRENT_USER" \
  --body "Closes #<NUMBER>

## Summary
<brief description of the work>

## Test plan
- [ ] \`task test\`
- [ ] \`task quality\`
"

# 5. Move issue to In progress (resolve IDs — see above)
#    Use STATUS_NAME = "In progress"
```

**Issue comment**:
```bash
gh issue comment <NUMBER> --repo "$REPO" \
  --body "Draft PR: #<PR_NUMBER>"
```

### Phase 5: Work Complete → In Review

When the implementation is done, tests pass, and review has been run:

```bash
# 1. Mark PR ready for review
gh pr ready <PR_NUMBER> --repo "$REPO"

# 2. Move issue to In review (resolve IDs — see above)
#    Use STATUS_NAME = "In review"
```

No issue comment needed here -- the PR status change is visible.

### Phase 6: Merged → Done

After the PR is merged (by human or when explicitly instructed):

```bash
# 1. Merge (only when explicitly told to)
gh pr merge <PR_NUMBER> --repo "$REPO" --merge --delete-branch

# 2. Move issue to Done (resolve IDs — see above)
#    Use STATUS_NAME = "Done"

# 3. Close issue if not auto-closed
gh issue close <NUMBER> --repo "$REPO" --reason completed
```

## Issue Comment Guidelines

Comments should be **useful signal, not noise**. Each comment should give someone scanning the issue a clear picture of progress.

**Do comment when**:
- Linking a brainstorm or plan doc (with URL)
- Creating a draft PR (with PR number)
- Significant scope change discovered during implementation
- Blocked on something external

**Do not comment when**:
- Moving the issue between board columns (the board shows this)
- Making routine commits
- Running tests (unless they reveal something unexpected)
- Marking PR ready for review (the PR status change is visible)

**Tone**: Write like a brief commit message or changelog entry. State what happened and why, in one or two sentences. No filler, no emoji, no "I'm working on...".

Good: `Plan: [docs/plans/2026-03-11-001-fix-repo-default-warning-plan.md](url)`
Good: `Scope expanded: discover command also affected, added to fix.`
Bad: `I've started working on this issue and will keep you updated!`

## Integration with Other Skills

This workflow wraps around other compound-engineering skills:

| Skill runs | This workflow does |
|------------|-------------------|
| `/ce:brainstorm` produces a doc | Link doc to issue, keep in Backlog |
| `/ce:plan` produces a plan | Link plan to issue, move to Ready, assign |
| `/ce:work` starts coding | Create draft PR, move to In Progress |
| `/ce:review` finishes | Mark PR ready, move to In Review |
| Human merges PR | Move to Done, close issue |

**The other skills do not handle board transitions.** This skill does. If you run `/ce:work` and it doesn't create a draft PR or move the issue, you must do it.

## Docs Directory Policy

The `docs/` directory is managed by compound-engineering pipeline skills (`/ce:brainstorm`, `/ce:plan`, `/ce:compound`). This workflow skill:

- **MAY read** files in `docs/` to get titles and paths
- **MAY link** to files in `docs/` via GitHub permalink URLs
- **MUST NOT create, edit, move, or delete** files in `docs/`

To generate a permalink URL for a doc on the current branch:

```bash
BRANCH=$(gh pr view --json headRefName --jq '.headRefName' 2>/dev/null || git branch --show-current)
# https://github.com/${REPO}/blob/${BRANCH}/docs/plans/filename.md
```

## Checklist

Before considering any piece of work "done", verify:

- [ ] Issue exists and is assigned
- [ ] Issue is on the project board
- [ ] Plan doc linked to issue (if planning was done)
- [ ] Draft PR was created before coding started
- [ ] PR has `Closes #N` in the body
- [ ] PR is assigned to the active user
- [ ] `task quality` passes
- [ ] PR marked ready for review
- [ ] Issue is in "In Review" status
