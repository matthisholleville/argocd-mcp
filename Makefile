BINARY  := argocd-mcp
CMD     := ./cmd/argocd-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build run test e2e docker-build clean

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/$(BINARY) $(CMD)

run: build
	ARGOCD_BASE_URL=$(ARGOCD_BASE_URL) \
	ARGOCD_TOKEN=$(ARGOCD_TOKEN) \
	bin/$(BINARY)

test:
	go test -race -count=1 ./...

e2e:
	go test -tags e2e -count=1 -v -timeout 15m ./e2e/...

docker-build:
	docker build -t $(BINARY):$(VERSION) .

clean:
	rm -rf bin
