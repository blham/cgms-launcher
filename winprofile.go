package main

// 1.5.8 런처 창(엣지·크롬 "앱 창")이 쓰는 전용 프로필 정리.
// 엣지는 창을 닫아도 그 프로필의 프로세스가 뒤에 남는 일이 있고, 남은 프로세스가 지난 창을 되살리면
// 런처가 꺼진 뒤에도 빈 하얀 창이 생김. 그래서
//  - 창을 띄우기 전과 런처가 끝날 때: 이 프로필(--user-data-dir=...\window)로 켜진 브라우저 프로세스만 끔
//  - 지난 창 되살리기(세션 복원)·뒤에 남기(백그라운드 모드)를 끔

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

func windowProfileDir() string { return filepath.Join(dataDir, "window") }

// 이 런처 전용 프로필로 켜진 브라우저 프로세스만 끔 (다른 엣지·크롬 창은 건드리지 않음)
func killWindowProfile() {
	dir := windowProfileDir()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		pat := strings.ReplaceAll(dir, "'", "''")
		ps := "Get-CimInstance Win32_Process -Filter \"Name='msedge.exe' OR Name='chrome.exe' OR Name='brave.exe'\" | " +
			"Where-Object { $_.CommandLine -and $_.CommandLine.Contains('--user-data-dir=" + pat + "') } | " +
			"ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }"
		// 따옴표가 섞여도 안 깨지게 -EncodedCommand (UTF-16LE base64)
		u := utf16.Encode([]rune(ps))
		raw := make([]byte, len(u)*2)
		for i, c := range u {
			raw[2*i], raw[2*i+1] = byte(c), byte(c>>8)
		}
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	default:
		cmd = exec.Command("pkill", "-f", "--", "--user-data-dir="+dir)
	}
	hideWindow(cmd)
	done := make(chan struct{})
	go func() { _ = cmd.Run(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

// 지난 창을 되살리지 않게: 세션 기록 지우기 + "정상 종료" 표시 + 백그라운드 모드 끔
func cleanWindowProfile() {
	dir := windowProfileDir()
	def := filepath.Join(dir, "Default")
	_ = os.RemoveAll(filepath.Join(def, "Sessions"))
	for _, n := range []string{"Current Session", "Current Tabs", "Last Session", "Last Tabs"} {
		_ = os.Remove(filepath.Join(def, n))
	}
	editJSON(filepath.Join(def, "Preferences"), func(m map[string]any) {
		prof, _ := m["profile"].(map[string]any)
		if prof == nil {
			prof = map[string]any{}
			m["profile"] = prof
		}
		prof["exit_type"] = "Normal"
		prof["exited_cleanly"] = true
		sess, _ := m["session"].(map[string]any)
		if sess == nil {
			sess = map[string]any{}
			m["session"] = sess
		}
		sess["restore_on_startup"] = 5 // 새 탭으로 시작 (지난 창 안 열기)
	})
	editJSON(filepath.Join(dir, "Local State"), func(m map[string]any) {
		bg, _ := m["background_mode"].(map[string]any)
		if bg == nil {
			bg = map[string]any{}
			m["background_mode"] = bg
		}
		bg["enabled"] = false
	})
}

// 있는 JSON 파일만 고침 (처음 켤 땐 브라우저가 만듦)
func editJSON(path string, f func(map[string]any)) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil || m == nil {
		return
	}
	f(m)
	if nb, err := json.Marshal(m); err == nil {
		_ = os.WriteFile(path, nb, 0o600)
	}
}

// 런처가 끝날 때: 창 프로필 정리하고 끝냄
func quitLauncher(code int) {
	killWindowProfile()
	cleanWindowProfile()
	os.Exit(code)
}
