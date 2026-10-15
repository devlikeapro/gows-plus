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

PG_TEST_DSN ?= postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable

test-pg:
	docker run -d --rm --name gows-test-pg -e POSTGRES_PASSWORD=postgres -p 55432:5432 postgres:16
	until docker exec gows-test-pg pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; do sleep 1; done
	cd src && GOWS_TEST_PG_DSN="$(PG_TEST_DSN)" go test ./storage/sqlstorage/...; status=$$?; docker stop gows-test-pg; exit $$status
