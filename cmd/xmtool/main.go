// xmtool - developer tool: call Mi Home cloud API with the account of the app.
// Usage: xmtool <api path> <json params>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
)

func main() {
	dir, _ := os.UserConfigDir()
	cfg := filepath.Join(dir, "xiaomi-cam-viewer", "go2rtc.yaml")
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
	res, err := xiaomi.CloudRequest(string(src), apiPath, params)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(string(res))
}
