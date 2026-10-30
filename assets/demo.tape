# Records assets/demo.gif. Run from the repo root: vhs assets/demo.tape

Output assets/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1200
Set Height 560
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms
Set Framerate 24

Hide
Type "go build -o /tmp/typeghast-demo/typeghast . && export PATH=/tmp/typeghast-demo:$PATH PS1='$ ' && clear"
Enter
Sleep 3s
Show

Type "typeghast --demo"
Sleep 400ms
Enter
# Wait for the game to start, or the first keys are lost.
Sleep 3.5s

# Fast and clean: speed lines, then a shake for the perfect sentence.
Type "Added retries with backoff to the webhook client so flaky deliveries recover. "
Sleep 1s

# The agent's context fills up and the ghosts close in.
PageUp 3
Sleep 1.2s
Type@70ms "Fixed an off-by-one in the pagination cursor and added a regression test. "
PageUp 3
Sleep 1.5s
Type@80ms "Moved sessions to Redis so a restart "
Sleep 1.5s

# /compact: the ghosts are exorcised.
Ctrl+L
Sleep 2.5s
Type@60ms "no longer logs everyone out. "
Sleep 1.5s
