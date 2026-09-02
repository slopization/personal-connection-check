GO=PATH="$(CURDIR)/.tools/go/bin:$$PATH" go
web-assets:
	cd web && npm run build
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist && cp -R web/dist/. internal/webui/dist/
fmt-check:
	@test -z "$$($(GO)fmt -l $$(find . -name '*.go' -not -path './.tools/*'))"
	cd web && npm run format
lint: web-assets
	$(GO) vet ./...
typecheck:
	cd web && npm run typecheck
test-go: web-assets
	$(GO) test ./...
test-race: web-assets
	$(GO) test -race ./...
test-web:
	cd web && npm test -- --run
build: web-assets
	$(GO) build ./cmd/pcc
container:
	docker build -t personal-connection-check:local .
container-smoke:
	@docker run --rm --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m personal-connection-check:local healthcheck
e2e:
	cd web && ./node_modules/.bin/playwright test
e2e-install:
	cd web && ./node_modules/.bin/playwright install --with-deps chromium firefox webkit
check: fmt-check lint typecheck test-go test-race test-web build
release-gate: check e2e container container-smoke
