import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { createPortal } from 'react-dom'
import { X, Save, RotateCcw, Plus, Trash2, Settings, Cpu, Hammer, Layers, Database, ChevronDown, ChevronUp, Brain } from 'lucide-react'

const SettingsModal = ({ isOpen, onClose, onSave }) => {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [config, setConfig] = useState({
    models: {},
    agents: {},
    tools: {
      python_path: 'python',
      tools_dir: 'tools',
      default_timeout: 300
    },
    tool_generator: {
      enabled: true,
      model: ''
    },
    conversation_compress: {
      disabled: false,
      round: 20,
      length: 2048
    },
    memory: {
      enabled: false,
      splade: {
        model_name: 'naver/splade-v3',
        source: 'modelscope',
        cache_dir: '.splade_models',
        auto_download: true,
        download_timeout: 300,
        vector_dim: 30522
      },
      qdrant: {
        storage_backend: 'local',
        storage_path: '.memory_vectors',
        host: 'localhost',
        port: 6334,
        api_key: '',
        enable_tls: false,
        collection_name: 'garlic_memories',
        distance: 'Cosine',
        max_memories: 10000,
        top_k: 5,
        similarity_threshold: 0.1
      },
      storage: {
        metadata_dir: '.memory_metadata',
        auto_import: true
      },
      cleanup_interval: 1,
      max_inactive_days: 15
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

  const handleMemoryChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      memory: {
        ...prev.memory,
        [field]: value
      }
    }))
    setHasChanges(true)
  }

  const handleSpladeChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      memory: {
        ...prev.memory,
        splade: {
          ...prev.memory.splade,
          [field]: value
        }
      }
    }))
    setHasChanges(true)
  }

  const handleQdrantChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      memory: {
        ...prev.memory,
        qdrant: {
          ...prev.memory.qdrant,
          [field]: value
        }
      }
    }))
    setHasChanges(true)
  }

  const handleMemoryStorageChange = (field, value) => {
    setConfig(prev => ({
      ...prev,
      memory: {
        ...prev.memory,
        storage: {
          ...prev.memory.storage,
          [field]: value
        }
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

                <div className="form-row">
                  <div className="form-group">
                    <label>{t('settings.tools.defaultTimeout')}</label>
                    <input
                      type="number"
                      value={config.tools.default_timeout || 300}
                      onChange={(e) => handleToolsChange('default_timeout', parseInt(e.target.value) || 300)}
                      min={10}
                      max={3600}
                    />
                    <p className="hint">{t('settings.tools.timeoutHint')}</p>
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

              {/* Memory 配置 */}
              <div className="settings-section">
                <div className="section-header">
                  <Brain size={18} />
                  <h3>{t('settings.memory.title')}</h3>
                </div>
                <p className="section-desc">{t('settings.memory.description')}</p>

                <div className="form-row">
                  <div className="form-group checkbox-group">
                    <label>
                      <input
                        type="checkbox"
                        checked={config.memory.enabled}
                        onChange={(e) => handleMemoryChange('enabled', e.target.checked)}
                      />
                      {t('settings.memory.enable')}
                    </label>
                  </div>
                </div>

                {config.memory.enabled && (
                  <>
                    {/* 存储后端选择 */}
                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.storageBackend')}</label>
                        <select
                          value={config.memory.qdrant.storage_backend}
                          onChange={(e) => handleQdrantChange('storage_backend', e.target.value)}
                        >
                          <option value="local">{t('settings.memory.storageBackendLocal')}</option>
                          <option value="qdrant">{t('settings.memory.storageBackendQdrant')}</option>
                        </select>
                      </div>
                    </div>

                    {/* 本地存储配置 */}
                    {config.memory.qdrant.storage_backend === 'local' && (
                      <div className="form-row">
                        <div className="form-group">
                          <label>{t('settings.memory.storagePath')}</label>
                          <input
                            type="text"
                            value={config.memory.qdrant.storage_path}
                            onChange={(e) => handleQdrantChange('storage_path', e.target.value)}
                            placeholder=".memory_vectors"
                          />
                          <p className="hint">{t('settings.memory.storagePathHint')}</p>
                        </div>
                      </div>
                    )}

                    {/* Qdrant 配置 */}
                    {config.memory.qdrant.storage_backend === 'qdrant' && (
                      <div className="form-row">
                        <div className="form-group">
                          <label>{t('settings.memory.host')}</label>
                          <input
                            type="text"
                            value={config.memory.qdrant.host}
                            onChange={(e) => handleQdrantChange('host', e.target.value)}
                            placeholder="localhost"
                          />
                        </div>
                        <div className="form-group">
                          <label>{t('settings.memory.port')}</label>
                          <input
                            type="number"
                            value={config.memory.qdrant.port}
                            onChange={(e) => handleQdrantChange('port', parseInt(e.target.value))}
                          />
                        </div>
                      </div>
                    )}

                    {/* 通用 Qdrant 配置 */}
                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.collectionName')}</label>
                        <input
                          type="text"
                          value={config.memory.qdrant.collection_name}
                          onChange={(e) => handleQdrantChange('collection_name', e.target.value)}
                          placeholder="garlic_memories"
                        />
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.distance')}</label>
                        <select
                          value={config.memory.qdrant.distance}
                          onChange={(e) => handleQdrantChange('distance', e.target.value)}
                        >
                          <option value="Cosine">Cosine</option>
                          <option value="Euclidean">Euclidean</option>
                          <option value="Dot">Dot Product</option>
                        </select>
                      </div>
                    </div>

                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.maxMemories')}</label>
                        <input
                          type="number"
                          value={config.memory.qdrant.max_memories}
                          onChange={(e) => handleQdrantChange('max_memories', parseInt(e.target.value))}
                        />
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.topK')}</label>
                        <input
                          type="number"
                          value={config.memory.qdrant.top_k}
                          onChange={(e) => handleQdrantChange('top_k', parseInt(e.target.value))}
                        />
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.similarityThreshold')}</label>
                        <div className="range-input">
                          <input
                            type="range"
                            min="0"
                            max="1"
                            step="0.05"
                            value={config.memory.qdrant.similarity_threshold}
                            onChange={(e) => handleQdrantChange('similarity_threshold', parseFloat(e.target.value))}
                          />
                          <span className="range-value">{config.memory.qdrant.similarity_threshold}</span>
                        </div>
                        <p className="hint">{t('settings.memory.thresholdHint')}</p>
                      </div>
                    </div>

                    {/* SPLADE 配置 */}
                    <div className="form-section-title">{t('settings.memory.splade.title')}</div>
                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.splade.modelName')}</label>
                        <input
                          type="text"
                          value={config.memory.splade.model_name}
                          onChange={(e) => handleSpladeChange('model_name', e.target.value)}
                          placeholder="naver/splade-v3"
                        />
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.splade.source')}</label>
                        <select
                          value={config.memory.splade.source}
                          onChange={(e) => handleSpladeChange('source', e.target.value)}
                        >
                          <option value="modelscope">{t('settings.memory.splade.modelscope')}</option>
                          <option value="huggingface">{t('settings.memory.splade.huggingface')}</option>
                        </select>
                      </div>
                    </div>

                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.splade.cacheDir')}</label>
                        <input
                          type="text"
                          value={config.memory.splade.cache_dir}
                          onChange={(e) => handleSpladeChange('cache_dir', e.target.value)}
                          placeholder=".splade_models"
                        />
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.splade.vectorDim')}</label>
                        <input
                          type="number"
                          value={config.memory.splade.vector_dim}
                          onChange={(e) => handleSpladeChange('vector_dim', parseInt(e.target.value))}
                        />
                      </div>
                    </div>

                    <div className="form-row">
                      <div className="form-group checkbox-group">
                        <label>
                          <input
                            type="checkbox"
                            checked={config.memory.splade.auto_download}
                            onChange={(e) => handleSpladeChange('auto_download', e.target.checked)}
                          />
                          {t('settings.memory.splade.autoDownload')}
                        </label>
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.splade.downloadTimeout')}</label>
                        <input
                          type="number"
                          value={config.memory.splade.download_timeout}
                          onChange={(e) => handleSpladeChange('download_timeout', parseInt(e.target.value))}
                        />
                      </div>
                    </div>

                    {/* 元数据存储配置 */}
                    <div className="form-section-title">{t('settings.memory.storage.title')}</div>
                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.storage.metadataDir')}</label>
                        <input
                          type="text"
                          value={config.memory.storage.metadata_dir}
                          onChange={(e) => handleMemoryStorageChange('metadata_dir', e.target.value)}
                          placeholder=".memory_metadata"
                        />
                      </div>
                      <div className="form-group checkbox-group">
                        <label>
                          <input
                            type="checkbox"
                            checked={config.memory.storage.auto_import}
                            onChange={(e) => handleMemoryStorageChange('auto_import', e.target.checked)}
                          />
                          {t('settings.memory.storage.autoImport')}
                        </label>
                      </div>
                    </div>

                    {/* 记忆清理配置 */}
                    <div className="form-section-title">{t('settings.memory.cleanup.title')}</div>
                    <div className="form-row">
                      <div className="form-group">
                        <label>{t('settings.memory.cleanup.interval')}</label>
                        <input
                          type="number"
                          value={config.memory.cleanup_interval}
                          onChange={(e) => handleMemoryChange('cleanup_interval', parseInt(e.target.value))}
                        />
                        <p className="hint">{t('settings.memory.cleanup.intervalHint')}</p>
                      </div>
                      <div className="form-group">
                        <label>{t('settings.memory.cleanup.maxInactiveDays')}</label>
                        <input
                          type="number"
                          value={config.memory.max_inactive_days}
                          onChange={(e) => handleMemoryChange('max_inactive_days', parseInt(e.target.value))}
                        />
                        <p className="hint">{t('settings.memory.cleanup.maxInactiveDaysHint')}</p>
                      </div>
                    </div>
                  </>
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
