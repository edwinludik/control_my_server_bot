BINARY_NAME=control_my_server_bot
VERSION=1.0.0
ARCH=amd64

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=$(ARCH) go build -o $(BINARY_NAME) main.go

.PHONY: package-deb
package-deb: build-linux
	nfpm pkg --packager deb --target .

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -f *.deb
