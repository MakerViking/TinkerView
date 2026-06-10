import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button, Notice } from '@/components/ui'

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
        <Notice tone="error" className="m-4">
          <div className="font-display font-semibold mb-1">Something broke in the UI</div>
          <div className="text-sm opacity-80 font-mono">{this.state.error.message}</div>
          <Button
            variant="danger-soft"
            size="xs"
            className="mt-3"
            onClick={() => this.setState({ error: null })}
          >
            Dismiss
          </Button>
        </Notice>
      )
    }
    return this.props.children
  }
}
