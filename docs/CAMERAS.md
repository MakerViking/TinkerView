# Cameras: preparation and streams

Do this before you fill in `frigate/config.yml`. It saves most first-start problems.

## 1. List your camera URLs

- **Printer cameras (MJPEG).** Confirm the real endpoint and strip trailing slashes:
  - Mainsail or Fluidd: `http://printer/webcam/?action=stream`
  - raw OctoPi: `http://printer/?action=stream`
  - ustreamer or camera-streamer: `http://printer:8080/stream`
- **Security cameras (RTSP).** The main `rtsp://...` URL and a low-resolution substream URL.

Note which printer cameras genuinely allow only one client. Those are the ones that gain
most from go2rtc's fan-out.

## 2. Prepare each security camera

In the camera's own settings:

- Set the main stream to **H.264 with AAC** audio.
- Create a substream of about **1280x720, H.264**, for detection.
- Set the **keyframe interval equal to the frame rate**.
- **Turn off "smart" codecs** (H.264+, H.265+, and similar) for anything you will view in a
  browser. Browsers cannot play them, which shows up as black tiles.

## 3. Add the streams

Either add cameras in the dashboard (**Add camera** on the Cameras tab) or edit
`frigate/config.yml` (see [CONFIGURATION.md](CONFIGURATION.md#frigate-frigateconfigyml)).
The stream name must match the camera name.

Security cameras use the native RTSP stream with no re-encode. Printer cameras are
transcoded from MJPEG to H.264 so they can use WebRTC and be recorded:

```yaml
go2rtc:
  streams:
    printer1:
      - ffmpeg:http://192.168.1.80/webcam/?action=stream#video=h264#hardware
```

`#hardware` uses the GPU encoder; on a host without one, drop it and accept the CPU cost,
or keep the plain MJPEG stream for a cheap thumbnail tier.

Printer cameras get **no object detection** in the shipped example. Person and car
detection is meaningless on a print bed.

## GPU "HQ" streams

The dashboard can show a `<cam>_hq` variant per tile, with denoise, sharpen and optional
upscale, encoded on an Nvidia GPU only while someone watches. You write the stream by hand
in `frigate/config.yml`; a commented recipe is in the file. It needs
`GO2RTC_ALLOW_ARBITRARY_EXEC=true` in `.env`, so read the warning in
[CONFIGURATION.md](CONFIGURATION.md#env) first. Things the recipe depends on:

- Output to a stdout pipe with `-f mpegts -`. Do not use `-f rtsp {output}`; Frigate's
  `{}` substitution breaks it.
- Keep `-timeout 15000000` before `-i`. If go2rtc restarts the exec producer without
  reaping the old ffmpeg, the orphan keeps an NVENC session forever. Consumer GPUs cap
  simultaneous NVENC sessions (often 8 on current drivers; check yours), and when the cap is full new encoders fail
  with `OpenEncodeSessionEx failed: incompatible client key (21)`. The timeout lets a
  stalled orphan end itself. To clear a saturated GPU now: `pkill -f h264_nvenc`
  (go2rtc respawns what is in use).
- Use the ffmpeg path that matches your Frigate build.

## 4. Check that it works

- **Fan-out.** Open the same printer camera in three or more browser tabs and its printer
  UI at once. All stay live. On the go2rtc page (`:1984`) there is a single upstream consumer.
- **Low latency.** In the go2rtc stream page the transport should say `webrtc` (about
  0.5 s). Video playing does not prove WebRTC is active.
- **Live wall in Frigate.** Create a Camera Group set to "Continuous Streaming". The
  default "All Cameras" page shows 1 fps stills and never uses WebRTC.
- **Detection.** Walk past a security camera: an object event and a clip appear in Frigate.
- **Recording.** Segments appear under `STORAGE_PATH` (`/media/frigate/recordings` in the
  container).
- **Do not treat recording as your only backup.** Watch disk usage and keep a second copy
  of footage you cannot lose.
