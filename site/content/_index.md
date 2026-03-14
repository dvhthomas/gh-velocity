---
title: "GitHub Velocity"
type: docs
---

<div style="text-align: center; margin: 3rem 0 1rem;">
{{< asset-img src="images/logo-lockup.svg" alt="velocity" style="max-width: 500px; width: 100%; image-rendering: pixelated;" >}}
</div>

<h1 style="text-align: center; margin-bottom: 0.5rem; font-size: 2rem;">Know your team's delivery pace.</h1>

<p style="text-align: center; color: #666; font-size: 1.15rem; margin-bottom: 2.5rem;">
  Sprint velocity. Lead time. Cycle time. Defect rate.<br/>
  Computed from your GitHub data. Posted where the work happens.
</p>

<p style="text-align: center; margin-bottom: 3rem;">
  <a href="{{< relref "getting-started" >}}" style="display: inline-block; background: #22C55E; color: white; padding: 0.75rem 2rem; border-radius: 6px; text-decoration: none; font-weight: bold; font-size: 1.1rem;">Get Started</a>
</p>

## In a hurry?

```bash
gh extension install dvhthomas/gh-velocity
gh velocity config preflight --write
gh velocity report
```

Run `gh velocity config preflight --help` to see all options — the repo is auto-detected from your git remote.

---

Maintained by [BitsByD](https://bitsby.me/about) · [Source on GitHub](https://github.com/dvhthomas/gh-velocity) · [Meet Shelly]({{< relref "shelly" >}})
