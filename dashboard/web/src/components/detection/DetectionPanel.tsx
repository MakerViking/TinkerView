import { useEffect, useState } from 'react'
import { Field, inputCls } from '@/components/devices/forms'
import { Button, Notice, SectionHeader, Skeleton } from '@/components/ui'
import { IconActivity, IconCheck } from '@/components/icons'
import type { DetectionSettings } from '@/types/ws'

const DETECTORS: { value: string; label: string }[] = [
  { value: 'cpu', label: 'CPU — most compatible (works everywhere)' },
  { value: 'openvino', label: 'Intel iGPU / Arc / NPU (OpenVINO)' },
  { value: 'edgetpu', label: 'Google Coral (EdgeTPU, USB/PCIe)' },
  { value: 'onnx', label: 'Nvidia GPU (TensorRT / ONNX)' },
  { value: 'rocm', label: 'AMD GPU (ROCm)' },
  { value: 'hailo8l', label: 'Raspberry Pi (Hailo-8L)' },
]
const HWACCELS: { value: string; label: string }[] = [
  { value: '', label: 'None (software decode)' },
  { value: 'vaapi', label: 'Intel / AMD (VAAPI)' },
  { value: 'qsv-h264', label: 'Intel QuickSync — H.264' },
  { value: 'qsv-h265', label: 'Intel QuickSync — H.265' },
  { value: 'nvidia', label: 'Nvidia (NVDEC)' },
  { value: 'rpi-64-h264', label: 'Raspberry Pi 5' },
]

// Default device when switching detector.
function deviceFor(detector: string): string {
  if (detector === 'openvino') return 'GPU'
  if (detector === 'edgetpu') return 'usb'
  return ''
}

// hostSteps returns the host-level prerequisites the dashboard CANNOT perform for a
// given choice (it only writes Frigate's config). These are ONE-TIME: once the host
// has the image/device/model, switching back later is just Apply. `active` is the
// detector currently running — when the selection matches it, the host is already set
// up (the image + model don't get removed by switching away). CPU and OpenVINO-on-CPU
// never need anything.
function hostSteps(s: DetectionSettings, active: string): { ok: boolean; oneTime?: boolean; lines: string[] } {
  if (s.detector === 'cpu')
    return { ok: true, lines: ['Nothing else needed — boots anywhere. (Test-only: keep to 1–2 low-fps cameras.)'] }
  if (s.detector === 'openvino' && (s.device ?? '').toUpperCase() === 'CPU')
    return { ok: true, lines: ['Nothing else needed — runs on the CPU via OpenVINO with the bundled model.'] }
  if (s.detector === active)
    return {
      ok: true,
      lines: [
        "This is the detector running now — the image, device and model are already in place, and they stay even if you switch away. Just Apply (it re-asserts the config; no manual editing).",
      ],
    }
  switch (s.detector) {
    case 'openvino':
      return {
        ok: false,
        oneTime: true,
        lines: ['Recreate Frigate so the iGPU is passed in:', 'docker compose -f docker-compose.yml -f compose.intel.yml up -d'],
      }
    case 'edgetpu':
      return {
        ok: false,
        oneTime: true,
        lines: [
          'Recreate Frigate so the Coral device is passed in:',
          'docker compose -f docker-compose.yml -f compose.coral.yml up -d',
          '(USB → /dev/bus/usb, PCIe/M.2 → /dev/apex_0 via the host driver)',
        ],
      }
    case 'onnx':
      return {
        ok: false,
        oneTime: true,
        lines: [
          '1) In .env set FRIGATE_IMAGE=ghcr.io/blakeblackshear/frigate:0.17.1-tensorrt and recreate with compose.nvidia.yml (the plain image runs ONNX on CPU only).',
          '2) Export the .onnx into frigate/model_cache/ and set the model path below.',
          'See frigate/host-presets/README.md (and nvidia.yml) for the export command.',
        ],
      }
    case 'rocm':
      return {
        ok: false,
        oneTime: true,
        lines: [
          '1) Use the -rocm image + devices: recreate with compose.amd-rocm.yml (set HSA_OVERRIDE_GFX_VERSION if your card misdetects).',
          '2) Place the yolo-nas .onnx in frigate/model_cache/ and set the model path below.',
          'See frigate/host-presets/amd-rocm.yml.',
        ],
      }
    case 'hailo8l':
      return {
        ok: false,
        oneTime: true,
        lines: ['Raspberry Pi 5 with a Hailo-8/8L AI HAT; recreate with compose.pi.yml (passes /dev/hailo0).'],
      }
    default:
      return { ok: false, lines: [] }
  }
}

