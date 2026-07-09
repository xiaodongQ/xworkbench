## Why

Mac keyboards only have a Delete key (⌦), not a separate Backspace key. When the user presses Delete in the remote PTY terminal (rpty.js), the keystroke is reported as `Backspace keyCode 8` by the browser. The code then sends `\x08` (ASCII BS - backspace), which only moves the cursor back one position without deleting any character. Unix PTY line disciplines use `\x7f` (ASCII DEL) as the erase character.

## What Changes

- Change the Backspace key handler in `rpty.js` to send `\x7f` (DEL) instead of `\x08` (BS)
- This aligns with Unix PTY line discipline conventions where `\x7f` is the erase character

## Capabilities

### New Capabilities
- (none)

### Modified Capabilities
- (none - this is a bug fix to existing behavior, not a spec-level change)

## Impact

**Affected file:**
- `cmd/server/static/js/views/rpty.js` — line 242, Backspace handler

**Before:**
```js
rptyWs.send('\x08');  // BS - only moves cursor back
```

**After:**
```js
rptyWs.send('\x7f');  // DEL - deletes character (Unix PTY erase)
```
