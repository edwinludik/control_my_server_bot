BINARY_NAME=control_my_server_bot
VERSION=1.1.4
ARCH=amd64

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=$(ARCH) go build -o $(BINARY_NAME) ./src

.PHONY: package-deb
package-deb: build-linux
	nfpm pkg --packager deb --target .

.PHONY: package-rpm
package-rpm: build-linux
	nfpm pkg --packager rpm --target .

.PHONY: package-arch
package-arch: build-linux
	nfpm pkg --packager archlinux --target .

.PHONY: packages
packages: package-deb package-rpm package-arch

.PHONY: test
test:
	go test -v ./src/...

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -f *.deb *.rpm *.pkg.tar.zst
