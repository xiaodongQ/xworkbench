## Summary

This change is a bug fix to existing behavior and does not introduce new capabilities or modify existing spec requirements.

## No New Requirements

The proposal lists no new capabilities and no modified capabilities. This is a corrective change to align implementation with Unix PTY conventions.

### Root Cause

The remote PTY terminal handler sends `\x08` (ASCII BS) when the user presses Delete on Mac keyboards. However, Unix PTY line disciplines use `\x7f` (ASCII DEL) as the erase character.

### Corrective Action

Change the character sent from `\x08` to `\x7f` in `rpty.js` line 242.
