GO?=go
IMAGE?=personal-connection-check:local
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
	docker build -t $(IMAGE) .
container-smoke:
	IMAGE=$(IMAGE) ./scripts/container-smoke.sh
e2e:
	cd web && ./node_modules/.bin/playwright test
e2e-install:
	cd web && ./node_modules/.bin/playwright install --with-deps chromium firefox
check: fmt-check lint typecheck test-go test-race test-web build
release-gate: check e2e container container-smoke
