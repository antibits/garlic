import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Brain, RefreshCw, Trash2 } from 'lucide-react'
import api from '../services/api'
import './MemoryPanel.css'

/**
 * MemoryPanel 组件 - 显示和管理 AI Agent 的记忆
 */
const MemoryPanel = () => {
  const { t } = useTranslation()
  const [memories, setMemories] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    loadMemories()
  }, [])

  const loadMemories = async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await api.getMemories()
      setMemories(data || [])
    } catch (err) {
      console.error('Failed to load memories:', err)
      setError(t('memory.loadFailed'))
    } finally {
      setLoading(false)
    }
  }

  const handleClearMemories = async () => {
    if (!window.confirm(t('memory.clearConfirm'))) return

    try {
      await api.clearMemories()
      setMemories([])
    } catch (err) {
      console.error('Failed to clear memories:', err)
      setError(t('memory.clearFailed'))
    }
  }

  return (
    <div className="memory-panel">
      <div className="memory-header">
        <h2>{t('memory.title')}</h2>
        <div className="memory-actions">
          <button
            className="btn-memory-action btn-refresh"
            onClick={loadMemories}
            disabled={loading}
            title={t('memory.refresh')}
          >
            <RefreshCw size={16} className={loading ? 'spinning' : ''} />
          </button>
          <button
            className="btn-memory-action btn-clear"
            onClick={handleClearMemories}
            disabled={loading || memories.length === 0}
            title={t('memory.clearAll')}
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>

      <div className="memory-content">
        {loading ? (
          <div className="loading-state">
            <div className="spinner-small"></div>
            <span>{t('memory.loading')}</span>
          </div>
        ) : error ? (
          <div className="error-state">
            <p>{error}</p>
            <button onClick={loadMemories} className="btn-primary">
              {t('memory.retry')}
            </button>
          </div>
        ) : memories.length === 0 ? (
          <div className="empty-state">
            <Brain size={48} strokeWidth={1.5} />
            <p>{t('memory.noMemories')}</p>
            <p className="empty-hint">
              {t('memory.memoryHint')}
            </p>
          </div>
        ) : (
          <div className="memory-list">
            {memories.map((memory, index) => (
              <div key={index} className="memory-item">
                <div className="memory-icon">
                  <Brain size={16} strokeWidth={1.5} />
                </div>
                <div className="memory-info">
                  <div className="memory-content-text">
                    {memory.content || memory}
                  </div>
                  {memory.timestamp && (
                    <div className="memory-timestamp">
                      {new Date(memory.timestamp).toLocaleString('zh-CN')}
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export default MemoryPanel
