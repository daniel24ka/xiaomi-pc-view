// xc-rdtprobe - developer tool: list the SD card files of the first Xiaomi camera of the
// app's config and their event types over the RDT channel (its own camera session: stop the app first).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/xiaomi"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
)

func main() {
	dir, _ := os.UserConfigDir()
	cfg := filepath.Join(dir, "xiaomi-cam-viewer", "go2rtc.yaml")
	b, err := os.ReadFile(cfg)
	if err != nil {
		panic(err)
	}
	src := string(regexp.MustCompile(`xiaomi://\S+`).Find(b))
	os.Args = []string{os.Args[0], "-config", cfg}
	app.Init()
	xiaomi.Init()
	u, err := xiaomi.MissURL(src + "&p2p=server")
	if err != nil {
		panic(err)
	}
	c, err := miss.NewClient(u)
	if err != nil {
		panic(err)
	}
	defer c.Close()
	miss.Debugf = func(format string, args ...any) { fmt.Printf(format+"\n", args...) }
	for range 1 {
		t0 := time.Now()
		files, err := c.SDFiles(10 * time.Second)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Printf("%d files in %v\n", len(files), time.Since(t0))
		if len(files) == 0 {
			continue
		}
		days := map[string]int{}
		var order []string
		var motion int
		for _, f := range files {
			d := time.Unix(f.Start, 0).Format("2006-01-02")
			if days[d] == 0 {
				order = append(order, d)
			}
			days[d]++
			if f.Motion {
				motion++
			}
		}
		fmt.Printf("from %s to %s, motion %d\n", time.Unix(files[0].Start, 0).Format("2006-01-02 15:04:05"),
			time.Unix(files[len(files)-1].Start, 0).Format("2006-01-02 15:04:05"), motion)
		for _, d := range order {
			fmt.Printf("  %s: %d files\n", d, days[d])
		}
		for _, f := range files[len(files)-3:] {
			fmt.Printf("  last: %s +%d\n", time.Unix(f.Start, 0).Format("15:04:05"), f.Duration)
		}
		// event types (command 11): all days, then today
		for _, day := range []string{"", time.Now().Format("20060102")} {
			t0 = time.Now()
			events, err := c.SDEvents(day, 10*time.Second)
			if err != nil {
				fmt.Printf("events %q: error: %v\n", day, err)
				continue
			}
			types := map[int]int{}
			for _, e := range events {
				types[e.Type]++
			}
			fmt.Printf("events %q: %d in %v, by type %v\n", day, len(events), time.Since(t0), types)
			for _, e := range events[max(len(events)-5, 0):] {
				fmt.Printf("  event: %s type %d\n", time.Unix(e.Start, 0).Format("2006-01-02 15:04:05"), e.Type)
			}
		}
	}
}
