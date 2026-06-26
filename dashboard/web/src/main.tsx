import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { installFetchAuth } from './lib/auth'
import './index.css'

installFetchAuth() // attach the shared token (if any) to /api/* requests

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
