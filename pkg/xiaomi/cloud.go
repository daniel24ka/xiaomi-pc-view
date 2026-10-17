package xiaomi

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Cloud struct {
	client *http.Client
	lc     *http.Client // the login steps: one cookie jar for the whole login (identity_session, ick...)

	sid       string
	cookies   string // for auth
	ssecurity []byte // for encryption

	userID    string
	passToken string

	auth map[string]string
}

func NewCloud(sid string) *Cloud {
	return &Cloud{
		client: &http.Client{Timeout: 15 * time.Second},
		sid:    sid,
	}
}

const accountURL = "https://account.xiaomi.com"

// loginClient returns the client of the started login (a new one with an empty jar).
func (c *Cloud) loginClient() *http.Client {
	if c.lc == nil {
		jar, _ := cookiejar.New(nil)
		c.lc = &http.Client{Timeout: 15 * time.Second, Jar: jar}
	}
	return c.lc
}

// jarCookie returns a cookie of the login jar for account.xiaomi.com.
func (c *Cloud) jarCookie(name string) string {
	if c.lc == nil {
		return ""
	}
	u, _ := url.Parse(accountURL + "/")
	for _, cookie := range c.lc.Jar.Cookies(u) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func (c *Cloud) Login(username, password string) error {
	captcha := c.auth != nil && c.auth["captcha_code"] != ""
	if !captcha {
		c.lc = nil // a new login: new cookies
	}
	client := c.loginClient()
	if !captcha {
		u, _ := url.Parse(accountURL + "/")
		client.Jar.SetCookies(u, []*http.Cookie{{Name: "deviceId", Value: core.RandString(16, 62)}})
	}

	res, err := client.Get(accountURL + "/pass/serviceLogin?_json=true&sid=" + c.sid)
	if err != nil {
		return err
	}

	var v1 struct {
		Qs       string `json:"qs"`
		Sign     string `json:"_sign"`
		Sid      string `json:"sid"`
		Callback string `json:"callback"`
	}
	if _, err = readLoginResponse(res.Body, &v1); err != nil {
		return err
	}

	hash := fmt.Sprintf("%X", md5.Sum([]byte(password)))

	form := url.Values{
		"_json":    {"true"},
		"hash":     {hash},
		"sid":      {v1.Sid},
		"callback": {v1.Callback},
		"_sign":    {v1.Sign},
		"qs":       {v1.Qs},
		"user":     {username},
	}

	// login after captcha (the ick cookie of the captcha picture is in the jar)
	if captcha {
		form.Set("captCode", c.auth["captcha_code"])
	}

	req := Request{
		Method: "POST",
		URL:    accountURL + "/pass/serviceLoginAuth2",
		Body:   form,
	}.Encode()

	res, err = client.Do(req)
	if err != nil {
		return err
	}

	var v2 struct {
		Ssecurity []byte `json:"ssecurity"`
		PassToken string `json:"passToken"`
		Location  string `json:"location"`

		CaptchaURL      string `json:"captchaURL"`
		NotificationURL string `json:"notificationUrl"`
	}
	body, err := readLoginResponse(res.Body, &v2)
	if err != nil {
		return err
	}

	// save auth for two-step verification
	c.auth = map[string]string{
		"username": username,
		"password": password,
	}

	if v2.CaptchaURL != "" {
		return c.getCaptcha(v2.CaptchaURL)
	}

	if v2.NotificationURL != "" {
		return c.authStart(v2.NotificationURL)
	}

	if v2.Location == "" {
		return fmt.Errorf("xiaomi: %s", body)
	}

	c.auth = nil
	c.ssecurity = v2.Ssecurity
	c.passToken = v2.PassToken

	return c.finishAuth(v2.Location)
}

func (c *Cloud) LoginWithCaptcha(captcha string) error {
	if c.auth == nil || c.auth["ick"] == "" {
		return errors.New("xiaomi: wrong login step, sign in again")
	}

	c.auth["captcha_code"] = captcha

	// check if captcha after verify
	if c.auth["flag"] != "" {
		return c.sendTicket()
	}

	return c.Login(c.auth["username"], c.auth["password"])
}

// identityParams - the query of the identity pages (verify, send ticket) of this login
func (c *Cloud) identityParams() url.Values {
	return url.Values{
		"_flag":   {c.auth["flag"]},
		"_json":   {"true"},
		"sid":     {c.sid},
		"context": {c.auth["context"]},
		"mask":    {"0"},
		"_locale": {"en_US"},
	}
}

// LoginWithVerify sends the code from the e-mail / SMS. A wrong code returns an error and
// keeps the login step, so the same code can be typed again: it never asks Xiaomi for a
// new code (each new code counts towards the 24 h account lock).
func (c *Cloud) LoginWithVerify(ticket string) error {
	if c.auth == nil || c.auth["flag"] == "" {
		return errors.New("xiaomi: wrong login step, sign in again")
	}

	ticket = strings.TrimSpace(ticket)
	form := url.Values{
		"_flag":  {c.auth["flag"]},
		"ticket": {ticket},
		"trust":  {"true"}, // a trusted device: the saved token logs in without a new code
		"_json":  {"true"},
		"ick":    {c.jarCookie("ick")},
	}
	req := Request{
		Method:    "POST",
		URL:       accountURL + "/identity/auth/verify" + c.verifyName(),
		RawParams: c.identityParams().Encode(),
		Body:      form,
	}.Encode()

	client := c.loginClient()
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	var v1 struct {
		Code        int    `json:"code"`
		Location    string `json:"location"`
		Description string `json:"description"`
		Desc        string `json:"desc"`
	}
	body, err := readLoginResponseAny(res, &v1)
	if err != nil {
		return err
	}
	if v1.Code != 0 || v1.Location == "" {
		// 70014: a wrong code
		msg := v1.Description
		if msg == "" {
			msg = v1.Desc
		}
		if msg == "" {
			msg = string(body)
		}
		return &VerifyError{Code: v1.Code, Msg: msg}
	}

	return c.finishVerify(v1.Location)
}

// VerifyError - Xiaomi did not take the code (the login step stays: the code can be typed again).
type VerifyError struct {
	Code int
	Msg  string
}

func (e *VerifyError) Error() string {
	return fmt.Sprintf("xiaomi: the code was not accepted (%d %s)", e.Code, e.Msg)
}

// finishVerify follows the end of the verification: identity/result/check →
// serviceLoginAuth2/end (passToken, userId cookies, ssecurity in Extension-Pragma; sometimes
// a "Tips" page first) → sts (serviceToken).
func (c *Cloud) finishVerify(location string) error {
	client := c.loginClient()
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	get := func(rawURL string) (*http.Response, []byte, error) {
		res, err := noRedirect.Get(rawURL)
		if err != nil {
			return nil, nil, err
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		return res, body, err
	}

	next := location
	var sts string
	for i := 0; i < 8 && next != ""; i++ {
		res, body, err := get(next)
		if err != nil {
			return err
		}
		if s := res.Header.Get("Extension-Pragma"); s != "" {
			var v struct {
				Ssecurity []byte `json:"ssecurity"`
			}
			if json.Unmarshal([]byte(s), &v) == nil && v.Ssecurity != nil {
				c.ssecurity = v.Ssecurity
			}
		}
		loc := res.Header.Get("Location")
		if loc == "" && res.StatusCode == http.StatusOK && bytes.Contains(body, []byte("Tips")) {
			continue // the "Tips" page: the same URL again gives the redirect
		}
		if loc == "" {
			if j := bytes.Index(body, []byte("https://sts.api.io.mi.com/sts")); j >= 0 {
				end := bytes.IndexAny(body[j:], "\"' <")
				if end < 0 {
					end = len(body) - j
				}
				loc = string(body[j : j+end])
			}
		}
		if loc == "" {
			break
		}
		if u, err := res.Request.URL.Parse(loc); err == nil {
			loc = u.String()
		}
		if strings.Contains(loc, "sts.api.io.mi.com") {
			sts = loc
			break
		}
		next = loc
	}

	c.passToken = c.jarCookie("passToken")
	c.userID = c.jarCookie("userId")
	if c.passToken == "" || c.userID == "" {
		return errors.New("xiaomi: the code was accepted, but Xiaomi did not give the login token")
	}
	c.auth = nil

	if sts != "" {
		_ = c.finishAuth(sts) // the service token; the saved passToken is what matters
	}
	return nil
}

func (c *Cloud) getCaptcha(captchaURL string) error {
	res, err := c.loginClient().Get(accountURL + captchaURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	c.auth["ick"] = findCookie(res, "ick")

	return &LoginError{
		Captcha: body,
	}
}

func (c *Cloud) authStart(notificationURL string) error {
	client := c.loginClient()

	// open the page first (sets the identity cookies), then ask for the verification options
	if res, err := client.Get(notificationURL); err == nil {
		_ = res.Body.Close()
	}

	if u, err := url.Parse(notificationURL); err == nil {
		c.auth["context"] = u.Query().Get("context")
	}

	rawURL := strings.Replace(notificationURL, "/fe/service/identity/authStart", "/identity/list", 1)
	res, err := client.Get(rawURL)
	if err != nil {
		return err
	}

	var v1 struct {
		Code int `json:"code"`
		Flag int `json:"flag"`
	}
	if _, err = readLoginResponse(res.Body, &v1); err != nil {
		return err
	}

	c.auth["flag"] = strconv.Itoa(v1.Flag)

	return c.sendTicket()
}

func findCookie(res *http.Response, name string) string {
	for _, cookie := range res.Cookies() {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func (c *Cloud) verifyName() string {
	switch c.auth["flag"] {
	case "4":
		return "Phone"
	case "8":
		return "Email"
	}
	return ""
}

func (c *Cloud) sendTicket() error {
	name := c.verifyName()
	client := c.loginClient()

	req := Request{
		URL:       accountURL + "/identity/auth/verify" + name,
		RawParams: c.identityParams().Encode(),
	}.Encode()

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	var v1 struct {
		Code        int    `json:"code"`
		MaskedPhone string `json:"maskedPhone"`
		MaskedEmail string `json:"maskedEmail"`
	}
	if _, err = readLoginResponseAny(res, &v1); err != nil {
		return err
	}

	form := url.Values{
		"_json": {"true"},
		"icode": {c.auth["captcha_code"]}, // verify after captcha
		"retry": {"0"},
		"ick":   {c.jarCookie("ick")},
	}
	params := c.identityParams()
	params.Del("_flag")
	params.Del("_json")
	params.Set("_dc", strconv.FormatInt(time.Now().UnixMilli(), 10))

	req = Request{
		Method:    "POST",
		URL:       accountURL + "/identity/auth/send" + name + "Ticket",
		RawParams: params.Encode(),
		Body:      form,
	}.Encode()

	res, err = client.Do(req)
	if err != nil {
		return err
	}

	var v2 struct {
		Code       int    `json:"code"`
		CaptchaURL string `json:"captchaURL"`
	}
	body, err := readLoginResponseAny(res, &v2)
	if err != nil {
		return err
	}

	if v2.CaptchaURL != "" {
		return c.getCaptcha(v2.CaptchaURL)
	}

	if v2.Code != 0 {
		return fmt.Errorf("xiaomi: %s", body)
	}

	return &LoginError{
		VerifyPhone: v1.MaskedPhone,
		VerifyEmail: v1.MaskedEmail,
	}
}

type LoginError struct {
	Captcha     []byte `json:"captcha,omitempty"`
	VerifyPhone string `json:"verify_phone,omitempty"`
	VerifyEmail string `json:"verify_email,omitempty"`
	QR          []byte `json:"qr,omitempty"`     // a PNG to scan with the phone: then LoginQRWait
	QRURL       string `json:"qr_url,omitempty"` // the same login as a link
	QRWait      bool   `json:"qr_wait,omitempty"`
}

func (l *LoginError) Error() string {
	return ""
}

// --- QR code login: scanned with the phone (Mi Home app or Xiaomi phone settings), no
// password and no e-mail/SMS code.

// LoginQR starts a QR code login: the LoginError has the picture, then call LoginQRWait.
func (c *Cloud) LoginQR() error {
	c.lc = nil
	client := c.loginClient()
	params := url.Values{
		"_qrsize":      {"480"},
		"qs":           {"%3Fsid%3D" + c.sid + "%26_json%3Dtrue"},
		"callback":     {"https://sts.api.io.mi.com/sts"},
		"_hasLogo":     {"false"},
		"sid":          {c.sid},
		"serviceParam": {""},
		"_locale":      {"en_US"},
		"_dc":          {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	res, err := client.Get(accountURL + "/longPolling/loginUrl?" + params.Encode())
	if err != nil {
		return err
	}
	var v1 struct {
		QR       string `json:"qr"`
		LoginURL string `json:"loginUrl"`
		LP       string `json:"lp"`
		Timeout  int    `json:"timeout"`
	}
	body, err := readLoginResponseAny(res, &v1)
	if err != nil {
		return err
	}
	if v1.QR == "" || v1.LP == "" {
		return fmt.Errorf("xiaomi: %s", body)
	}
	if v1.Timeout <= 0 {
		v1.Timeout = 300
	}

	res, err = client.Get(v1.QR)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	png, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	c.auth = map[string]string{
		"qr_lp":      v1.LP,
		"qr_expires": strconv.FormatInt(time.Now().Add(time.Duration(v1.Timeout)*time.Second).Unix(), 10),
	}
	return &LoginError{QR: png, QRURL: v1.LoginURL}
}

// ErrQRExpired - nobody scanned the QR code in time
var ErrQRExpired = errors.New("xiaomi: the QR code expired, show a new one")

// LoginQRWait waits up to wait for the QR code to be scanned and confirmed on the phone.
// Still waiting: a LoginError with QRWait (call again).
func (c *Cloud) LoginQRWait(ctx context.Context, wait time.Duration) error {
	if c.auth == nil || c.auth["qr_lp"] == "" {
		return errors.New("xiaomi: wrong login step, sign in again")
	}
	expires, _ := strconv.ParseInt(c.auth["qr_expires"], 10, 64)
	if time.Now().Unix() > expires {
		return ErrQRExpired
	}

	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.auth["qr_lp"], nil)
	if err != nil {
		return err
	}
	client := *c.loginClient()
	client.Timeout = 0 // the long poll: the server answers when the code is scanned
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return &LoginError{QRWait: true}
		}
		return err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return &LoginError{QRWait: true}
	}

	var v1 struct {
		Code      int    `json:"code"`
		UserID    any    `json:"userId"`
		Ssecurity []byte `json:"ssecurity"`
		PassToken string `json:"passToken"`
		Location  string `json:"location"`
	}
	body, err := readLoginResponseAny(res, &v1)
	if err != nil {
		return err
	}
	if v1.PassToken == "" || v1.UserID == nil {
		return fmt.Errorf("xiaomi: %s", body)
	}

	c.auth = nil
	c.userID = fmt.Sprint(v1.UserID)
	if f, ok := v1.UserID.(float64); ok {
		c.userID = strconv.FormatInt(int64(f), 10)
	}
	c.passToken = v1.PassToken
	c.ssecurity = v1.Ssecurity
	if v1.Location != "" {
		_ = c.finishAuth(v1.Location) // the service token; the saved passToken is what matters
	}
	return nil
}

// readLoginResponseAny reads a JSON answer with or without the &&&START&&& prefix.
func readLoginResponseAny(res *http.Response, v any) ([]byte, error) {
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	body = bytes.TrimPrefix(bytes.TrimSpace(body), []byte("&&&START&&&"))
	if err = json.Unmarshal(body, v); err != nil {
		return body, fmt.Errorf("xiaomi: %s %.200s", res.Status, body)
	}
	return body, nil
}

func (c *Cloud) finishAuth(location string) error {
	res, err := c.client.Get(location)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// LoginWithVerify
	//   - userId, cUserId, serviceToken from cookies
	//   - passToken from redirect cookies
	//   - ssecurity from extra header
	// LoginWithToken
	//   - userId, cUserId, serviceToken from cookies
	var cUserId, serviceToken string

	for res != nil {
		for _, cookie := range res.Cookies() {
			switch cookie.Name {
			case "userId":
				c.userID = cookie.Value
			case "cUserId":
				cUserId = cookie.Value
			case "serviceToken":
				serviceToken = cookie.Value
			case "passToken":
				c.passToken = cookie.Value
			}
		}

		if s := res.Header.Get("Extension-Pragma"); s != "" {
			var v1 struct {
				Ssecurity []byte `json:"ssecurity"`
			}
			if err = json.Unmarshal([]byte(s), &v1); err != nil {
				return err
			}
			c.ssecurity = v1.Ssecurity
		}

		res = res.Request.Response
	}

	c.cookies = fmt.Sprintf("userId=%s; cUserId=%s; serviceToken=%s", c.userID, cUserId, serviceToken)

	return nil
}

func (c *Cloud) LoginWithToken(userID, passToken string) error {
	req, err := http.NewRequest("GET", "https://account.xiaomi.com/pass/serviceLogin?_json=true&sid="+c.sid, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", fmt.Sprintf("userId=%s; passToken=%s", userID, passToken))

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}

	var v1 struct {
		Ssecurity []byte `json:"ssecurity"`
		PassToken string `json:"passToken"`
		Location  string `json:"location"`
	}
	if _, err = readLoginResponse(res.Body, &v1); err != nil {
		return err
	}

	if v1.Location == "" {
		return errors.New("xiaomi: the saved login is not valid any more, sign in again")
	}

	c.ssecurity = v1.Ssecurity
	c.passToken = v1.PassToken

	return c.finishAuth(v1.Location)
}

func (c *Cloud) UserToken() (string, string) {
	return c.userID, c.passToken
}

func (c *Cloud) Request(baseURL, apiURL, params string, headers map[string]string) ([]byte, error) {
	form := url.Values{"data": {params}}

	nonce := genNonce()
	signedNonce := genSignedNonce(c.ssecurity, nonce)

	// 1. gen hash for data param
	form.Set("rc4_hash__", genSignature64("POST", apiURL, form, signedNonce))

	// 2. encrypt data and hash params
	for _, v := range form {
		ciphertext, err := crypt(signedNonce, []byte(v[0]))
		if err != nil {
			return nil, err
		}
		v[0] = base64.StdEncoding.EncodeToString(ciphertext)
	}

	// 3. add signature for encrypted data and hash params
	form.Set("signature", genSignature64("POST", apiURL, form, signedNonce))

	// 4. add nonce
	form.Set("_nonce", base64.StdEncoding.EncodeToString(nonce))

	req, err := http.NewRequest("POST", baseURL+apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, err
	}

	plaintext, err := crypt(signedNonce, ciphertext)
	if err != nil {
		return nil, err
	}

	var res1 struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err = json.Unmarshal(plaintext, &res1); err != nil {
		return nil, err
	}

	if res1.Code != 0 {
		return nil, errors.New("xiaomi: " + res1.Message)
	}

	return res1.Result, nil
}

// Get calls a GET API of another Mi Home host (e.g. the camera API
// https://de.business.smartcamera.api.io.mi.com/common/app/get/eventlist) with the same
// encryption as Request (signed with the whole path).
// It returns the whole decrypted answer (its fields differ between the hosts).
func (c *Cloud) Get(rawURL, params string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	form := url.Values{"data": {params}}

	nonce := genNonce()
	signedNonce := genSignedNonce(c.ssecurity, nonce)
	path := u.Path

	form.Set("rc4_hash__", genSignature64("GET", path, form, signedNonce))
	for _, v := range form {
		ciphertext, err := crypt(signedNonce, []byte(v[0]))
		if err != nil {
			return nil, err
		}
		v[0] = base64.StdEncoding.EncodeToString(ciphertext)
	}
	form.Set("signature", genSignature64("GET", path, form, signedNonce))
	form.Set("_nonce", base64.StdEncoding.EncodeToString(nonce))
	form.Set("ssecurity", base64.StdEncoding.EncodeToString(c.ssecurity))

	req, err := http.NewRequest("GET", rawURL+"?"+form.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", c.cookies)

	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xiaomi: %s %.200s", res.Status, body)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, fmt.Errorf("xiaomi: %.200s", body)
	}
	return crypt(signedNonce, ciphertext)
}

func readLoginResponse(rc io.ReadCloser, v any) ([]byte, error) {
	defer rc.Close()

	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	body, ok := bytes.CutPrefix(body, []byte("&&&START&&&"))
	if !ok {
		return nil, fmt.Errorf("xiaomi: %s", body)
	}

	return body, json.Unmarshal(body, &v)
}

func genNonce() []byte {
	ts := time.Now().Unix() / 60

	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce[:8])
	binary.BigEndian.PutUint32(nonce[8:], uint32(ts))
	return nonce
}

func genSignedNonce(ssecurity, nonce []byte) []byte {
	hasher := sha256.New()
	hasher.Write(ssecurity)
	hasher.Write(nonce)
	return hasher.Sum(nil)
}

func crypt(key, plaintext []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}

	tmp := make([]byte, 1024)
	cipher.XORKeyStream(tmp, tmp)

	ciphertext := make([]byte, len(plaintext))
	cipher.XORKeyStream(ciphertext, plaintext)

	return ciphertext, nil
}

func genSignature64(method, path string, values url.Values, signedNonce []byte) string {
	s := method + "&" + path + "&data=" + values.Get("data")
	if values.Has("rc4_hash__") {
		s += "&rc4_hash__=" + values.Get("rc4_hash__")
	}
	s += "&" + base64.StdEncoding.EncodeToString(signedNonce)

	hasher := sha1.New()
	hasher.Write([]byte(s))
	signature := hasher.Sum(nil)

	return base64.StdEncoding.EncodeToString(signature)
}

type Request struct {
	Method     string
	URL        string
	RawParams  string
	Body       url.Values
	Headers    url.Values
	RawCookies string
}

func (r Request) Encode() *http.Request {
	if r.RawParams != "" {
		r.URL += "?" + r.RawParams
	}

	var body io.Reader
	if r.Body != nil {
		body = strings.NewReader(r.Body.Encode())
	}

	req, err := http.NewRequest(r.Method, r.URL, body)
	if err != nil {
		return nil
	}

	if r.Headers != nil {
		req.Header = http.Header(r.Headers)
	}
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if r.RawCookies != "" {
		req.Header.Set("Cookie", r.RawCookies)
	}

	return req
}
