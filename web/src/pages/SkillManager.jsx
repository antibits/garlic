import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { getSkills, getSkill, createSkill, updateSkill, deleteSkill } from '../services/api'
import './SkillManager.css'

const SkillManager = ({ onBack }) => {
  const { t } = useTranslation()
  const [skills, setSkills] = useState([])
  const [selectedSkill, setSelectedSkill] = useState(null)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [showEditForm, setShowEditForm] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  // 创建表单状态
  const [createForm, setCreateForm] = useState({
    name: '',
    description: '',
    content: ''
  })

  // 编辑表单状态
  const [editForm, setEditForm] = useState({
    name: '',
    description: '',
    content: ''
  })

  // 加载 skills 列表
  const loadSkills = async () => {
    try {
      setLoading(true)
      const response = await getSkills()
      if (response.success) {
        setSkills(response.data?.skills || [])
      }
    } catch (err) {
      setError(err.message || 'Failed to load skills')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadSkills()
  }, [])

  // 查看 skill 详情
  const handleViewSkill = async (name) => {
    try {
      setLoading(true)
      const response = await getSkill(name)
      if (response.success) {
        setSelectedSkill(response.data)
      }
    } catch (err) {
      setError(err.message || 'Failed to load skill')
    } finally {
      setLoading(false)
    }
  }

  // 创建 skill
  const handleCreateSkill = async (e) => {
    e.preventDefault()
    try {
      setLoading(true)
      setError('')
      const response = await createSkill(createForm.name, createForm.description, createForm.content)
      if (response.success) {
        setSuccess(response.data?.message || 'Skill created successfully')
        setShowCreateForm(false)
        setCreateForm({ name: '', description: '', content: '' })
        await loadSkills()
      }
    } catch (err) {
      setError(err.response?.data?.error || err.message || 'Failed to create skill')
    } finally {
      setLoading(false)
    }
  }

  // 更新 skill
  const handleUpdateSkill = async (e) => {
    e.preventDefault()
    try {
      setLoading(true)
      setError('')
      const response = await updateSkill(editForm.name, editForm.description, editForm.content)
      if (response.success) {
        setSuccess(response.data?.message || 'Skill updated successfully')
        setShowEditForm(false)
        setSelectedSkill(null)
        await loadSkills()
      }
    } catch (err) {
      setError(err.response?.data?.error || err.message || 'Failed to update skill')
    } finally {
      setLoading(false)
    }
  }

  // 删除 skill
  const handleDeleteSkill = async (name) => {
    if (!window.confirm(`Are you sure you want to delete skill "${name}"?`)) {
      return
    }
    try {
      setLoading(true)
      setError('')
      const response = await deleteSkill(name)
      if (response.success) {
        setSuccess(response.data?.message || 'Skill deleted successfully')
        setSelectedSkill(null)
        await loadSkills()
      }
    } catch (err) {
      setError(err.response?.data?.error || err.message || 'Failed to delete skill')
    } finally {
      setLoading(false)
    }
  }

  // 打开编辑表单
  const openEditForm = (skill) => {
    setEditForm({
      name: skill.name,
      description: skill.description,
      content: skill.content || ''
    })
    setShowEditForm(true)
  }

  return (
    <div className="skill-manager">
      <div className="skill-manager-header">
        <button className="back-btn" onClick={onBack}>
          ← {t('common.back', 'Back')}
        </button>
        <h1>{t('skill.title', 'Skill Management')}</h1>
        <button
          className="create-btn"
          onClick={() => setShowCreateForm(true)}
        >
          + {t('skill.create', 'Create Skill')}
        </button>
      </div>

      {error && (
        <div className="alert alert-error">
          {error}
          <button className="close-btn" onClick={() => setError('')}>×</button>
        </div>
      )}

      {success && (
        <div className="alert alert-success">
          {success}
          <button className="close-btn" onClick={() => setSuccess('')}>×</button>
        </div>
      )}

      <div className="skill-manager-content">
        {/* Skills 列表 */}
        <div className="skills-list">
          <h2>{t('skill.available', 'Available Skills')} ({skills.length})</h2>
          {loading && skills.length === 0 ? (
            <div className="loading">Loading...</div>
          ) : skills.length === 0 ? (
            <div className="empty-state">
              <p>{t('skill.noSkills', 'No skills available')}</p>
              <button
                className="create-btn"
                onClick={() => setShowCreateForm(true)}
              >
                {t('skill.createFirst', 'Create Your First Skill')}
              </button>
            </div>
          ) : (
            <div className="skills-grid">
              {skills.map((skill) => (
                <div
                  key={skill.name}
                  className="skill-card"
                  onClick={() => handleViewSkill(skill.name)}
                >
                  <h3>{skill.name}</h3>
                  <p className="skill-description">{skill.description}</p>
                  <div className="skill-actions">
                    <button
                      className="btn btn-primary"
                      onClick={(e) => {
                        e.stopPropagation()
                        handleViewSkill(skill.name)
                      }}
                    >
                      {t('skill.view', 'View')}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Skill 详情 */}
        {selectedSkill && (
          <div className="skill-detail">
            <div className="detail-header">
              <h2>{selectedSkill.name}</h2>
              <div className="detail-actions">
                <button
                  className="btn btn-secondary"
                  onClick={() => openEditForm(selectedSkill)}
                >
                  {t('skill.edit', 'Edit')}
                </button>
                <button
                  className="btn btn-danger"
                  onClick={() => handleDeleteSkill(selectedSkill.name)}
                >
                  {t('skill.delete', 'Delete')}
                </button>
              </div>
            </div>

            <div className="detail-meta">
              {selectedSkill.description && (
                <div className="meta-item">
                  <strong>{t('skill.description', 'Description')}:</strong>
                  <p>{selectedSkill.description}</p>
                </div>
              )}
              {selectedSkill.version && (
                <div className="meta-item">
                  <strong>{t('skill.version', 'Version')}:</strong>
                  <span>{selectedSkill.version}</span>
                </div>
              )}
              {selectedSkill.author && (
                <div className="meta-item">
                  <strong>{t('skill.author', 'Author')}:</strong>
                  <span>{selectedSkill.author}</span>
                </div>
              )}
              {selectedSkill.created && (
                <div className="meta-item">
                  <strong>{t('skill.created', 'Created')}:</strong>
                  <span>{selectedSkill.created}</span>
                </div>
              )}
              {selectedSkill.updated && (
                <div className="meta-item">
                  <strong>{t('skill.updated', 'Updated')}:</strong>
                  <span>{selectedSkill.updated}</span>
                </div>
              )}
              {selectedSkill.tags && selectedSkill.tags.length > 0 && (
                <div className="meta-item">
                  <strong>{t('skill.tags', 'Tags')}:</strong>
                  <div className="tags">
                    {selectedSkill.tags.map((tag, index) => (
                      <span key={index} className="tag">{tag}</span>
                    ))}
                  </div>
                </div>
              )}
              {selectedSkill.path && (
                <div className="meta-item">
                  <strong>{t('skill.path', 'Path')}:</strong>
                  <code>{selectedSkill.path}</code>
                </div>
              )}
            </div>

            {selectedSkill.content && (
              <div className="detail-content">
                <h3>{t('skill.content', 'Content')}</h3>
                <div className="content-preview">
                  <pre>{selectedSkill.content}</pre>
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* 创建表单 */}
      {showCreateForm && (
        <div className="modal-overlay" onClick={() => setShowCreateForm(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>{t('skill.createSkill', 'Create New Skill')}</h2>
              <button className="close-btn" onClick={() => setShowCreateForm(false)}>×</button>
            </div>
            <form onSubmit={handleCreateSkill}>
              <div className="form-group">
                <label>{t('skill.name', 'Name')} *</label>
                <input
                  type="text"
                  value={createForm.name}
                  onChange={(e) => setCreateForm({ ...createForm, name: e.target.value })}
                  required
                  placeholder="Enter skill name"
                />
              </div>
              <div className="form-group">
                <label>{t('skill.description', 'Description')} *</label>
                <textarea
                  value={createForm.description}
                  onChange={(e) => setCreateForm({ ...createForm, description: e.target.value })}
                  required
                  placeholder="Enter skill description"
                  rows="3"
                />
              </div>
              <div className="form-group">
                <label>{t('skill.content', 'Content')}</label>
                <textarea
                  value={createForm.content}
                  onChange={(e) => setCreateForm({ ...createForm, content: e.target.value })}
                  placeholder="Enter skill content (Markdown format)"
                  rows="10"
                />
                <small>Supports Markdown format. Leave empty to use default template.</small>
              </div>
              <div className="form-actions">
                <button type="button" className="btn btn-secondary" onClick={() => setShowCreateForm(false)}>
                  {t('common.cancel', 'Cancel')}
                </button>
                <button type="submit" className="btn btn-primary" disabled={loading}>
                  {loading ? 'Creating...' : t('skill.create', 'Create')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 编辑表单 */}
      {showEditForm && (
        <div className="modal-overlay" onClick={() => setShowEditForm(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2>{t('skill.editSkill', 'Edit Skill')}</h2>
              <button className="close-btn" onClick={() => setShowEditForm(false)}>×</button>
            </div>
            <form onSubmit={handleUpdateSkill}>
              <div className="form-group">
                <label>{t('skill.name', 'Name')}</label>
                <input
                  type="text"
                  value={editForm.name}
                  disabled
                  className="disabled"
                />
              </div>
              <div className="form-group">
                <label>{t('skill.description', 'Description')}</label>
                <textarea
                  value={editForm.description}
                  onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
                  placeholder="Enter skill description"
                  rows="3"
                />
              </div>
              <div className="form-group">
                <label>{t('skill.content', 'Content')} *</label>
                <textarea
                  value={editForm.content}
                  onChange={(e) => setEditForm({ ...editForm, content: e.target.value })}
                  required
                  placeholder="Enter skill content (Markdown format)"
                  rows="15"
                />
              </div>
              <div className="form-actions">
                <button type="button" className="btn btn-secondary" onClick={() => setShowEditForm(false)}>
                  {t('common.cancel', 'Cancel')}
                </button>
                <button type="submit" className="btn btn-primary" disabled={loading}>
                  {loading ? 'Saving...' : t('common.save', 'Save')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default SkillManager
