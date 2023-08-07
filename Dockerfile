FROM quay.io/konveyor/windup-shim:latest as shim

FROM registry.access.redhat.com/ubi9-minimal as rulesets

RUN microdnf -y install git
RUN git clone https://github.com/konveyor/rulesets

# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY cmd/ cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o kantra main.go

FROM quay.io/konveyor/analyzer-lsp:latest

RUN mkdir /opt/rulesets
COPY --from=builder /workspace/kantra /usr/local/bin/kantra
COPY --from=shim /usr/bin/windup-shim /usr/local/bin
COPY --from=rulesets /rulesets/default/generated /opt/rulesets

ENTRYPOINT ["kantra"]
