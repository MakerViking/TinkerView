# Vendoring go2rtc's player (optional, for low-latency WebRTC/MSE tiles)

Without these files the camera tiles fall back to **MJPEG** through the proxy —
which works everywhere but at higher latency/bandwidth. To get **WebRTC (~0.5s)**
and MSE, vendor go2rtc's dependency-free player (MIT) into this folder:

```sh
# from dashboard/web/public
curl -O https://raw.githubusercontent.com/AlexxIT/go2rtc/master/www/video-rtc.js
curl -O https://raw.githubusercontent.com/AlexxIT/go2rtc/master/www/video-stream.js
```

> Pin to the same go2rtc version bundled in your Frigate (there is **no npm
> package**, so it must be re-pinned manually on upgrades).

**Re-pinning checklist:** `VideoStream.tsx` reaches into two player internals to
surface stream errors to React (it needs them to fall back off a failed HQ tile).
Both are how `video-stream.js` extends the base class itself, but neither is a
public API — re-check them after any re-pin:

- `onopen()` is overridden on the element and chains to the original.
- Inside that override, an extra handler is registered in the `onmessage` map. It
  must be re-registered there on every call, because `video-rtc.js`'s `onopen()`
  resets `onmessage = {}` on each (re)connect.

If a future version drops either, HQ tiles stop auto-recovering (they revert to the
old behaviour: raw error text and a 15s retry loop) — the tiles still play.

Then uncomment the two `<script>` tags in `dashboard/web/index.html`. The
`VideoStream` component auto-detects the `<video-stream>` custom element and uses
it instead of the MJPEG fallback. Verify the **actual** transport (WebRTC vs MSE)
in the go2rtc UI at `:1984` — a playing tile does not prove WebRTC is active.
