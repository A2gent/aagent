.PHONY: build run clean test install

# Binary name
BINARY=aagent

# Build the binary
build:
	go build -o $(BINARY) ./cmd/aagent

# Run the agent
run: build
	./$(BINARY)

# Clean build artifacts
clean:
	rm -f $(BINARY)
	go clean

# Run tests
test:
	go test -v ./...

# Install to GOPATH/bin
install:
	go install ./cmd/aagent

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	go vet ./...

# Build for all platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 ./cmd/aagent
	GOOS=linux GOARCH=arm64 go build -o $(BINARY)-linux-arm64 ./cmd/aagent
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 ./cmd/aagent
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 ./cmd/aagent
	GOOS=windows GOARCH=amd64 go build -o $(BINARY)-windows-amd64.exe ./cmd/aagent
