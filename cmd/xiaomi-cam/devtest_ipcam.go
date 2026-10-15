//go:build devtest

package main

import (
	"fmt"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

// XC_IPTEST=<dir>: IP cameras with the simulated camera (cmd/xc-camsim running with the
// default options): adds RTSP, ONVIF and HTTP cameras, plays each, switches the quality,
// takes snapshots, removes the cameras. Back up go2rtc.yaml around it.
func init() {
	dir := os.Getenv("XC_IPTEST")
	if dir == "" {
		return
	}
	simHost := os.Getenv("XC_SIMHOST") // xc-camsim address (not localhost: it needs no login)
	devHook = func(u *ui) {
		shot := func(name string) {
			fyne.DoAndWait(func() {
				img := u.win.Canvas().Capture()
				if f, err := os.Create(filepath.Join(dir, name)); err == nil {
					_ = jpeg.Encode(f, img, nil)
					_ = f.Close()
				}
			})
		}
		state := func(tag string) {
			fyne.DoAndWait(func() {
				fmt.Printf("%s camera=%q quality=%q options=%v conn.visible=%v status=%q frame=%v caps=%+v\n", tag,
					u.camera.Selected, u.quality.Selected, u.quality.Options, u.connection.Visible(),
					u.status.Text, u.gotFrame, u.caps)
			})
		}
		// waitFrame waits for the first picture (at most 20 s)
		waitFrame := func(tag string) {
			start := time.Now()
			time.Sleep(300 * time.Millisecond)
			for time.Since(start) < 20*time.Second {
				var got bool
				fyne.DoAndWait(func() { got = u.gotFrame })
				if got {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			time.Sleep(time.Second)
			state(fmt.Sprintf("%s (%.1fs)", tag, time.Since(start).Seconds()-1))
		}
		time.Sleep(8 * time.Second)
		forms := []struct {
			name string
			f    ipForm
		}{
			{"Test RTSP", ipForm{Kind: "rtsp", Host: simHost, Port: 28554, User: "admin", Pass: "secret", MainPath: "/main", SubPath: "/sub"}},
			{"Test zły login", ipForm{Kind: "rtsp", Host: simHost, Port: 28554, User: "admin", Pass: "bad", MainPath: "/main"}},
			{"Test pełny URL", ipForm{Kind: "rtsp", Host: "rtsp://" + simHost + ":28554/sub", User: "admin", Pass: "secret"}},
			{"Test ONVIF", ipForm{Kind: "onvif", Host: simHost, Port: 21984, User: "admin", Pass: "secret"}},
			{"Test HTTP", ipForm{Kind: "http", Host: simHost, Port: 21984, MainPath: "/api/stream.mjpeg?src=mjpeg"}},
		}
		var added []string
		for i, t := range forms {
			main, sub, err := t.f.sources()
			fmt.Printf("== %s: main=%s sub=%s err=%v\n", t.name, main, sub, err)
			if err != nil {
				continue
			}
			start := time.Now()
			r, err := probeSource(main)
			fmt.Printf("   probe main: %+v err=%v (%.1fs)\n", r, err, time.Since(start).Seconds())
			if err != nil {
				continue
			}
			c := &ipCamera{Kind: t.f.Kind, Main: main, W: r.W, H: r.H, Codec: r.Codec}
			if sub != "" {
				if r, err := probeSource(sub); err == nil {
					c.Sub, c.SubW, c.SubH, c.SubCodec = sub, r.W, r.H, r.Codec
				} else {
					fmt.Printf("   probe sub: err=%v\n", err)
				}
			}
			if err = addIPCamera(t.name, c); err != nil {
				fmt.Println("   add:", err)
				continue
			}
			added = append(added, t.name)
			fyne.Do(func() { u.selectCamera(t.name) })
			waitFrame("   playing")
			shot(fmt.Sprintf("ip%d.jpg", i))
			if c.Sub != "" {
				fyne.Do(func() { u.quality.SetSelectedIndex(1) })
				waitFrame("   sub")
				if hold, _ := time.ParseDuration(os.Getenv("XC_IPHOLD")); hold > 0 {
					fmt.Println("   holding", hold)
					time.Sleep(hold)
				}
				shot(fmt.Sprintf("ip%d_sub.jpg", i))
				fmt.Println("   config:", ipCam(t.name).Quality, streamSource(t.name))
			}
			b, err := snapshotJPEG(t.name, 0)
			fmt.Printf("   snapshot: %d bytes err=%v\n", len(b), err)
			if rec, err := startRecording(t.name, 4*time.Second); err != nil {
				fmt.Println("   record:", err)
			} else {
				<-rec.done
				out, _ := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name,width:format=duration", "-of", "compact", rec.path).CombinedOutput()
				fmt.Printf("   recorded %s: %s\n", filepath.Base(rec.path), strings.ReplaceAll(strings.TrimSpace(string(out)), "\n", " | "))
				_ = os.Remove(rec.path)
			}
		}
		fmt.Println("cameras:", cameraNames())
		// rename keeps the IP camera settings
		if len(added) > 0 {
			fyne.DoAndWait(func() { u.player.stop() })
			old := added[0]
			c := ipCam(old)
			err := addIPCamera(old+" 2", c)
			if err == nil {
				err = removeCamera(old)
			}
			added[0] = old + " 2"
			fmt.Println("rename:", err, ipCam(old) == nil, ipCam(old+" 2") != nil)
		}
		for _, n := range added {
			fmt.Println("remove", n, removeCamera(n))
		}
		fyne.DoAndWait(func() {
			fmt.Printf("before select: selected=%q options=%v syncing=%v visible=%v grid=%v\n", u.camera.Selected, u.camera.Options, u.syncing, u.visible, u.grid)
			u.camerasChanged()
			if len(u.camera.Options) > 0 {
				u.camera.SetSelectedIndex(0)
			}
		})
		state("after select")
		time.Sleep(8 * time.Second)
		state("back to Xiaomi")
		fmt.Println("cameras:", cameraNames())
		fyne.Do(u.app.Quit)
	}
}
