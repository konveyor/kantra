ARG VERSION=latest
FROM quay.io/konveyor/windup-shim:${VERSION} as shim

FROM registry.access.redhat.com/ubi9-minimal as rulesets

ARG RULESETS_REF=main
RUN microdnf -y install git &&\
    git clone https://github.com/konveyor/rulesets -b ${RULESETS_REF} &&\
    git clone https://github.com/windup/windup-rulesets -b 6.3.1.Final

FROM quay.io/konveyor/static-report:${VERSION} as static-report

# Build the manager binary
FROM golang:1.22 as builder

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
COPY pkg/ pkg/

# Build
ARG VERSION=latest
ARG BUILD_COMMIT
ARG IMAGE=quay.io/konveyor/kantra
ARG NAME=kantra
ARG JAVA_BUNDLE=/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar
ARG JAVA_PROVIDER_IMG=quay.io/konveyor/java-external-provider
ARG GENERIC_PROVIDER_IMG=quay.io/konveyor/generic-external-provider
ARG DOTNET_PROVIDER_IMG=quay.io/konveyor/dotnet-external-provider

RUN CGO_ENABLED=0 GOOS=linux go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE' -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG' -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'" -a -o kantra main.go

RUN CGO_ENABLED=0 GOOS=darwin go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE' -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG' -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'" -a -o darwin-kantra main.go

RUN CGO_ENABLED=0 GOOS=windows go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE' -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG' -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'" -a -o windows-kantra main.go

FROM quay.io/konveyor/analyzer-lsp:${VERSION}

RUN mkdir -p /opt/rulesets /opt/rulesets/input /opt/rulesets/convert /opt/openrewrite /opt/input /opt/input/rules /opt/input/rules/custom /opt/output /opt/xmlrules /opt/shimoutput /tmp/source-app /tmp/source-app/input

COPY --from=builder /workspace/kantra /usr/local/bin/kantra
COPY --from=builder /workspace/darwin-kantra /usr/local/bin/darwin-kantra
COPY --from=builder /workspace/windows-kantra /usr/local/bin/windows-kantra
COPY --from=shim /usr/bin/windup-shim /usr/local/bin
COPY --from=rulesets /rulesets/default/generated /opt/rulesets
COPY --from=rulesets /windup-rulesets/rules/rules-reviewed/openrewrite /opt/openrewrite
COPY --from=static-report /usr/bin/js-bundle-generator /usr/local/bin
COPY --from=static-report /usr/local/static-report /usr/local/static-report
COPY --chmod=755 entrypoint.sh /usr/bin/entrypoint.sh
COPY --chmod=755 openrewrite_entrypoint.sh /usr/bin/openrewrite_entrypoint.sh

ENTRYPOINT ["kantra"]
