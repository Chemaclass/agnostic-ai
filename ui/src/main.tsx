import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { unlockSession } from './settings-api'
import './styles/index.css'

// Automated browsers and unit tests skip it, so test output stays quiet.
if (import.meta.env.MODE !== 'test' && !navigator.webdriver) {
  console.info(
    '%cagnostic-ai%c\nMade by Chemaclass · https://chemaclass.com\nhttps://github.com/Chemaclass/agnostic-ai',
    'color: #256f61; font: 600 20px Fraunces, Georgia, serif',
    'color: inherit; font: inherit',
  )
}

// A page view opened carries a one-time code in the address; trade it before anything reads the address.
void unlockSession()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
