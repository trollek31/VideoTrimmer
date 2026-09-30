# Build status

## Verified in this workspace

- Windows x64 release build compiles with `CGO_ENABLED=0`.
- `go vet` passes for the Windows target.
- Final executable is a Windows PE32+ GUI executable and imports only `kernel32.dll`.
- The app is now a true portable **folder/ZIP** build: `VideoTrimmer.exe` + `bin\\ffmpeg.exe` + `bin\\ffprobe.exe` + `bin\\ffplay.exe`.
- No FFmpeg/Go/.NET/VC++/Python dependency is required on the user's machine beyond Windows itself.
- The supplied FFmpeg 9.0.1 binaries were checked for exact byte integrity.
- FFmpeg trim command patterns were tested on an H.264/AAC MP4:
  - copy mode preserved H.264/AAC and 1280x720 with the expected keyframe-boundary duration behavior;
  - exact mode produced the requested 2.500 s fragment as H.264/AAC at the same resolution.
- The previous `ffprobe` parsing problem was fixed: duration is read from `format.duration`.
- The UI was rebuilt as a dark native Win32 interface with a large preview, timeline handles, editable time fields, mode selector, keyboard shortcuts and drag-and-drop.

## Environment limitation

The current sandbox is Linux, so a real Windows GUI launch cannot be executed here. The release is therefore verified by Windows-target compilation/static checks and by running the underlying FFmpeg operations against test media. A final GUI smoke test should still be run once on Windows 10/11.
