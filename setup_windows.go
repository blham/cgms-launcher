//go:build windows

package main

// 설치형 (윈도우): 받은 exe(KoreaWarSetup.exe 등)를 켜면 %LOCALAPPDATA%\Programs\KoreaWarLauncher 에 복사하고
// 바탕화면·시작 메뉴 바로가기 + "앱 및 기능" 제거 항목을 만든 뒤, 설치된 런처를 켬.
// 설치된 런처는 켤 때마다 서버에서 새 버전을 확인해 스스로 바꿈 (update.go).

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
)

const appName = "CGMS Launcher"

func installDir() string {
	la := os.Getenv("LOCALAPPDATA")
	if la == "" {
		home, _ := os.UserHomeDir()
		la = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(la, "Programs", "KoreaWarLauncher")
}

func installedExe() string { return filepath.Join(installDir(), "KoreaWarLauncher.exe") }

func selfExe() string {
	exe, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe
}

func msgBox(text string, flags uint32) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(appName)
	r, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), uintptr(flags))
	return int(r)
}

// 파워셸 스크립트 실행 (한글·따옴표 걱정 없게 -EncodedCommand)
func powershell(script string) error {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i], b[2*i+1] = byte(v), byte(v>>8)
	}
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	hideWindow(c)
	out, err := c.CombinedOutput()
	if err != nil {
		logger.Printf("powershell 실패: %v %s", err, out)
	}
	return err
}

