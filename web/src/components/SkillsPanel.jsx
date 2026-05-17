import React, { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { BookOpen, Plus, Eye, Edit2, Trash2 } from 'lucide-react'
import { getSkills, getSkill, createSkill, updateSkill, deleteSkill } from '../services/api'
import './SkillsPanel.css'

const SkillsPanel = () => {
  const { t } = useTranslation()
  const [skills, setSkills] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedSkill, setSelectedSkill] = useState(null)
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [showEditForm, setShowEditForm] = useState(false)
  const [createForm, setCreateForm] = useState({ name: '', description: '', content: '' })
  const [editForm, setEditForm] = useState({ name: '', description: '', content: '' })
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const loadSkills = async () => {
    try {
      setLoading(true)
      const response = await getSkills()
      if (response.success) {
        setSkills(response.data?.skills || [])
      }
    } catch (err) {
      console.error('Failed to load skills:', err)
      setError(err.message || 'Failed to load skills')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadSkills()
  }, [])

  const handleViewSkill = async (name) => {
    try {
      const response = await getSkill(name)
      if (response.success) {
        setSelectedSkill(response.data)
      }
    } catch (err) {
      setError(err.message || 'Failed to load skill')
    }
  }

  const handleCreateSkill = async (e) => {
    e.preventDefault()
    try {
      setError('')
      const response = await createSkill(createForm.name, createForm.description, createForm.content)
      if (response.success) {
        setSuccess(response.data?.message || 'Skill created successfully')
        setShowCreateForm(false)
        setCreateForm({ name: '', description: '', content: '' })
        await loadSkills()
        setTimeout(() => setSuccess(''), 3000)
      }
    } catch (err) {
      setError(err.response?.data?.error || err.message || 'Failed to create skill')
    }
  }

  const handleUpdateSkill = async (e) => {
    e.preventDefault()
    try {
      setError('')
      const response = await updateSkill(editForm.name, editForm.description, editForm.content)
      if (response.success) {
        setSuccess(response.data?.message || 'Skill updated successfully')
        setShowEditForm(false)
        setSelectedSkill(null)
        await loadSkills()
        setTimeout(() => setSuccess(''), 3000)
      }
    } catch (err) {
      setError(err.response?.data?.error || err.message || 'Failed to update skill')
    }
  }

  const handleDeleteSkill = async (name) => {
    if (!window.confirm(`Are you sure you want to delete skill "${name}"?`)) {
      return
    }
    try {
      setError('')
      const response = await deleteSkill(name)
      if (response.success) {
        setSuccess(response.data?.message || 'Skill deleted successfully')
        setSelectedSkill(null)
        await loadSkills()
        setTimeout(() => setSuccess(''), 3000)
      }
    } catch (err) {
      setError(err.response?.data?.error || err.message || 'Failed to delete skill')
    }
  }

  const openEditForm = (skill) => {
    setEditForm({
      name: skill.name,
      description: skill.description,
      content: skill.content || ''
    })
    setShowEditForm(true)
  }

  return (
    <div className="skills-panel">
      <div className="skills-header">
        <h2>{t('skill.title', 'Skills')} ({skills.length})</h2>
        <button 
          className="btn-add-skill" 
          title={t('skill.create', 'Create Skill')}
          onClick={() => setShowCreateForm(true)}
        >
          <Plus size={18} />
        </button>
      </div>

      {error && (
        <div className="skill-alert skill-alert-error">
          {error}
          <button className="skill-alert-close" onClick={() => setError('')}>×</button>
        </div>
      )}

      {success && (
        <div className="skill-alert skill-alert-success">
          {success}
          <button className="skill-alert-close" onClick={() => setSuccess('')}>×</button>
        </div>
      )}

      <div className="skills-content">
        {loading ? (
          <div className="loading">{t('common.loading', 'Loading...')}</div>
        ) : skills.length === 0 ? (
          <div className="empty-state">
            <BookOpen size={48} />
            <p>{t('skill.noSkills', 'No skills available')}</p>
            <button 
              className="btn-primary"
              onClick={() => setShowCreateForm(true)}
            >
              {t('skill.createFirst', 'Create Your First Skill')}
            </button>
          </div>
        ) : (
          <div className="skills-list">
            {skills.map((skill) => (
              <div 
                key={skill.name} 
                className={`skill-item ${selectedSkill?.name === skill.name ? 'active' : ''}`}
                onClick={() => handleViewSkill(skill.name)}
              >
                <div className="skill-icon">
                  <BookOpen size={20} />
                </div>
                <div className="skill-info">
                  <div className="skill-name">{skill.name}</div>
                  <div className="skill-description">{skill.description}</div>
                </div>
                <div className="skill-actions">
                  <button 
                    className="btn-skill-action" 
                    title={t('skill.view', 'View')}
                    onClick={(e) => {
                      e.stopPropagation()
                      handleViewSkill(skill.name)
                    }}
                  >
                    <Eye size={16} />
                  </button>
                  <button 
                    className="btn-skill-action" 
                    title={t('skill.edit', 'Edit')}
                    onClick={(e) => {
                      e.stopPropagation()
                      openEditForm(skill)
                    }}
                  >
                    <Edit2 size={16} />
                  </button>
                  <button 
                    className="btn-skill-action btn-delete" 
                    title={t('skill.delete', 'Delete')}
                    onClick={(e) => {
                      e.stopPropagation()
                      handleDeleteSkill(skill.name)
                    }}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Skill 详情 */}
      {selectedSkill && (
        <div className="skill-detail-panel">
          <div className="detail-panel-header">
            <h3>{selectedSkill.name}</h3>
            <button className="btn-close-detail" onClick={() => setSelectedSkill(null)}>×</button>
          </div>
          <div className="detail-panel-content">
            {selectedSkill.description && (
              <div className="detail-meta-item">
                <strong>{t('skill.description', 'Description')}:</strong>
                <p>{selectedSkill.description}</p>
              </div>
            )}
            {selectedSkill.tags && selectedSkill.tags.length > 0 && (
              <div className="detail-meta-item">
                <strong>{t('skill.tags', 'Tags')}:</strong>
                <div className="tags">
                  {selectedSkill.tags.map((tag, index) => (
                    <span key={index} className="tag">{tag}</span>
                  ))}
                </div>
              </div>
            )}
            {selectedSkill.content && (
              <div className="detail-content">
                <strong>{t('skill.content', 'Content')}:</strong>
                <pre className="content-preview">{selectedSkill.content}</pre>
              </div>
            )}
            <div className="detail-actions">
              <button 
                className="btn-primary"
                onClick={() => openEditForm(selectedSkill)}
              >
                <Edit2 size={16} />
                {t('skill.edit', 'Edit')}
              </button>
              <button 
                className="btn-danger"
                onClick={() => handleDeleteSkill(selectedSkill.name)}
              >
                <Trash2 size={16} />
                {t('skill.delete', 'Delete')}
              </button>
            </div>
          </div>
        </div>
      )}

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
                  rows="8"
                />
                <small>Supports Markdown format. Leave empty to use default template.</small>
              </div>
              <div className="form-actions">
                <button type="button" className="btn-secondary" onClick={() => setShowCreateForm(false)}>
                  {t('common.cancel', 'Cancel')}
                </button>
                <button type="submit" className="btn-primary">
                  {t('skill.create', 'Create')}
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
                  rows="12"
                />
              </div>
              <div className="form-actions">
                <button type="button" className="btn-secondary" onClick={() => setShowEditForm(false)}>
                  {t('common.cancel', 'Cancel')}
                </button>
                <button type="submit" className="btn-primary">
                  {t('common.save', 'Save')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}

export default SkillsPanel
