build:
	go build -o retrieval_worker ./pkg/cmd/retrieval_worker
	go build -o stub_worker ./worker/stub/cmd
	go build -o http_worker ./worker/http/cmd
	go build -o bitswap_worker ./worker/bitswap/cmd
	go build -o spadev0 ./integration/spadev0

lint:
	gofmt -s -w .
	golangci-lint run --fix --timeout 10m
	staticcheck ./...
