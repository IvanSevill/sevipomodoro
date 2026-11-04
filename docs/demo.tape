# Regenerate from the repository root:
#   make build && vhs docs/demo.tape
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1000
Set Height 640
Set Padding 16
Set Framerate 20

# A temporary config directory keeps your own statistics and preferences out of the demo.
Hide
Type 'XDG_CONFIG_HOME="$(mktemp -d)" ./bin/sevipomodoro'
Enter
Sleep 700ms
Show

# Start the timer
Sleep 1500ms
Space
Sleep 3000ms

# Preview themes live in settings, then discard
Type "t"
Sleep 1000ms
Down 6
Sleep 700ms
Right
Sleep 1200ms
Right
Sleep 1200ms
Right
Sleep 1200ms
Escape
Sleep 1000ms

# Skip to a break
Type "s"
Sleep 4500ms

Type "q"
Sleep 300ms
