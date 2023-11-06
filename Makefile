compile:
	make clean
	make test
	make vet
	make vuln
	CGO_ENABLED=0 go build -o ./bin/tracing-proxy -tags netgo -ldflags '-s -extldflags "-static"' ./cmd/tracing-proxy/main.go

deps:
	go mod download

vet:
	golangci-lint run ./...

vuln:
	govulncheck ./...

test:
	go test ./...

clean:
	[ ! -e ./main ] || rm ./main
	[ ! -e ./bin ] || rm -rf ./bin
