BINARY_NAME=control_my_server_bot
VERSION=1.0.0
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

.PHONY: package-zip
package-zip:
	zip -r $(BINARY_NAME)-$(VERSION)-source.zip . -x "$(BINARY_NAME)" "*.deb" "*.rpm" "*.pkg.tar.zst" ".git/*" ".idea/*" ".junie/*" "go.sum" ".env" ".user_ids*" ".user_ids.db*"

.PHONY: packages
packages: package-deb package-rpm package-arch package-zip

.PHONY: test
test:
	go test -v ./src/...

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -f *.deb *.rpm *.pkg.tar.zst *.zip
