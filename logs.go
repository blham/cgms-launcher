package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// 1.9.5 문제 신고용 기록: 런처 기록과 게임 기록의 끝부분을 모아 복사하거나 다운로드 폴더에 저장.
// 서버 주소, 로그인 표처럼 보이는 긴 글자, 컴퓨터 사용자 폴더 이름은 가림.
var tokenRe = regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-\.]{10,}|[A-Za-z0-9_\-]{80,}`)

func scrubLog(t string) string {
	t = hideHost(t)
	if home, err := os.UserHomeDir(); err == nil && len(home) > 3 {
		t = strings.ReplaceAll(t, home, "~")
		t = strings.ReplaceAll(t, filepath.ToSlash(home), "~")
	}
	return tokenRe.ReplaceAllString(t, "(가림)")
}

func collectLog() string {
	var b strings.Builder
	kind := ""
	if packaged() {
		kind = " 스토어판"
	}
	fmt.Fprintf(&b, "CGMS Launcher %s%s (%s/%s)\n모은 시각 %s\n", LauncherVersion, kind, runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n===== 런처 기록 (끝 300줄) =====\n")
	b.WriteString(tail(filepath.Join(dataDir, "logs", "launcher.log"), 300))
	// 1.9.9 게임이 켜지다 바로 꺼졌을 때 자바가 남긴 글 (logs/game.log): 1.9.5~1.9.8 은 이걸 빠뜨려 원인을 볼 수 없었음
	b.WriteString("\n\n===== 게임 실행 출력 (끝 80줄) =====\n")
	if g := tail(dirs("logs", "game.log"), 80); g != "" {
		b.WriteString(g)
	} else {
		b.WriteString("(없음)")
	}
	b.WriteString("\n\n===== 게임 기록 (끝 200줄) =====\n")
	if g := tail(filepath.Join(gameDir(), "logs", "latest.log"), 200); g != "" {
		b.WriteString(g)
	} else {
		b.WriteString("(아직 없음)")
	}
	b.WriteString("\n")
	return scrubLog(b.String())
}

// 다운로드 폴더에 글 파일로 저장 (없으면 홈 폴더)
func saveLog() (string, error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Downloads")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		dir = home
	}
	out := filepath.Join(dir, "CGMS-Launcher-log-"+time.Now().Format("20060102-150405")+".txt")
	txt := collectLog()
	if runtime.GOOS == "windows" {
		txt = "\xef\xbb\xbf" + strings.ReplaceAll(strings.ReplaceAll(txt, "\r\n", "\n"), "\n", "\r\n")
	}
	if err := os.WriteFile(out, []byte(txt), 0o644); err != nil {
		return "", fmt.Errorf("파일을 만들지 못했어요: %v", err)
	}
	return out, nil
}
