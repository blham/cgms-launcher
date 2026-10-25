package main

// 직접 실행 모드: 모장 승인을 받은 앱 ID(client id)가 서명된 서버 정보에 있을 때만 씀.
// 앱 ID가 없으면 공식 런처 연동(official.go)으로 실행함.
// 게임 파일(마인크래프트·Fabric·라이브러리·애셋·자바)을 받아 이 런처의 폴더에 두고 직접 켬.

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	gameCmd   *exec.Cmd
	gameMu    sync.Mutex
	gameStart time.Time
)

func directRunning() bool {
	gameMu.Lock()
	defer gameMu.Unlock()
	return gameCmd != nil
}

// ---------------------------------------------------------------- 버전 JSON
type Rule struct {
	Action string `json:"action"`
	OS     *struct {
		Name         string `json:"name"`
		Arch         string `json:"arch"`
		VersionRange *struct {
			Min string `json:"min"`
			Max string `json:"max"`
		} `json:"versionRange"`
	} `json:"os"`
	Features map[string]bool `json:"features"`
}

type Artifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Sha1 string `json:"sha1"`
	Size int64  `json:"size"`
	ID   string `json:"id"`
}

type Library struct {
	Name      string `json:"name"`
	URL       string `json:"url"` // Fabric: 메이븐 주소
	Sha1      string `json:"sha1"`
	Size      int64  `json:"size"`
	Downloads *struct {
		Artifact    *Artifact            `json:"artifact"`
		Classifiers map[string]*Artifact `json:"classifiers"`
	} `json:"downloads"`
	Natives map[string]string `json:"natives"`
	Rules   []Rule            `json:"rules"`
}

type VersionJSON struct {
	ID           string `json:"id"`
	InheritsFrom string `json:"inheritsFrom"`
	MainClass    string `json:"mainClass"`
	Type         string `json:"type"`
	Arguments    struct {
		Game       []json.RawMessage `json:"game"`
		JVM        []json.RawMessage `json:"jvm"`
		DefaultJVM []json.RawMessage `json:"default-user-jvm"`
	} `json:"arguments"`
	AssetIndex struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		Sha1 string `json:"sha1"`
	} `json:"assetIndex"`
	Downloads struct {
		Client Artifact `json:"client"`
	} `json:"downloads"`
	JavaVersion struct {
		Component    string `json:"component"`
		MajorVersion int    `json:"majorVersion"`
	} `json:"javaVersion"`
	Logging struct {
		Client struct {
			Argument string   `json:"argument"`
			File     Artifact `json:"file"`
		} `json:"client"`
	} `json:"logging"`
	Libraries []Library `json:"libraries"`
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "osx"
	}
	return "linux"
}

// 규칙: 맞는 규칙의 마지막 action (규칙이 있으면 기본은 안 됨)
func rulesAllow(rules []Rule, feats map[string]bool) bool {
	if len(rules) == 0 {
		return true
	}
	ok := false
	for _, r := range rules {
		match := true
		if r.OS != nil {
			if r.OS.Name != "" && r.OS.Name != osName() {
				match = false
			}
			if r.OS.Arch != "" {
				a := runtime.GOARCH
				if !(r.OS.Arch == "x86" && a == "386") && r.OS.Arch != a {
					match = false
				}
			}
			if r.OS.VersionRange != nil && r.OS.VersionRange.Max != "" && r.OS.VersionRange.Min == "" {
				match = false // 아주 옛 윈도용 (10.0.17134 이전) 은 안 씀
			}
		}
		for k, v := range r.Features {
			if feats[k] != v {
				match = false
			}
		}
		if match {
			ok = r.Action == "allow"
		}
	}
	return ok
}

// "group:artifact:version[:classifier]" -> 메이븐 경로
func mavenPath(name string) string {
	p := strings.Split(name, ":")
	if len(p) < 3 {
		return ""
	}
	g := strings.ReplaceAll(p[0], ".", "/")
	file := p[1] + "-" + p[2]
	if len(p) > 3 {
		file += "-" + p[3]
	}
	return g + "/" + p[1] + "/" + p[2] + "/" + file + ".jar"
}

