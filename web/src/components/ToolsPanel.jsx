import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Wrench, Plus, Play, Trash2, Edit2 } from 'lucide-react'

const ToolsPanel = () => {
  const { t } = useTranslation()
  const [tools, setTools] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // TODO: 从后端获取工具列表
    // 暂时使用模拟数据
    const mockTools = [
      { id: 'webrowser', name: t('tools.webrowser'), description: t('tools.webrowserDesc'), enabled: true },
      { id: 'fileviewer', name: t('tools.fileViewer'), description: t('tools.fileViewerDesc'), enabled: true },
      { id: 'codegen', name: t('tools.codeGen'), description: t('tools.codeGenDesc'), enabled: true }
    ]
    setTools(mockTools)
    setLoading(false)
  }, [t])

  return (
    <div className="tools-panel">
      <div className="tools-header">
        <h2>{t('tools.title')}</h2>
        <button className="btn-add-tool" title={t('tools.addTool')}>
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
              <div key={tool.id} className="tool-item">
                <div className="tool-icon">
                  <Wrench size={20} />
                </div>
                <div className="tool-info">
                  <div className="tool-name">{tool.name}</div>
                  <div className="tool-description">{tool.description}</div>
                </div>
                <div className="tool-actions">
                  <button
                    className={`btn-toggle ${tool.enabled ? 'enabled' : ''}`}
                    title={tool.enabled ? t('common.disable') : t('common.enable')}
                  >
                    {tool.enabled ? t('common.enable') : t('common.disable')}
                  </button>
                  <button className="btn-tool-action" title={t('common.run')}>
                    <Play size={16} />
                  </button>
                  <button className="btn-tool-action" title={t('common.edit')}>
                    <Edit2 size={16} />
                  </button>
                  <button className="btn-tool-action btn-delete" title={t('common.delete')}>
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export default ToolsPanel
