compile:
	make clean
	CGO_ENABLED=0 go build -o ./bin/tracing-proxy -tags netgo -ldflags '-s -extldflags "-static"' ./cmd/tracing-proxy/main.go

deps:
	go mod download

clean:
	[ ! -e ./main ] || rm ./main
	[ ! -e ./bin ] || rm -rf ./bin