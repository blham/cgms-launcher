package main

// 1.7.4 설치 화면 (이용약관 동의 → 단계별 진행률 → 완료). 윈도우 설치(setup_windows.go)가 자체 창으로 띄움.
// 화면은 web/setup.html, 약관 글은 web/terms.html (런처 첫 화면의 약관 창도 같은 글).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var linkRe = regexp.MustCompile(`^https://(discord\.gg|discord\.com|(www\.)?github\.com|modrinth\.com|(www\.)?curseforge\.com|ko-fi\.com|toss\.me|qr\.kakaopay\.com|(www\.)?minecraft\.net)/[A-Za-z0-9/_\-.]+$`)

// 1.2.7 정해진 곳만 브라우저로 엶 (1.7.4 마인크래프트 EULA 추가)
func allowedLink(u string) bool { return linkRe.MatchString(u) }

func acceptTerms() {
	cfgMu.Lock()
	cfg.Terms = TermsVersion
	cfgMu.Unlock()
	saveConfig()
	logger.Printf("이용약관 %s 동의", TermsVersion)
}

type setupStep struct {
	Name string
	Run  func(prog func(f float64)) error
}

type setupUI struct {
	mu     sync.Mutex
	Phase  string   `json:"phase"` // ready | installing | done | error
	Step   int      `json:"step"`
	Pct    float64  `json:"pct"`
	Err    string   `json:"error,omitempty"`
	Steps  []string `json:"steps"`
	Where  string   `json:"where"`
	steps  []setupStep
	token  string
	URL    string `json:"-"`
	srv    *http.Server
	launch chan bool
	closed chan bool
}

func (u *setupUI) snapshot() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	return map[string]any{"phase": u.Phase, "step": u.Step, "pct": u.Pct, "error": u.Err, "steps": u.Steps, "where": u.Where, "version": LauncherVersion}
}

func (u *setupUI) set(f func()) { u.mu.Lock(); f(); u.mu.Unlock() }

func (u *setupUI) phase() string { u.mu.Lock(); defer u.mu.Unlock(); return u.Phase }

func (u *setupUI) run() {
	u.set(func() { u.Phase, u.Err, u.Pct, u.Step = "installing", "", 0, 0 })
	n := float64(len(u.steps))
	for i, s := range u.steps {
		i := i
		u.set(func() { u.Step = i; u.Pct = float64(i) / n * 100 })
		err := s.Run(func(f float64) {
			if f < 0 {
				f = 0
			} else if f > 1 {
				f = 1
			}
			u.set(func() { u.Pct = (float64(i) + f) / n * 100 })
		})
		if err != nil {
			logger.Printf("설치 실패 (%s): %v", s.Name, err)
			u.set(func() { u.Phase, u.Err = "error", s.Name+": "+err.Error() })
			return
		}
	}
	u.set(func() { u.Phase, u.Step, u.Pct = "done", len(u.steps), 100 })
	logger.Printf("설치 화면: 설치 끝")
}

func startSetupUI(steps []setupStep, where string) (*setupUI, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	tb := make([]byte, 16)
	_, _ = rand.Read(tb)
	u := &setupUI{Phase: "ready", steps: steps, token: hex.EncodeToString(tb), Where: where, launch: make(chan bool, 1), closed: make(chan bool, 1)}
	for _, s := range steps {
		u.Steps = append(u.Steps, s.Name)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	u.URL = fmt.Sprintf("http://127.0.0.1:%d/", port)
	okHost := fmt.Sprintf("127.0.0.1:%d", port)
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(v)
	}
	post := func(h func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" || r.Header.Get("X-KW-Token") != u.token {
				http.Error(w, "forbidden", 403)
				return
			}
			h(w, r)
		}
	}
	static := map[string]string{"/terms.html": "text/html; charset=utf-8", "/serif700.woff2": "font/woff2", "/serif900.woff2": "font/woff2",
		"/PretendardVariable.woff2": "font/woff2", "/icon.png": "image/png"}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if ct, ok := static[r.URL.Path]; ok {
			b, err := webFS.ReadFile("web" + r.URL.Path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write(b)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, _ := webFS.ReadFile("web/setup.html")
		nb := make([]byte, 16)
		_, _ = rand.Read(nb)
		nonce := hex.EncodeToString(nb)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'nonce-"+nonce+"'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; object-src 'none'")
		html := strings.Replace(string(b), "__TOKEN__", u.token, 1)
		_, _ = w.Write([]byte(strings.ReplaceAll(html, "__NONCE__", nonce)))
	})
	mux.HandleFunc("/api/setup/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, u.snapshot()) })
	mux.HandleFunc("/api/setup/install", post(func(w http.ResponseWriter, r *http.Request) {
		if p := u.phase(); p == "ready" || p == "error" {
			go u.run()
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("/api/setup/open", post(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			URL string `json:"url"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in)
		if allowedLink(in.URL) {
			openBrowser(in.URL)
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("/api/setup/launch", post(func(w http.ResponseWriter, r *http.Request) {
		select {
		case u.launch <- true:
		default:
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("/api/setup/cancel", post(func(w http.ResponseWriter, r *http.Request) {
		select {
		case u.closed <- true:
		default:
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
	u.srv = &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != okHost { // 다른 사이트가 이 주소로 요청을 보내지 못하게
			http.Error(w, "forbidden", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})}
	go func() { _ = u.srv.Serve(ln) }()
	return u, nil
}

func (u *setupUI) close() { _ = u.srv.Close() }

// 개발용 미리보기: 가짜 단계로 설치 화면만 띄움 (go build -tags dev 후 --setup-preview)
func setupPreview() {
	fake := func(d time.Duration) func(func(float64)) error {
		return func(p func(float64)) error {
			for i := 1; i <= 10; i++ {
				time.Sleep(d / 10)
				p(float64(i) / 10)
			}
			return nil
		}
	}
	u, err := startSetupUI([]setupStep{{"켜져 있는 런처 정리", fake(600 * time.Millisecond)}, {"런처 파일 복사", fake(2 * time.Second)},
		{"바탕화면·시작 메뉴 바로가기", fake(time.Second)}, {"\"앱 및 기능\"에 등록", fake(time.Second)}, {"동의 저장", fake(300 * time.Millisecond)}},
		`C:\Users\이름\AppData\Local\Programs\AsiaWarLauncher`)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("설치 화면 미리보기:", u.URL)
	select {
	case <-u.launch:
	case <-u.closed:
	case <-time.After(30 * time.Minute):
	}
	time.Sleep(300 * time.Millisecond)
}
