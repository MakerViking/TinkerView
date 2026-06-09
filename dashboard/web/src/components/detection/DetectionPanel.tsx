import { useEffect, useState, type ReactNode } from 'react'
import { Field, inputCls } from '@/components/devices/forms'
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
// given choice (it only writes Frigate's config). CPU and OpenVINO-on-CPU need none.
function hostSteps(s: DetectionSettings): { ok: boolean; lines: string[] } {
  switch (s.detector) {
    case 'cpu':
      return { ok: true, lines: ['Nothing else needed — boots anywhere. (Test-only: keep to 1–2 low-fps cameras.)'] }
    case 'openvino':
      if ((s.device ?? '').toUpperCase() === 'CPU')
        return { ok: true, lines: ['Nothing else needed — runs on the CPU via OpenVINO with the bundled model.'] }
      return {
        ok: false,
        lines: ['Recreate Frigate so the iGPU is passed in:', 'docker compose -f docker-compose.yml -f compose.intel.yml up -d'],
      }
    case 'edgetpu':
      return {
        ok: false,
        lines: [
          'Recreate Frigate so the Coral device is passed in:',
          'docker compose -f docker-compose.yml -f compose.coral.yml up -d',
          '(USB → /dev/bus/usb, PCIe/M.2 → /dev/apex_0 via the host driver)',
        ],
      }
    case 'onnx':
      return {
        ok: false,
        lines: [
          '1) In .env set FRIGATE_IMAGE=ghcr.io/blakeblackshear/frigate:0.17.1-tensorrt and recreate with compose.nvidia.yml (the plain image runs ONNX on CPU only).',
          '2) Export the .onnx into frigate/model_cache/ and set the model path below.',
          'See frigate/host-presets/README.md (and nvidia.yml) for the export command.',
        ],
      }
    case 'rocm':
      return {
        ok: false,
        lines: [
          '1) Use the -rocm image + devices: recreate with compose.amd-rocm.yml (set HSA_OVERRIDE_GFX_VERSION if your card misdetects).',
          '2) Place the yolo-nas .onnx in frigate/model_cache/ and set the model path below.',
          'See frigate/host-presets/amd-rocm.yml.',
        ],
      }
    case 'hailo8l':
      return {
        ok: false,
        lines: ['Raspberry Pi 5 with a Hailo-8/8L AI HAT; recreate with compose.pi.yml (passes /dev/hailo0).'],
      }
    default:
      return { ok: false, lines: [] }
  }
}

export function DetectionPanel() {
  const [s, setS] = useState<DetectionSettings | null>(null)
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
      .then((d) => d && setS(d))
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
    return error ? <Notice tone="error">{error}</Notice> : <div className="p-8 text-center text-slate-400">Loading…</div>
  }

  const isModelDetector = s.detector === 'onnx' || s.detector === 'rocm'
  const steps = hostSteps(s)

  return (
    <div className="flex max-w-2xl flex-col gap-4">
      <div>
        <h1 className="text-sm font-medium text-slate-300">Detection hardware</h1>
        <p className="mt-1 text-xs text-slate-500">
          Choose what runs Frigate's object detection and video decode. The dashboard writes Frigate's config and
          restarts it; anything it can't do (passing in a GPU/Coral device, swapping the image, exporting a model)
          is listed under "Host prerequisites". <span className="text-slate-400">CPU is the safe default that works
          everywhere.</span>
        </p>
      </div>

      {s.advanced && (
        <Notice tone="warn">
          Your current Frigate config looks hand-tuned (multiple detectors or custom settings). Applying a choice here
          will <strong>replace</strong> the detectors/model/decode blocks. To keep the custom config, edit{' '}
          <code className="text-slate-300">frigate/config.yml</code> directly instead.
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
                className={inputCls}
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

      <div
        className={
          'rounded-lg border p-3 text-xs ' +
          (steps.ok ? 'border-emerald-800 bg-emerald-950/30 text-emerald-200' : 'border-amber-800 bg-amber-950/30 text-amber-200')
        }
      >
        <div className="mb-1 font-medium">{steps.ok ? 'Host prerequisites: none ✅' : 'Host prerequisites (do these on the host)'}</div>
        {steps.lines.map((l, i) =>
          l.startsWith('docker compose') || l.includes('FRIGATE_IMAGE=') ? (
            <code key={i} className="mt-1 block whitespace-pre-wrap break-all rounded bg-black/40 px-1.5 py-1 text-slate-200">
              {l}
            </code>
          ) : (
            <div key={i} className="mt-0.5">
              {l}
            </div>
          ),
        )}
      </div>

      <div className="flex items-center gap-3">
        <button
          onClick={apply}
          disabled={busy}
          className="rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-500 disabled:opacity-50"
        >
          Apply &amp; restart Frigate
        </button>
        <button onClick={load} className="text-sm text-slate-400 hover:text-slate-200">
          Reload current
        </button>
      </div>
      {error && <div className="text-sm text-rose-300">{error}</div>}
      {status && <div className="text-sm text-emerald-300">{status}</div>}
    </div>
  )
}

function Notice({ children, tone }: { children: ReactNode; tone?: 'error' | 'warn' }) {
  const cls =
    tone === 'error'
      ? 'border-rose-800 bg-rose-950/40 text-rose-200'
      : tone === 'warn'
        ? 'border-amber-800 bg-amber-950/40 text-amber-200'
        : 'border-slate-800 bg-slate-900/60 text-slate-300'
  return <div className={'rounded-lg border p-3 text-sm ' + cls}>{children}</div>
}
