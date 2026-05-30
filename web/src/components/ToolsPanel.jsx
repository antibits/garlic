import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Wrench, Plus, ToggleLeft, ToggleRight, Trash2, Upload, ArrowLeft, Play, X, Check } from 'lucide-react'
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
  const [showRunForm, setShowRunForm] = useState(false)
  const [runArgs, setRunArgs] = useState({})
  const [runResult, setRunResult] = useState(null)
  const [running, setRunning] = useState(false)

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

  const handleBackToList = () => {
    setSelectedTool(null)
    setShowRunForm(false)
    setRunResult(null)
    setRunArgs({})
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

  const handleRunTool = async () => {
    setRunning(true)
    setRunResult(null)
    try {
      const response = await api.executeTool(selectedTool.name, runArgs)
      setRunResult(response)
    } catch (error) {
      setRunResult({ success: false, error: error.message })
    } finally {
      setRunning(false)
    }
  }

  const getDefaultArgValue = (param) => {
    if (param.default !== undefined && param.default !== null) return param.default
    switch (param.type) {
      case 'integer': case 'number': return 0
      case 'boolean': return false
      default: return ''
    }
  }

  const initRunArgs = (tool) => {
    if (!tool.parameters || tool.parameters.length === 0) return
    const args = {}
    tool.parameters.forEach(p => {
      args[p.name] = getDefaultArgValue(p)
    })
    setRunArgs(args)
  }

  const handleSelectTool = (tool) => {
    setSelectedTool(tool)
    setShowRunForm(false)
    setRunResult(null)
    initRunArgs(tool)
  }

  const handleArgChange = (paramName, value, type) => {
    setRunArgs(prev => {
      const newArgs = { ...prev }
      if (type === 'integer' || type === 'number') {
        newArgs[paramName] = value === '' ? '' : Number(value)
      } else if (type === 'boolean') {
        newArgs[paramName] = value === 'true'
      } else {
        newArgs[paramName] = value
      }
      return newArgs
    })
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

          {selectedTool.parameters && selectedTool.parameters.length > 0 && (
            <div className="detail-section">
              <h3>{t('tools.parameters')}</h3>
              <div className="params-table">
                {selectedTool.parameters.map((param, idx) => (
                  <div key={idx} className="param-row">
                    <div className="param-header">
                      <span className="param-name">{param.name}</span>
                    </div>
                    <div className="param-tags">
                      <span className={`param-required ${param.required ? 'is-required' : 'is-optional'}`}>
                        <Check size={10} />
                        {param.required ? t('tools.requiredParam') : t('tools.optionalParam')}
                      </span>
                      <span className="param-type-badge">{param.type === 'integer' ? 'int' : param.type === 'string' ? 'str' : param.type === 'boolean' ? 'bool' : param.type === 'number' ? 'num' : param.type}</span>
                    </div>
                    {param.description && (
                      <p className="param-description">{param.description}</p>
                    )}
                    <div className="param-meta">
                      {param.default !== undefined && param.default !== null && (
                        <span className="param-meta-item">
                          {t('tools.default')}: <code>{String(param.default)}</code>
                        </span>
                      )}
                      {param.choices && param.choices.length > 0 && (
                        <span className="param-meta-item">
                          {t('tools.choices')}: <code>{param.choices.join(', ')}</code>
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

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
            {selectedTool.type === 'python' && selectedTool.enabled && (
              <button
                className="btn btn-action btn-run"
                onClick={() => {
                  if (!selectedTool.parameters || selectedTool.parameters.length === 0) {
                    // 无参数工具直接执行
                    setRunning(true)
                    setRunResult(null)
                    api.executeTool(selectedTool.name, {}).then(res => {
                      setRunResult(res)
                      setRunning(false)
                    }).catch(err => {
                      setRunResult({ success: false, error: err.message })
                      setRunning(false)
                    })
                  } else {
                    initRunArgs(selectedTool)
                    setShowRunForm(!showRunForm)
                    setRunResult(null)
                  }
                }}
              >
                <Play size={16} />
                {running ? t('tools.running') : t('tools.run')}
              </button>
            )}
          </div>

          {/* 手动执行工具 */}
          {showRunForm && selectedTool.parameters && selectedTool.parameters.length > 0 && (
            <div className="detail-section run-section">
              <div className="run-section-header">
                <h3>{t('tools.run')}: {selectedTool.name}</h3>
                <button className="btn-close-sm" onClick={() => setShowRunForm(false)}>
                  <X size={14} />
                </button>
              </div>
              <div className="run-form">
                {selectedTool.parameters.map((param, idx) => (
                  <div key={idx} className="form-group">
                    <label>
                      {param.name}
                      {param.required && <span className="required-mark">*</span>}
                      <span className="param-type-hint">{param.type === 'integer' ? 'int' : param.type === 'string' ? 'str' : param.type === 'boolean' ? 'bool' : param.type === 'number' ? 'num' : param.type}</span>
                    </label>
                    {param.choices && param.choices.length > 0 ? (
                      <select
                        value={runArgs[param.name] ?? ''}
                        onChange={(e) => handleArgChange(param.name, e.target.value, param.type)}
                      >
                        {!param.required && <option value="">--</option>}
                        {param.choices.map((c, i) => (
                          <option key={i} value={c}>{c}</option>
                        ))}
                      </select>
                    ) : param.type === 'boolean' ? (
                      <select
                        value={runArgs[param.name]?.toString() ?? 'false'}
                        onChange={(e) => handleArgChange(param.name, e.target.value, param.type)}
                      >
                        <option value="true">true</option>
                        <option value="false">false</option>
                      </select>
                    ) : (
                      <input
                        type={param.type === 'integer' || param.type === 'number' ? 'number' : 'text'}
                        value={runArgs[param.name] ?? ''}
                        onChange={(e) => handleArgChange(param.name, e.target.value, param.type)}
                        placeholder={param.description || param.name}
                      />
                    )}
                    {param.description && (
                      <small>{param.description}</small>
                    )}
                  </div>
                ))}
                <button
                  className="btn btn-primary btn-run-execute"
                  onClick={handleRunTool}
                  disabled={running}
                >
                  <Play size={14} />
                  {running ? t('tools.running') : t('tools.run')}
                </button>
              </div>
            </div>
          )}

          {/* 执行结果 */}
          {runResult && (
            <div className="detail-section run-result-section">
              <div className="run-result-header">
                <h3>{t('tools.result')}</h3>
                <span className={`run-result-status ${runResult.success ? 'success' : 'failed'}`}>
                  {runResult.success ? t('tools.runSuccess') : t('tools.runFailed')}
                </span>
              </div>
              <pre className="run-result-output">
                {runResult.success
                  ? (typeof runResult.data === 'string' ? runResult.data : JSON.stringify(runResult.data, null, 2))
                  : (runResult.error || JSON.stringify(runResult, null, 2))}
              </pre>
            </div>
          )}
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
