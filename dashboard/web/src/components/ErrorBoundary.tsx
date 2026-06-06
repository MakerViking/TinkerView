import { Component, type ErrorInfo, type ReactNode } from 'react'

// Catches render-time errors anywhere below it, logs them to the console, and
// shows a recoverable message instead of a blank screen.
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('UI error boundary caught:', error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="m-4 p-4 rounded-lg border border-rose-800 bg-rose-950/40 text-rose-200">
          <div className="font-semibold mb-1">Something broke in the UI</div>
          <div className="text-sm opacity-80 font-mono">{this.state.error.message}</div>
          <button
            onClick={() => this.setState({ error: null })}
            className="mt-2 text-xs px-2 py-1 rounded bg-rose-800/60 hover:bg-rose-800"
          >
            Dismiss
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