// 같은 라이브러리(그룹:이름[:분류])의 다른 버전이 겹치면 자식(Fabric) 것을 씀
func libKey(name string) string {
	p := strings.Split(name, ":")
	if len(p) >= 4 {
		return p[0] + ":" + p[1] + ":" + p[3]
	}
	if len(p) >= 2 {
		return p[0] + ":" + p[1]
	}
	return name
}

func mergeVersions(child, parent *VersionJSON) *VersionJSON {
	v := *parent
	v.ID = child.ID
	if child.MainClass != "" {
		v.MainClass = child.MainClass
	}
	seen := map[string]bool{}
	var libs []Library
	for _, l := range child.Libraries {
		seen[libKey(l.Name)] = true
		libs = append(libs, l)
	}
	for _, l := range parent.Libraries {
		if !seen[libKey(l.Name)] {
			libs = append(libs, l)
		}
	}
	v.Libraries = libs
	v.Arguments.Game = append(append([]json.RawMessage{}, parent.Arguments.Game...), child.Arguments.Game...)
	v.Arguments.JVM = append(append([]json.RawMessage{}, parent.Arguments.JVM...), child.Arguments.JVM...)
	return &v
}

// ---------------------------------------------------------------- 준비 (받기·업데이트)
type Prepared struct {
	Version   *VersionJSON
	ClientJar string
	Classpath []string
	JavaPath  string
	Natives   string
	LogConfig string
	Manifest  *Manifest
}


