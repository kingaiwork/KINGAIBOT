# KINGAIBOT Zero-Bill VPS Execution

KINGAIBOT automatic CI, security scanning, container validation, release building and cross-platform compilation must run on the owned KING AI VPS. The repository must not automatically consume GitHub-hosted runner minutes, Actions artifact storage, GitHub CodeQL/GHAS, paid AI APIs, paid cloud storage or automatic release-upload services.

## VPS runner labels

Register the GitHub self-hosted runner with:

```text
self-hosted
linux
x64
kingai-vps
```

## Required local toolchain

```bash
go version        # 1.26.x
node --version
docker --version
sudo mkdir -p /srv/kingai/{builds/KINGAIBOT,qa/KINGAIBOT,releases/KINGAIBOT}
sudo chown -R "$USER":"$USER" /srv/kingai
```

`govulncheck` may be installed locally on the VPS. No paid model/provider key is required for deterministic CI.

## Storage paths

- CI/cross-platform build evidence: `/srv/kingai/builds/KINGAIBOT/<sha>`
- full validation evidence: `/srv/kingai/qa/KINGAIBOT/<sha>`
- release output: `/srv/kingai/releases/KINGAIBOT/<tag>`

macOS/Windows hosted runners are not automatic dependencies. Automatic validation cross-compiles Darwin/Windows targets on Linux. If real native runtime testing is needed later, use owned/local Mac or Windows hardware; do not silently route to paid cloud runners.

## AI execution

KINGAIBOT must prefer deterministic code and local/VPS models. Common paid API keys (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `DEEPSEEK_API_KEY`, `GEMINI_API_KEY`) are forbidden from autonomous production and CI environments. If a local/free model is unavailable, queue/stop rather than purchasing credits.

## OpenClaw/Codex execution instruction

```text
Operate KINGAIBOT under .kingai/zero-bill.json and the central KINGAIASE Zero-Bill policy. Run all CI, Go tests, vulnerability checks, Docker builds, release builds and cross-compilation on the owned VPS. Never enable GitHub-hosted runners, CodeQL/GHAS billing, Actions artifact storage, paid AI APIs, paid cloud storage or automatic public release upload. Keep outputs under /srv/kingai. Deterministic checks first; Codex may prepare a Draft PR but must not merge, publish or deploy automatically.
```

## Verification

After the runner is active, trigger CI and confirm:

```bash
find /srv/kingai/builds/KINGAIBOT -maxdepth 3 -type f | head
find /srv/kingai/qa/KINGAIBOT -maxdepth 4 -type f | head
```

Source changes do not prove runner activation. Record runner-online status and workflow output from the real VPS before merging.
