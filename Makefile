.PHONY: build build-all clean test

APP = starter
BUILD_DIR = build

build:
	go build -o $(BUILD_DIR)/$(APP) ./cmd/starter/

build-all:
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP)-windows-amd64.exe ./cmd/starter/
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP)-linux-amd64 ./cmd/starter/
	GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(APP)-darwin-amd64 ./cmd/starter/
	GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(APP)-darwin-arm64 ./cmd/starter/

test:
	go test ./... -v

clean:
	rm -rf $(BUILD_DIR)/
