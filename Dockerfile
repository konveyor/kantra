ARG VERSION=latest

FROM registry.access.redhat.com/ubi9-minimal as rulesets

ARG RULESETS_REF=main
RUN microdnf -y install git &&\
    git clone https://github.com/konveyor/rulesets -b ${RULESETS_REF} &&\
    git clone https://github.com/windup/windup-rulesets -b 6.3.1.Final

FROM quay.io/konveyor/static-report:${VERSION} as static-report

# Build the manager binary  
FROM golang:1.23.9 as builder

# install sqlite headers and cross-compilation tools for CGO
RUN apt-get update && apt-get install -y \
    sqlite3 \
    libsqlite3-dev \
    gcc \
    gcc-mingw-w64 \
    clang \
    git \
    wget \
    xz-utils \
    make \
    patch \
    cmake \
    libssl-dev \
    lzma-dev \
    libxml2-dev \
    && rm -rf /var/lib/apt/lists/*

# build OSXCross for macOS cross-compilation for sqlite3
RUN git clone https://github.com/tpoechtrager/osxcross /tmp/osxcross && \
    cd /tmp/osxcross && \
    wget -O tarballs/MacOSX11.3.sdk.tar.xz \
        "https://github.com/joseluisq/macosx-sdks/releases/download/11.3/MacOSX11.3.sdk.tar.xz" && \
    UNATTENDED=yes OSX_VERSION_MIN=10.12 TARGET_DIR=/usr/local/osxcross ./build.sh && \
    test -f /usr/local/osxcross/bin/x86_64-apple-darwin20.4-clang && \
    rm -rf /tmp/osxcross

ENV PATH="/usr/local/osxcross/bin:$PATH"

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
ARG GOARCH=amd64
ARG JAVA_BUNDLE=/jdtls/java-analyzer-bundle/java-analyzer-bundle.core/target/java-analyzer-bundle.core-1.0.0-SNAPSHOT.jar
ARG JAVA_PROVIDER_IMG=quay.io/konveyor/java-external-provider
ARG GENERIC_PROVIDER_IMG=quay.io/konveyor/generic-external-provider
ARG DOTNET_PROVIDER_IMG=quay.io/konveyor/dotnet-external-provider

RUN CGO_ENABLED=1 GOOS=linux GOARCH=$GOARCH go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE' -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG' -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'" -a -o kantra main.go

RUN CGO_ENABLED=1 GOOS=darwin GOARCH=$GOARCH CC=x86_64-apple-darwin20.4-clang CXX=x86_64-apple-darwin20.4-clang++ go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE' -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG' -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'" -a -o darwin-kantra main.go

RUN CGO_ENABLED=1 GOOS=windows GOARCH=$GOARCH CC=x86_64-w64-mingw32-gcc go build --ldflags="-X 'github.com/konveyor-ecosystem/kantra/cmd.Version=$VERSION' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.RunnerImage=$IMAGE' -X 'github.com/konveyor-ecosystem/kantra/cmd.BuildCommit=$BUILD_COMMIT' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.JavaBundlesLocation=$JAVA_BUNDLE' -X 'github.com/konveyor-ecosystem/kantra/cmd.JavaProviderImage=$JAVA_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.DotnetProviderImage=$DOTNET_PROVIDER_IMG' \
-X 'github.com/konveyor-ecosystem/kantra/cmd.GenericProviderImage=$GENERIC_PROVIDER_IMG' -X 'github.com/konveyor-ecosystem/kantra/cmd.RootCommandName=$NAME'" -a -o windows-kantra.exe main.go

FROM quay.io/konveyor/analyzer-lsp:${VERSION}

RUN echo -e "[almalinux9-appstream]" \
 "\nname = almalinux9-appstream" \
 "\nbaseurl = https://repo.almalinux.org/almalinux/9/AppStream/\$basearch/os/" \
 "\nenabled = 1" \
 "\ngpgcheck = 0" > /etc/yum.repos.d/almalinux.repo

RUN microdnf -y install podman
RUN echo mta:x:1000:0:1000 user:/home/mta:/sbin/nologin > /etc/passwd
RUN echo mta:10000:5000 > /etc/subuid
RUN echo mta:10000:5000 > /etc/subgid
RUN mkdir -p /home/mta/.config/containers/
RUN cp /etc/containers/storage.conf /home/mta/.config/containers/storage.conf
RUN sed -i "s/^driver.*/driver = \"vfs\"/g" /home/mta/.config/containers/storage.conf
RUN echo -ne '[containers]\nvolumes = ["/proc:/proc",]\ndefault_sysctls = []' > /home/mta/.config/containers/containers.conf
RUN chown -R 1000:1000 /home/mta

RUN mkdir -p /opt/rulesets /opt/rulesets/input /opt/rulesets/convert /opt/openrewrite /opt/input /opt/input/rules /opt/input/rules/custom /opt/output  /tmp/source-app /tmp/source-app/input

COPY --from=builder /workspace/kantra /usr/local/bin/kantra
COPY --from=builder /workspace/darwin-kantra /usr/local/bin/darwin-kantra
COPY --from=builder /workspace/windows-kantra.exe /usr/local/bin/windows-kantra.exe
COPY --from=rulesets /rulesets/default/generated /opt/rulesets
COPY --from=rulesets /windup-rulesets/rules/rules-reviewed/openrewrite /opt/openrewrite
COPY --from=static-report /usr/bin/js-bundle-generator /usr/local/bin
COPY --from=static-report /usr/local/static-report /usr/local/static-report
COPY --chmod=755 entrypoint.sh /usr/bin/entrypoint.sh
COPY --chmod=755 openrewrite_entrypoint.sh /usr/bin/openrewrite_entrypoint.sh

ENTRYPOINT ["kantra"]