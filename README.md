# Xiaomi Cam – podgląd kamer Xiaomi Mi Home na PC (Linux)

[English below](#english)

Aplikacja desktopowa na Ubuntu/Linux do oglądania kamer z ekosystemu **Xiaomi Mi Home**,
które oficjalnie działają tylko w aplikacji Mi Home na telefonie. Testowana z
**Xiaomi Outdoor Camera CW700S** (`isa.camera.700sa`).

Jeden plik wykonywalny, z wbudowanym [go2rtc](https://github.com/AlexxIT/go2rtc).

## Funkcje

- obraz na żywo do **2.5K** (2560×1440), przełączanie jakości 2.5K / 720p / SD,
- **połączenie zdalne**: działa z dowolnej sieci, tak jak Mi Home, bez przekierowania
  portów i bez VPN; jest też tryb lokalny (LAN),
- **obrót kamery (PTZ)**: przyciski, strzałki na klawiaturze, przeciąganie obrazu myszą,
- **zoom cyfrowy**: kółko myszy, `+` / `−`, przesuwanie powiększenia myszą,
- **reflektor** (włącz/wyłącz, jasność) i **syrena**,
- dźwięk z kamery,
- zdjęcie z podglądu (`~/Obrazy`),
- tryb **„Tylko obraz”** (ukrywa panele) i pełny ekran,
- ikona w **zasobniku systemowym**: zamknięcie okna chowa aplikację i zatrzymuje transmisję,
- język **polski / angielski**,
- logowanie kontem Mi Home (z obsługą captcha i kodu weryfikacyjnego).

## Wymagania

- Linux x86-64 (testowane na Ubuntu 26.04, GNOME/Wayland),
- `ffmpeg` (dekodowanie H.265): `sudo apt install ffmpeg`,
- ikona w zasobniku na GNOME wymaga rozszerzenia AppIndicator (w Ubuntu jest domyślnie).

## Instalacja

Z [Releases](../../releases) pobierz `xiaomi-cam-…-linux-amd64.tar.gz`, potem:

```sh
tar xzf xiaomi-cam-*-linux-amd64.tar.gz
install -Dm755 xiaomi-cam/xiaomi-cam ~/.local/bin/xiaomi-cam
install -Dm644 xiaomi-cam/xiaomi-cam.desktop ~/.local/share/applications/xiaomi-cam.desktop
```

Albo zbuduj ze źródeł (Go ≥ 1.25, kompilator C, nagłówki X11/GL):

```sh
sudo apt install golang gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
make install        # buduje i instaluje do ~/.local
```

## Pierwsze uruchomienie

1. Uruchom **Xiaomi Cam** z menu aplikacji.
2. Zaloguj się kontem Mi Home. Hasło trafia tylko do wbudowanego go2rtc i do serwerów
   Xiaomi; zapisywany jest jedynie token, w `~/.config/xiaomi-cam-viewer/go2rtc.yaml`
   (prawa 600).
3. Wybierz region (Polska to **Europa**) i zaznacz kamery.

## Skróty

| Klawisz / mysz | Akcja |
|---|---|
| strzałki | obrót kamery |
| przeciąganie obrazu | obrót kamery (przy zoomie: przesuwanie powiększenia) |
| `+` / `−`, kółko myszy | zoom |
| `0` | zoom 1:1 |
| `H` | tylko obraz; `Esc` lub prawy przycisk myszy – powrót |
| `F`, `F11`, podwójne kliknięcie | pełny ekran |
| `S` | zdjęcie |

## Jak to działa

Kamery Xiaomi nie udostępniają RTSP. Obraz jest pobierany protokołem Xiaomi **MISS**
przez P2P **CS2**, dokładnie jak w Mi Home:

1. Chmura Xiaomi zwraca klucze szyfrowania i identyfikator P2P kamery.
2. Aplikacja wysyła prośbę o połączenie do serwerów CS2 Xiaomi. Serwery przekazują
   kamerze i aplikacji swoje adresy, a obie strony przebijają NAT (UDP hole punching).
3. Obraz idzie bezpośrednio z kamery do komputera i jest szyfrowany. Wewnątrz aplikacji
   go2rtc wystawia go jako RTSP na `127.0.0.1`, a ffmpeg go dekoduje.

Ten tryb („Zdalne”) dodałem do go2rtc w tym repozytorium, bo niektóre kamery, np. CW700S,
nie odpowiadają na wyszukiwanie w sieci lokalnej, którego używa oficjalne go2rtc.

Reflektor i syrena są sterowane przez chmurę Xiaomi (MIoT), obrót przez sesję MISS.

## Ograniczenia

- Adresy serwerów CS2 są wpisane dla regionu **Europa**. Dla innych regionów można je
  podać w źródle kamery w konfiguracji: `&servers=ip1,ip2`.
- Obsługiwane są kamery z protokołem `cs2` (większość nowszych kamer Xiaomi).
  Właściwości reflektora i syreny są zgodne ze specyfikacją `isa.camera.700sa`.

## Licencja i podziękowania

MIT. Projekt bazuje na [go2rtc](https://github.com/AlexxIT/go2rtc) autorstwa
Alexey'a Khita (MIT); oryginalne README jest w [README.go2rtc.md](README.go2rtc.md).
Sterowanie silnikiem kamery wzorowane na PR AlexxIT/go2rtc#2531.

---

## English

Desktop app for Linux to watch **Xiaomi Mi Home** cameras, which officially work only
in the Mi Home phone app. Tested with **Xiaomi Outdoor Camera CW700S** (`isa.camera.700sa`).
Single binary with embedded [go2rtc](https://github.com/AlexxIT/go2rtc).

**Features:** live video up to 2.5K (2560×1440) with 2.5K/720p/SD quality, **remote
connection from any network** (like Mi Home, no port forwarding or VPN) or local (LAN),
pan/tilt (buttons, arrow keys, dragging the picture), digital zoom, spotlight with
brightness, siren, camera audio, snapshots, video-only and full-screen modes, system
tray icon (closing the window hides the app and stops streaming), Polish/English UI,
Mi Home login with captcha and 2FA code.

**Requirements:** Linux x86-64, `ffmpeg` (`sudo apt install ffmpeg`). On GNOME the
tray icon needs the AppIndicator extension (default in Ubuntu).

**Install:** download `xiaomi-cam-…-linux-amd64.tar.gz` from [Releases](../../releases)
and copy `xiaomi-cam` to `~/.local/bin` and `xiaomi-cam.desktop` to
`~/.local/share/applications`, or build with `make install`.

**First run:** log in with your Mi Home account. Only a token is stored, in
`~/.config/xiaomi-cam-viewer/go2rtc.yaml` with mode 600. Choose the region
(Europe for EU accounts) and the cameras.

**Shortcuts:** arrows – pan/tilt; drag – pan/tilt (move the zoomed area when zoomed);
`+`/`−`/wheel – zoom; `0` – zoom 1:1; `H` – video only (`Esc` or right click to go back);
`F`/`F11`/double click – full screen; `S` – snapshot.

**How it works:** the app uses the Xiaomi MISS protocol over CS2 P2P like Mi Home.
It asks the Xiaomi CS2 servers to introduce the camera, then uses UDP hole punching
and an end-to-end encrypted direct stream. This "server" mode was added to go2rtc in
this repository, because some cameras (e.g. CW700S) don't answer the LAN search used
by upstream go2rtc. CS2 server addresses are set for the EU region; other regions can
pass `&servers=ip1,ip2` in the camera source.

**License:** MIT, based on go2rtc by Alexey Khit (MIT).
