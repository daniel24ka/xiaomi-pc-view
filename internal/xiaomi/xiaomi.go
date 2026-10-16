package xiaomi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

func Init() {
	var v struct {
		Cfg map[string]string `yaml:"xiaomi"`
	}
	app.LoadConfig(&v)

	tokens = v.Cfg

	log := app.GetLogger("xiaomi")

	streams.HandleFunc("xiaomi", func(rawURL string) (core.Producer, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}

		if u.User != nil {
			rawURL, err = getCameraURL(u)
			if err != nil {
				return nil, err
			}
		}

		log.Debug().Msgf("xiaomi: dial %s", rawURL)

		return xiaomi.Dial(rawURL)
	})

	api.HandleFunc("api/xiaomi", apiXiaomi)
}

var tokens map[string]string
var clouds map[string]*xiaomi.Cloud
var cloudsMu sync.Mutex

func getCloud(userID string) (*xiaomi.Cloud, error) {
	cloudsMu.Lock()
	defer cloudsMu.Unlock()

	if cloud := clouds[userID]; cloud != nil {
		return cloud, nil
	}

	cloud := xiaomi.NewCloud(AppXiaomiHome)
	if err := cloud.LoginWithToken(userID, tokens[userID]); err != nil {
		return nil, err
	}
	if clouds == nil {
		clouds = map[string]*xiaomi.Cloud{userID: cloud}
	} else {
		clouds[userID] = cloud
	}
	return cloud, nil
}

func cloudRequest(userID, region, apiURL, params string) ([]byte, error) {
	cloud, err := getCloud(userID)
	if err != nil {
		return nil, err
	}
	return cloud.Request(GetBaseURL(region), apiURL, params, nil)
}

func cloudUserRequest(user *url.Userinfo, apiURL, params string) ([]byte, error) {
	userID := user.Username()
	region, _ := user.Password()
	return cloudRequest(userID, region, apiURL, params)
}

func getCameraURL(url *url.URL) (string, error) {
	model := url.Query().Get("model")

	// It is not known which models need to be awakened.
	// Probably all the doorbells and all the battery cameras.
	if strings.Contains(model, ".cateye.") {
		_ = wakeUpCamera(url)
	}

	// The getMissURL request has a fallback to getP2PURL.
	// But for known models we can save one request to the cloud.
	if xiaomi.IsLegacy(model) {
		return getLegacyURL(url)
	}
	return getMissURL(url)
}

func getLegacyURL(url *url.URL) (string, error) {
	query := url.Query()

	clientPublic, clientPrivate, err := crypto.GenerateKey()
	if err != nil {
		return "", err
	}

	params := fmt.Sprintf(`{"did":"%s","toSignAppData":"%x"}`, query.Get("did"), clientPublic)

	userID := url.User.Username()
	region, _ := url.User.Password()
	res, err := cloudRequest(userID, region, "/device/devicepass", params)
	if err != nil {
		return "", err
	}

	var v struct {
		UID       string `json:"p2p_id"`
		Password  string `json:"password"`
		PublicKey string `json:"p2p_dev_public_key"`
		Sign      string `json:"signForAppData"`
	}
	if err = json.Unmarshal(res, &v); err != nil {
		return "", err
	}

	query.Set("uid", v.UID)

	if v.Sign != "" {
		query.Set("client_public", hex.EncodeToString(clientPublic))
		query.Set("client_private", hex.EncodeToString(clientPrivate))
		query.Set("device_public", v.PublicKey)
		query.Set("sign", v.Sign)
	} else {
		query.Set("password", v.Password)
	}

	url.RawQuery = query.Encode()
	return url.String(), nil
}

