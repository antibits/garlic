import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { FileText, Download, Trash2, ExternalLink } from 'lucide-react'

const OutputsPanel = () => {
  const { t } = useTranslation()
  const [outputs, setOutputs] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    // TODO: 从后端获取产出列表
    // 暂时使用模拟数据
    const mockOutputs = [
      { id: '1', name: '数据分析报告.md', type: 'markdown', size: '24 KB', createdAt: '2026-04-18 10:30' },
      { id: '2', name: 'Python 脚本.py', type: 'code', size: '8 KB', createdAt: '2026-04-18 09:15' },
      { id: '3', name: '项目总结.txt', type: 'text', size: '12 KB', createdAt: '2026-04-17 16:45' }
    ]
    setOutputs(mockOutputs)
    setLoading(false)
  }, [])

  const getFileIcon = (type) => {
    switch (type) {
      case 'markdown':
        return '📝'
      case 'code':
        return '💻'
      case 'text':
        return '📄'
      default:
        return '📁'
    }
  }

  return (
    <div className="outputs-panel">
      <div className="outputs-header">
        <h2>{t('outputs.title')}</h2>
      </div>

      <div className="outputs-content">
        {loading ? (
          <div className="loading">{t('common.loading')}</div>
        ) : outputs.length === 0 ? (
          <div className="empty-state">
            <FileText size={48} />
            <p>{t('outputs.noOutputs')}</p>
          </div>
        ) : (
          <div className="outputs-list">
            {outputs.map((output) => (
              <div key={output.id} className="output-item">
                <div className="output-icon">
                  {getFileIcon(output.type)}
                </div>
                <div className="output-info">
                  <div className="output-name">{output.name}</div>
                  <div className="output-meta">
                    <span className="output-size">{output.size}</span>
                    <span className="output-date">{output.createdAt}</span>
                  </div>
                </div>
                <div className="output-actions">
                  <button className="btn-output-action" title={t('common.view')}>
                    <ExternalLink size={16} />
                  </button>
                  <button className="btn-output-action" title={t('common.download')}>
                    <Download size={16} />
                  </button>
                  <button className="btn-output-action btn-delete" title={t('common.delete')}>
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

export default OutputsPanel
