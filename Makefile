.PHONY: all build build-relay clean install uninstall

APP_NAME = techat
RELAY_NAME = techat-relay
VERSION = $(shell git describe --tags --always 2>/dev/null || echo "dev")
LDFLAGS = -ldflags="-s -w -X main.Version=$(VERSION)"
GOFLAGS = -trimpath

all: build build-relay

build:
	go build $(GOFLAGS) $(LDFLAGS) -o $(APP_NAME) ./cmd/techat

build-relay:
	go build $(GOFLAGS) $(LDFLAGS) -o $(RELAY_NAME) ./cmd/relay

build-all:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(APP_NAME)-linux-amd64 ./cmd/techat
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(APP_NAME)-linux-arm64 ./cmd/techat
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(APP_NAME)-darwin-amd64 ./cmd/techat
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(APP_NAME)-darwin-arm64 ./cmd/techat
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(APP_NAME)-windows-amd64.exe ./cmd/techat
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(RELAY_NAME)-linux-amd64 ./cmd/relay
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(RELAY_NAME)-darwin-amd64 ./cmd/relay
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o build/$(RELAY_NAME)-windows-amd64.exe ./cmd/relay

install: build
	install -m 755 $(APP_NAME) /usr/local/bin/$(APP_NAME)
	@echo "Installed $(APP_NAME) to /usr/local/bin/$(APP_NAME)"

install-relay: build-relay
	install -m 755 $(RELAY_NAME) /usr/local/bin/$(RELAY_NAME)
	@echo "Installed $(RELAY_NAME) to /usr/local/bin/$(RELAY_NAME)"

uninstall:
	rm -f /usr/local/bin/$(APP_NAME) /usr/local/bin/$(RELAY_NAME)

clean:
	rm -f $(APP_NAME) $(RELAY_NAME)
	rm -rf build/

deps:
	go mod tidy
	go mod verify

run: build
	./$(APP_NAME)

run-relay: build-relay
	./$(RELAY_NAME)

lint:
	gofmt -l -s .
	go vet ./...

test:
	go test ./...