func getMissURL(url *url.URL) (string, error) {
	clientPublic, clientPrivate, err := crypto.GenerateKey()
	if err != nil {
		return "", err
	}

	query := url.Query()
	params := fmt.Sprintf(
		`{"app_pubkey":"%x","did":"%s","support_vendors":"TUTK_CS2_MTP"}`,
		clientPublic, query.Get("did"),
	)

	res, err := cloudUserRequest(url.User, "/v2/device/miss_get_vendor", params)
	if err != nil {
		if strings.Contains(err.Error(), "no available vendor support") {
			return getLegacyURL(url)
		}
		return "", err
	}

	var v struct {
		Vendor struct {
			ID     byte `json:"vendor"`
			Params struct {
				UID string `json:"p2p_id"`
			} `json:"vendor_params"`
		} `json:"vendor"`
		PublicKey string `json:"public_key"`
		Sign      string `json:"sign"`
	}
	if err = json.Unmarshal(res, &v); err != nil {
		return "", err
	}

	query.Set("client_public", hex.EncodeToString(clientPublic))
	query.Set("client_private", hex.EncodeToString(clientPrivate))
	query.Set("device_public", v.PublicKey)
	query.Set("sign", v.Sign)
	query.Set("vendor", getVendorName(v.Vendor.ID))

	// uid is used by tutk and by cs2 connection via servers (p2p=server)
	if v.Vendor.Params.UID != "" {
		query.Set("uid", v.Vendor.Params.UID)
	}

	url.RawQuery = query.Encode()
	return url.String(), nil
}

func getVendorName(i byte) string {
	switch i {
	case 1:
		return "tutk"
	case 3:
		return "agora"
	case 4:
		return "cs2"
	case 6:
		return "mtp"
	}
	return fmt.Sprintf("%d", i)
}

func wakeUpCamera(url *url.URL) error {
	const params = `{"id":1,"method":"wakeup","params":{"video":"1"}}`
	did := url.Query().Get("did")
	_, err := cloudUserRequest(url.User, "/home/rpc/"+did, params)
	return err
}

func apiXiaomi(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		apiDeviceList(w, r)
	case "POST":
		apiAuth(w, r)
	}
}

