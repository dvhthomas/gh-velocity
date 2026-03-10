# Example Configurations

Ready-to-use configs for popular open source projects. Each demonstrates different gh-velocity features.

## Usage

```bash
gh velocity release <tag> --since <prev-tag> -R <owner/repo> --config docs/examples/<file>.yml -f json
```

## Feature Matrix

| Config | Repo | bug_labels | feature_labels | active_labels | backlog_labels | commit_ref |
|--------|------|:---:|:---:|:---:|:---:|:---:|
| cli-cli.yml | cli/cli | x | x | x | x | x |
| kubernetes-kubernetes.yml | kubernetes/kubernetes | x | x | x | x | |
| hashicorp-terraform.yml | hashicorp/terraform | x | x | | x | |
| astral-sh-uv.yml | astral-sh/uv | x | x | | | |
| facebook-react.yml | facebook/react | x | x | x | x | |

## E2E Testing

These configs are validated by the E2E test suite:

```bash
task e2e:configs
```

This builds the binary and runs each config against its target repo, verifying that valid JSON output with metrics is returned.

## Adding a New Example

1. Create `<owner>-<repo>.yml` in this directory
2. Add an entry to `scripts/e2e-configs.sh` with `config|repo|tag|since_tag`
3. Run `task e2e:configs` to verify
