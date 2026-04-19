BINARY_NAME=control_my_server_bot
VERSION=$(shell cat VERSION)
ARCH=amd64
LDFLAGS=-ldflags "-X main.AppVersion=$(VERSION)"

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=$(ARCH) go build $(LDFLAGS) -o $(BINARY_NAME) ./src

.PHONY: package-deb
package-deb: build-linux
	VERSION=$(VERSION) nfpm pkg --packager deb --target .

.PHONY: package-rpm
package-rpm: build-linux
	VERSION=$(VERSION) nfpm pkg --packager rpm --target .

.PHONY: package-arch
package-arch: build-linux
	VERSION=$(VERSION) nfpm pkg --packager archlinux --target .

.PHONY: packages
packages: package-deb package-rpm package-arch

.PHONY: test
test:
	go test -v ./src/...

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -f *.deb *.rpm *.pkg.tar.zst
