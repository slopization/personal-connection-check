package contracts

import (
	"os"
	"strings"
	"testing"
)

const (
	nodeBase = "node:26.0.0-alpine3.22@sha256:60ff302eeca050b69ab0740f91c66207dc70f5fed1fb3c778c844d769a311443"
	goBase   = "golang:1.27.1-alpine3.23@sha256:c8500dc1e6c8d8db60a2c6986bc591517c3be360ca448b9df43449dec430cc34"
	distBase = "gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func requireContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("missing %q", want)
	}
}

func TestDockerContract(t *testing.T) {
	s := read(t, "../../Dockerfile")
	for _, want := range []string{
		"FROM " + nodeBase + " AS web",
		"FROM " + goBase + " AS build",
		"FROM " + distBase,
		"RUN rm -rf /src/internal/webui/dist && mkdir -p /src/internal/webui",
		"COPY --from=web /src/web/dist /src/internal/webui/dist",
		"USER nonroot:nonroot",
		"HEALTHCHECK",
	} {
		requireContains(t, s, want)
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "FROM ") && !strings.Contains(line, "@sha256:") {
			t.Fatalf("unpinned Docker base: %q", line)
		}
	}
	if strings.Contains(s, "golang:1.27.1-alpine3.22") {
		t.Fatal("Dockerfile uses nonexistent golang:1.27.1-alpine3.22")
	}
}

func TestIgnoreContract(t *testing.T) {
	for _, path := range []string{"../../.gitignore", "../../.dockerignore"} {
		requireContains(t, read(t, path), "internal/webui/dist/")
	}
}

func TestMakeReleaseGateContract(t *testing.T) {
	s := read(t, "../../Makefile")
	for _, want := range []string{
		"GO?=go",
		"rm -rf internal/webui/dist",
		"scripts/container-smoke.sh",
		"release-gate: check e2e container container-smoke",
	} {
		requireContains(t, s, want)
	}
	if strings.Contains(s, "rm -rf dist") {
		t.Fatal("Makefile must remove only the named embedded-output directory")
	}
}

func TestCIInvokesReleaseGateAfterBrowserInstall(t *testing.T) {
	s := read(t, "../../.forgejo/workflows/ci.yml")
	for _, want := range []string{"git checkout --detach \"$GITHUB_SHA\"", "GITHUB_SHA", "GITHUB_PATH", "/usr/local/go/bin", "make e2e-install", "make release-gate"} {
		requireContains(t, s, want)
	}
	if strings.Index(s, "make e2e-install") > strings.Index(s, "make release-gate") {
		t.Fatal("CI must install Playwright browsers before release-gate")
	}
}

func TestCICacheUsesPinnedForgejoActionAndVersionedPaths(t *testing.T) {
	s := read(t, "../../.forgejo/workflows/ci.yml")
	for _, want := range []string{
		"https://code.forgejo.org/actions/cache/restore@1bd1e32a3bdc45362d1e726936510720a7c30a57",
		"https://code.forgejo.org/actions/cache/save@1bd1e32a3bdc45362d1e726936510720a7c30a57",
		"~/.cache/pcc-toolchains",
		"~/.cache/ms-playwright",
		"~/.cache/go-build",
		"~/go/pkg/mod",
		"~/.npm",
		"go1.27.1-node26.0.0-playwright1.57.0",
		"hashFiles('go.sum', 'web/package-lock.json')",
		"go mod download",
		"if: always()",
	} {
		requireContains(t, s, want)
	}
	if strings.Index(s, "actions/cache/restore@") > strings.Index(s, "make e2e-install") {
		t.Fatal("CI cache must be restored before dependency and browser installation")
	}
	if strings.Index(s, "actions/cache/save@") < strings.Index(s, "make e2e-install") || strings.Index(s, "actions/cache/save@") > strings.Index(s, "make release-gate") {
		t.Fatal("CI cache must be saved after installation and before the fallible release gate")
	}
}

func TestPlaywrightSerializesProjectsAgainstGlobalLimits(t *testing.T) {
	s := read(t, "../../web/playwright.config.ts")
	requireContains(t, s, "workers: 1")
}

func TestContainerWorkflowContract(t *testing.T) {
	s := read(t, "../../.forgejo/workflows/container.yml")
	for _, want := range []string{
		"git checkout --detach \"$GITHUB_SHA\"",
		"set -euo pipefail",
		"docker build",
		"docker pull \"$IMAGE:latest\" || true",
		"--cache-from \"$IMAGE:latest\"",
		"BUILDKIT_INLINE_CACHE=1",
		"make container-smoke",
		"docker push \"$IMAGE:sha-${GITHUB_SHA:0:12}\"",
		"docker pull \"$IMAGE:sha-${GITHUB_SHA:0:12}\"",
		"RepoDigests",
		"sha256:",
		"test -n \"$digest\"",
		"image-digest.txt",
		"test -s image-digest.txt",
		"GITHUB_OUTPUT",
		"image_digest:",
		"echo \"$PACKAGE_TOKEN\" | docker login git.kyu.sh -u kyush-ci --password-stdin",
		"PACKAGE_TOKEN: ${{ secrets.PACKAGE_TOKEN }}",
		"refs/heads/main",
		"^refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+$",
	} {
		requireContains(t, s, want)
	}
	if strings.Contains(s, "REGISTRY_TOKEN:") || strings.Contains(s, "FORGEJO_TOKEN") || strings.Contains(s, "actions/") {
		t.Fatal("workflow must use only the dedicated PACKAGE_TOKEN and avoid job-token package publishing or third-party actions")
	}
	if strings.Index(s, "docker push \"$IMAGE:sha-${GITHUB_SHA:0:12}\"") > strings.Index(s, "docker pull \"$IMAGE:sha-${GITHUB_SHA:0:12}\"") {
		t.Fatal("workflow must push the immutable SHA tag before pulling it")
	}
	publishStart := strings.Index(s, "name: publish immutable and release tags")
	if publishStart < 0 {
		t.Fatal("missing publish step")
	}
	prefix := s[:publishStart]
	if strings.Contains(prefix, "PACKAGE_TOKEN") || strings.Contains(prefix, "secrets.") {
		t.Fatal("PACKAGE_TOKEN must not be referenced before the publish step")
	}
}

func TestContainerSmokeExercisesDeliveredWebAssets(t *testing.T) {
	s := read(t, "../../scripts/container-smoke.sh")
	for _, want := range []string{
		"--read-only",
		"/healthz",
		"/assets/",
		"docker inspect",
	} {
		requireContains(t, s, want)
	}
}
