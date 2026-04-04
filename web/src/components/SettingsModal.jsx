import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { createPortal } from 'react-dom'
import { X, Save, RotateCcw, Plus, Trash2, Settings, Cpu, Hammer, Layers, Database, ChevronDown, ChevronUp } from 'lucide-react'

const SettingsModal = ({ isOpen, onClose, onSave }) => {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [config, setConfig] = useState({
    models: {},
    agents: {},
    tools: {
      python_path: 'python',
      tools_dir: 'tools'
    },
    tool_generator: {
      enabled: true,
      model: ''
    },
    conversation_compress: {
      disabled: false,
      round: 20,
      length: 2048
    }
  })

  const [expandedModels, setExpandedModels] = useState({})
  const [hasChanges, setHasChanges] = useState(false)

  // 加载配置
  useEffect(() => {
    if (isOpen) {
      loadConfig()
    }
  }, [isOpen])

  const loadConfig = async () => {
    setLoading(true)
    try {
      // 从 API 加载配置
      const response = await fetch('/api/config')
      const data = await response.json()
      if (data.success) {
        setConfig(data.data)
      }
    } catch (error) {
      console.error('Failed to load config:', error)
    } finally {
      setLoading(false)
    }
  }

  const toggleModelExpand = (modelName) => {
    setExpandedModels(prev => ({
      ...prev,
      [modelName]: !prev[modelName]
    }))
  }

  const handleModelChange = (modelName, field, value) => {
    setConfig(prev => ({
      ...prev,
      models: {
        ...prev.models,
        [modelName]: {
          ...prev.models[modelName],
          [field]: value
        }
      }
    }))
    setHasChanges(true)
  }

  const handleAgentChange = (agentName, field, value) => {
    setConfig(prev => ({
      ...prev,
      agents: {
        ...prev.agents,
        [agentName]: {
          ...prev.agents[agentName],
          [field]: value
        }
      }
    }))
    setHasChanges(true)
  }

  const handleToolsChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      tools: {
        ...prev.tools,
        [field]: value
      }
    }))
    setHasChanges(true)
  }

  const handleToolGeneratorChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      tool_generator: {
        ...prev.tool_generator,
        [field]: value
      }
    }))
    setHasChanges(true)
  }

  const handleConvCompressChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      conversation_compress: {
        ...prev.conversation_compress,
        [field]: value
      }
    }))
    setHasChanges(true)
  }

  const handleAddModel = () => {
    const newModelName = `new-model-${Date.now()}`
    setConfig(prev => ({
      ...prev,
      models: {
        ...prev.models,
        [newModelName]: {
          provider: 'openai',
          model: '',
          base_url: '',
          temperature: 0.7,
          max_tokens: 2048,
          enable_thinking: null
        }
      }
    }))
    setExpandedModels(prev => ({
      ...prev,
      [newModelName]: true
    }))
    setHasChanges(true)
  }

  const handleDeleteModel = (modelName) => {
    if (Object.keys(config.models).length <= 1) {
      alert(t('settings.models.atLeastOne'))
      return
    }
    
    // 检查是否有 agent 正在使用此模型
    for (const [agentName, agent] of Object.entries(config.agents)) {
      if (agent.model === modelName) {
        alert(t('settings.models.inUse', { name: agentName }))
        return
      }
    }

    if (confirm(t('settings.models.deleteConfirm', { name: modelName }))) {
      const newModels = { ...config.models }
      delete newModels[modelName]
      setConfig(prev => ({
        ...prev,
        models: newModels
      }))
      setHasChanges(true)
    }
  }

  const handleSave = async () => {
    setLoading(true)
    try {
      const response = await fetch('/api/config', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(config)
      })
      const data = await response.json()
      if (data.success) {
        alert(t('settings.saveSuccess'))
        setHasChanges(false)
        onClose()
      } else {
        alert(t('settings.saveFailed') + ': ' + (data.error || 'Unknown error'))
      }
    } catch (error) {
      alert(t('settings.saveFailed') + ': ' + error.message)
    } finally {
      setLoading(false)
    }
  }

  const handleReset = () => {
    if (confirm(t('settings.resetConfirm'))) {
      loadConfig()
      setHasChanges(false)
    }
  }

  if (!isOpen) return null

  const modelNames = Object.keys(config.models)
  const agentNames = Object.keys(config.agents)

  return createPortal(
    <div className="settings-modal-overlay" onClick={onClose}>
      <div className="settings-modal" onClick={(e) => e.stopPropagation()}>
        <div className="settings-header">
          <div className="header-title">
            <Settings size={24} />
            <h2>{t('settings.title')}</h2>
          </div>
          <button className="btn-close" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="settings-content">
          {loading && !config.models ? (
            <div className="loading-state">
              <div className="spinner"></div>
              <p>{t('settings.loading')}</p>
            </div>
          ) : (
            <>
              {/* Models 配置 */}
              <div className="settings-section">
                <div className="section-header">
                  <Cpu size={18} />
                  <h3>{t('settings.models.title')}</h3>
                  <button className="btn-icon btn-add" onClick={handleAddModel} title={t('settings.models.add')}>
                    <Plus size={16} />
                  </button>
                </div>
                <p className="section-desc">{t('settings.models.description')}</p>
                
                <div className="models-list">
                  {modelNames.map(modelName => (
                    <div key={modelName} className="model-item">
                      <div 
                        className="model-header"
                        onClick={() => toggleModelExpand(modelName)}
                      >
                        <div className="model-title">
                          {expandedModels[modelName] ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
                          <span className="model-name">{modelName}</span>
                          <span className="model-provider-badge">{config.models[modelName].provider}</span>
                          <span className="model-model-name">{config.models[modelName].model}</span>
                        </div>
                        <div className="model-actions">
                          <button
                            className="btn-icon btn-delete"
                            onClick={(e) => {
                              e.stopPropagation()
                              handleDeleteModel(modelName)
                            }}
                            title={t('settings.models.delete')}
                          >
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </div>
                      
                      {expandedModels[modelName] && (
                        <div className="model-content">
                          <div className="form-row">
                            <div className="form-group">
                              <label>Provider</label>
                              <select 
                                value={config.models[modelName].provider || 'openai'}
                                onChange={(e) => handleModelChange(modelName, 'provider', e.target.value)}
                              >
                                <option value="openai">OpenAI</option>
                                <option value="bailian">Bailian (百炼)</option>
                              </select>
                            </div>
                            <div className="form-group">
                              <label>{t('settings.models.modelName')}</label>
                              <input
                                type="text"
                                value={config.models[modelName].model || ''}
                                onChange={(e) => handleModelChange(modelName, 'model', e.target.value)}
                                placeholder={t('settings.models.placeholder.modelName')}
                              />
                            </div>
                          </div>

                          <div className="form-row">
                            <div className="form-group">
                              <label>{t('settings.models.baseUrl')}</label>
                              <input
                                type="text"
                                value={config.models[modelName].base_url || ''}
                                onChange={(e) => handleModelChange(modelName, 'base_url', e.target.value)}
                                placeholder={t('settings.models.placeholder.baseUrl')}
                              />
                            </div>
                            <div className="form-group">
                              <label>{t('settings.models.apiKey')}</label>
                              <input
                                type="text"
                                value={config.models[modelName].api_key || ''}
                                onChange={(e) => handleModelChange(modelName, 'api_key', e.target.value)}
                                placeholder={t('settings.models.placeholder.apiKey')}
                              />
                            </div>
                          </div>
                          
                          <div className="form-row">
                            <div className="form-group">
                              <label>{t('settings.models.temperature')}</label>
                              <div className="range-input">
                                <input
                                  type="range"
                                  min="0"
                                  max="2"
                                  step="0.1"
                                  value={config.models[modelName].temperature || 0.7}
                                  onChange={(e) => handleModelChange(modelName, 'temperature', parseFloat(e.target.value))}
                                />
                                <span className="range-value">{config.models[modelName].temperature || 0.7}</span>
                              </div>
                            </div>
                            <div className="form-group">
                              <label>{t('settings.models.maxTokens')}</label>
                              <input
                                type="number"
                                value={config.models[modelName].max_tokens || 2048}
                                onChange={(e) => handleModelChange(modelName, 'max_tokens', parseInt(e.target.value))}
                              />
                            </div>
                          </div>

                          <div className="form-row">
                            <div className="form-group checkbox-group">
                              <label>
                                <input
                                  type="checkbox"
                                  checked={config.models[modelName].enable_thinking === true}
                                  onChange={(e) => handleModelChange(modelName, 'enable_thinking', e.target.checked ? true : null)}
                                />
                                {t('settings.models.enableThinking')}
                              </label>
                              <p className="hint">{t('settings.models.thinkingHint')}</p>
                            </div>
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>

              {/* Agents 配置 */}
              <div className="settings-section">
                <div className="section-header">
                  <Layers size={18} />
                  <h3>{t('settings.agents.title')}</h3>
                </div>
                <p className="section-desc">{t('settings.agents.description')}</p>

                <div className="agents-grid">
                  {agentNames.map(agentName => (
                    <div key={agentName} className="agent-card">
                      <div className="agent-name">{agentName}</div>
                      <div className="form-group">
                        <label>{t('settings.agents.useModel')}</label>
                        <select
                          value={config.agents[agentName]?.model || ''}
                          onChange={(e) => handleAgentChange(agentName, 'model', e.target.value)}
                        >
                          <option value="">{t('settings.agents.selectModel')}</option>
                          {modelNames.map(mName => (
                            <option key={mName} value={mName}>{mName}</option>
                          ))}
                        </select>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              {/* Tools 配置 */}
              <div className="settings-section">
                <div className="section-header">
                  <Hammer size={18} />
                  <h3>{t('settings.tools.title')}</h3>
                </div>
                <p className="section-desc">{t('settings.tools.description')}</p>

                <div className="form-row">
                  <div className="form-group">
                    <label>{t('settings.tools.pythonPath')}</label>
                    <input
                      type="text"
                      value={config.tools.python_path}
                      onChange={(e) => handleToolsChange('python_path', e.target.value)}
                      placeholder={t('settings.tools.placeholder.pythonPath')}
                    />
                  </div>
                  <div className="form-group">
                    <label>{t('settings.tools.toolsDir')}</label>
                    <input
                      type="text"
                      value={config.tools.tools_dir}
                      onChange={(e) => handleToolsChange('tools_dir', e.target.value)}
                      placeholder={t('settings.tools.placeholder.toolsDir')}
                    />
                  </div>
                </div>
              </div>

              {/* Tool Generator 配置 - 暂时隐藏 */}
              {/* <div className="settings-section">
                <div className="section-header">
                  <Settings size={18} />
                  <h3>工具生成器</h3>
                </div>
                <p className="section-desc">允许 AI 自动生成新的 Python 工具</p>

                <div className="form-row">
                  <div className="form-group checkbox-group">
                    <label>
                      <input
                        type="checkbox"
                        checked={config.tool_generator.enabled}
                        onChange={(e) => handleToolGeneratorChange('enabled', e.target.checked)}
                      />
                      启用工具生成器
                    </label>
                  </div>
                  <div className="form-group">
                    <label>生成模型</label>
                    <select
                      value={config.tool_generator.model || ''}
                      onChange={(e) => handleToolGeneratorChange('model', e.target.value)}
                      disabled={!config.tool_generator.enabled}
                    >
                      <option value="">请选择模型</option>
                      {modelNames.map(mName => (
                        <option key={mName} value={mName}>{mName}</option>
                      ))}
                    </select>
                  </div>
                </div>
              </div> */}

              {/* Conversation Compress 配置 */}
              <div className="settings-section">
                <div className="section-header">
                  <Database size={18} />
                  <h3>{t('settings.compress.title')}</h3>
                </div>
                <p className="section-desc">{t('settings.compress.description')}</p>

                <div className="form-row">
                  <div className="form-group checkbox-group">
                    <label>
                      <input
                        type="checkbox"
                        checked={!config.conversation_compress.disabled}
                        onChange={(e) => handleConvCompressChange('disabled', !e.target.checked)}
                      />
                      {t('settings.compress.enable')}
                    </label>
                  </div>
                </div>

                {!config.conversation_compress.disabled && (
                  <div className="form-row">
                    <div className="form-group">
                      <label>{t('settings.compress.round')}</label>
                      <input
                        type="number"
                        value={config.conversation_compress.round}
                        onChange={(e) => handleConvCompressChange('round', parseInt(e.target.value))}
                      />
                      <p className="hint">{t('settings.compress.roundHint')}</p>
                    </div>
                    <div className="form-group">
                      <label>{t('settings.compress.length')}</label>
                      <input
                        type="number"
                        value={config.conversation_compress.length}
                        onChange={(e) => handleConvCompressChange('length', parseInt(e.target.value))}
                      />
                      <p className="hint">{t('settings.compress.lengthHint')}</p>
                    </div>
                  </div>
                )}
              </div>
            </>
          )}
        </div>

        <div className="settings-footer">
          <button className="btn-reset" onClick={handleReset} disabled={loading}>
            <RotateCcw size={16} />
            <span>{t('common.reset')}</span>
          </button>
          <div className="settings-actions">
            <button className="btn-cancel" onClick={onClose} disabled={loading}>
              {t('common.cancel')}
            </button>
            <button
              className={`btn-save ${!hasChanges ? 'disabled' : ''}`}
              onClick={handleSave}
              disabled={loading || !hasChanges}
            >
              {loading ? <span className="spinner-small"></span> : <Save size={16} />}
              <span>{loading ? t('common.saving') : t('common.save')}</span>
            </button>
          </div>
        </div>
      </div>
    </div>,
    document.body
  )
}

export default SettingsModal