export function DetectionPanel() {
  const [s, setS] = useState<DetectionSettings | null>(null)
  // active = the detector Frigate is running now (from the last load); when the
  // selection matches it, the host is already set up.
  const [active, setActive] = useState('')
  const [unavailable, setUnavailable] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = () => {
    setError(null)
    fetch('/api/detection')
      .then(async (r) => {
        if (r.status === 503) {
          setUnavailable(true)
          return null
        }
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return (await r.json()) as DetectionSettings
      })
      .then((d) => {
        if (d) {
          setS(d)
          setActive(d.detector)
        }
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
  }
  useEffect(load, [])

  const set = (patch: Partial<DetectionSettings>) => setS((cur) => ({ ...(cur as DetectionSettings), ...patch }))

  const apply = async () => {
    if (!s) return
    setError(null)
    setStatus(null)
    setBusy(true)
    try {
      const r = await fetch('/api/detection', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(s),
      })
      if (r.status === 202) {
        setStatus('Applying — Frigate will restart (~25s). Watch the toast for the result.')
        return
      }
      if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`)
      setStatus('Saved.')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  if (unavailable) {
    return (
      <Notice>
        Frigate config editing isn't available on this host, so detection hardware can't be changed from the UI.
      </Notice>
    )
  }
  if (!s) {
    return error ? (
      <Notice tone="error">{error}</Notice>
    ) : (
      <div className="flex max-w-2xl flex-col gap-3" aria-label="Loading detection settings">
        <Skeleton className="h-12" />
        <Skeleton className="h-12" />
        <Skeleton className="h-24" />
      </div>
    )
  }

  const isModelDetector = s.detector === 'onnx' || s.detector === 'rocm'
  const steps = hostSteps(s, active)

  return (
    <div className="flex max-w-2xl flex-col gap-4">
      <div>
        <SectionHeader title="Detection hardware" />
        <p className="mt-1.5 text-xs leading-relaxed text-ink-500">
          Choose what runs Frigate's object detection and video decode. The dashboard writes Frigate's config and
          restarts it; anything it can't do (passing in a GPU/Coral device, swapping the image, exporting a model)
          is listed under "Host prerequisites". <span className="text-ink-300">CPU is the safe default that works
          everywhere.</span>
        </p>
      </div>

      <LiveSpeed />

      {s.advanced && (
        <Notice tone="warn">
          Your current Frigate config looks hand-tuned (multiple detectors or custom settings). Applying a choice here
          will <strong>replace</strong> the detectors/model/decode blocks. To keep the custom config, edit{' '}
          <code className="text-ink-100">frigate/config.yml</code> directly instead.
        </Notice>
      )}

      <Field label="Detector (object detection)">
        <select
          className={inputCls}
          value={s.detector}
          onChange={(e) => set({ detector: e.target.value, device: deviceFor(e.target.value) })}
        >
          {DETECTORS.map((d) => (
            <option key={d.value} value={d.value}>
              {d.label}
            </option>
          ))}
        </select>
      </Field>

      {s.detector === 'openvino' && (
        <Field label="OpenVINO device" hint="GPU = iGPU Gen9+/Arc · NPU = Core Ultra · CPU = any (never crashes) · AUTO">
          <select className={inputCls} value={s.device ?? 'GPU'} onChange={(e) => set({ device: e.target.value })}>
            {['GPU', 'NPU', 'CPU', 'AUTO'].map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>
        </Field>
      )}

      {s.detector === 'edgetpu' && (
        <Field label="Coral device">
          <select className={inputCls} value={s.device ?? 'usb'} onChange={(e) => set({ device: e.target.value })}>
            <option value="usb">USB</option>
            <option value="pci">PCIe / M.2</option>
          </select>
        </Field>
      )}

      {isModelDetector && (
        <div className="grid grid-cols-2 gap-3">
          <div className="col-span-2">
            <Field label="Model path" hint="the .onnx you exported, e.g. /config/model_cache/yolov9-t-320.onnx">
              <input
                className={inputCls + ' font-mono'}
                value={s.modelPath ?? ''}
                onChange={(e) => set({ modelPath: e.target.value })}
                placeholder="/config/model_cache/…onnx"
              />
            </Field>
          </div>
          <Field label="Input size">
            <input
              className={inputCls}
              type="number"
              value={s.modelWidth || 320}
              onChange={(e) => set({ modelWidth: Number(e.target.value), modelHeight: Number(e.target.value) })}
            />
          </Field>
          <Field label="Precision" hint="float16 on Turing/RTX+ ; float on Pascal/older">
            <select className={inputCls} value={s.modelDtype || 'float'} onChange={(e) => set({ modelDtype: e.target.value })}>
              <option value="float">float (FP32)</option>
              <option value="float16">float16 (FP16)</option>
            </select>
          </Field>
        </div>
      )}

      <Field label="Video decode (separate from detection)" hint="hardware decode for camera streams; None = software">
        <select className={inputCls} value={s.hwaccel ?? ''} onChange={(e) => set({ hwaccel: e.target.value })}>
          {HWACCELS.map((h) => (
            <option key={h.value} value={h.value}>
              {h.label}
            </option>
          ))}
        </select>
      </Field>

      <Notice tone={steps.ok ? 'ok' : 'warn'} className="text-xs">
        <div className="mb-1 flex items-center gap-1.5 font-display font-semibold tracking-wide">
          {steps.ok && <IconCheck className="text-[13px]" />}
          {steps.ok
            ? s.detector === active && s.detector !== 'cpu'
              ? 'Already set up on this host'
              : 'Host prerequisites: none'
            : 'One-time host setup (skip if this host already has it)'}
        </div>
        {steps.lines.map((l, i) =>
          l.startsWith('docker compose') || l.includes('FRIGATE_IMAGE=') ? (
            <code key={i} className="mt-1 block whitespace-pre-wrap break-all rounded-md bg-black/40 px-2 py-1 font-mono text-ink-100">
              {l}
            </code>
          ) : (
            <div key={i} className="mt-0.5">
              {l}
            </div>
          ),
        )}
      </Notice>

      <div className="flex items-center gap-3">
        <Button variant="primary" onClick={apply} disabled={busy}>
          Apply &amp; restart Frigate
        </Button>
        <Button variant="ghost" onClick={load}>
          Reload current
        </Button>
      </div>
      {error && <div className="text-sm text-rose-300">{error}</div>}
      {status && <div className="text-sm text-emerald-300">{status}</div>}
    </div>
  )
}

// LiveSpeed polls Frigate's /api/stats (through the dashboard proxy) and shows each
// detector's rolling inference time, so you can apply a config and watch the speed —
// and compare configs. inference_speed stays 0 until a detect-enabled camera sends
// frames.
function LiveSpeed() {
  const [speeds, setSpeeds] = useState<Record<string, number> | null>(null)
  useEffect(() => {
    let stop = false
    const tick = async () => {
      try {
        const r = await fetch('/frigate/api/stats')
        if (!r.ok) throw new Error()
        const d = (await r.json()) as { detectors?: Record<string, { inference_speed?: number }> }
        const out: Record<string, number> = {}
        for (const [k, v] of Object.entries(d.detectors ?? {})) out[k] = v.inference_speed ?? 0
        if (!stop) setSpeeds(out)
      } catch {
        if (!stop) setSpeeds(null)
      }
    }
    tick()
    const id = window.setInterval(tick, 3000)
    return () => {
      stop = true
      window.clearInterval(id)
    }
  }, [])
  const entries = speeds ? Object.entries(speeds) : []
  if (!entries.length) return null
  return (
    <div className="rounded-xl border border-ink-800 bg-ink-900/80 p-3.5 shadow-elev">
      <div className="mb-2 flex items-center gap-1.5 font-display text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-400">
        <IconActivity className="text-[12px] text-brand-400" />
        Live detector speed
        <span className="font-sans font-normal normal-case tracking-normal text-ink-600">(lower = faster)</span>
      </div>
      {entries.map(([name, ms]) => (
        <div key={name} className="flex items-baseline justify-between font-mono text-xs tabular-nums">
          <span className="text-ink-400">{name}</span>
          {ms > 0 ? (
            <span className="text-emerald-300">
              <span className="text-base font-medium">{ms.toFixed(1)}</span> ms/inference
            </span>
          ) : (
            <span className="text-ink-500">idle — no frames being detected</span>
          )}
        </div>
      ))}
    </div>
  )
}
