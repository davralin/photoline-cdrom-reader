# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.4 AS build

ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod LICENSE *.go ./
COPY web ./web
RUN mkdir -p /out \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/photoline-cdrom-reader .

FROM scratch
COPY --from=build /out/photoline-cdrom-reader /photoline-cdrom-reader
COPY docs/THIRD_PARTY_NOTICES.md /licenses/THIRD_PARTY_NOTICES.md
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/photoline-cdrom-reader"]
