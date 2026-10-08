ifeq ($(OS),Windows_NT)
	EXE := .exe
else
	EXE :=
endif

.PHONY: all server worker test clean

all: server worker

server:
	go build -o bin/runbooks-server$(EXE) ./cmd/runbooks-server

worker:
	go build -o bin/runbooks-worker$(EXE) ./cmd/runbooks-worker

test:
	go test ./...

clean:
	rm -rf bin/ runbook.db runbook.db-wal runbook.db-shm data/ logs/ .worker-id

run-server: server
	./bin/runbooks-server$(EXE)

run-worker: worker
	./bin/runbooks-worker$(EXE)

tidy:
	go mod tidy
