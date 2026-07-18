import React, { useState, useEffect, useRef, useCallback, useMemo, memo } from 'react'
import { useTranslation } from 'react-i18next'
import { Send, StopCircle, ArrowDown } from 'lucide-react'
import MessageBubble from './MessageBubble'
import './ChatBox.css'

/**
 * WelcomeScreen - 欢迎界面，独立组件避免随消息列表重渲染
 */
const WelcomeScreen = memo(({ t }) => (
  <div className="welcome-message">
    <div className="welcome-icon">🧄</div>
    <h3>{t('chat.welcome')}</h3>
    <p>{t('chat.welcomeMessage')}</p>
    <div className="welcome-tips">
      <div className="tip-item">{t('chat.tips.multiTurn')}</div>
      <div className="tip-item">{t('chat.tips.realtime')}</div>
      <div className="tip-item">{t('chat.tips.shortcut')}</div>
    </div>
  </div>
))

WelcomeScreen.displayName = 'WelcomeScreen'

/**
 * ChatBox 组件 - 支持多会话并发
 *
 * 性能优化：
 * - React.memo 避免父组件无关更新导致的重渲染
 * - useMemo 缓存 getMessageGroups 计算结果
 * - 提取 MessageBubble 为 memo 组件，已完成的消息不重渲染
 * - requestAnimationFrame 优化滚动性能
 */