func apiDeviceList(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	user := query.Get("id")
	if user == "" {
		cloudsMu.Lock()
		users := make([]string, 0, len(tokens))
		for s := range tokens {
			users = append(users, s)
		}
		cloudsMu.Unlock()

		api.ResponseJSON(w, users)
		return
	}

	err := func() error {
		region := query.Get("region")
		res, err := cloudRequest(user, region, "/v2/home/device_list_page", "{}")
		if err != nil {
			return err
		}
		var v struct {
			List []*Device `json:"list"`
		}

		if err = json.Unmarshal(res, &v); err != nil {
			return err
		}

		var items []*api.Source

		for _, device := range v.List {
			if !strings.Contains(device.Model, ".camera.") && !strings.Contains(device.Model, ".cateye.") {
				continue
			}
			items = append(items, &api.Source{
				Name: device.Name,
				Info: fmt.Sprintf("ip: %s, mac: %s", device.IP, device.MAC),
				URL:  fmt.Sprintf("xiaomi://%s:%s@%s?did=%s&model=%s", user, region, device.IP, device.Did, device.Model),
			})
		}

		api.ResponseSources(w, items)
		return nil
	}()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type Device struct {
	Did   string `json:"did"`
	Name  string `json:"name"`
	Model string `json:"model"`
	MAC   string `json:"mac"`
	IP    string `json:"localip"`
}

var auth *xiaomi.Cloud

func apiAuth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	username := r.Form.Get("username")
	password := r.Form.Get("password")
	captcha := r.Form.Get("captcha")
	verify := r.Form.Get("verify")

	var err error

	switch {
	case username != "" || password != "":
		auth = xiaomi.NewCloud(AppXiaomiHome)
		err = auth.Login(username, password)
	case (captcha != "" || verify != "") && auth == nil:
		// the login was started by another process (the app was restarted meanwhile)
		http.Error(w, "the login expired, sign in again", http.StatusBadRequest)
		return
	case captcha != "":
		err = auth.LoginWithCaptcha(captcha)
	case verify != "":
		err = auth.LoginWithVerify(verify)
	default:
		http.Error(w, "wrong request", http.StatusBadRequest)
		return
	}

	if err == nil {
		userID, token := auth.UserToken()
		auth = nil

		cloudsMu.Lock()
		if tokens == nil {
			tokens = map[string]string{userID: token}
		} else {
			tokens[userID] = token
		}
		cloudsMu.Unlock()

		err = app.PatchConfig([]string{"xiaomi", userID}, token)
	}

	if err != nil {
		var login *xiaomi.LoginError
		if errors.As(err, &login) {
			w.Header().Set("Content-Type", api.MimeJSON)
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(err)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

const AppXiaomiHome = "xiaomiio"

func GetBaseURL(region string) string {
	switch region {
	case "de", "i2", "ru", "sg", "us":
		return "https://" + region + ".api.io.mi.com/app"
	}
	return "https://api.io.mi.com/app"
}

// Motorer is a source that can move its camera one pan/tilt step.
type Motorer interface {
	Motor(operation int) error
}

// Motor moves the camera behind stream `name` one step (1 left, 2 right, 3 up, 4 down).
// It uses the session the stream already holds, so the stream must be running.
func Motor(name string, operation int) error {
	stream := streams.Get(name)
	if stream == nil {
		return fmt.Errorf("xiaomi: motor: no stream %q", name)
	}
	for _, conn := range stream.Conns() {
		if m, ok := conn.(Motorer); ok {
			return m.Motor(operation)
		}
	}
	return errors.New("xiaomi: motor: camera is not connected")
}

// Commander is a source that can send MISS commands.
type Commander interface {
	Command(cmd uint32, params string) error
}

// Command sends a MISS command with JSON params to the camera behind stream `name`,
// over the session the stream already holds.
func Command(name string, cmd uint32, params string) error {
	stream := streams.Get(name)
	if stream == nil {
		return fmt.Errorf("xiaomi: command: no stream %q", name)
	}
	for _, conn := range stream.Conns() {
		if c, ok := conn.(Commander); ok {
			return c.Command(cmd, params)
		}
	}
	return errors.New("xiaomi: command: camera is not connected")
}

// Requester is a source that can send MISS commands and wait for a response.
type Requester interface {
	Request(cmd uint32, params string, resp uint32, timeout time.Duration) ([]byte, error)
}

// Request sends a MISS command to the camera behind stream `name` and waits for the
// response with code resp.
func Request(name string, cmd uint32, params string, resp uint32, timeout time.Duration) ([]byte, error) {
	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("xiaomi: request: no stream %q", name)
	}
	for _, conn := range stream.Conns() {
		if r, ok := conn.(Requester); ok {
			return r.Request(cmd, params, resp, timeout)
		}
	}
	return nil, errors.New("xiaomi: request: camera is not connected")
}

// CloudRequest calls Mi Home cloud API with the account from xiaomi:// source URL.
func CloudRequest(rawURL, apiURL, params string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.User == nil {
		return nil, errors.New("xiaomi: source without account")
	}
	return cloudUserRequest(u.User, apiURL, params)
}

// MissURL resolves a xiaomi:// source to the camera session URL (keys from the cloud).
func MissURL(src string) (string, error) {
	u, err := url.Parse(src)
	if err != nil {
		return "", err
	}
	return getCameraURL(u)
}

// SDFileLister is a source that can list the files on the camera's SD card.
type SDFileLister interface {
	SDFiles(timeout time.Duration) ([]miss.SDFile, error)
}

// SDFiles lists the SD card files of the camera behind stream `name`, over the session the
// stream already holds.
func SDFiles(name string, timeout time.Duration) ([]miss.SDFile, error) {
	stream := streams.Get(name)
	if stream == nil {
		return nil, fmt.Errorf("xiaomi: SD files: no stream %q", name)
	}
	for _, conn := range stream.Conns() {
		if l, ok := conn.(SDFileLister); ok {
			return l.SDFiles(timeout)
		}
	}
	return nil, errors.New("xiaomi: SD files: camera is not connected")
}
