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

Then uncomment the two `<script>` tags in `dashboard/web/index.html`. The
`VideoStream` component auto-detects the `<video-stream>` custom element and uses
it instead of the MJPEG fallback. Verify the **actual** transport (WebRTC vs MSE)
in the go2rtc UI at `:1984` — a playing tile does not prove WebRTC is active.
