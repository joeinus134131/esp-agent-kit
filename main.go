package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"fyne.io/systray"
	autostart "github.com/emersion/go-autostart"
	"github.com/ncruces/zenity"
	"go.bug.st/serial"

	"tokenmeter/pair"
)

var version = "dev" // diisi saat build: -X main.version=1.0.0

const sendEvery = 15 * time.Second

func main() { systray.Run(onReady, func() {}) }

func onReady() {
	systray.SetIcon(icon)
	systray.SetTooltip("TokenMeter")
	status := systray.AddMenuItem("Mencari perangkat...", "")
	status.Disable()
	systray.AddMenuItem("v"+version, "").Disable()
	quit := systray.AddMenuItem("Keluar", "")
	go func() { <-quit.ClickedCh; systray.Quit() }()

	enableAutostart()
	go run(status.SetTitle)
}

// Windows diurus installer, Linux lewat /etc/xdg/autostart. Hanya macOS yang perlu di sini.
func enableAutostart() {
	if runtime.GOOS != "darwin" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	app := &autostart.App{Name: "tokenmeter", DisplayName: "TokenMeter", Exec: []string{exe}}
	if !app.IsEnabled() {
		_ = app.Enable()
	}
}

func run(setStatus func(string)) {
	key, err := loadAgentKey()
	if err != nil {
		log.Fatal(err)
	}
	trust, err := newFileTrust()
	if err != nil {
		log.Fatal(err)
	}
	agent := &pair.Agent{
		Key:       key,
		VendorPub: nil, // isi public key vendor saat produksi
		Trust:     trust,
		Confirm:   confirm,
	}
	var src Source = CcusageSource{}

	for {
		name, err := findPort()
		if err != nil {
			setStatus("Colok perangkat...")
			time.Sleep(2 * time.Second)
			continue
		}
		setStatus("Menghubungkan " + name)

		err = session(agent, src, name, setStatus)
		log.Println("sesi berakhir:", err)
		if errors.Is(err, pair.ErrDeclined) {
			setStatus("Pairing ditolak")
			time.Sleep(30 * time.Second) // jangan spam dialog
		} else {
			setStatus("Terputus, mencoba lagi...")
			time.Sleep(2 * time.Second)
		}
	}
}

func session(a *pair.Agent, src Source, name string, setStatus func(string)) error {
	p, err := serial.Open(name, &serial.Mode{BaudRate: 115200})
	if err != nil {
		return err
	}
	defer p.Close()
	_ = p.SetDTR(false) // cegah reset chip saat port dibuka
	_ = p.SetRTS(false)
	_ = p.SetReadTimeout(500 * time.Millisecond)

	time.Sleep(1500 * time.Millisecond) // beri waktu kalau chip sempat restart
	_ = p.ResetInputBuffer()

	var serialNo string
	for i := 0; i < 3; i++ {
		// _ = p.ResetInputBuffer()
		serialNo, err = a.Handshake(pair.NewConn(p))
		if err == nil || errors.Is(err, pair.ErrDeclined) {
			break
		}
		log.Println("handshake:", err)
	}
	if err != nil {
		return err
	}
	setStatus("Terhubung: " + serialNo)

	for {
		u, err := src.Fetch(context.Background())
		if err != nil {
			log.Println("fetch:", err)
			u = Usage{V: 1, T: "usage"} // tetap kirim idle agar device tidak timeout
		}
		b, _ := json.Marshal(u)
		if _, err := p.Write(append(b, '\n')); err != nil {
			return err // kabel dicabut
		}
		time.Sleep(sendEvery)
	}
}

func confirm(serial, code string) bool {
	err := zenity.Question(
		fmt.Sprintf("Perangkat %s meminta pairing.\n\nKode di layar perangkat: %s\n\n"+
			"Jika kode sama, klik Allow lalu tekan tombol di perangkat (maks. 60 detik).", serial, code),
		zenity.Title("TokenMeter"),
		zenity.OKLabel("Allow"),
		zenity.CancelLabel("Deny"),
		zenity.QuestionIcon,
	)
	return err == nil
}
