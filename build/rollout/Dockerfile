FROM --platform=${BUILDPLATFORM} golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /workspace
COPY . /workspace

RUN echo "We are running on ${BUILDPLATFORM}, building for ${TARGETPLATFORM}, TARGETOS=${TARGETOS}, TARGETARCH=${TARGETARCH}"

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -v -o manager kusionstack.io/rollout/cmd/rollout

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian12:debug
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
