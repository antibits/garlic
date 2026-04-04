import React, { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MessageSquare, Wrench, FileText, Brain, Settings, Sun, Moon, Plus, Trash2, Edit2, X, Check } from 'lucide-react'
import { useTheme } from '../contexts/ThemeContext'
import ToolsPanel from './ToolsPanel'
import OutputsPanel from './OutputsPanel'
import MemoryPanel from './MemoryPanel'
import api from '../services/api'
import './SidebarTabs.css'

/**
 * SidebarTabs 组件 - 整合了会话列表功能
 * 
 * 设计说明：
 * - 会话管理逻辑移到这里，避免在 ChatPage 和 SessionList 之间传递复杂回调
 * - 切换会话时只更新选中状态，不断开 WebSocket 连接
 */
const SidebarTabs = ({
  currentSessionId,
  sessions,
  setSessions,
  onSelectSession,
  onCreateSession,
  onDeleteSession,
  onOpenSettings
}) => {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState('sessions')
  const { theme, toggleTheme } = useTheme()
  
  // 会话编辑状态
  const [editingId, setEditingId] = useState(null)
  const [editingName, setEditingName] = useState('')
  const [loading, setLoading] = useState(false)

  const tabs = [
    { id: 'sessions', label: t('tabs.sessions'), icon: MessageSquare },
    { id: 'tools', label: t('tabs.tools'), icon: Wrench },
    { id: 'outputs', label: t('tabs.outputs'), icon: FileText },
    { id: 'memory', label: t('tabs.memory'), icon: Brain }
  ]

  const handleSelectSession = (sessionId) => {
    // 切换后端当前会话
    api.switchSession(sessionId).catch(console.error)
    // 更新前端选中状态（不断开 WebSocket）
    onSelectSession(sessionId)
  }

  const handleDeleteSession = async (e, sessionId) => {
    e.stopPropagation()
    if (!window.confirm(t('session.deleteConfirm'))) return

    try {
      await api.deleteSession(sessionId)
      // 通知父组件清理状态和连接
      onDeleteSession(sessionId)
    } catch (error) {
      console.error('Failed to delete session:', error)
    }
  }

  const handleStartEdit = (e, session) => {
    e.stopPropagation()
    setEditingId(session.id)
    setEditingName(session.name || '')
  }

  const handleSaveEdit = async (e) => {
    e.stopPropagation()
    if (!editingName.trim()) return

    try {
      await api.updateSession(editingId, { name: editingName.trim() })
      setSessions(prev => prev.map(s =>
        s.id === editingId ? { ...s, name: editingName.trim() } : s
      ))
      setEditingId(null)
      setEditingName('')
    } catch (error) {
      console.error('Failed to update session:', error)
    }
  }

  const handleCancelEdit = (e) => {
    e.stopPropagation()
    setEditingId(null)
    setEditingName('')
  }

  const handleEditKeyPress = (e) => {
    if (e.key === 'Enter') {
      handleSaveEdit(e)
    } else if (e.key === 'Escape') {
      handleCancelEdit(e)
    }
  }

  const renderTabContent = () => {
    switch (activeTab) {
      case 'sessions':
        return (
          <div className="session-list">
            <div className="session-list-header">
              <h2>{t('session.title')}</h2>
              <button
                className="btn-new-session"
                onClick={onCreateSession}
                disabled={loading}
                title={t('session.newSession')}
              >
                <Plus size={18} />
              </button>
            </div>

            <div className="session-list-content">
              {loading ? (
                <div className="loading-state">
                  <div className="spinner-small"></div>
                  <span>{t('session.loading')}</span>
                </div>
              ) : sessions.length === 0 ? (
                <div className="empty-state">
                  <MessageSquare size={48} strokeWidth={1.5} />
                  <p>{t('session.noSessions')}</p>
                  <button onClick={onCreateSession} className="btn-primary">
                    {t('session.createFirst')}
                  </button>
                </div>
              ) : (
                <div className="sessions">
                  {sessions.map((session) => (
                    <div
                      key={session.id}
                      className={`session-item ${
                        currentSessionId === session.id ? 'active' : ''
                      }`}
                      onClick={() => handleSelectSession(session.id)}
                    >
                      {editingId === session.id ? (
                        <div className="session-edit" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="text"
                            value={editingName}
                            onChange={(e) => setEditingName(e.target.value)}
                            onKeyDown={handleEditKeyPress}
                            autoFocus
                            className="session-edit-input"
                            placeholder={t('session.inputSessionName')}
                          />
                          <div className="session-edit-actions">
                            <button
                              className="btn-edit-action btn-save"
                              onClick={handleSaveEdit}
                              title={t('session.save')}
                            >
                              <Check size={14} />
                            </button>
                            <button
                              className="btn-edit-action btn-cancel"
                              onClick={handleCancelEdit}
                              title={t('session.cancel')}
                            >
                              <X size={14} />
                            </button>
                          </div>
                        </div>
                      ) : (
                        <>
                          <div className="session-icon">
                            <MessageSquare size={18} strokeWidth={1.5} />
                          </div>
                          <div className="session-info">
                            <div className="session-name">
                              {session.name || `${t('session.session')} ${session.id.slice(-6)}`}
                            </div>
                            <div className="session-meta-info">
                              <span className="session-message-count">
                                {t('session.messageCount', { count: session.message_count || 0 })}
                              </span>
                            </div>
                          </div>
                          <div className="session-actions">
                            <button
                              className="btn-session-action btn-edit"
                              onClick={(e) => handleStartEdit(e, session)}
                              title={t('session.rename')}
                            >
                              <Edit2 size={14} strokeWidth={1.5} />
                            </button>
                            <button
                              className="btn-session-action btn-delete"
                              onClick={(e) => handleDeleteSession(e, session.id)}
                              title={t('common.delete')}
                            >
                              <Trash2 size={14} strokeWidth={1.5} />
                            </button>
                          </div>
                        </>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="session-list-footer">
              <button
                className="btn-settings"
                onClick={onOpenSettings}
                title={t('common.settings')}
              >
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <circle cx="12" cy="12" r="3"></circle>
                  <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                </svg>
                <span>{t('common.settings')}</span>
              </button>
            </div>
          </div>
        )
      case 'tools':
        return <ToolsPanel />
      case 'outputs':
        return <OutputsPanel />
      case 'memory':
        return <MemoryPanel />
      default:
        return null
    }
  }

  return (
    <div className="sidebar-tabs">
      {/* 活动栏 - 只有图标 */}
      <div className="activity-bar">
        {tabs.map((tab) => {
          const Icon = tab.icon
          return (
            <button
              key={tab.id}
              className={`activity-item ${activeTab === tab.id ? 'active' : ''}`}
              onClick={() => setActiveTab(tab.id)}
              title={tab.label}
            >
              <Icon size={24} />
            </button>
          )
        })}

        <div className="activity-spacer" />

        <button
          className="activity-item"
          onClick={toggleTheme}
          title={theme === 'light' ? t('theme.switchToDark') : t('theme.switchToLight')}
        >
          {theme === 'light' ? <Moon size={24} /> : <Sun size={24} />}
        </button>

        <button
          className="activity-item"
          onClick={onOpenSettings}
          title={t('common.settings')}
        >
          <Settings size={24} />
        </button>
      </div>

      {/* 侧边面板 - 显示内容 */}
      <div className="side-panel">
        <div className="side-panel-content">
          {renderTabContent()}
        </div>
      </div>
    </div>
  )
}

export default SidebarTabs
