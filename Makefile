.PHONY: build test lint clean dist install-local install-local-osx-x64 build-linux-arm64 build-win-arm64 build-linux-x64 build-win-x64 build-osx-arm64 build-osx-x64 build-all windows-resources worker-win-arm64 worker-linux-arm64 worker-win-x64 worker-linux-x64 worker-osx-arm64 worker-osx-x64 worker-all

# Four numeric PE version components; override explicitly for local builds.
WINDOWS_FILE_VERSION ?= 0.0.0.0
WINDOWS_RESOURCE_GENERATOR := github.com/tc-hib/go-winres@v0.3.3

windows-resources:
	go run $(WINDOWS_RESOURCE_GENERATOR) make --in cmd/mi-lsp/winres.json --out cmd/mi-lsp/rsrc --arch amd64,arm64 --file-version "$(WINDOWS_FILE_VERSION)" --product-version "$(WINDOWS_FILE_VERSION)"

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

install-local:
	sh scripts/release/install-local.sh

install-local-osx-x64:
	sh scripts/release/install-local.sh --rid osx-x64

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/mi-lsp-linux-arm64 ./cmd/mi-lsp

build-win-arm64: windows-resources
	GOOS=windows GOARCH=arm64 go build -ldflags="-s -w" -o bin/mi-lsp-win-arm64.exe ./cmd/mi-lsp

build-linux-x64:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/mi-lsp-linux-x64 ./cmd/mi-lsp

build-win-x64: windows-resources
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/mi-lsp-win-x64.exe ./cmd/mi-lsp

build-osx-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/mi-lsp-osx-arm64 ./cmd/mi-lsp

build-osx-x64:
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/mi-lsp-osx-x64 ./cmd/mi-lsp

build-all: build-linux-arm64 build-win-arm64 build-linux-x64 build-win-x64 build-osx-arm64 build-osx-x64

worker-win-arm64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r win-arm64 --self-contained true -o bin/workers/win-arm64

worker-linux-arm64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r linux-arm64 --self-contained true -o bin/workers/linux-arm64

worker-win-x64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r win-x64 --self-contained true -o bin/workers/win-x64

worker-linux-x64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r linux-x64 --self-contained true -o bin/workers/linux-x64

worker-osx-arm64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r osx-arm64 --self-contained true -o bin/workers/osx-arm64

worker-osx-x64:
	dotnet publish worker-dotnet/MiLsp.Worker/MiLsp.Worker.csproj -c Release -r osx-x64 --self-contained true -o bin/workers/osx-x64

worker-all: worker-win-arm64 worker-linux-arm64 worker-win-x64 worker-linux-x64 worker-osx-arm64 worker-osx-x64
