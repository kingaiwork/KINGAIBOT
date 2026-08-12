# Publication Validation / 发布验证

## English

This source publication is gated by GitHub-hosted CI before merge to `main`. The validation includes Go 1.26.5 formatting/vet/tests, race detection, vulnerability scanning, native macOS and Windows builds/tests, Docker build validation, and CodeQL analysis.

The source branch was additionally normalized with `gofmt` and passed `go test -count=1 ./...` plus `go vet ./...` on GitHub-hosted Go 1.26.5 before the full publication gate was re-triggered.

---

## 中文

本次源码发布在合并到 `main` 之前必须经过 GitHub 托管 CI 闸门，包括 Go 1.26.5 格式化/vet/测试、race detector、漏洞扫描、macOS 与 Windows 原生构建测试、Docker 构建验证以及 CodeQL 分析。

发布分支还先在 GitHub 托管的 Go 1.26.5 环境中完成 `gofmt`，并通过 `go test -count=1 ./...` 与 `go vet ./...`，之后才重新触发完整发布验证。
