import { useState } from 'react'
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom'
import { ThemeProvider } from './contexts/ThemeContext'
import ChatPage from './pages/ChatPage'
import SettingsModal from './components/SettingsModal'
import api from './services/api'
import './App.css'

function App() {
  const [settingsOpen, setSettingsOpen] = useState(false)

  const handleSaveSettings = async (newSettings) => {
    try {
      await api.saveConfig(newSettings)
    } catch (error) {
      console.error('Failed to save settings:', error)
      alert('保存配置失败：' + error.message)
    }
  }

  return (
    <ThemeProvider>
      <Router>
        <div className="app">
          <Routes>
            <Route
              path="/"
              element={
                <ChatPage onOpenSettings={() => setSettingsOpen(true)} />
              }
            />
          </Routes>
        </div>
      </Router>
      <SettingsModal
        isOpen={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        onSave={handleSaveSettings}
      />
    </ThemeProvider>
  )
}

export default App
