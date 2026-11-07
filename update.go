package main

// 업데이트: 켤 때·10분마다 서버(manifest 의 launcher 버전)와 비교해 새 버전이 있으면 화면에 「업데이트」 버튼을 띄우고,
// 1.8.8 버튼을 누르면 받아서(sha256 확인) 실행 파일을 바꾸고 다시 켬.
// 서버장은 plugins/AsiaWar/launcher/ 에 플랫폼별 런처 파일을 넣어 두면 됨 (이름은 updateAssetName).

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// 1.8.8 받을 수 있는 새 런처 (버튼을 누를 때까지 기다림)
type pendingUpd struct{ Target, URL, Sha string }

var (
	updMu   sync.Mutex
	updPend *pendingUpd
)

func pendingVersion() string {
	updMu.Lock()
	defer updMu.Unlock()
	if updPend == nil {
		return ""
	}
	return updPend.Target
}

func updateAssetName() string {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	switch runtime.GOOS {
	case "windows":
		return "AsiaWarLauncher-windows-" + arch + ".exe"
	case "darwin":
		return "AsiaWarLauncher-mac-" + arch
	default:
		return "AsiaWarLauncher-linux-" + arch
	}
}

// 지난번 업데이트에서 남은 예전 실행 파일 지우기 (예전 프로세스가 끝날 때까지 몇 번 시도)
func cleanOldExe() {
	old := selfExe() + ".old"
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(old); err != nil {
			return
		}
		if os.Remove(old) == nil {
			return
		}
		time.Sleep(time.Second)
	}
}

var restartHook func() // 서버(포트) 닫기

var updating atomic.Bool

func checkUpdate() {
	if devBuild && os.Getenv("KW_TEST_UPDATE") == "" {
		return
	}
	if !updating.CompareAndSwap(false, true) { // 이미 확인 중
		return
	}
	defer updating.Store(false)
	doc, err := fetchSignedDoc() // 1.2.7 업데이트 여부·파일 모두 서명된 값으로만
	base := filesBase()
	if gd, gerr := fetchGithubDoc(); gerr == nil && (err != nil || verLess(doc.LauncherVersion, gd.LauncherVersion)) {
		doc, err, base = gd, nil, githubLauncher // 1.8.0 서버가 꺼져 있거나 깃허브 쪽이 더 새 판이면 깃허브에서
		logger.Printf("깃허브의 런처 %s 를 봄", gd.LauncherVersion)
	} else if gerr != nil {
		logger.Printf("깃허브 확인 안 됨: %v", gerr)
	}
	if err != nil {
		logger.Printf("런처 업데이트 확인 안 함: %v", err)
		return
	}
	target := doc.LauncherVersion
	if target == "" || !verLess(LauncherVersion, target) { // 같거나 낮은 버전으로는 절대 안 바꿈
		return
	}
	name := updateAssetName()
	sha := doc.Files["launcher/"+name]
	if sha == "" {
		logger.Printf("새 런처 %s 이 있지만 %s 에 서명이 없음", target, name)
		return
	}
	url := filesBase() + "/launcher/" + name
	if base == githubLauncher {
		url = githubLauncher + "/" + name
	}
	updMu.Lock()
	if updPend == nil || updPend.Target != target {
		logger.Printf("새 런처 %s 이 있음 (업데이트 버튼을 기다림)", target)
	}
	updPend = &pendingUpd{Target: target, URL: url, Sha: sha}
	updMu.Unlock()
}

// 1.8.8 「업데이트」 버튼: 받아서 바꿔 넣고 다시 켬
func applyUpdate() error {
	updMu.Lock()
	p := updPend
	updMu.Unlock()
	if p == nil {
		return errors.New("받을 새 런처가 없어요")
	}
	if gameRunning() || busy() {
		return errors.New("게임이 켜져 있거나 준비 중이라 지금은 못 바꿔요. 게임을 끈 뒤 다시 눌러 주세요")
	}
	if !updating.CompareAndSwap(false, true) {
		return errors.New("이미 받는 중이에요")
	}
	defer updating.Store(false)
	target, url, sha := p.Target, p.URL, p.Sha
	exe := selfExe()
	tmp := exe + ".new"
	setStatus("prepare", "새 런처 "+target+" 받는 중", 0)
	if err := download(dlTask{URL: url, Path: tmp, Sha256: sha}); err != nil {
		logger.Printf("런처 업데이트 받기 실패: %v", err)
		setStatus("idle", "", 0)
		_ = os.Remove(tmp)
		return errors.New("새 런처를 받지 못했어요. 잠시 뒤 다시 눌러 주세요")
	}
	_ = os.Chmod(tmp, 0o755)
	_ = os.Remove(exe + ".old")
	if err := os.Rename(exe, exe+".old"); err != nil {
		logger.Printf("런처 업데이트 실패 (이름 바꾸기): %v", err)
		setStatus("idle", "", 0)
		return errors.New("런처 파일을 바꾸지 못했어요")
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(exe+".old", exe)
		logger.Printf("런처 업데이트 실패 (바꿔 넣기): %v", err)
		setStatus("idle", "", 0)
		return errors.New("런처 파일을 바꾸지 못했어요")
	}
	logger.Printf("런처 %s -> %s 로 바꿈, 다시 켬", LauncherVersion, target)
	setStatus("prepare", "새 런처 "+target+" 로 다시 켜는 중", 0)
	time.Sleep(700 * time.Millisecond) // 화면이 글을 한 번 보게
	if restartHook != nil {
		restartHook()
	}
	c := exec.Command(exe, "--updated")
	detach(c)
	if err := c.Start(); err != nil {
		logger.Printf("새 런처를 켜지 못함: %v", err)
	}
	os.Exit(0)
	return nil
}
