VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX  ?= $(HOME)/.local
BIN     := xiaomi-cam

.PHONY: build install uninstall clean release deb aar apk windows

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

# --- Windows (cross-built here): xiaomi-cam.exe with its icon and manifest (go-winres), ffmpeg
# of BtbN (GPL, shared) next to it, the installer by Inno Setup under Wine.
# Toolchain in ~/Windows: llvm-mingw, ffmpeg, a Wine prefix with Inno Setup.
WIN_CC     = $(HOME)/Windows/llvm-mingw-20240619-ucrt-ubuntu-20.04-x86_64/bin
WIN_FFMPEG = $(HOME)/Windows/ffmpeg/ffmpeg-n8.1-latest-win64-gpl-shared-8.1
WIN_ISCC   = WINEPREFIX=$(HOME)/Windows/wine WINEDEBUG=-all wine $(HOME)/Windows/wine/drive_c/InnoSetup/ISCC.exe
WIN_VER    = $(shell echo $(VERSION:v%=%) | sed 's/-.*//')

windows:
	rm -rf dist/windows && mkdir -p dist/windows/ffmpeg
	cd cmd/xiaomi-cam && go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --manifest gui \
		--icon ../../packaging/windows/icon256.png --product-name "Xiaomi Cam" --file-description "Xiaomi Cam" \
		--product-version $(WIN_VER).0 --file-version $(WIN_VER).0 --original-filename xiaomi-cam.exe --out rsrc
	PATH=$(WIN_CC):$(PATH) GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-clang \
		CXX=x86_64-w64-mingw32-clang++ go build -trimpath \
		-ldflags "-H windowsgui -s -w -X main.version=$(VERSION)" -o dist/windows/xiaomi-cam.exe ./cmd/xiaomi-cam; \
		s=$$?; rm -f cmd/xiaomi-cam/rsrc_windows_amd64.syso; exit $$s
	cp $(WIN_FFMPEG)/bin/ffmpeg.exe $(WIN_FFMPEG)/bin/ffprobe.exe $(WIN_FFMPEG)/bin/*.dll dist/windows/ffmpeg/
	cp $(WIN_FFMPEG)/LICENSE.txt dist/windows/ffmpeg/LICENSE-ffmpeg.txt
	cd packaging/windows && $(WIN_ISCC) /Q /DVersion=$(WIN_VER) /O"..\..\dist" xiaomi-cam.iss
	ls -la dist/xiaomi-cam-$(WIN_VER)-windows-setup.exe
