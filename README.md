# CGMS Launcher

Launcher for Korea War, a small Minecraft: Java Edition server I run with my friends (a nation-war game on a map of the Korean peninsula).
My friends were having a hard time installing Fabric and keeping our companion mod up to date by hand, so I made this.

![screenshot](docs/screenshot.png)

What it does:

- signs in with the player's own Microsoft account (Minecraft Java Edition required)
- downloads the game, Fabric and our server's mods, checks them, and keeps them updated
- starts the game and joins the server
- shows server status, notices and the war map
- has an optional donation page so friends can chip in for the server bill (cosmetic titles only)

It only works with our server. There are no ads and it doesn't collect any data. There's an optional donation page where friends can chip in a dollar or two for hosting. Anyone who does gets a cosmetic title in game, nothing that affects gameplay. Sign-in happens on Microsoft's own page in your default browser, so the launcher never sees your password. See [PRIVACY.md](PRIVACY.md).

Until our app ID is approved for Minecraft sign-in, the launcher sets everything up and then opens the official Minecraft Launcher with a "Korea War" installation, and you sign in there.

## Build

Go 1.24 or newer, standard library only.

```
./build.sh 1.4.0
```

## 한국어

Korea War 서버용 런처예요. Fabric이랑 서버 모드를 손으로 깔고 업데이트하는 게 너무 번거로워서 만들었어요.
마이크로소프트 계정으로 로그인하면 게임·Fabric·모드를 받아서 서버로 바로 접속해요. 로그인은 브라우저의 마이크로소프트 페이지에서 해서 비밀번호는 런처가 볼 수 없어요. 광고 없음, 개인정보 수집 없음. 서버비 후원 화면이 있는데 보상은 게임 칭호뿐이에요.

Questions or problems: open an issue here.

MIT License
