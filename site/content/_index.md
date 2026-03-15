---
title: "GitHub Velocity"
type: docs
---

<div style="text-align: center; margin: 4rem 0 2rem;">
{{< asset-img src="images/logo-lockup.svg" alt="velocity" style="max-width: 600px; width: 100%; image-rendering: pixelated;" >}}
</div>

<h1 class="no-shelly" style="text-align: center; font-size: 2.4rem; margin-bottom: 1rem; line-height: 1.2;">
  Measure what matters.
</h1>

<p style="text-align: center; color: #666; font-size: 1.2rem; max-width: 560px; margin: 0 auto 2.5rem;">
  gh-velocity computes sprint velocity, lead time, cycle time, and quality metrics from your GitHub issues, PRs, and releases — then posts results right where your team works.
</p>

<p style="text-align: center; margin-bottom: 4rem;">
  <a href="{{< relref "getting-started" >}}" style="display: inline-block; background: #22C55E; color: white; padding: 0.85rem 2.5rem; border-radius: 6px; text-decoration: none; font-weight: bold; font-size: 1.2rem;">Get Started</a>
</p>

## In a hurry?

```bash
gh extension install dvhthomas/gh-velocity
cd your-repo
gh velocity config preflight --write
gh velocity report --since 30d
```

Run these from inside your repo — gh-velocity auto-detects the repo from your git remote. See [Quick Start]({{< relref "/getting-started/quick-start" >}}) for a full walkthrough.

---

Maintained by [BitsByD](https://bitsby.me/about) · [Source on GitHub](https://github.com/dvhthomas/gh-velocity) · [Meet Shelly]({{< relref "shelly" >}})
