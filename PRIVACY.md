# Privacy

CGMS Launcher doesn't send your account information to us. The only personal info it handles is the in-game name you type in.

**Sign-in.** You sign in on Microsoft's page (login.microsoftonline.com) in your default browser. The launcher gets a sign-in token from Microsoft, exchanges it with Xbox Live and Minecraft services, and stores the result only on your computer (on Windows it's encrypted with your Windows account). The token is only sent to Microsoft, Xbox Live and Mojang. Logging out deletes it.

**In-game name.** The launcher asks for the name you want to be shown as in game (on our server we use real names among friends). It's saved in the launcher's settings on your computer and in a small file in the game folder (readable only by you); our companion mod sends it only to our server, when you join it, and other players see it above your head, in the player list and in chat. You can change it in the launcher settings, and the server owner can remove it on request.

**Discord (optional).** If you press "Discord link" in settings, the launcher makes a random code, saves it on your computer and opens our server's page, which sends you to Discord's own approval page (scope: identify). Our server only receives your Discord user ID and display name, shows you which Minecraft account it will be linked to, and links them only after you press "Link" on that page. The launcher never sees your Discord password or token.

**Our server.** The launcher downloads the mod list, mod files and launcher updates from our game server. It doesn't send your account or token there. Your Minecraft name reaches our server only when you join it in game, like any server.

**Other sites it talks to:** Mojang (game files, Java runtime), FabricMC (Fabric loader), and mc-heads.net (player head pictures by Minecraft name).

**Donations.** The donation page only shows links (Toss, KakaoPay, Ko-fi) and our bank account from the server's settings, and opens them in your browser. The launcher doesn't handle any payment itself.

**Files on your computer:** the launcher's own folder (game files, settings, logs). If you use the official launcher mode, it also adds a "Korea War" installation to the official launcher's settings and keeps a backup of that file.

Questions: open an issue on this repository.
