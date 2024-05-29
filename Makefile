compile:
	make clean
	make test
	make vet
	make vuln
	CGO_ENABLED=0 go build -o ./bin/tracing-proxy -tags netgo -ldflags '-s -extldflags "-static"' ./cmd/tracing-proxy/main.go

dlv-compile:
	make clean
	CGO_ENABLED=0  go build -o ./bin/tracing-proxy -gcflags "all=-N -l" ./cmd/tracing-proxy/main.go

deps:
	go mod download

vet:
	golangci-lint run --fix ./...

vuln:
	govulncheck ./...

test:
	go test ./...

clean:
	[ ! -e ./main ] || rm ./main
	[ ! -e ./bin ] || rm -rf ./bin
