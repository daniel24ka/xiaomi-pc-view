package main

// Polish is the source language; T returns English text when English is selected.

const prefLanguage = "language"

var languages = []struct{ code, name string }{
	{"pl", "Polski"},
	{"en", "English"},
}

var lang = "pl"

func T(pl string) string {
	if lang == "en" {
		if en, ok := english[pl]; ok {
			return en
		}
	}
	return pl
}

var english = map[string]string{
	"Anuluj": "Cancel",
	"Błąd: ": "Error: ",
	"Brak kamer w tym regionie – wybierz inny.": "No cameras in this region – choose another one.",
	"Brak kamery – kliknij „Dodaj”":             "No camera – click “Add”",
	"Chiny":                                     "China",
	"Dodaj":                                     "Add",
	"Dodaj kamerę":                              "Add camera",
	"Dźwięk":                                    "Sound",
	"e-mail, telefon lub Xiaomi ID":             "e-mail, phone or Xiaomi ID",
	"Europa (Polska)":                           "Europe",
	"ffmpeg: %w (zainstaluj: sudo apt install ffmpeg)": "ffmpeg: %w (install: sudo apt install ffmpeg)",
	"Hasło":                              "Password",
	"Indie":                              "India",
	"Jakość:":                            "Quality:",
	"Język:":                             "Language:",
	"Kamera:":                            "Camera:",
	"Konto":                              "Account",
	"Łączenie z kamerą…":                 "Connecting to camera…",
	"Login":                              "Login",
	"Logowanie do Mi Home":               "Mi Home login",
	"logowanie nieudane: %s":             "login failed: %s",
	"Lokalne (LAN)":                      "Local (LAN)",
	"Masz już zalogowane konto Mi Home.": "A Mi Home account is already logged in.",
	"Na żywo (%s)":                       "Live (%s)",
	"Nie udało się odczytać stanu kamery: ": "Can't read camera state: ",
	"Obrót: ":                 "Pan/tilt: ",
	"Pokaż":                   "Show",
	"Pokaż panele":            "Show panels",
	"Połączenie:":             "Connection:",
	"Przepisz kod z obrazka:": "Type the code from the picture:",
	"Reflektor":               "Spotlight",
	"Region":                  "Region",
	"Rosja":                   "Russia",
	"Rozłączono (%v) – ponawiam za 3 s…": "Disconnected (%v) – retrying in 3 s…",
	"Singapur":                        "Singapore",
	"Syrena":                          "Siren",
	"Syrena alarmowa":                 "Siren",
	"Tylko obraz":                     "Video only",
	"Ukryj do zasobnika":              "Hide to tray",
	"USA":                             "USA",
	"Usunąć „%s” z aplikacji?":        "Remove “%s” from the app?",
	"Usuń kamerę":                     "Remove camera",
	"Weryfikacja":                     "Verification",
	"Włączyć syrenę alarmową kamery?": "Turn on the camera siren?",
	"Wybierz kamery":                  "Choose cameras",
	"Wyślij":                          "Send",
	"Xiaomi wysłało kod weryfikacyjny na: ": "Xiaomi sent a verification code to: ",
	"Zakończ":             "Quit",
	"Zaloguj":             "Log in",
	"Zaloguj inne konto":  "Log in another account",
	"Skróty klawiaturowe": "Keyboard shortcuts",
	`Strzałki – obrót kamery
Przeciąganie obrazu myszą – obrót kamery
+ / − (także na klawiaturze numerycznej), kółko myszy – zoom kamery (do 9×)
0 – zoom 1×
H – tylko obraz (ukrywa panele), prawy przycisk myszy lub Esc – powrót
F, F11, podwójne kliknięcie – pełny ekran
S – zdjęcie do ~/Obrazy
Zamknięcie okna chowa aplikację do zasobnika`: `Arrows – pan/tilt the camera
Drag the picture with the mouse – pan/tilt
+ / − (also on the numeric keypad), mouse wheel – camera zoom (up to 9×)
0 – zoom 1×
H – video only (hides panels), right click or Esc – back
F, F11, double click – full screen
S – snapshot to ~/Pictures
Closing the window hides the app to the tray`,
	"Historia":                   "History",
	"Historia:":                  "History:",
	"Historia: od %s, +%s (%d×)": "History: from %s, +%s (%d×)",
	"Prędkość:":                  "Speed:",
	"Szukam nagrania…":           "Looking for the recording…",
	"Na żywo":                    "Live",
	"Brak kamer.":                "No cameras.",
	"Kamery":                     "Cameras",
	"Zamknij":                    "Close",
	"Zmień nazwę":                "Rename",
	"Zapisz":                     "Save",
	"Nazwa":                      "Name",
	"Kamera „%s” już istnieje":   "Camera “%s” already exists",
	"Zmień…":                     "Change…",
	"Nagrania":                   "Recordings",
	"Zdjęcia":                    "Snapshots",
	"Ustawienia":                 "Settings",
	"● Nagrywanie %s":            "● Recording %s",
	"Zapisuję zdjęcie…":          "Saving snapshot…",
	"Zapisano ":                  "Saved ",
	"Zdalne":                     "Remote",
	"Znaleziono kamer: %d":       "Cameras found: %d",
}
