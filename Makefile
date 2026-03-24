.PHONY: build test lint clean dist build-linux-arm64 build-win-arm64 build-linux-x64 build-win-x64 build-all worker-win-arm64 worker-linux-arm64 worker-win-x64 worker-linux-x64 worker-all

build:
	go build -ldflags="-s -w" -o bin/mi-lsp ./cmd/mi-lsp

test:
	@GOOS_VAL=$$(go env GOOS); GOARCH_VAL=$$(go env GOARCH); \
	if [ "$$GOOS_VAL" = "windows" ] && [ "$$GOARCH_VAL" = "arm64" ]; then \
		echo "Race detector is not supported on $$GOOS_VAL/$$GOARCH_VAL; running go test without -race."; \
		go test -v ./...; \
	else \
		go test -v -race ./...; \
	fi

lint:
	go fmt ./...
	go vet ./...

clean:
	rm -rf bin/ dist/ .goreleaser/

dist:
	pwsh -NoProfile -File scripts/release/build-dist.ps1 -Clean

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/mi-lsp-linux-arm64 ./cmd/mi-lsp

build-win-arm64:
	GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o bin/mi-lsp-win-arm64.exe ./cmd/mi-lsp

build-linux-x64:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/mi-lsp-linux-x64 ./cmd/mi-lsp

build-win-x64:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/mi-lsp-win-x64.exe ./cmd/mi-lsp

build-all: build-linux-arm64 build-win-arm64 build-linux-x64 build-win-x64

worker-win-arm64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r win-arm64 --self-contained true -o bin/workers/win-arm64

worker-linux-arm64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r linux-arm64 --self-contained true -o bin/workers/linux-arm64

worker-win-x64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r win-x64 --self-contained true -o bin/workers/win-x64

worker-linux-x64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r linux-x64 --self-contained true -o bin/workers/linux-x64

worker-all: worker-win-arm64 worker-linux-arm64 worker-win-x64 worker-linux-x64
