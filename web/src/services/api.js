import axios from 'axios'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api'

const api = axios.create({
  baseURL: API_BASE_URL,
  timeout: 60000,
  headers: {
    'Content-Type': 'application/json'
  }
})

// 更新 baseURL
export const setBaseUrl = (url) => {
  api.defaults.baseURL = url
}

// ==================== 会话管理 API ====================

// 获取所有会话
export const getSessions = async () => {
  const response = await api.get('/sessions')
  return response.data
}

// 创建新会话
export const createSession = async (name = '') => {
  const response = await api.post('/sessions', { name })
  return response.data
}

// 获取单个会话详情
export const getSession = async (sessionId) => {
  const response = await api.get(`/sessions/${sessionId}`)
  return response.data
}

// 获取会话历史消息列表
export const getSessionMessages = async (sessionId) => {
  const response = await api.get(`/sessions/${sessionId}/messages`)
  return response.data
}

// 删除会话
export const deleteSession = async (sessionId) => {
  const response = await api.delete(`/sessions/${sessionId}`)
  return response.data
}

// 更新会话（切换当前会话或修改名称）
export const updateSession = async (sessionId, data) => {
  const response = await api.put(`/sessions/${sessionId}`, data)
  return response.data
}

// 切换当前会话
export const switchSession = async (sessionId) => {
  const response = await api.put(`/sessions/${sessionId}`, { set_current: true })
  return response.data
}

// 停止会话中正在进行的请求
export const stopSession = async (sessionId) => {
  const response = await api.post(`/sessions/${sessionId}/stop`)
  return response.data
}

// ==================== 消息 API ====================

// 发送消息（HTTP 非流式）
export const sendMessage = async (sessionId, message) => {
  const response = await api.post(`/messages/${sessionId}`, {
    message: message
  })
  return response.data
}

// 发送流式消息（使用 WebSocket）
export const createWebSocketConnection = (sessionId, onMessage, onError, onOpen, onClose) => {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${protocol}//${window.location.host}/ws/${sessionId}`

  const ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    if (onOpen) onOpen(sessionId)
  }

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (onMessage) onMessage(data)
    } catch (error) {
      console.error('Failed to parse WebSocket message:', error)
    }
  }

  ws.onerror = (error) => {
    console.error('WebSocket error:', error)
    if (onError) onError(error)
  }

  ws.onclose = () => {
    if (onClose) onClose()
  }

  return ws
}

// 发送 WebSocket 消息
export const sendWebSocketMessage = (ws, content) => {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({
      type: 'message',
      content: content
    }))
  }
}

// ==================== 工具 API ====================

// 获取所有工具
export const getAllTools = async () => {
  const response = await api.get('/tools')
  return response.data
}

// 获取单个工具详情
export const getTool = async (toolName) => {
  const response = await api.get(`/tools/${toolName}`)
  return response.data
}

// 启用工具
export const enableTool = async (toolName) => {
  const response = await api.put(`/tools/${toolName}/enable`)
  return response.data
}

// 禁用工具
export const disableTool = async (toolName) => {
  const response = await api.put(`/tools/${toolName}/disable`)
  return response.data
}

// 导入工具
export const importTool = async (formData) => {
  const response = await api.post('/tools/import', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
  return response.data
}

// 删除工具
export const deleteTool = async (toolName) => {
  const response = await api.delete(`/tools/${toolName}`)
  return response.data
}

// 执行工具
export const executeTool = async (toolName, args) => {
  const response = await api.post('/tools/execute', {
    tool: toolName,
    args: args
  })
  return response.data
}

// ==================== 记忆 API ====================

// 获取记忆列表
export const getMemories = async () => {
  const response = await api.get('/memories')
  return response.data
}

// 清除所有记忆
export const clearMemories = async () => {
  const response = await api.delete('/memories')
  return response.data
}

// ==================== 健康检查 ====================

export const healthCheck = async () => {
  const response = await api.get('/health')
  return response.data
}

// ==================== 配置 API ====================

// 获取配置
export const getConfig = async () => {
  const response = await api.get('/config')
  return response.data
}

// 保存配置
export const saveConfig = async (config) => {
  const response = await api.put('/config', config)
  return response.data
}

// ==================== Skill 管理 API ====================

// 获取所有 skills
export const getSkills = async () => {
  const response = await api.get('/skills')
  return response.data
}

// 获取单个 skill 详情
export const getSkill = async (name) => {
  const response = await api.get(`/skills/${name}`)
  return response.data
}

// 创建新 skill
export const createSkill = async (name, description, content = '', withScripts = false) => {
  const response = await api.post('/skills', {
    name,
    description,
    content,
    with_scripts: withScripts
  })
  return response.data
}

// 导入 skill
export const importSkill = async (formData) => {
  const response = await api.post('/skills/import', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
  return response.data
}

// 更新 skill
export const updateSkill = async (name, description, content) => {
  const response = await api.put(`/skills/${name}`, {
    description,
    content
  })
  return response.data
}

// 删除 skill
export const deleteSkill = async (name) => {
  const response = await api.delete(`/skills/${name}`)
  return response.data
}

// 启用 skill
export const enableSkill = async (name) => {
  const response = await api.put(`/skills/${name}/enable`)
  return response.data
}

// 禁用 skill
export const disableSkill = async (name) => {
  const response = await api.put(`/skills/${name}/disable`)
  return response.data
}

// 导出默认对象
const apiService = {
  setBaseUrl,
  // 会话管理
  getSessions,
  createSession,
  getSession,
  getSessionMessages,
  deleteSession,
  updateSession,
  switchSession,
  stopSession,
  // 消息
  sendMessage,
  createWebSocketConnection,
  sendWebSocketMessage,
  // 工具
  getAllTools,
  getTool,
  enableTool,
  disableTool,
  importTool,
  deleteTool,
  executeTool,
  // 记忆
  getMemories,
  clearMemories,
  // 健康检查
  healthCheck,
  // 配置
  getConfig,
  saveConfig,
  // Skill 管理
  getSkills,
  getSkill,
  createSkill,
  importSkill,
  updateSkill,
  deleteSkill,
  enableSkill,
  disableSkill
}

export default apiService
