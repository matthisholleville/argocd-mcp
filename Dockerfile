# ---- builder ----
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /bin/argocd-mcp \
    ./cmd/argocd-mcp

# ---- runtime ----
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /bin/argocd-mcp /argocd-mcp

EXPOSE 8080

ENTRYPOINT ["/argocd-mcp"]
