VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX  ?= $(HOME)/.local
BIN     := xiaomi-cam

.PHONY: build install uninstall clean release

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/xiaomi-cam

install: build
	install -Dm755 $(BIN) $(PREFIX)/bin/$(BIN)
	install -Dm644 packaging/xiaomi-cam.desktop $(PREFIX)/share/applications/xiaomi-cam.desktop
	sed -i "s|^Exec=.*|Exec=$(PREFIX)/bin/$(BIN)|" $(PREFIX)/share/applications/xiaomi-cam.desktop

uninstall:
	rm -f $(PREFIX)/bin/$(BIN) $(PREFIX)/share/applications/xiaomi-cam.desktop

# dist/xiaomi-cam-linux-amd64.tar.gz with binary, desktop file and README
release: build
	rm -rf dist && mkdir -p dist/xiaomi-cam
	cp $(BIN) README.md LICENSE packaging/xiaomi-cam.desktop dist/xiaomi-cam/
	tar -C dist -czf dist/xiaomi-cam-$(VERSION)-linux-amd64.tar.gz xiaomi-cam
	cp $(BIN) dist/xiaomi-cam-$(VERSION)-linux-amd64

clean:
	rm -rf $(BIN) dist
