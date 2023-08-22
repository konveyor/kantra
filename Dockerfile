FROM quay.io/konveyor/windup-shim:latest as shim

FROM registry.access.redhat.com/ubi9-minimal as rulesets

RUN microdnf -y install git &&\
    git clone https://github.com/konveyor/rulesets &&\
    git clone https://github.com/windup/windup-rulesets -b 6.2.3.Final

FROM quay.io/konveyor/static-report as static-report

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

RUN mkdir /opt/rulesets /opt/rulesets/input /opt/openrewrite /opt/input /opt/output /opt/xmlrules /opt/shimoutput

COPY --from=builder /workspace/kantra /usr/local/bin/kantra
COPY --from=shim /usr/bin/windup-shim /usr/local/bin
COPY --from=rulesets /rulesets/default/generated /opt/rulesets
COPY --from=rulesets /windup-rulesets/rules/rules-reviewed/openrewrite /opt/openrewrite
COPY --from=static-report /usr/bin/js-bundle-generator /usr/local/bin
COPY --from=static-report /usr/local/static-report /usr/local/static-report

ENTRYPOINT ["kantra"]
