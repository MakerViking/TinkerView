# Hardware acceleration presets

Frigate uses hardware in **two independent places**, and you choose each separately:

1. **Object detection** — the AI that finds people/cars. Pick a *detector*.
2. **Video decode/transcode** — turning camera H.264/H.265 into frames (and the
   printer-cam MJPEG→H.264 path). Pick an *ffmpeg hwaccel*.

Each host gets **one compose overlay** (device passthrough + image) and **one
config preset** (the `detectors:`/`model:`/`ffmpeg:` block pasted into
`../config.yml`). A **Coral** can be added to *any* host for detection while that
host's GPU still does decode.

## Pick your combo

| Host hardware | Compose overlay | Config preset | Detector | Decode |
|---|---|---|---|---|
| **Intel iGPU** (Gen9+ / Skylake 2015+) | `compose.intel.yml` | `intel.yml` (`device: GPU`) | OpenVINO · iGPU | VAAPI / QSV |
| **Intel Arc** dGPU (A/B-series) | `compose.intel.yml` | `intel.yml` (`device: GPU`) | OpenVINO · Arc | QSV |
| **Intel Core Ultra** (Meteor/Lunar/Arrow Lake) | `compose.intel.yml` | `intel.yml` (`device: NPU`) | OpenVINO · NPU | QSV |
| **Old/Atom Intel** (pre-Gen9, J-series…) | `compose.intel.yml` | `intel.yml` (`device: CPU`) | OpenVINO · CPU | VAAPI (`i965`) |
| **Nvidia** GTX 10xx (Pascal) | `compose.nvidia.yml` + `-tensorrt` image | `nvidia.yml` (FP32, 320) | ONNX · TensorRT | NVDEC |
| **Nvidia** RTX 20xx+ (Turing+) | `compose.nvidia.yml` + `-tensorrt` image | `nvidia.yml` (FP16, 320/640) | ONNX · TensorRT | NVDEC |
| **AMD** Radeon dGPU (RDNA2/3) | `compose.amd-rocm.yml` (`-rocm` image) | `amd-rocm.yml` (+GFX override) | ROCm | VAAPI |
| **AMD** APU / iGPU (680M/780M…) | `compose.amd-rocm.yml` or fall back | ROCm *or* Coral/OpenVINO-CPU | ROCm (spotty) | VAAPI |
| **Coral** USB/PCIe (any host) | `compose.coral.yml` (+ host overlay) | `coral.yml` (+ host's ffmpeg) | EdgeTPU | host's GPU |
| **Raspberry Pi** 4/5 | `compose.pi.yml` | `pi.yml` (Hailo/Coral/CPU) | Hailo8L / Coral | VideoCore |
| **Anything** (no accel / first boot) | `compose.cpu.yml` | `cpu.yml` | tflite CPU *(test only)* | software |

Run, for example:

```sh
docker compose -f docker-compose.yml -f compose.intel.yml up -d
# then paste host-presets/intel.yml into the HOST-SPECIFIC BLOCK of frigate/config.yml
```

## Notes & gotchas

- **Detection vs decode are separate.** `preset-nvidia`/`preset-vaapi` only speed up
  *video* — they do NOT give GPU *detection*. Detection needs the matching detector
  (and, for Nvidia, the **`-tensorrt`** image — the plain image runs ONNX on CPU).
- **Intel `device:`** — `GPU` (Gen9+), `NPU` (Core Ultra), `CPU` (universal, never
  crashes), `AUTO` (let OpenVINO pick). `device: GPU` *aborts* on a chip with no
  OpenVINO GPU plugin → use `CPU` there. OpenVINO-on-CPU still beats the tflite CPU
  detector by a wide margin.
- **AMD ROCm** misdetects many consumer cards — set `HSA_OVERRIDE_GFX_VERSION`
  (see `amd-rocm.yml`). On APUs/iGPUs ROCm is unreliable; a **Coral** or
  OpenVINO-CPU is the dependable fallback.
- **Nvidia** — Pascal (10xx) is FP32-only; Turing/RTX (20xx+) can use `float16` and
  bigger models for more throughput.
- **Coral** is the most reliable, lowest-power detector and is hardware-agnostic:
  add `compose.coral.yml` on top of any host overlay and keep that host's decode.
- **CPU detection is test-only** — fine to *see* the dashboard and verify cameras,
  but limit it to 1–2 low-fps cameras and don't software-transcode several cams.
- **mix & match** — e.g. Coral detect + Intel decode:
  `-f docker-compose.yml -f compose.intel.yml -f compose.coral.yml` (compose
  concatenates the device lists).
