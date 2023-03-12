GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GORUN=$(GOCMD) run

VERSION=$(shell git describe --exact-match --tags 2>/dev/null)
BUILD_DIR=build
PACKAGE_RPI=hkcam-$(VERSION)_linux_armhf

export GO111MODULE=on

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

run:
	$(GOBUILD) -o $(BUILD_DIR)/hkcam -ldflags "-X main.Version=dev -X main.Date=$$(date +%FT%TZ%z)" cmd/hkcam/main.go cmd/hkcam/fs.go
	$(BUILD_DIR)/hkcam --verbose --data_dir=cmd/hkcam/db

package: build
	tar -cvzf $(PACKAGE_RPI).tar.gz -C $(BUILD_DIR) $(PACKAGE_RPI)

build:
	GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -o $(BUILD_DIR)/$(PACKAGE_RPI)/usr/bin/hkcam cmd/hkcam/main.go