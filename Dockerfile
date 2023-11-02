FROM golang:1.19-alpine as builder
WORKDIR /app
COPY . .
RUN go build -o build/retrieval_worker ./pkg/cmd/retrieval_worker
RUN go build -o build/stub_worker ./worker/stub/cmd
RUN go build -o build/http_worker ./worker/http/cmd
RUN go build -o build/bitswap_worker ./worker/bitswap/cmd
RUN go build -o build/spadev0 ./integration/spadev0

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/build/ .
CMD ["/app/retrieval_worker"]
