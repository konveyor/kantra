FROM quay.io/konveyor/windup-shim:latest as shim

FROM registry.access.redhat.com/ubi9-minimal as rulesets

RUN microdnf -y install git &&\
    git clone https://github.com/konveyor/rulesets &&\
    git clone https://github.com/windup/windup-rulesets -b 6.3.1.Final

FROM quay.io/konveyor/static-report:latest as static-report

# Build the manager binary
FROM golang:1.19 as builder

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
ARG VERSION
ARG BUILD_COMMIT
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT'" -a -o kantra main.go
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT'" -a -o darwin-kantra main.go
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT'" -a -o windows-kantra main.go

FROM quay.io/konveyor/analyzer-lsp:latest

RUN mkdir /opt/rulesets /opt/rulesets/input /opt/rulesets/convert /opt/openrewrite /opt/input /opt/input/rules /opt/input/rules/custom /opt/output /opt/xmlrules /opt/shimoutput

COPY --from=builder /workspace/kantra /usr/local/bin/kantra
COPY --from=builder /workspace/darwin-kantra /usr/local/bin/darwin-kantra
COPY --from=builder /workspace/windows-kantra /usr/local/bin/windows-kantra
COPY --from=shim /usr/bin/windup-shim /usr/local/bin
COPY --from=rulesets /rulesets/default/generated /opt/rulesets
COPY --from=rulesets /windup-rulesets/rules/rules-reviewed/openrewrite /opt/openrewrite
COPY --from=static-report /usr/bin/js-bundle-generator /usr/local/bin
COPY --from=static-report /usr/local/static-report /usr/local/static-report
COPY entrypoint.sh /usr/bin/entrypoint.sh

ENTRYPOINT ["kantra"]
