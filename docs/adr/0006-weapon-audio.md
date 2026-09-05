# 6. Weapon audio: ogg cues, full argument capture, positional playback

Date: 2026-09-05

## Status

Accepted

## Context

`audio/player.go` is written but unused. Its cue naming — `directory/name` from
`LoadCuesFromDir` — already matches ZDoom's sound names, so
`A_PlaySound("hellboundshotgun/fire", ...)` maps onto `PlayCue` with no
translation layer, provided the migration writes
`sounds/hellboundshotgun/fire.ogg`.

Three gaps: `loadCue` decodes vorbis only; `PlayCue` ignores volume, channel,
looping and pitch; and playback is mono with no notion of where a sound is.

## Decision

**Format.** Migration copies `.ogg` through untouched. For any other format it
converts with `ffmpeg` when `ffmpeg` is on PATH, and fails naming the file and
the exact conversion command when it is not. Provenance records the ffmpeg
version alongside the source hash, and golden tests hash the *source* audio, not
the converted output, so an ffmpeg upgrade cannot break them.

**Arguments.** The compiler captures every `A_PlaySound` argument. The engine
implements volume (`effects.Volume`), pitch (`beep.ResampleRatio`) and looping
(`beep.Loop`). Channels get a mixer each, so starting a sound on a channel stops
what was playing there, which is ZDoom's behaviour.

**Position.** Sounds carry a world position. Pan comes from the angle to the
listener, attenuation from distance. The pair is computed once when the sound
starts, not tracked while it plays.

## Consequences

- The compiled data is lossless even where the engine is not yet: an argument we
  do not use is still recorded, so support can be added without re-migrating.
- `ffmpeg` is an optional migration-time dependency, never a runtime one, and
  never silent — its absence is an error with a command to run, not a skipped
  sound.
- A short sound from a moving source keeps the pan it started with. For a
  gunshot or a shell hitting concrete that is inaudible; a long sound on a fast
  actor would smear. Live tracking needs a streamer wrapper reading position per
  buffer, which is the upgrade path if it ever matters.
- No HRTF, no occlusion, no reverb. Pan and distance only.
