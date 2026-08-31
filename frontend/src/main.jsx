import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'

import App from './App.jsx'
import './index.css'
import { readFrontendMode } from './routing.js'

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <App runtimeMode={readFrontendMode()} />
  </StrictMode>,
)
