ARG GOLANG_IMAGE=docker.1ms.run/library/golang:1.26.5-alpine
ARG ALPINE_IMAGE=docker.1ms.run/library/alpine:3.21
ARG GOPROXY=https://goproxy.cn,direct

FROM ${GOLANG_IMAGE} AS build

ARG GOPROXY
ENV GOPROXY=${GOPROXY}

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/qingyun-update-agent ./cmd/qingyun-update-agent

FROM ${ALPINE_IMAGE}
COPY --from=build /out/qingyun-update-agent /usr/local/bin/qingyun-update-agent
EXPOSE 8787
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 CMD ["/usr/local/bin/qingyun-update-agent", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/qingyun-update-agent"]
