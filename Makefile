.PHONY: build build-release clean test

APP = starter
BUILD_DIR = build
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(APP).exe ./cmd/starter/

build-release:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION) -H windowsgui" -o $(BUILD_DIR)/$(APP)-$(VERSION)-x64.exe ./cmd/starter/

test:
	go test ./... -v -count=1

bench:
	go test ./... -bench=. -benchmem

clean:
	rm -rf $(BUILD_DIR)/

size: build
	@echo "binary size:"
	@ls -lh $(BUILD_DIR)/$(APP).exe
	@echo ""
	@echo "if upx is available:"
	@which upx 2>/dev/null && upx --best $(BUILD_DIR)/$(APP).exe && ls -lh $(BUILD_DIR)/$(APP).exe || echo "upx not installed, skipping"