func prepare() (*Prepared, error) {
	setStatus("prepare", "서버 정보 확인 중", 0)
	man, err := fetchManifest()
	if err != nil {
		return nil, err
	}
	mcv := man.Minecraft
	setStatus("prepare", "마인크래프트 "+mcv+" 정보 받는 중", 0)
	var vm struct {
		Versions []struct {
			ID   string `json:"id"`
			URL  string `json:"url"`
			Sha1 string `json:"sha1"`
		} `json:"versions"`
	}
	vmPath := dirs("versions", "version_manifest_v2.json")
	if err := getJSON("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &vm); err == nil {
		b, _ := json.Marshal(vm)
		_ = os.MkdirAll(filepath.Dir(vmPath), 0o755)
		_ = os.WriteFile(vmPath, b, 0o644)
	} else if b, e2 := os.ReadFile(vmPath); e2 == nil {
		_ = json.Unmarshal(b, &vm)
	} else {
		return nil, fmt.Errorf("모장 서버에 연결하지 못했어요: %v", err)
	}
	var vurl, vsha string
	for _, v := range vm.Versions {
		if v.ID == mcv {
			vurl, vsha = v.URL, v.Sha1
		}
	}
	if vurl == "" {
		return nil, fmt.Errorf("마인크래프트 %s 버전을 찾지 못했어요", mcv)
	}
	var base VersionJSON
	if err := cachedJSON(vurl, dirs("versions", mcv, mcv+".json"), vsha, &base); err != nil {
		return nil, fmt.Errorf("버전 정보를 받지 못했어요: %v", err)
	}
	ver := &base
	if man.FabricLoader != "" {
		var fab VersionJSON
		if !verRe.MatchString(man.FabricLoader) || !verRe.MatchString(mcv) { // 1.2.7 경로·주소에 들어가므로
			return nil, fmt.Errorf("서버가 알려 준 버전 글자가 이상해요")
		}
		fid := "fabric-loader-" + man.FabricLoader + "-" + mcv
		fpath := dirs("versions", fid, fid+".json")
		furl := "https://meta.fabricmc.net/v2/versions/loader/" + mcv + "/" + man.FabricLoader + "/profile/json"
		if _, err := os.Stat(fpath); err != nil {
			if err := download(dlTask{URL: furl, Path: fpath}); err != nil {
				return nil, fmt.Errorf("Fabric 정보를 받지 못했어요: %v", err)
			}
		}
		b, _ := os.ReadFile(fpath)
		if err := json.Unmarshal(b, &fab); err != nil {
			return nil, err
		}
		ver = mergeVersions(&fab, &base)
	}

	// 게임 jar + 라이브러리
	feats := map[string]bool{}
	var tasks []dlTask
	clientJar := dirs("versions", mcv, mcv+".jar")
	tasks = append(tasks, dlTask{URL: base.Downloads.Client.URL, Path: clientJar, Sha1: base.Downloads.Client.Sha1, Size: base.Downloads.Client.Size})
	var cp []string
	natives := dirs("natives", ver.ID)
	type extract struct{ jar string }
	var toExtract []string
	for _, l := range ver.Libraries {
		if !rulesAllow(l.Rules, feats) {
			continue
		}
		if l.Downloads != nil && l.Downloads.Artifact != nil && l.Downloads.Artifact.URL != "" {
			a := l.Downloads.Artifact
			p := dirs("libraries", filepath.FromSlash(a.Path))
			tasks = append(tasks, dlTask{URL: a.URL, Path: p, Sha1: a.Sha1, Size: a.Size})
			cp = append(cp, p)
		} else if l.Downloads == nil || l.Downloads.Artifact == nil {
			mp := mavenPath(l.Name)
			if mp == "" {
				continue
			}
			base := l.URL
			if base == "" {
				base = "https://libraries.minecraft.net/"
			}
			if !strings.HasSuffix(base, "/") {
				base += "/"
			}
			p := dirs("libraries", filepath.FromSlash(mp))
			tasks = append(tasks, dlTask{URL: base + mp, Path: p, Sha1: l.Sha1, Size: l.Size})
			cp = append(cp, p)
		}
		// 옛 방식 네이티브 (분류 jar 를 풀어 씀)
		if l.Natives != nil && l.Downloads != nil {
			if cls, ok := l.Natives[osName()]; ok {
				cls = strings.ReplaceAll(cls, "${arch}", map[bool]string{true: "64", false: "32"}[strings.HasSuffix(runtime.GOARCH, "64")])
				if a := l.Downloads.Classifiers[cls]; a != nil {
					p := dirs("libraries", filepath.FromSlash(a.Path))
					tasks = append(tasks, dlTask{URL: a.URL, Path: p, Sha1: a.Sha1, Size: a.Size})
					toExtract = append(toExtract, p)
				}
			}
		}
	}
	if err := downloadAll("게임 파일 받는 중", tasks, 8); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(natives, 0o755)
	for _, j := range toExtract {
		_ = unzipNatives(j, natives)
	}

	// 애셋
	setStatus("prepare", "애셋 목록 받는 중", 0)
	var idx struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
			Size int64  `json:"size"`
		} `json:"objects"`
	}
	if err := cachedJSON(ver.AssetIndex.URL, dirs("assets", "indexes", ver.AssetIndex.ID+".json"), ver.AssetIndex.Sha1, &idx); err != nil {
		return nil, fmt.Errorf("애셋 목록을 받지 못했어요: %v", err)
	}
	seenHash := map[string]bool{}
	var atasks []dlTask
	for _, o := range idx.Objects {
		if seenHash[o.Hash] {
			continue
		}
		seenHash[o.Hash] = true
		atasks = append(atasks, dlTask{URL: "https://resources.download.minecraft.net/" + o.Hash[:2] + "/" + o.Hash,
			Path: dirs("assets", "objects", o.Hash[:2], o.Hash), Sha1: o.Hash, Size: o.Size, TrustSize: true})
	}
	if err := downloadAll("소리·언어 파일 받는 중", atasks, 16); err != nil {
		return nil, err
	}
	logCfg := ""
	if ver.Logging.Client.File.URL != "" {
		logCfg = dirs("assets", "log_configs", ver.Logging.Client.File.ID)
		_ = download(dlTask{URL: ver.Logging.Client.File.URL, Path: logCfg, Sha1: ver.Logging.Client.File.Sha1})
	}

	// 자바
	javaPath, err := ensureJava(ver.JavaVersion.Component)
	if err != nil {
		return nil, err
	}

	// 보조 모드
	if err := syncMods(man); err != nil {
		return nil, err
	}
	writeGameDefaults()

	cp = append(cp, clientJar)
	return &Prepared{Version: ver, ClientJar: clientJar, Classpath: cp, JavaPath: javaPath, Natives: natives, LogConfig: logCfg, Manifest: man}, nil
}

