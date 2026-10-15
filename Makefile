VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX  ?= $(HOME)/.local
BIN     := xiaomi-cam

.PHONY: build install uninstall clean release deb aar apk

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/xiaomi-cam

install: build
	install -Dm755 $(BIN) $(PREFIX)/bin/$(BIN)
	install -Dm644 packaging/xiaomi-cam.desktop $(PREFIX)/share/applications/xiaomi-cam.desktop
	install -Dm644 packaging/xiaomi-cam.svg $(PREFIX)/share/icons/hicolor/scalable/apps/xiaomi-cam.svg
	install -Dm644 packaging/xiaomi-cam.png $(PREFIX)/share/icons/hicolor/256x256/apps/xiaomi-cam.png
	-gtk-update-icon-cache -f -t $(PREFIX)/share/icons/hicolor 2>/dev/null
	sed -i "s|^Exec=.*|Exec=$(PREFIX)/bin/$(BIN)|" $(PREFIX)/share/applications/xiaomi-cam.desktop

uninstall:
	rm -f $(PREFIX)/bin/$(BIN) $(PREFIX)/share/applications/xiaomi-cam.desktop \
		$(PREFIX)/share/icons/hicolor/scalable/apps/xiaomi-cam.svg $(PREFIX)/share/icons/hicolor/256x256/apps/xiaomi-cam.png

# dist/: .deb (install), plain binary (run without installing), .tar.gz (binary + icon + desktop file)
release: deb
	rm -rf dist/xiaomi-cam && mkdir -p dist/xiaomi-cam
	cp $(BIN) README.md LICENSE packaging/xiaomi-cam.desktop packaging/xiaomi-cam.svg packaging/xiaomi-cam.png dist/xiaomi-cam/
	tar -C dist -czf dist/xiaomi-cam-$(VERSION)-linux-amd64.tar.gz xiaomi-cam
	cp $(BIN) dist/xiaomi-cam-$(VERSION)-linux-amd64

# dist/xiaomi-cam_<version>_amd64.deb: sudo apt install ./dist/xiaomi-cam_*.deb
DEBVER := $(shell echo $(VERSION) | sed -e 's/^v//' -e 's/-\([0-9]*\)-g/+\1.g/' -e 's/-dirty/+dirty/')
deb: build
	rm -rf dist/deb && mkdir -p dist/deb/DEBIAN
	install -Dm755 $(BIN) dist/deb/usr/bin/xiaomi-cam
	install -Dm644 packaging/xiaomi-cam.desktop dist/deb/usr/share/applications/xiaomi-cam.desktop
	install -Dm644 packaging/xiaomi-cam.svg dist/deb/usr/share/icons/hicolor/scalable/apps/xiaomi-cam.svg
	install -Dm644 packaging/xiaomi-cam.png dist/deb/usr/share/icons/hicolor/256x256/apps/xiaomi-cam.png
	install -Dm644 packaging/deb/copyright dist/deb/usr/share/doc/xiaomi-cam/copyright
	install -Dm644 README.md dist/deb/usr/share/doc/xiaomi-cam/README.md
	chmod 755 dist/deb
	sed -e "s/@VERSION@/$(DEBVER)/" -e "s/@SIZE@/$$(du -sk dist/deb/usr | cut -f1)/" packaging/deb/control > dist/deb/DEBIAN/control
	dpkg-deb --root-owner-group --build dist/deb dist/xiaomi-cam_$(DEBVER)_amd64.deb
	rm -rf dist/deb

clean:
	rm -rf $(BIN) dist

# --- Android app (android/): the Go core as an .aar (gomobile), then the APKs (Gradle).
# Toolchain in ~/Android (JDK 17, SDK, NDK), gomobile in ~/go/bin.
ANDROID_ENV = PATH=$(HOME)/go/bin:$(HOME)/Android/jdk/bin:$(PATH) JAVA_HOME=$(HOME)/Android/jdk \
	ANDROID_HOME=$(HOME)/Android/sdk ANDROID_NDK_HOME=$(HOME)/Android/sdk/ndk/27.3.13750724

aar:
	$(ANDROID_ENV) gomobile bind -target=android/arm,android/arm64,android/386,android/amd64 \
		-androidapi 26 -javapkg pl.lawinski.xiaomicam.core -ldflags "-s -w" \
		-o android/app/libs/xccore.aar ./mobile

# release APKs (per ABI + universal) in dist/
apk: aar
	cd android && $(ANDROID_ENV) ./gradlew -q assembleRelease -PappVersion=$(VERSION)
	mkdir -p dist
	for f in android/app/build/outputs/apk/release/*.apk; do \
		cp $$f dist/xiaomi-cam-$(VERSION:v%=%)-$$(basename $$f .apk | sed 's/^app-//; s/-release//').apk; done
