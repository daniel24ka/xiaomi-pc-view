// xc-xprobe: connects to a Xiaomi camera of the app config and prints the real error.
// Usage: xc-xprobe <config dir> [extra query, e.g. "&p2p=server"] (stop the app first)
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/xccore"
)

func main() {
	dir := os.Args[1]
	if err := xccore.Start(dir); err != nil {
		fmt.Println("start:", err)
		return
	}
	b, _ := os.ReadFile(dir + "/go2rtc.yaml")
	src := regexp.MustCompile(`xiaomi://\S+`).FindString(string(b))
	src = strings.ReplaceAll(src, "&p2p=server", "")
	if len(os.Args) > 2 {
		src += os.Args[2]
	}
	t0 := time.Now()
	p, err := streams.GetProducer(src)
	fmt.Printf("after %.1fs: err=%v\n", time.Since(t0).Seconds(), err)
	if p != nil {
		for _, m := range p.GetMedias() {
			fmt.Println("media:", m.String())
		}
		_ = p.Stop()
	}
	os.Exit(0)
}
