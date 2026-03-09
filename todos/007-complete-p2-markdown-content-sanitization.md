---
status: complete
priority: p2
issue_id: 007
tags: [code-review, security, markdown, posting]
dependencies: []
---

# Sanitize Third-Party Content in Markdown Output

## Problem Statement

The `--post` flag writes markdown containing issue titles and commit messages to GitHub. These are attacker-controlled strings that could contain markdown injection (breaking table layout, injecting misleading content) or XSS vectors (though GitHub's renderer typically catches those).

**Raised by:** Security Sentinel (HIGH)

## Findings

- Issue titles like `| DROP TABLE |` would break markdown table formatting
- Commit messages could contain markdown links to phishing sites
- GitHub's sanitizer handles most XSS but the tool should not rely solely on it
- Current `sanitizeMarkdown` in `internal/format/markdown.go` only escapes `|` — incomplete

## Proposed Solutions

### Option A: Use existing Go markdown sanitization library (Recommended)
- Use an established Go library (e.g., `bluemonday` for HTML sanitization, or a markdown-aware escaping library) instead of rolling our own
- Truncate titles to 200 chars
- Never embed raw HTML in posted markdown
- **Effort:** Small (add dependency + wire it up)
- **Risk:** Low — battle-tested library vs hand-rolled escaping

### Option B: Hand-rolled escaping (Not Recommended)
- Write custom `escapeMarkdown(s string) string` function
- **Cons:** Easy to miss edge cases, maintenance burden
- **Effort:** Small
- **Risk:** Medium — incomplete coverage likely

## Acceptance Criteria

- [ ] Use an existing Go markdown/HTML sanitization library (not hand-rolled)
- [ ] All third-party text (titles, messages) escaped before markdown insertion
- [ ] Table output not breakable by adversarial issue titles
- [ ] No raw HTML in posted content