// ---------------------------------------------------------------- 자바 (모장이 게임용으로 주는 런타임)
func javaPlatform() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return "windows-arm64"
		}
		if runtime.GOARCH == "386" {
			return "windows-x86"
		}
		return "windows-x64"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "mac-os-arm64"
		}
		return "mac-os"
	}
	if runtime.GOARCH == "386" {
		return "linux-i386"
	}
	return "linux"
}

func ensureJava(component string) (string, error) {
	if component == "" {
		component = "java-runtime-epsilon"
	}
	root := dirs("runtime", component, javaPlatform())
	marker := filepath.Join(root, ".kw-ok")
	javaExe := func() string {
		cands := []string{filepath.Join(root, "bin", "javaw.exe"), filepath.Join(root, "bin", "java.exe"),
			filepath.Join(root, "jre.bundle", "Contents", "Home", "bin", "java"), filepath.Join(root, "bin", "java")}
		for _, c := range cands {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
		return ""
	}
	setStatus("prepare", "자바 확인 중", 0)
	var all map[string]map[string][]struct {
		Manifest struct {
			URL  string `json:"url"`
			Sha1 string `json:"sha1"`
		} `json:"manifest"`
		Version struct {
			Name string `json:"name"`
		} `json:"version"`
	}
	if err := getJSON("https://launchermeta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json", &all); err != nil {
		if j := javaExe(); j != "" {
			return j, nil // 인터넷이 안 되면 받아 둔 것
		}
		return "", fmt.Errorf("자바 정보를 받지 못했어요: %v", err)
	}
	list := all[javaPlatform()][component]
	if len(list) == 0 {
		return "", fmt.Errorf("이 컴퓨터(%s)용 자바(%s)가 없어요", javaPlatform(), component)
	}
	mf := list[0].Manifest
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == mf.Sha1 {
		if j := javaExe(); j != "" {
			return j, nil
		}
	}
	var files struct {
		Files map[string]struct {
			Type       string `json:"type"`
			Executable bool   `json:"executable"`
			Target     string `json:"target"`
			Downloads  struct {
				Raw Artifact `json:"raw"`
			} `json:"downloads"`
		} `json:"files"`
	}
	if err := getJSON(mf.URL, &files); err != nil {
		return "", fmt.Errorf("자바 파일 목록을 받지 못했어요: %v", err)
	}
	var tasks []dlTask
	for p, f := range files.Files {
		full := filepath.Join(root, filepath.FromSlash(p))
		switch f.Type {
		case "directory":
			_ = os.MkdirAll(full, 0o755)
		case "file":
			tasks = append(tasks, dlTask{URL: f.Downloads.Raw.URL, Path: full, Sha1: f.Downloads.Raw.Sha1, Size: f.Downloads.Raw.Size, Exec: f.Executable})
		}
	}
	if err := downloadAll("자바 받는 중 ("+list[0].Version.Name+")", tasks, 12); err != nil {
		return "", err
	}
	for p, f := range files.Files { // 링크 (맥·리눅스)
		if f.Type == "link" && runtime.GOOS != "windows" {
			full := filepath.Join(root, filepath.FromSlash(p))
			_ = os.Remove(full)
			_ = os.MkdirAll(filepath.Dir(full), 0o755)
			_ = os.Symlink(f.Target, full)
		}
	}
	j := javaExe()
	if j == "" {
		return "", errors.New("자바를 받았지만 실행 파일을 찾지 못했어요")
	}
	_ = os.WriteFile(marker, []byte(mf.Sha1), 0o644)
	return j, nil
}

// ---------------------------------------------------------------- 보조 모드 (서버의 plugins/KoreaWar/mod)
func unzipNatives(jar, dest string) error {
	r, err := zip.OpenReader(jar)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "META-INF") {
			continue
		}
		name := filepath.Base(f.Name)
		rc, err := f.Open()
		if err != nil {
			continue
		}
		out, err := os.Create(filepath.Join(dest, name))
		if err == nil {
			_, _ = io.Copy(out, rc)
			out.Close()
		}
		rc.Close()
	}
	return nil
}

