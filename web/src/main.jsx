import React from 'react'
import ReactDOM from 'react-dom/client'
import { ThemeProvider } from './contexts/ThemeContext'
import App from './App.jsx'
import './index.css'
import i18n, { i18nConfig } from './i18n'

// Highlight.js 主题
import 'highlight.js/styles/github-dark.css'

// KaTeX 样式
import 'katex/dist/katex.min.css'

// 确保 i18n 初始化完成后再渲染应用
i18n.init(i18nConfig).then(() => {
  console.log('✅ i18n initialized successfully')
  console.log('🌐 Detected language:', i18n.language)
  console.log('🌐 Detected languages:', i18n.languages)
  
  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <ThemeProvider>
        <App />
      </ThemeProvider>
    </React.StrictMode>,
  )
}).catch(err => {
  console.error('❌ Failed to initialize i18n:', err)
  // 即使初始化失败也渲染应用
  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <ThemeProvider>
        <App />
      </ThemeProvider>
    </React.StrictMode>,
  )
})
