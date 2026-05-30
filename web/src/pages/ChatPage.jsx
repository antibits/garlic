import React, { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import SidebarTabs from '../components/SidebarTabs'
import ChatBox from '../components/ChatBox'
import api from '../services/api'
import './ChatPage.css'

/**
 * ChatPage 组件 - 支持多会话并发
 *
 * 设计说明：
 * - 维护所有会话的状态（消息、WebSocket 连接、加载状态等）
 * - 切换会话时只改变当前展示的会话，不关闭任何 WebSocket 连接
 * - 每个会话独立运行，互不干扰
 */
const ChatPage = ({ onOpenSettings }) => {
  const { t } = useTranslation()
  // 当前选中的会话 ID
  const [currentSessionId, setCurrentSessionId] = useState(null)
  const [initialized, setInitialized] = useState(false)

  // 会话状态管理 - 核心改进：为每个会话维护独立的状态
  const sessionStatesRef = useRef({}) // { [sessionId]: { messages, loading, streamingMessageId, copiedMessageId, wsRef } }

  // 会话列表
  const [sessions, setSessions] = useState([])

  // 强制更新，用于触发重新渲染
  const [, forceUpdate] = useState(0)

  // 流式更新节流：避免每个 token 都触发全量重渲染
  const lastUpdateRef = useRef(0)
  const pendingUpdateRef = useRef(null)

  const throttledForceUpdate = useCallback(() => {
    const now = Date.now()
    const elapsed = now - lastUpdateRef.current
    if (elapsed >= 50) {
      lastUpdateRef.current = now
      if (pendingUpdateRef.current) {
        clearTimeout(pendingUpdateRef.current)
        pendingUpdateRef.current = null
      }
      forceUpdate(n => n + 1)
    } else if (!pendingUpdateRef.current) {
      pendingUpdateRef.current = setTimeout(() => {
        lastUpdateRef.current = Date.now()
        pendingUpdateRef.current = null
        forceUpdate(n => n + 1)
      }, 50 - elapsed)
    }
  }, [])

  // 立即刷新：取消待处理的节流更新，立即渲染
  const flushForceUpdate = useCallback(() => {
    if (pendingUpdateRef.current) {
      clearTimeout(pendingUpdateRef.current)
      pendingUpdateRef.current = null
    }
    lastUpdateRef.current = Date.now()
    forceUpdate(n => n + 1)
  }, [])

  // 刷新会话元信息（会话名、消息数等）
  const refreshSessionMetadata = useCallback(async (sessionId) => {
    try {
      // 获取最新的会话列表信息
      const response = await api.getSessions()
      const sessionsData = response.data?.sessions || []
      const updatedSession = sessionsData.find(s => s.id === sessionId)

      if (updatedSession) {
        // 更新会话列表中的会话信息
        setSessions(prev => prev.map(s =>
          s.id === sessionId ? updatedSession : s
        ))
        console.log('Session metadata refreshed:', sessionId, 'message_count:', updatedSession.message_count)
      }
    } catch (error) {
      console.error('Failed to refresh session metadata:', error)
    }
  }, [])

  // WebSocket 消息处理
  const handleWebSocketMessage = useCallback((sessionId, data) => {
    const state = sessionStatesRef.current[sessionId]
    if (!state) {
      console.warn('No state found for session:', sessionId)
      return
    }

    console.log('WebSocket message received:', sessionId, data)

    switch (data.type) {
      case 'chunk':
        const chunkMessageType = data.data?.message_type || 'user'
        const chunkContent = data.data?.content || ''

        if (chunkMessageType === 'hidden') {
          return
        }

        const messages = state.messages
        const lastBotMessage = messages.findLast(msg => msg.role === 'assistant' && msg.streaming)

        if (lastBotMessage) {
          const lastMsgType = lastBotMessage.message_type || 'user'
          if (chunkMessageType !== lastMsgType) {
            const updatedMessages = messages.map(msg =>
              msg.id === lastBotMessage.id
                ? { ...msg, streaming: false }
                : msg
            )
            if (chunkMessageType === 'auto') {
              state.messages = [...updatedMessages, {
                id: `auto-${Date.now()}`,
                role: 'assistant',
                content: chunkContent,
                timestamp: new Date().toISOString(),
                streaming: true,
                message_type: 'auto'
              }]
            } else {
              state.messages = [...updatedMessages, {
                id: `bot-${Date.now()}`,
                role: 'assistant',
                content: chunkContent,
                timestamp: new Date().toISOString(),
                streaming: true,
                message_type: chunkMessageType
              }]
            }
          } else {
            state.messages = messages.map(msg =>
              msg.id === lastBotMessage.id
                ? { ...msg, content: msg.content + chunkContent, message_type: chunkMessageType }
                : msg
            )
          }
        } else {
          if (chunkMessageType === 'auto') {
            state.messages = [...messages, {
              id: `auto-${Date.now()}`,
              role: 'assistant',
              content: chunkContent,
              timestamp: new Date().toISOString(),
              streaming: true,
              message_type: 'auto'
            }]
          } else {
            state.messages = [...messages, {
              id: `bot-${Date.now()}`,
              role: 'assistant',
              content: chunkContent,
              timestamp: new Date().toISOString(),
              streaming: true,
              message_type: chunkMessageType
            }]
          }
        }
        throttledForceUpdate()
        break

      case 'message':
        const finalMessageType = data.data?.message_type || 'user'
        const finalContent = data.data?.content || data.data

        const lastBotMessage2 = state.messages.findLast(msg => msg.role === 'assistant' && msg.streaming)

        if (lastBotMessage2) {
          state.messages = state.messages.map(msg =>
            msg.id === lastBotMessage2.id
              ? { ...msg, content: msg.content || finalContent, streaming: false, message_type: finalMessageType }
              : msg
          )
        } else {
          const existingMessage = state.messages.find(msg =>
            msg.role === 'assistant' &&
            msg.content === finalContent &&
            !msg.streaming
          )
          if (!existingMessage) {
            state.messages = [...state.messages, {
              id: `bot-${Date.now()}`,
              role: 'assistant',
              content: finalContent,
              timestamp: new Date().toISOString(),
              streaming: false,
              message_type: finalMessageType
            }]
          }
        }
        state.streamingMessageId = null
        state.loading = false
        flushForceUpdate()

        // 会话完成后，刷新会话元信息（会话名、消息数等）
        refreshSessionMetadata(sessionId)
        break

      case 'error':
        const lastBotMessage3 = state.messages.findLast(msg => msg.role === 'assistant' && msg.streaming)
        if (lastBotMessage3) {
          state.messages = state.messages.map(msg =>
            msg.id === lastBotMessage3.id
              ? { ...msg, streaming: false, content: t('chat.error', { error: data.data }) }
              : msg
          )
        }
        state.streamingMessageId = null
        state.loading = false
        flushForceUpdate()
        break

      case 'stopped':
        // Handle user-initiated stop
        const lastBotMessage4 = state.messages.findLast(msg => msg.role === 'assistant' && msg.streaming)
        if (lastBotMessage4) {
          state.messages = state.messages.map(msg =>
            msg.id === lastBotMessage4.id
              ? { ...msg, streaming: false }
              : msg
          )
        }
        state.streamingMessageId = null
        state.loading = false
        flushForceUpdate()
        break

      default:
        console.log('Unknown message type:', data.type)
    }
  }, [t, refreshSessionMetadata, flushForceUpdate])

  const handleWebSocketOpen = useCallback((sessionId) => {
    console.log('WebSocket connected for session:', sessionId)
    // 触发重新渲染，更新输入框的禁用状态
    forceUpdate(n => n + 1)
  }, [])

  const handleWebSocketError = useCallback((error) => {
    console.error('WebSocket error:', error)
  }, [])

  const handleWebSocketClose = useCallback(() => {
    console.log('WebSocket disconnected')
  }, [])

  // 连接 WebSocket（每个会话独立连接）
  const connectWebSocket = useCallback((sessionId, retryCount = 0) => {
    return new Promise((resolve, reject) => {
      const state = sessionStatesRef.current[sessionId]
      if (!state) {
        console.warn('Cannot connect WebSocket, no state for session:', sessionId)
        reject(new Error('No state for session'))
        return
      }

      // 如果已有连接且状态正常，直接返回
      if (state.wsRef.current) {
        const readyState = state.wsRef.current.readyState
        if (readyState === WebSocket.OPEN) {
          console.log('WebSocket already connected for session:', sessionId)
          resolve(state.wsRef.current)
          return
        }
        if (readyState === WebSocket.CONNECTING) {
          console.log('WebSocket already connecting for session:', sessionId)
          // 等待连接完成
          state.wsRef.current.addEventListener('open', () => resolve(state.wsRef.current))
          state.wsRef.current.addEventListener('error', (err) => reject(err))
          return
        }
        // 连接已关闭，清理
        console.log('Closing stale WebSocket connection for session:', sessionId)
        state.wsRef.current.close()
        state.wsRef.current = null
      }

      console.log('Connecting WebSocket for session:', sessionId, 'retry:', retryCount)

      const ws = api.createWebSocketConnection(
        sessionId,
        (data) => handleWebSocketMessage(sessionId, data),
        (error) => {
          console.error('WebSocket error event:', error)
          // 不立即 reject，等待 onclose 事件
        },
        () => {
          console.log('WebSocket connected successfully')
          handleWebSocketOpen(sessionId)
          resolve(ws)
        },
        () => {
          console.log('WebSocket closed')
          handleWebSocketClose()
          // 连接关闭时，如果是非正常关闭，尝试重连
          if (retryCount < 3) {
            console.log(`Retrying WebSocket connection... attempt ${retryCount + 1}`)
            setTimeout(() => {
              connectWebSocket(sessionId, retryCount + 1)
                .then(resolve)
                .catch(reject)
            }, 1000 * (retryCount + 1)) // 递增延迟：1s, 2s, 3s
          } else {
            reject(new Error('WebSocket connection failed after retries'))
          }
        }
      )

      state.wsRef.current = ws

      // 超时处理（30秒）
      const timeoutId = setTimeout(() => {
        if (ws.readyState !== WebSocket.OPEN) {
          console.error('WebSocket connection timeout', {
            sessionId,
            retryCount,
            readyState: ws.readyState,
            url: ws.url
          })
          ws.close()
          if (retryCount < 3) {
            connectWebSocket(sessionId, retryCount + 1)
              .then(resolve)
              .catch(reject)
          } else {
            reject(new Error('WebSocket connection timeout after 30 seconds'))
          }
        }
      }, 30000)

      // 清理超时定时器
      ws.addEventListener('open', () => {
        clearTimeout(timeoutId)
      })
    })
  }, [handleWebSocketMessage, handleWebSocketError, handleWebSocketOpen, handleWebSocketClose])

  // 断开 WebSocket
  const disconnectWebSocket = useCallback((sessionId) => {
    const state = sessionStatesRef.current[sessionId]
    if (state && state.wsRef.current) {
      console.log('Disconnecting WebSocket for session:', sessionId)
      state.wsRef.current.close()
      state.wsRef.current = null
    }
  }, [])

  // 初始化会话
  useEffect(() => {
    if (initialized) return // 防止重复初始化

    const initSession = async () => {
      try {
        const response = await api.getSessions()
        const sessionsData = response.data?.sessions || []
        const currentId = response.data?.current_id

        console.log('Initialize sessions:', sessionsData, 'current_id:', currentId)

        if (sessionsData.length > 0) {
          const sessionIdToUse = currentId || sessionsData[0].id
          setCurrentSessionId(sessionIdToUse)
          setSessions(sessionsData)
          // 为每个已有会话初始化状态
          sessionsData.forEach(session => {
            if (!sessionStatesRef.current[session.id]) {
              sessionStatesRef.current[session.id] = {
                messages: [],
                loading: false,
                streamingMessageId: null,
                copiedMessageId: null,
                wsRef: { current: null }
              }
            }
          })

          // 加载当前会话的历史消息
          const currentSession = sessionsData.find(s => s.id === sessionIdToUse)
          if (currentSession && currentSession.message_count > 0) {
            try {
              const historyResponse = await api.getSessionMessages(sessionIdToUse)
              const messagesData = historyResponse.data?.messages || []
              console.log('Loaded initial history messages:', messagesData.length)

              const formattedMessages = messagesData.map(msg => ({
                id: `${msg.role}-${msg.timestamp}-${Math.random().toString(36).substr(2, 9)}`,
                role: msg.role,
                content: msg.content,
                timestamp: msg.timestamp,
                message_type: msg.type || 'user',
                streaming: false
              }))

              sessionStatesRef.current[sessionIdToUse].messages = formattedMessages
            } catch (error) {
              console.error('Failed to load initial history messages:', error)
            }
          }

          forceUpdate(n => n + 1)

          // 初始化完成后，为当前会话连接 WebSocket（不阻塞初始化）
          connectWebSocket(sessionIdToUse).catch(error => {
            console.error('Failed to connect WebSocket for initial session:', error)
          })
        } else {
          const createResponse = await api.createSession()
          const newSession = createResponse.data
          console.log('Created new session:', newSession)
          setCurrentSessionId(newSession.id)
          setSessions([newSession])
          sessionStatesRef.current[newSession.id] = {
            messages: [],
            loading: false,
            streamingMessageId: null,
            copiedMessageId: null,
            wsRef: { current: null }
          }
          forceUpdate(n => n + 1)
          // 新会话创建后连接 WebSocket（不阻塞）
          connectWebSocket(newSession.id).catch(error => {
            console.error('Failed to connect WebSocket for new session:', error)
          })
        }
      } catch (error) {
        console.error('Failed to initialize session:', error)
      } finally {
        setInitialized(true)
      }
    }

    initSession()
  }, [initialized, connectWebSocket])

  // 发送消息
  const handleSendMessage = useCallback(async (sessionId, content) => {
    const state = sessionStatesRef.current[sessionId]
    if (!state) {
      console.error('Cannot send message, no state for session:', sessionId)
      return
    }

    // 检查 WebSocket 连接状态，如果断开则重新连接
    if (!state.wsRef.current || state.wsRef.current.readyState !== WebSocket.OPEN) {
      console.log('WebSocket not connected, reconnecting before sending message...')
      try {
        await connectWebSocket(sessionId)
        console.log('WebSocket reconnected successfully')
      } catch (error) {
        console.error('Failed to reconnect WebSocket:', error)
        // 连接失败，但仍尝试发送消息（可能会失败）
      }
    }

    const userMessage = {
      id: `user-${Date.now()}`,
      role: 'user',
      content: content,
      timestamp: new Date().toISOString()
    }

    const botMessageId = `bot-${Date.now()}`
    const botMessage = {
      id: botMessageId,
      role: 'assistant',
      content: '',
      timestamp: new Date().toISOString(),
      streaming: true
    }

    state.messages = [...state.messages, userMessage, botMessage]
    state.loading = true
    state.streamingMessageId = botMessageId
    forceUpdate(n => n + 1)

    api.sendWebSocketMessage(state.wsRef.current, content)
  }, [connectWebSocket])

  // 停止生成
  const handleStopGenerating = useCallback(async (sessionId) => {
    try {
      // 调用后端停止 API
      await api.stopSession(sessionId)
      console.log('Session request cancelled:', sessionId)
      
      // 更新前端状态
      const state = sessionStatesRef.current[sessionId]
      if (state) {
        // 将正在流式传输的消息标记为非流式
        state.messages = state.messages.map(msg =>
          msg.streaming ? { ...msg, streaming: false } : msg
        )
        state.loading = false
        state.streamingMessageId = null
        forceUpdate(n => n + 1)
      }
    } catch (error) {
      console.error('Failed to stop session:', error)
    }
  }, [])

  // 处理会话选择 - 关键改进：只切换显示，不断开连接
  const handleSelectSession = useCallback((sessionId) => {
    console.log('Switching to session:', sessionId)
    // 切换后端当前会话
    api.switchSession(sessionId).catch(console.error)
    // 更新前端选中状态
    setCurrentSessionId(sessionId)
    // 刷新会话元信息
    refreshSessionMetadata(sessionId)

    // 如果当前会话没有消息，从后台加载历史消息
    const state = sessionStatesRef.current[sessionId]
    if (state && state.messages.length === 0) {
      console.log('Loading history messages for session:', sessionId)
      api.getSessionMessages(sessionId)
        .then(response => {
          const messagesData = response.data?.messages || []
          console.log('Loaded history messages:', messagesData.length)

          // 将历史消息转换为前端格式
          const formattedMessages = messagesData.map(msg => ({
            id: `${msg.role}-${msg.timestamp}-${Math.random().toString(36).substr(2, 9)}`,
            role: msg.role,
            content: msg.content,
            timestamp: msg.timestamp,
            message_type: msg.type || 'user',
            streaming: false
          }))

          // 更新会话状态
          const currentState = sessionStatesRef.current[sessionId]
          if (currentState) {
            currentState.messages = formattedMessages
            forceUpdate(n => n + 1)
          }
        })
        .catch(error => {
          console.error('Failed to load history messages:', error)
        })
    }
    // 注意：不断开任何 WebSocket 连接，所有会话保持活跃
  }, [refreshSessionMetadata])

  // 处理创建会话
  const handleCreateSession = useCallback(async () => {
    try {
      const response = await api.createSession()
      const newSession = response.data
      console.log('Created new session:', newSession)
      setSessions(prev => [newSession, ...prev])
      sessionStatesRef.current[newSession.id] = {
        messages: [],
        loading: false,
        streamingMessageId: null,
        copiedMessageId: null,
        wsRef: { current: null }
      }
      setCurrentSessionId(newSession.id)
      // 新会话创建后连接 WebSocket（不阻塞）
      connectWebSocket(newSession.id).catch(error => {
        console.error('Failed to connect WebSocket for new session:', error)
      })
      // 刷新会话元信息
      refreshSessionMetadata(newSession.id)
      forceUpdate(n => n + 1)
    } catch (error) {
      console.error('Failed to create session:', error)
    }
  }, [connectWebSocket, refreshSessionMetadata])

  // 处理删除会话
  const handleDeleteSession = useCallback((sessionId) => {
    console.log('Deleting session:', sessionId)
    // 先断开 WebSocket 连接
    disconnectWebSocket(sessionId)
    // 清理状态
    delete sessionStatesRef.current[sessionId]
    // 更新会话列表
    setSessions(prev => prev.filter(s => s.id !== sessionId))
    if (currentSessionId === sessionId) {
      setCurrentSessionId(null)
    }
    forceUpdate(n => n + 1)
  }, [disconnectWebSocket, currentSessionId])

  // 自动为当前会话连接 WebSocket
  useEffect(() => {
    if (currentSessionId && initialized) {
      const state = sessionStatesRef.current[currentSessionId]
      console.log('Effect: currentSessionId=', currentSessionId, 'state exists:', !!state, 'ws:', state?.wsRef?.current)
      if (state) {
        if (!state.wsRef.current || state.wsRef.current.readyState !== WebSocket.OPEN) {
          console.log('Auto-connecting WebSocket for session:', currentSessionId)
          connectWebSocket(currentSessionId)
        } else {
          console.log('WebSocket already connected for session:', currentSessionId)
        }
      }
    }
  }, [currentSessionId, initialized, connectWebSocket])

  // 组件卸载时断开所有连接
  useEffect(() => {
    return () => {
      console.log('Cleanup: disconnecting all WebSocket connections')
      Object.keys(sessionStatesRef.current).forEach(sessionId => {
        disconnectWebSocket(sessionId)
      })
    }
  }, [disconnectWebSocket])

  if (!initialized) {
    return (
      <div className="chat-page loading">
        <div className="loading-screen">
          <div className="loading-icon">🧄</div>
          <h2>Garlic AI Agent</h2>
          <p>{t('chat.initializing')}</p>
        </div>
      </div>
    )
  }

  // 获取当前会话的状态
  const currentState = currentSessionId ? sessionStatesRef.current[currentSessionId] : null

  return (
    <div className="chat-page">
      <SidebarTabs
        currentSessionId={currentSessionId}
        sessions={sessions}
        setSessions={setSessions}
        onSelectSession={handleSelectSession}
        onCreateSession={handleCreateSession}
        onDeleteSession={handleDeleteSession}
        onOpenSettings={onOpenSettings}
      />

      <div className="main-content">
        {currentSessionId && currentState ? (
          <ChatBox
            sessionId={currentSessionId}
            wsRef={currentState.wsRef}
            wsReadyState={currentState.wsRef?.current?.readyState}
            messages={currentState.messages}
            loading={currentState.loading}
            streamingMessageId={currentState.streamingMessageId}
            copiedMessageId={currentState.copiedMessageId}
            setCopiedMessageId={(id) => {
              currentState.copiedMessageId = id
              forceUpdate(n => n + 1)
            }}
            onSendMessage={(content) => handleSendMessage(currentSessionId, content)}
            onStopGenerating={() => handleStopGenerating(currentSessionId)}
          />
        ) : (
          <div className="empty-chat">
            <div className="empty-state-large">
              <span className="empty-icon">🧄</span>
              <h2>{t('chat.welcome')}</h2>
              <p>{t('chat.selectOrCreateSession')}</p>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default ChatPage