var placeholder = regexp.MustCompile(`\$\{([a-zA-Z_]+)\}`)

func subst(s string, vars map[string]string) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		k := m[2 : len(m)-1]
		if v, ok := vars[k]; ok {
			return v
		}
		return m
	})
}

// 버전 JSON 의 arguments 항목: 글자 하나, 또는 {rules, value(글자 또는 목록)}
func expandArgs(raw []json.RawMessage, feats map[string]bool, vars map[string]string) []string {
	var out []string
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, subst(s, vars))
			continue
		}
		var obj struct {
			Rules []Rule          `json:"rules"`
			Value json.RawMessage `json:"value"`
		}
		if json.Unmarshal(r, &obj) != nil || !rulesAllow(obj.Rules, feats) {
			continue
		}
		var one string
		if json.Unmarshal(obj.Value, &one) == nil {
			out = append(out, subst(one, vars))
			continue
		}
		var many []string
		if json.Unmarshal(obj.Value, &many) == nil {
			for _, m := range many {
				out = append(out, subst(m, vars))
			}
		}
	}
	return out
}

func launchGame(p *Prepared) error {
	a := currentAccount()
	if a == nil {
		return fmt.Errorf("먼저 로그인해 주세요")
	}
	c := getCfg()
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	gd := gameDir()
	vars := map[string]string{
		"auth_player_name":     a.Name,
		"version_name":         p.Version.ID,
		"game_directory":       gd,
		"assets_root":          dirs("assets"),
		"assets_index_name":    p.Version.AssetIndex.ID,
		"auth_uuid":            a.UUID,
		"auth_access_token":    a.McToken,
		"clientid":             a.ClientID,
		"auth_xuid":            a.Xuid,
		"user_type":            "msa",
		"version_type":         "release",
		"natives_directory":    p.Natives,
		"launcher_name":        "CGMSLauncher",
		"launcher_version":     LauncherVersion,
		"classpath":            strings.Join(p.Classpath, sep),
		"classpath_separator":  sep,
		"library_directory":    dirs("libraries"),
		"quickPlayMultiplayer": serverAddr(),
		"path":                 p.LogConfig,
	}
	feats := map[string]bool{"is_quick_play_multiplayer": c.AutoJoin}
	for _, d := range []string{"java", "jna", "lwjgl", "netty"} {
		_ = os.MkdirAll(filepath.Join(p.Natives, d), 0o755)
	}
	// 기본 JVM 옵션 (메모리는 런처 설정으로)
	var jvm []string
	for _, s := range expandArgs(p.Version.Arguments.DefaultJVM, feats, vars) {
		if strings.HasPrefix(s, "-Xmx") || strings.HasPrefix(s, "-Xms") || strings.HasPrefix(s, "-XX:+AlwaysPreTouch") {
			continue
		}
		jvm = append(jvm, s)
	}
	ms := c.MemoryMB / 2
	if ms > 2048 {
		ms = 2048
	}
	jvm = append([]string{fmt.Sprintf("-Xmx%dM", c.MemoryMB), fmt.Sprintf("-Xms%dM", ms), "-XX:-OmitStackTraceInFastThrow"}, jvm...) // 1.5.6 튕길 때 오류 위치가 크래시 리포트에 남게
	if p.LogConfig != "" && p.Version.Logging.Client.Argument != "" {
		jvm = append(jvm, subst(p.Version.Logging.Client.Argument, vars))
	}
	jvmMain := expandArgs(p.Version.Arguments.JVM, feats, vars)
	game := expandArgs(p.Version.Arguments.Game, feats, vars)
	if c.AutoJoin && !contains(game, "--quickPlayMultiplayer") {
		game = append(game, "--quickPlayMultiplayer", serverAddr())
	}
	args := append(append(append(jvm, jvmMain...), p.Version.MainClass), game...)

	cmd := exec.Command(p.JavaPath, args...)
	cmd.Dir = gd
	detach(cmd)
	logPath := dirs("logs", "game.log")
	lf, _ := os.Create(logPath)
	if lf != nil {
		cmd.Stdout = lf
		cmd.Stderr = lf
	}
	safe := make([]string, len(args))
	for i, s := range args {
		safe[i] = s
		if s == a.McToken {
			safe[i] = "(토큰)"
		}
	}
	logger.Printf("실행: %s %s", p.JavaPath, strings.Join(safe, " "))
	if err := cmd.Start(); err != nil {
		if lf != nil {
			lf.Close()
		}
		return fmt.Errorf("게임을 켜지 못했어요: %v", err)
	}
	gameMu.Lock()
	gameCmd = cmd
	gameStart = time.Now()
	gameMu.Unlock()
	if len(lastBlocked) > 0 {
		setStatus("running", "미니맵·지도 모드는 이 서버에서 못 써서 빼 두었어요 (mods-blocked 폴더): "+strings.Join(lastBlocked, ", "), 0)
	} else if len(lastUnsigned) > 0 {
		setStatus("running", "서명이 없어 안 받은 편의성 모드가 있어요 (서버장에게 알려 주세요): "+strings.Join(lastUnsigned, ", "), 0)
	} else {
		setStatus("running", a.Name+" 로 게임이 켜졌어요. 서버로 바로 접속해요.", 0)
	}
	go func() {
		err := cmd.Wait()
		if lf != nil {
			lf.Close()
		}
		ran := time.Since(gameStart)
		gameMu.Lock()
		gameCmd = nil
		gameMu.Unlock()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		logger.Printf("게임 끝 (코드 %d, %s) %v", code, ran.Round(time.Second), err)
		if code != 0 && ran < 2*time.Minute {
			statusMu.Lock()
			status = Status{Phase: "error", Text: "게임이 바로 꺼졌어요", Error: fmt.Sprintf("게임이 켜지다가 꺼졌어요 (코드 %d). 아래 기록을 서버장에게 보내 주세요.", code), LogTail: tail(logPath, 40)}
			statusMu.Unlock()
		} else {
			setStatus("idle", "게임이 끝났어요", 0)
		}
		gameEnded()
	}()
	return nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func tail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}



// 게임 시작 버튼 (직접 실행): 토큰 확인 -> 받기·업데이트 -> 실행
func playDirect() {
	if !playMu.TryLock() {
		return
	}
	defer playMu.Unlock()
	if directRunning() {
		return
	}
	fail := func(err error) {
		logger.Printf("실패: %v", err)
		statusMu.Lock()
		status = Status{Phase: "error", Text: "준비하지 못했어요", Error: err.Error()}
		statusMu.Unlock()
	}
	setStatus("prepare", "로그인 확인 중", 0)
	if err := ensureFreshToken(); err != nil {
		fail(err)
		return
	}
	p, err := prepare()
	if err != nil {
		fail(err)
		return
	}
	setStatus("prepare", "게임 켜는 중", 0)
	if err := launchGame(p); err != nil {
		fail(err)
	}
}
