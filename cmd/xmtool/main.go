// xmtool - developer tool: call Mi Home cloud API with the account of the app.
// Usage: xmtool <api path> <json params>
// A path "camera:/common/app/get/eventlist" calls the GET API of the camera host.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
)

func main() {
	dir, _ := os.UserConfigDir()
	cfg := filepath.Join(dir, "xiaomi-cam-viewer", "go2rtc.yaml")
	if c := os.Getenv("XMTOOL_CONFIG"); c != "" { // the portable config next to the app (Windows)
		cfg = c
	}
	b, err := os.ReadFile(cfg)
	if err != nil {
		panic(err)
	}
	src := regexp.MustCompile(`xiaomi://\S+`).Find(b)
	os.Args = []string{os.Args[0], "-config", cfg, os.Args[1], os.Args[2]}
	apiPath, params := os.Args[3], os.Args[4]
	os.Args = os.Args[:3]
	app.Init()
	xiaomi.Init()
	var res []byte
	if path, ok := strings.CutPrefix(apiPath, "camera:"); ok { // GET API of the camera host
		res, err = xiaomi.CloudGet(string(src), path, params)
	} else {
		res, err = xiaomi.CloudRequest(string(src), apiPath, params)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(string(res))
}
