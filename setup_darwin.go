//go:build darwin

package main

// 맥 설치: 다운로드 폴더 등에서 AsiaWar.app 을 켜면 응용 프로그램 폴더(/Applications, 안 되면 ~/Applications)로
// 복사하고 거기서 다시 켬. 자동 업데이트는 그 앱 안의 실행 파일을 바꿈.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func appBundle() string {
	d := filepath.Dir(selfExe()) // .../AsiaWar.app/Contents/MacOS
	if filepath.Base(d) != "MacOS" || filepath.Base(filepath.Dir(d)) != "Contents" {
		return ""
	}
	b := filepath.Dir(filepath.Dir(d))
	if !strings.HasSuffix(b, ".app") {
		return ""
	}
	return b
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".kwtest")
	if err != nil {
		return false
	}
	f.Close()
	_ = os.Remove(f.Name())
	return true
}

func maybeInstall(args []string) bool {
	if skipInstall(args) {
		return false
	}
	b := appBundle()
	if b == "" {
		return false
	}
	home, _ := os.UserHomeDir()
	userApps := filepath.Join(home, "Applications")
	if strings.HasPrefix(b, "/Applications/") || strings.HasPrefix(b, userApps+"/") {
		return false
	}
	dest := "/Applications"
	if !writable(dest) {
		_ = os.MkdirAll(userApps, 0o755)
		dest = userApps
	}
	target := filepath.Join(dest, "AsiaWar.app")
	_ = os.RemoveAll(target + ".old")
	_ = os.Rename(target, target+".old")
	if out, err := exec.Command("ditto", b, target).CombinedOutput(); err != nil {
		logger.Printf("응용 프로그램 폴더로 복사 실패: %v %s (그 자리에서 그냥 켬)", err, out)
		_ = os.Rename(target+".old", target)
		return false
	}
	_ = os.RemoveAll(target + ".old")
	_ = exec.Command("xattr", "-dr", "com.apple.quarantine", target).Run()
	logger.Printf("설치함: %s", target)
	if err := exec.Command("open", "-n", target, "--args", "--installed").Run(); err != nil {
		logger.Printf("설치된 앱을 켜지 못함: %v", err)
		return false
	}
	return true
}

// 1.8.1 이름 변경: 응용 프로그램 폴더의 KoreaWar.app 을 AsiaWar.app 으로 (켜져 있어도 이름은 바꿀 수 있음) + 보이는 이름
func renameLegacyBundle() {
	b := appBundle()
	if filepath.Base(b) != "KoreaWar.app" {
		return
	}
	nb := filepath.Join(filepath.Dir(b), "AsiaWar.app")
	if _, err := os.Stat(nb); err == nil {
		return
	}
	pl := filepath.Join(b, "Contents", "Info.plist")
	if raw, err := os.ReadFile(pl); err == nil {
		s := strings.ReplaceAll(string(raw), "Korea War 런처", "Asia War 런처")
		_ = os.WriteFile(pl, []byte(s), 0o644)
	}
	if err := os.Rename(b, nb); err != nil {
		logger.Printf("앱 이름을 바꾸지 못함: %v", err)
		return
	}
	logger.Printf("앱 이름을 바꿈: %s", nb)
}