const ChatBox = memo(({
  sessionId,
  wsRef,
  wsReadyState,
  messages,
  loading,
  streamingMessageId,
  copiedMessageId,
  onSendMessage,
  onStopGenerating,
  setCopiedMessageId
}) => {
  const { t } = useTranslation()
  const [input, setInput] = useState('')

  const messagesEndRef = useRef(null)
  const textareaRef = useRef(null)
  const messagesContainerRef = useRef(null)
  const scrollRafRef = useRef(null)
  const isNearBottomRef = useRef(true)
  const [showScrollButton, setShowScrollButton] = useState(false)

  // 使用 rAF 优化的滚动到底部
  const scrollToBottom = useCallback(() => {
    if (scrollRafRef.current) {
      cancelAnimationFrame(scrollRafRef.current)
    }
    scrollRafRef.current = requestAnimationFrame(() => {
      scrollRafRef.current = null
      const el = messagesEndRef.current
      if (el) {
        el.scrollIntoView({ behavior: 'instant', block: 'end' })
      }
    })
  }, [])

  // 检测用户是否在底部附近
  const checkNearBottom = useCallback(() => {
    const container = messagesContainerRef.current
    if (!container) return true
    const threshold = 150
    return container.scrollHeight - container.scrollTop - container.clientHeight < threshold
  }, [])

  // 消息变化时滚动，但仅在用户在底部附近时才自动滚动
  useEffect(() => {
    if (isNearBottomRef.current) {
      scrollToBottom()
    }
  }, [messages, scrollToBottom])

  // 监听用户滚动，记录是否在底部，并控制回到底部按钮的显示
  useEffect(() => {
    const container = messagesContainerRef.current
    if (!container) return

    const handleScroll = () => {
      const nearBottom = checkNearBottom()
      isNearBottomRef.current = nearBottom
      // 仅当状态真正变化时才更新，避免不必要的重渲染
      setShowScrollButton(prev => {
        const shouldShow = !nearBottom
        return prev !== shouldShow ? shouldShow : prev
      })
    }

    container.addEventListener('scroll', handleScroll, { passive: true })
    return () => container.removeEventListener('scroll', handleScroll)
  }, [checkNearBottom])

  // 点击回到底部按钮
  const handleScrollToBottom = useCallback(() => {
    isNearBottomRef.current = true
    setShowScrollButton(false)
    scrollToBottom()
  }, [scrollToBottom])

  // 清理 rAF
  useEffect(() => {
    return () => {
      if (scrollRafRef.current) {
        cancelAnimationFrame(scrollRafRef.current)
      }
    }
  }, [])

  // 调整 textarea 高度
  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 200) + 'px'
    }
  }, [input])

  const handleSend = useCallback(() => {
    const trimmedInput = input.trim()
    const wsConnected = wsRef?.current && wsRef.current.readyState === WebSocket.OPEN
    if (!trimmedInput || loading || !wsConnected) return

    onSendMessage(trimmedInput)
    setInput('')

    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }, [input, loading, wsRef, onSendMessage])

  const handleStop = useCallback(() => {
    onStopGenerating()
  }, [onStopGenerating])

  const handleKeyPress = useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }, [handleSend])

  const handleCopy = useCallback(async (content, messageId) => {
    if (!setCopiedMessageId) return
    try {
      await navigator.clipboard.writeText(content)
      setCopiedMessageId(messageId)
      setTimeout(() => setCopiedMessageId(null), 2000)
    } catch (error) {
      console.error('Failed to copy:', error)
    }
  }, [setCopiedMessageId])

  // useMemo 缓存消息分组计算
  const messageGroups = useMemo(() => {
    const groups = []
    let currentGroup = null

    for (let i = 0; i < messages.length; i++) {
      const msg = messages[i]

      if ((msg.role === 'assistant' && !msg.streaming && (!msg.content || msg.content.trim() === '')) ||
          msg.message_type === 'hidden') {
        continue
      }

      if (msg.message_type === 'auto') {
        if (currentGroup && currentGroup.message.role === 'assistant') {
          currentGroup.autoMessages.push(msg)
        } else if (currentGroup && currentGroup.message.role === 'user') {
          groups.push(currentGroup)
          currentGroup = { message: msg, autoMessages: [], isAutoAsBot: true }
        } else {
          currentGroup = { message: msg, autoMessages: [], isAutoAsBot: true }
        }
      } else if (msg.role === 'tool') {
        // Tool results are displayed as independent bubbles, separate from assistant.
        if (currentGroup) {
          groups.push(currentGroup)
        }
        currentGroup = { message: msg, autoMessages: [], isToolMessage: true }
      } else {
        if (currentGroup) {
          groups.push(currentGroup)
        }
        currentGroup = { message: msg, autoMessages: [] }
      }
    }
    if (currentGroup) {
      groups.push(currentGroup)
    }

    return groups
  }, [messages])

  // 渲染消息列表（提取为内部组件便于阅读）
  const renderedMessages = useMemo(() => (
    messageGroups.map((group, index) => {
      const msg = group.message
      const isStreamingBotMessage = msg.role === 'assistant' && msg.streaming
      const isUserMessage = msg.role === 'user'
      const isBotMessage = msg.role === 'assistant'

      return (
        <MessageBubble
          key={msg.id || `msg-${index}`}
          msg={msg}
          isStreaming={isStreamingBotMessage}
          isUserMessage={isUserMessage}
          isBotMessage={isBotMessage}
          isAutoAsBot={group.isAutoAsBot}
          isToolMessage={group.isToolMessage}
          followingAutoMessages={group.autoMessages}
          copiedMessageId={copiedMessageId}
          onCopy={handleCopy}
          t={t}
        />
      )
    })
  ), [messageGroups, copiedMessageId, handleCopy, t])

  return (
    <div className="chat-box">
      <div className="chat-messages" ref={messagesContainerRef}>
        {messages.length === 0 ? (
          <WelcomeScreen t={t} />
        ) : (
          renderedMessages
        )}
        <div ref={messagesEndRef} />
        {/* 回到底部浮动按钮 */}
        {showScrollButton && (
          <button
            className="scroll-to-bottom-btn"
            onClick={handleScrollToBottom}
            title={t('chat.scrollToBottom')}
            aria-label={t('chat.scrollToBottom')}
          >
            <ArrowDown size={18} />
          </button>
        )}
      </div>

      <div className="chat-input-area">
        <div className="chat-input-container">
          <div className="chat-input-wrapper">
            <textarea
              ref={textareaRef}
              className="chat-input"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyPress={handleKeyPress}
              placeholder={t('chat.inputPlaceholder')}
              disabled={loading || wsReadyState !== WebSocket.OPEN}
              rows="1"
            />
            <div className="chat-input-actions">
              {loading ? (
                <button
                  className="btn-action btn-stop"
                  onClick={handleStop}
                  title={t('chat.stopGenerating')}
                >
                  <StopCircle size={20} />
                  <span>{t('chat.stop')}</span>
                </button>
              ) : (
                <button
                  className="btn-send"
                  onClick={handleSend}
                  disabled={!input.trim() || loading || wsReadyState !== WebSocket.OPEN}
                >
                  <Send size={18} />
                  <span>{t('chat.send')}</span>
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}, (prevProps, nextProps) => {
  // 自定义比较函数：仅在消息内容或状态实际变化时才重渲染
  if (prevProps.messages !== nextProps.messages) return false
  if (prevProps.loading !== nextProps.loading) return false
  if (prevProps.streamingMessageId !== nextProps.streamingMessageId) return false
  if (prevProps.copiedMessageId !== nextProps.copiedMessageId) return false
  if (prevProps.wsReadyState !== nextProps.wsReadyState) return false
  if (prevProps.sessionId !== nextProps.sessionId) return false
  return true // 跳过重渲染
})

ChatBox.displayName = 'ChatBox'

export default ChatBox
