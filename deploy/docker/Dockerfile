FROM golang:1.26.5-trixie AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go test ./... && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X main.version=1.1.0' -o /out/kingagentd ./cmd/kingagentd && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/kingagent ./cmd/kingagent

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/kingagentd /usr/local/bin/kingagentd
COPY --from=build /out/kingagent /usr/local/bin/kingagent
COPY configs/config.example.json /etc/kingagent/config.json
VOLUME ["/data"]
EXPOSE 18888
ENTRYPOINT ["/usr/local/bin/kingagentd","-config","/etc/kingagent/config.json"]
