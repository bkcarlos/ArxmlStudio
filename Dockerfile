FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY internal ./internal
COPY examples ./examples
RUN go test -tags headless ./... && go vet -tags headless ./...
RUN CGO_ENABLED=0 go build -tags headless -trimpath -ldflags="-s -w" -o /out/arxml-server .

FROM scratch
COPY --from=build /out/arxml-server /arxml-server
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/arxml-server", "--healthcheck", "http://127.0.0.1:8080/health"]
ENTRYPOINT ["/arxml-server"]
CMD ["--api-only", "--listen", "0.0.0.0:8080"]
