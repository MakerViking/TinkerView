// Optional: lets you write <video-stream> directly in JSX once go2rtc's player is
// vendored (see public/README.md). The VideoStream component creates the element
// imperatively, so this is only needed if you choose JSX usage.
import 'react'

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'video-stream': React.DetailedHTMLProps<
        React.HTMLAttributes<HTMLElement>,
        HTMLElement
      > & {
        src?: string
        mode?: string
        media?: string
        background?: boolean
      }
    }
  }
}
