# dsh-pet-mygo

Feasibility probe: can [MyGo](https://github.com/egoist/mygo)'s transparent,
always-on-top, frameless window render a **HEVC-with-alpha `.mov`** transparently
through the system webview (WKWebView on macOS)?

**Result: PASSED.** The solid square floats over a fully transparent window —
no black box around it (verified by screenshotting the window and compositing
its alpha over a checkerboard). The only follow-up for a real pet is
`DisableShadow: true` to drop macOS's default window shadow.

## Why this matters

[dsh-pet](https://github.com/PC2005-cloud/dsh-pet) ships only VP9-alpha `.webm`,
which WKWebView (Safari's engine) cannot decode. Its macOS path is HEVC-alpha
`.mov`. This probe confirms the whole chain works without Electron:

| layer | status |
|---|---|
| MyGo transparent frameless window | ✓ |
| WKWebView transparent page | ✓ |
| HEVC-with-alpha `.mov` decoding | ✓ |
| alpha=0 regions show the desktop through | ✓ |

## Run

```sh
go run .          # builds and opens the floating square
```

The window shows `resources/index.html`, which plays
`resources/sample-alpha.mov` in a `<video>` loop.

## How the sample .mov was made

ffmpeg alone drops HEVC alpha (`libx265` on this machine rejects `yuva*`, and
`hevc_videotoolbox` strips the alpha). The working path is the same one dsh-pet
uses upstream: an Apple-native encoder.

```sh
# 1. compile dsh-pet's encoder (AVAssetWriter + hevcWithAlpha)
xcrun swiftc -O -framework AVFoundation -framework CoreVideo -framework VideoToolbox \
  hevc_alpha_encoder.swift -o /tmp/hevc-alpha-encoder

# 2. ffmpeg emits BGRA frames -> encoder writes HEVC-with-alpha mov
ffmpeg -f lavfi -i "color=blue:size=256x256:rate=24:duration=3" \
  -vf "format=rgba,geq=r='0':g='120':b='255':a='if(between(X,78,178)*between(Y,78,178),255,0)',format=bgra" \
  -f rawvideo -pix_fmt bgra pipe:1 \
  | /tmp/hevc-alpha-encoder out.mov 256 256 24
```

`hevc_alpha_encoder.swift` is dsh-pet's, MIT-licensed
(`scripts/hevc_alpha_encoder.swift` in the upstream repo).

## License note

dsh-pet is MIT. Reusing its assets and the shared physics logic in an
independent (non-fork) Go project is fine as long as the MIT notice is kept.
