//go:build windows

package main

// 1.6.1 윈도우: 엣지 브라우저 창을 따로 띄우지 않고 런처 자체 창 안에 화면을 그림 (WebView2).
// WebView2 는 윈도우 10·11 에 기본으로 깔려 있는 화면 부품이라, 따로 엣지가 뜨거나 뒤에 남는 일이 없고
// 창을 닫으면 런처와 함께 정리됨. WebView2 가 없는 PC 에서만 예전처럼 엣지 앱 창으로.

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	wvMu     sync.Mutex
	wvView   webview2.WebView
	wvFailed bool
	wvOpen   bool

	user32               = windows.NewLazySystemDLL("user32.dll")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procIsIconic         = user32.NewProc("IsIconic")
	procBringWindowToTop = user32.NewProc("BringWindowToTop")

	procSetDpiCtx   = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetDpiAware = user32.NewProc("SetProcessDPIAware")
	procGetDpiSys   = user32.NewProc("GetDpiForSystem")
)

// 1.6.3 화면 배율(125%·150% 등)을 런처가 직접 처리하게 함.
// 이걸 안 하면 윈도우가 작게 그린 화면을 억지로 늘려서 글씨·그림이 전부 흐릿하게 보임.
func init() {
	if procSetDpiCtx.Find() == nil {
		// -4 = 모니터마다 배율을 따로 처리 (v2)
		if r, _, _ := procSetDpiCtx.Call(^uintptr(3)); r != 0 {
			return
		}
	}
	if procSetDpiAware.Find() == nil {
		procSetDpiAware.Call()
	}
}

// 배율 적용한 창 크기 (96 DPI 기준 값을 화면 배율에 맞춰 키움)
func dpiScale(n int) uint {
	dpi := uintptr(96)
	if procGetDpiSys.Find() == nil {
		if d, _, _ := procGetDpiSys.Call(); d != 0 {
			dpi = d
		}
	}
	return uint(n) * uint(dpi) / 96
}

// WebView2 런타임이 깔려 있는지 (레지스트리의 버전 값)
func webview2Installed() bool {
	const id = `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\` + id},
		{registry.LOCAL_MACHINE, `SOFTWARE\` + id},
		{registry.CURRENT_USER, `SOFTWARE\` + id},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && v != "" && v != "0.0.0.0" {
			return true
		}
	}
	return false
}

func openWindow(url string) {
	wvMu.Lock()
	if wvOpen { // 이미 열려 있으면 앞으로 꺼내기만
		w := wvView
		wvMu.Unlock()
		if w != nil {
			w.Dispatch(func() { bringToFront(uintptr(w.Window())) })
		}
		return
	}
	failed := wvFailed
	if !failed && !webview2Installed() {
		failed, wvFailed = true, true
		logger.Printf("WebView2 가 없어 브라우저 앱 창으로 엶")
	}
	if failed {
		wvMu.Unlock()
		openAppWindow(url)
		return
	}
	wvOpen = true
	wvMu.Unlock()

	ready := make(chan bool, 1)
	go func() {
		runtime.LockOSThread() // 창과 메시지 처리는 한 스레드에서
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			DataPath:  filepath.Join(dataDir, "webview2"),
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title: "CGMS Launcher", Width: dpiScale(1180), Height: dpiScale(800), IconId: 1, Center: true,
			},
		})
		if w == nil {
			ready <- false
			return
		}
		w.SetSize(int(dpiScale(900)), int(dpiScale(620)), webview2.HintMin)
		w.Navigate(url)
		wvMu.Lock()
		wvView = w
		wvMu.Unlock()
		winMu.Lock()
		winOpen, usedApp, spawnedAt = true, true, time.Now()
		winMu.Unlock()
		logger.Printf("창: 런처 자체 창 (WebView2)")
		ready <- true
		bringToFront(uintptr(w.Window()))
		w.Run() // 창을 닫을 때까지
		wvMu.Lock()
		wvView, wvOpen = nil, false
		wvMu.Unlock()
		winMu.Lock()
		winOpen = false
		winMu.Unlock()
		logger.Printf("창 닫힘")
		go windowClosed()
	}()
	if !<-ready {
		wvMu.Lock()
		wvOpen, wvFailed = false, true
		wvMu.Unlock()
		logger.Printf("런처 자체 창을 만들지 못해 브라우저 앱 창으로 엶")
		openAppWindow(url)
	}
}

func bringToFront(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		_, _, _ = procShowWindow.Call(hwnd, 9) // SW_RESTORE
	} else {
		_, _, _ = procShowWindow.Call(hwnd, 5) // SW_SHOW
	}
	_, _, _ = procBringWindowToTop.Call(hwnd)
	_, _, _ = procSetForeground.Call(hwnd)
}

// 예전 판(엣지 앱 창)이 남긴 전용 프로필: 그 프로필로 떠 있는 엣지를 끄고 폴더를 지움 (한 번)
func cleanupOldWindowProfile() {
	if webview2Installed() {
		if _, err := os.Stat(windowProfileDir()); err == nil {
			killWindowProfile()
			if os.RemoveAll(windowProfileDir()) == nil {
				logger.Printf("예전 엣지 창 프로필을 정리함")
			}
		}
	}
}
