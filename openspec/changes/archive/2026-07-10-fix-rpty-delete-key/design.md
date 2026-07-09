## Context

The remote PTY terminal (rpty.js) uses `attachCustomKeyEventHandler` to intercept keyboard events. When the user presses Delete (⌦) on a Mac keyboard, the browser reports it as `Backspace keyCode 8`. The current code sends `\x08` (ASCII BS - backspace character) which only moves the cursor back one position without deleting the character.

Unix PTY line disciplines use `\x7f` (ASCII DEL) as the erase character, which actually deletes the character under the cursor.

## Goals / Non-Goals

**Goals:**
- Fix Delete key behavior in remote PTY terminal on Mac keyboards

**Non-Goals:**
- This is a bug fix only; no new capabilities

## Decisions

**Change `rpty.js` line 242** from `\x08` to `\x7f`:

| Character | Hex | ASCII Name | Behavior |
|-----------|-----|------------|----------|
| `\x08` | 0x08 | BS (Backspace) | Move cursor back, don't delete |
| `\x7f` | 0x7F | DEL (Delete) | Delete character under cursor (Unix erase) |

## Risks / Trade-offs

- **Minimal risk**: Single character change, well-understood behavior
- **Consideration**: Some unusual terminal configurations may expect BS instead of DEL, but `\x7f` is the Unix standard

## Open Questions

- (none)