func psq(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// 바로가기 + 제거 항목
func writeShortcuts(target string) { makeShortcuts(target); registerUninstall(target) }

func makeShortcuts(target string) {
	dir := filepath.Dir(target)
	_ = powershell(fmt.Sprintf(`
$ws = New-Object -ComObject WScript.Shell
foreach ($d in @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('Programs'))) {
  Remove-Item -LiteralPath (Join-Path $d 'Korea War 런처.lnk') -ErrorAction SilentlyContinue
  $l = $ws.CreateShortcut((Join-Path $d %s))
  $l.TargetPath = %s; $l.WorkingDirectory = %s; $l.IconLocation = %s; $l.Description = %s
  $l.Save()
}`, psq(appName+".lnk"), psq(target), psq(dir), psq(target+",0"), psq("Korea War 서버 전용 런처")))
}

func registerUninstall(target string) {
	var size int64
	if st, err := os.Stat(target); err == nil {
		size = st.Size() / 1024
	}
	_ = powershell(fmt.Sprintf(`
$k = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\KoreaWarLauncher'
New-Item -Path $k -Force | Out-Null
Set-ItemProperty $k DisplayName %s
Set-ItemProperty $k DisplayIcon %s
Set-ItemProperty $k DisplayVersion %s
Set-ItemProperty $k Publisher 'CGMS'
Set-ItemProperty $k InstallLocation %s
Set-ItemProperty $k UninstallString %s
New-ItemProperty $k EstimatedSize -Value %d -PropertyType DWord -Force | Out-Null
New-ItemProperty $k NoModify -Value 1 -PropertyType DWord -Force | Out-Null
New-ItemProperty $k NoRepair -Value 1 -PropertyType DWord -Force | Out-Null`,
		psq(appName), psq(target+",0"), psq(LauncherVersion), psq(filepath.Dir(target)), psq(`"`+target+`" --uninstall`), size))
}

func copyFile(src, dst string) error { return copyFileProgress(src, dst, nil) }

// 1.7.4 진행률을 알리며 복사 (설치 화면 막대)
func copyFileProgress(src, dst string, prog func(float64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	var total int64 = 1
	if st, err := in.Stat(); err == nil && st.Size() > 0 {
		total = st.Size()
	}
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	buf := make([]byte, 256<<10)
	var done int64
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
			done += int64(n)
			if prog != nil {
				prog(float64(done) / float64(total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	out.Close()
	_ = os.Remove(dst + ".old")
	_ = os.Rename(dst, dst+".old") // 켜져 있는 예전 런처도 이름은 바꿀 수 있음
	return os.Rename(tmp, dst)
}

// 설치된 자리가 아니면 설치하고 설치된 런처를 켬. true = 이 프로세스는 끝내야 함
func maybeInstall(args []string) bool {
	for _, a := range args {
		switch a {
		case "--uninstall":
			uninstall()
			return true
		case "--portable":
			return false
		}
	}
	if devBuild { // 개발·시험용 빌드는 설치하지 않고 그 자리에서 켬 (진짜 런처를 덮어쓰지 않게)
		return false
	}
	exe, target := selfExe(), installedExe()
	if strings.EqualFold(filepath.Clean(exe), filepath.Clean(target)) {
		return false
	}
	// 1.7.4 설치 화면 (약관 동의 → 진행률). WebView2 가 없으면 예전처럼 알림 창으로
	if webview2Installed() {
		if ok := runSetupWindow(exe, target); ok {
			return true
		}
	}
	if cfg.Terms != TermsVersion {
		if msgBox(appName+" 를 설치해요.\n\n"+termsSummary+"\n\n이용약관과 개인정보 수집·이용에 동의하고 설치할까요?\n(전문은 설치 뒤 런처 설정에서 볼 수 있어요)", 0x4|0x40) != 6 {
			return true
		}
		acceptTerms()
	}
	_ = os.MkdirAll(installDir(), 0o755)
	if err := copyFile(exe, target); err != nil {
		msgBox("설치하지 못했어요: "+err.Error()+"\n\n런처가 켜져 있다면 끄고 다시 해 주세요. (작업 관리자에서 KoreaWarLauncher 끝내기)", 0x10)
		return true
	}
	writeShortcuts(target)
	logger.Printf("설치함: %s", target)
	stopRunningLauncher()
	c := exec.Command(target, "--installed")
	c.Dir = installDir()
	detach(c)
	if err := c.Start(); err != nil {
		msgBox("설치는 됐지만 켜지 못했어요. 바탕화면의 \""+appName+"\" 를 눌러 주세요.", 0x30)
	}
	return true
}

func uninstall() {
	if msgBox(appName+"를 이 컴퓨터에서 지울까요?", 0x4|0x20) != 6 {
		return
	}
	data := msgBox("게임 자료(로그인 정보·마인크래프트·모드·스크린샷)도 같이 지울까요?\n\n[아니요]를 누르면 다시 설치했을 때 그대로 쓸 수 있어요.", 0x4|0x20|0x100) == 6
	// 켜져 있는 런처 끝내기 (창이 떠 있으면)
	quitRunning()
	_ = powershell(fmt.Sprintf(`
foreach ($d in @([Environment]::GetFolderPath('Desktop'), [Environment]::GetFolderPath('Programs'))) { Remove-Item -LiteralPath (Join-Path $d %s) -Force -ErrorAction SilentlyContinue }
Remove-Item -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\KoreaWarLauncher' -Recurse -Force -ErrorAction SilentlyContinue`, psq(appName+".lnk")))
	del := `ping 127.0.0.1 -n 3 > nul & rmdir /s /q "` + installDir() + `"`
	if data {
		del += ` & rmdir /s /q "` + dataDir + `"`
	}
	c := exec.Command("cmd", "/c", del)
	hideWindow(c)
	detach(c)
	_ = c.Start()
	time.Sleep(200 * time.Millisecond)
	msgBox(appName+"를 지웠어요.", 0x40)
}

// 설치된 런처가 새 버전으로 바뀐 뒤: 제거 항목의 버전 갱신
func afterUpdate() { registerUninstall(selfExe()) }

// 1.2.4 예전 런처가 켜져 있으면(게임 중이 아니면) 끄고 포트가 빌 때까지 기다림 → 새로 설치한 런처가 제대로 켜지게
func stopRunningLauncher() {
	addr := fmt.Sprintf("127.0.0.1:%d", uiPort)
	if c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err != nil {
		return
	} else {
		c.Close()
	}
	logger.Printf("켜져 있는 예전 런처를 끔")
	quitRunning()
	for i := 0; i < 25; i++ {
		time.Sleep(200 * time.Millisecond)
		c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err != nil {
			return
		}
		c.Close()
	}
}

func quitRunning() {
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/quit-uninstall", uiPort), nil)
	req.Header.Set("X-KW-Token", runningToken())
	if r, err := httpc.Do(req); err == nil {
		r.Body.Close()
	}
}

// 알림 창용 짧은 약관 (WebView2 가 없을 때)
const termsSummary = "· Mojang·Microsoft 와 관계없는 CGMS 서버 전용 런처예요. 정품 계정이 필요하고 마인크래프트 EULA 를 지켜야 해요.\n" +
	"· 마인크래프트·자바·모드(약 700MB)를 받고 스스로 새 버전으로 바뀌어요.\n" +
	"· 서버 규칙을 어기면 제재받을 수 있어요. 실명을 써 주세요.\n" +
	"· 모으는 것: 마인크래프트 닉네임·UUID, 실명, (연결하면) 디스코드 아이디, 접속 시간·IP. 입장 확인·이름 표시·제재에만 쓰고, 요청하면 지워요.\n" +
	"· 만 14세 미만은 보호자 동의가 필요해요."

// 1.7.4 설치 화면 창. false = 창을 못 띄움 (알림 창 방식으로)
func runSetupWindow(exe, target string) bool {
	steps := []setupStep{
		{"켜져 있는 예전 런처 정리", func(p func(float64)) error { stopRunningLauncher(); return nil }},
		{"런처 파일 복사", func(p func(float64)) error {
			if err := os.MkdirAll(installDir(), 0o755); err != nil {
				return err
			}
			if err := copyFileProgress(exe, target, p); err != nil {
				return fmt.Errorf("%v (런처가 켜져 있다면 작업 관리자에서 KoreaWarLauncher 를 끝내고 다시 해 주세요)", err)
			}
			return nil
		}},
		{"바탕화면·시작 메뉴 바로가기", func(p func(float64)) error { makeShortcuts(target); return nil }},
		{"\"앱 및 기능\"에 등록 (나중에 지울 수 있게)", func(p func(float64)) error { registerUninstall(target); return nil }},
		{"이용약관 동의 저장", func(p func(float64)) error { acceptTerms(); return nil }},
	}
	u, err := startSetupUI(steps, installDir())
	if err != nil {
		logger.Printf("설치 화면을 못 띄움: %v", err)
		return false
	}
	defer u.close()
	runtime.LockOSThread()
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(dataDir, "webview2-setup"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: appName + " 설치", Width: dpiScale(780), Height: dpiScale(640), IconId: 1, Center: true,
		},
	})
	if w == nil {
		return false
	}
	go func() {
		select {
		case <-u.launch:
		case <-u.closed:
		}
		w.Dispatch(func() { w.Terminate() })
	}()
	w.SetSize(int(dpiScale(640)), int(dpiScale(560)), webview2.HintMin)
	w.Navigate(u.URL)
	bringToFront(uintptr(w.Window()))
	w.Run()
	w.Destroy()
	// 설치하는 중에 창을 닫았으면 끝날 때까지 기다림 (반쯤 복사된 채로 끝나지 않게)
	for i := 0; i < 300 && u.phase() == "installing"; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if u.phase() != "done" {
		logger.Printf("설치 화면: 설치하지 않고 닫음")
		return true
	}
	logger.Printf("설치함: %s", target)
	c := exec.Command(target, "--installed")
	c.Dir = installDir()
	detach(c)
	if err := c.Start(); err != nil {
		msgBox("설치는 됐지만 켜지 못했어요. 바탕화면의 \""+appName+"\" 를 눌러 주세요.", 0x30)
	}
	return true
}
