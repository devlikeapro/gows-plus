MEOW_BRANCH ?= fork-main-2026-11-24

all: clean build-proto test build

test:
	cd src && \
	go test ./...

clean:
	rm -rf src/proto
	rm -rf bin

build-proto:
	mkdir -p src/proto
	protoc \
		-I=. \
		--go_out=./src/proto \
		--go-grpc_out=./src/proto \
		--experimental_allow_proto3_optional \
		 proto/*.proto

tidy: build-proto
	cd src && \
	go mod tidy

build:
	cd src && \
	go build -o ../bin/gows .

# Update whatsmeow fork to the latest commit on MEOW_BRANCH (override: make up-meow MEOW_BRANCH=...)
up-meow:
	cd src && \
	version=$$(GOPROXY=direct GONOSUMDB=github.com/devlikeapro go list -m -f '{{.Version}}' github.com/devlikeapro/whatsmeow@$(MEOW_BRANCH)) && \
	echo "whatsmeow $(MEOW_BRANCH) => $$version" && \
	go mod edit -replace go.mau.fi/whatsmeow=github.com/devlikeapro/whatsmeow@$$version && \
	sed -i "s|^\(replace go.mau.fi/whatsmeow => github.com/devlikeapro/whatsmeow [^ ]*\).*|\1 // branch: $(MEOW_BRANCH)|" go.mod && \
	GONOSUMDB=github.com/devlikeapro go mod tidy

# Point whatsmeow to the local checkout in ../whatsmeow (for testing changes before pushing the fork)
link-meow:
	cd src && \
	go mod edit -replace go.mau.fi/whatsmeow=../../whatsmeow && \
	sed -i "s|^\(replace go.mau.fi/whatsmeow => ../../whatsmeow\).*|\1 // LOCAL - do not commit, run 'make unlink-meow'|" go.mod && \
	go mod tidy && \
	grep "^replace go.mau.fi/whatsmeow" go.mod

# Switch whatsmeow back from the local checkout to the fork on GitHub (latest commit on MEOW_BRANCH)
unlink-meow: up-meow
