help:
	@echo "Peerchat Makefile (Requires Go v1.24)"
	@echo "'help' - Displays the command usage"
	@echo "'build' - Builds the application into a binary/executable" 
	@echo "'install' - Installs the application"
	@echo "'build-windows' - Builds the application for Windows platforms"
	@echo "'build-darwin' - Builds the application for MacOSX platforms"
	@echo "'build-linux' - Builds the application for Linux platforms"
	@echo "'build-all' - Builds the application for all platforms"

build:
	@echo Compiling PeerChat
	@go build .
	@echo Compile Complete. Run './peerchat(.exe)'

install:
	@echo Installing Peerchat
	go install .
	@echo install Complete. Run 'peerchat'.

build-windows:
	@echo Cross Compiling Peerchat for Windows x86
	@GOOS=windows GOARCH=386 go build -o ./bin/peerchat-windows-x32.exe
	@echo Cross Compiling Peerchat for Windows x64
	@GOOS=windows GOARCH=amd64 go build -o ./bin/peerchat-windows-x64.exe

build-darwin:
	@echo Cross Compiling Peerchat for MacOSX x64
	@GOOS=darwin GOARCH=amd64 go build -o ./bin/peerchat-darwin-x64

build-linux:
	@echo Cross Compiling Peerchat for Linux x32
	@GOOS=linux GOARCH=386 go build -o ./bin/peerchat-linux-x32
	@echo Cross Compiling Peerchat for Linux x64
	@GOOS=linux GOARCH=amd64 go build -o ./bin/peerchat-linux-x64
	@echo Cross Compiling Peerchat for Linux Arm32
	@GOOS=linux GOARCH=arm go build -o ./bin/peerchat-linux-arm32
	@echo Cross Compiling Peerchat for Linux Arm64
	@GOOS=linux GOARCH=arm64 go build -o ./bin/peerchat-linux-arm64

build-all: build-windows build-darwin build-linux
	@echo Cross Compiled Peerchat for all platforms

.PHONY: dep
dep: ## Install dependencies
	$(eval PACKAGE := $(shell go list -m))
	@go mod tidy
	@go mod vendor

test:
	go clean -testcache
	go test -race ./...
