import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Wrench, Plus, ToggleLeft, ToggleRight, Trash2, Upload, ArrowLeft } from 'lucide-react'
import api from '../services/api'
import './ToolsPanel.css'

const ToolsPanel = () => {
  const { t } = useTranslation()
  const [tools, setTools] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedTool, setSelectedTool] = useState(null)
  const [showImportModal, setShowImportModal] = useState(false)
  const [importForm, setImportForm] = useState({
    name: '',
    file: null,
    author: '',
    version: '1.0.0'
  })
  const [importing, setImporting] = useState(false)

  useEffect(() => {
    loadTools()
  }, [])

  const loadTools = async () => {
    try {
      setLoading(true)
      const response = await api.getAllTools()
      if (response.success) {
        setTools(response.data.tools || [])
      }
    } catch (error) {
      console.error('Failed to load tools:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleSelectTool = (tool) => {
    setSelectedTool(tool)
  }

  const handleBackToList = () => {
    setSelectedTool(null)
  }

  const handleToggleTool = async (tool) => {
    try {
      if (tool.type === 'builtin') {
        alert(t('tools.cannotDisable'))
        return
      }

      if (tool.enabled) {
        const response = await api.disableTool(tool.name)
        if (response.success) {
          loadTools()
          setSelectedTool({ ...tool, enabled: false })
        } else {
          alert(t('tools.disableFailed') + ': ' + response.error)
        }
      } else {
        const response = await api.enableTool(tool.name)
        if (response.success) {
          loadTools()
          setSelectedTool({ ...tool, enabled: true })
        } else {
          alert(t('tools.enableFailed') + ': ' + response.error)
        }
      }
    } catch (error) {
      console.error('Failed to toggle tool:', error)
      alert(t('tools.disableFailed') + ': ' + error.message)
    }
  }

  const handleDeleteTool = async (tool) => {
    if (tool.type === 'builtin') {
      alert(t('tools.cannotDisable'))
      return
    }

    if (!confirm(t('tools.deleteConfirm', { name: tool.name }))) {
      return
    }

    try {
      const response = await api.deleteTool(tool.name)
      if (response.success) {
        loadTools()
        setSelectedTool(null)
      } else {
        alert(t('tools.deleteFailed') + ': ' + response.error)
      }
    } catch (error) {
      console.error('Failed to delete tool:', error)
      alert(t('tools.deleteFailed') + ': ' + error.message)
    }
  }

  const handleImportSubmit = async (e) => {
    e.preventDefault()

    if (!importForm.name) {
      alert('工具名称不能为空')
      return
    }

    try {
      setImporting(true)

      const formData = new FormData()
      formData.append('name', importForm.name)
      formData.append('author', importForm.author)
      formData.append('version', importForm.version)

      if (importForm.file) {
        formData.append('file', importForm.file)
      }

      const response = await api.importTool(formData)
      if (response.success) {
        setShowImportModal(false)
        setImportForm({
          name: '',
          file: null,
          author: '',
          version: '1.0.0'
        })
        loadTools()
      } else {
        alert(t('tools.importFailed') + ': ' + response.error)
      }
    } catch (error) {
      console.error('Failed to import tool:', error)
      alert(t('tools.importFailed') + ': ' + error.message)
    } finally {
      setImporting(false)
    }
  }

  const getTypeLabel = (type) => {
    return type === 'builtin' ? t('tools.builtin') : t('tools.python')
  }

  const getTypeBadgeClass = (type) => {
    return type === 'builtin' ? 'badge badge-builtin' : 'badge badge-python'
  }

  // 工具详情视图
  if (selectedTool) {
    return (
      <div className="tools-panel">
        <div className="tools-header">
          <div className="header-left">
            <button className="btn-back" onClick={handleBackToList} title={t('common.back')}>
              <ArrowLeft size={18} />
            </button>
            <h2>{selectedTool.name}</h2>
          </div>
        </div>

        <div className="tool-detail-content">
          <div className="detail-section">
            <div className="detail-row">
              <span className="detail-label">{t('tools.type')}:</span>
              <span className={getTypeBadgeClass(selectedTool.type)}>
                {getTypeLabel(selectedTool.type)}
              </span>
            </div>
            <div className="detail-row">
              <span className="detail-label">{t('tools.status')}:</span>
              <span className={`status-badge ${selectedTool.enabled ? 'enabled' : 'disabled'}`}>
                {selectedTool.enabled ? t('tools.enabled') : t('tools.disabled')}
              </span>
            </div>
          </div>

          <div className="detail-section">
            <h3>{t('tools.description')}</h3>
            <p className="detail-description">{selectedTool.description}</p>
          </div>

          {selectedTool.tool_path && (
            <div className="detail-section">
              <h3>路径</h3>
              <code className="detail-path">{selectedTool.tool_path}</code>
            </div>
          )}

          {selectedTool.imported_at && (
            <div className="detail-section">
              <h3>导入信息</h3>
              <div className="detail-meta">
                <div className="meta-row">
                  <span className="meta-label">导入时间:</span>
                  <span className="meta-value">{new Date(selectedTool.imported_at).toLocaleString('zh-CN')}</span>
                </div>
                {selectedTool.imported_by && (
                  <div className="meta-row">
                    <span className="meta-label">导入者:</span>
                    <span className="meta-value">{selectedTool.imported_by}</span>
                  </div>
                )}
                {selectedTool.version && (
                  <div className="meta-row">
                    <span className="meta-label">版本:</span>
                    <span className="meta-value">{selectedTool.version}</span>
                  </div>
                )}
                {selectedTool.author && (
                  <div className="meta-row">
                    <span className="meta-label">作者:</span>
                    <span className="meta-value">{selectedTool.author}</span>
                  </div>
                )}
              </div>
            </div>
          )}

          <div className="detail-actions">
            {selectedTool.type !== 'builtin' && (
              <>
                <button
                  className={`btn btn-action ${selectedTool.enabled ? 'btn-disable' : 'btn-enable'}`}
                  onClick={() => handleToggleTool(selectedTool)}
                >
                  {selectedTool.enabled ? (
                    <>
                      <ToggleRight size={16} />
                      {t('common.disable')}
                    </>
                  ) : (
                    <>
                      <ToggleLeft size={16} />
                      {t('common.enable')}
                    </>
                  )}
                </button>
                <button
                  className="btn btn-action btn-delete"
                  onClick={() => handleDeleteTool(selectedTool)}
                >
                  <Trash2 size={16} />
                  {t('common.delete')}
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    )
  }

  // 工具列表视图
  return (
    <div className="tools-panel">
      <div className="tools-header">
        <h2>{t('tools.title')}</h2>
        <button
          className="btn-add-tool"
          title={t('tools.importTool')}
          onClick={() => setShowImportModal(true)}
        >
          <Plus size={18} />
        </button>
      </div>

      <div className="tools-content">
        {loading ? (
          <div className="loading">{t('common.loading')}</div>
        ) : tools.length === 0 ? (
          <div className="empty-state">
            <Wrench size={48} />
            <p>{t('tools.noTools')}</p>
          </div>
        ) : (
          <div className="tools-list">
            {tools.map((tool) => (
              <div
                key={tool.name}
                className={`tool-item ${selectedTool?.name === tool.name ? 'active' : ''}`}
                onClick={() => handleSelectTool(tool)}
              >
                <div className="tool-icon">
                  <Wrench size={18} />
                </div>
                <div className="tool-info">
                  <div className="tool-name-row">
                    <span className="tool-name">{tool.name}</span>
                    <span className={getTypeBadgeClass(tool.type)}>
                      {getTypeLabel(tool.type)}
                    </span>
                  </div>
                </div>
                <div className="tool-status">
                  <span className={`status-indicator ${tool.enabled ? 'enabled' : 'disabled'}`}>
                    {tool.enabled ? '●' : '○'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 导入工具模态框 */}
      {showImportModal && (
        <div className="modal-overlay" onClick={() => setShowImportModal(false)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>{t('tools.importForm.title')}</h3>
              <button
                className="btn-close"
                onClick={() => setShowImportModal(false)}
              >
                ×
              </button>
            </div>
            <form onSubmit={handleImportSubmit}>
              <div className="form-group">
                <label>{t('tools.importForm.name')} *</label>
                <input
                  type="text"
                  value={importForm.name}
                  onChange={(e) => setImportForm({ ...importForm, name: e.target.value })}
                  placeholder={t('tools.importForm.namePlaceholder')}
                  required
                />
              </div>
              <div className="form-group">
                <label>{t('tools.importForm.file')}</label>
                <input
                  type="file"
                  accept=".py,.zip"
                  onChange={(e) => setImportForm({ ...importForm, file: e.target.files[0] })}
                />
                <small>{t('tools.importForm.fileHint')}</small>
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label>{t('tools.importForm.author')}</label>
                  <input
                    type="text"
                    value={importForm.author}
                    onChange={(e) => setImportForm({ ...importForm, author: e.target.value })}
                    placeholder={t('tools.importForm.authorPlaceholder')}
                  />
                </div>
                <div className="form-group">
                  <label>{t('tools.importForm.version')}</label>
                  <input
                    type="text"
                    value={importForm.version}
                    onChange={(e) => setImportForm({ ...importForm, version: e.target.value })}
                    placeholder={t('tools.importForm.versionPlaceholder')}
                  />
                </div>
              </div>
              <div className="modal-actions">
                <button
                  type="button"
                  className="btn btn-secondary"
                  onClick={() => setShowImportModal(false)}
                  disabled={importing}
                >
                  {t('common.cancel')}
                </button>
                <button
                  type="submit"
                  className="btn btn-primary"
                  disabled={importing}
                >
                  {importing ? (
                    <>
                      <Upload size={16} className="spin" />
                      {t('tools.importForm.uploading')}
                    </>
                  ) : (
                    t('tools.importForm.import')
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default ToolsPanel
