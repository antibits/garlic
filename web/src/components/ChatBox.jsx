import React, { useState, useEffect, useRef, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { Send, Copy, Check, StopCircle } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeHighlight from 'rehype-highlight'
import rehypeRaw from 'rehype-raw'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'
import './ChatBox.css'

/**
 * ChatBox 组件 - 支持多会话并发
 *
 * 设计说明：
 * - 不再直接管理 WebSocket 连接，由父组件 ChatPage 统一管理连接池
 * - 只负责显示指定会话的消息和输入
 * - 通过 props 接收 WebSocket 相关方法
 */
const ChatBox = ({
  sessionId,
  wsRef,           // WebSocket 引用（由父组件管理）
  messages,        // 消息列表（由父组件管理）
  loading,         // 加载状态（由父组件管理）
  streamingMessageId,
  copiedMessageId,
  onSendMessage,   // 发送消息回调
  onStopGenerating, // 停止生成回调
  setCopiedMessageId // 设置复制状态
}) => {
  const { t } = useTranslation()
  const [input, setInput] = useState('')

  const messagesEndRef = useRef(null)
  const textareaRef = useRef(null)
  const messagesContainerRef = useRef(null)

  // 滚动到底部
  useEffect(() => {
    scrollToBottom()
  }, [messages])

  // 调整 textarea 高度
  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
      textareaRef.current.style.height = Math.min(textareaRef.current.scrollHeight, 200) + 'px'
    }
  }, [input])

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  const handleSend = useCallback(() => {
    const trimmedInput = input.trim()
    const wsConnected = wsRef?.current && wsRef.current.readyState === WebSocket.OPEN
    console.log('handleSend:', { trimmedInput: !!trimmedInput, loading, wsConnected, wsReadyState: wsRef?.current?.readyState })
    if (!trimmedInput || loading || !wsConnected) return

    onSendMessage(trimmedInput)
    setInput('')

    // 重置 textarea 高度
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }, [input, loading, wsRef, onSendMessage])

  const handleStop = useCallback(() => {
    onStopGenerating()
  }, [onStopGenerating])

  const handleKeyPress = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleCopy = async (content, messageId) => {
    if (!setCopiedMessageId) return
    try {
      await navigator.clipboard.writeText(content)
      setCopiedMessageId(messageId)
      setTimeout(() => setCopiedMessageId(null), 2000)
    } catch (error) {
      console.error('Failed to copy:', error)
    }
  }

  const handleInput = (e) => {
    setInput(e.target.value)
  }

  // 渲染消息内容（Markdown）
  const renderMarkdownContent = (content, isStreaming) => (
    <ReactMarkdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeRaw, rehypeHighlight, rehypeKatex]}
      components={{
        code: ({ node, inline, className, children, ...props }) => {
          const match = /language-(\w+)/.exec(className || '')
          return inline ? (
            <code className={className} {...props}>
              {children}
            </code>
          ) : (
            <div className="code-block-wrapper">
              <div className="code-block-header">
                <span className="code-block-language">{match ? match[1] : 'code'}</span>
                <button
                  className="btn-copy-code"
                  onClick={() => {
                    const codeText = String(children).replace(/\n$/, '')
                    navigator.clipboard.writeText(codeText)
                  }}
                >
                  <Copy size={12} />
                  <span>{t('chat.codeBlock.copy')}</span>
                </button>
              </div>
              <pre>
                <code className={className} {...props}>
                  {children}
                </code>
              </pre>
            </div>
          )
        }
      }}
    >
      {content || ''}
    </ReactMarkdown>
  )

  // 渲染 auto 类型的消息内容（思考框）
  const renderAutoMessageContent = (msg) => (
    <div className="thought-box">
      <div className="thought-content">
        {renderMarkdownContent(msg.content, msg.streaming)}
        {msg.streaming && (
          <div className="thinking-animation">
            <span className="dot"></span>
            <span className="dot"></span>
            <span className="dot"></span>
          </div>
        )}
      </div>
    </div>
  )

  // 将消息分组：user 消息和 bot 消息交替显示，auto 消息作为前一个 bot 消息的嵌套内容
  const getMessageGroups = () => {
    const messageGroups = []
    let currentGroup = null

    for (let i = 0; i < messages.length; i++) {
      const msg = messages[i]

      // 跳过空消息和 hidden 消息
      if ((msg.role === 'assistant' && !msg.streaming && (!msg.content || msg.content.trim() === '')) ||
          msg.message_type === 'hidden') {
        continue
      }

      if (msg.message_type === 'auto') {
        if (currentGroup && currentGroup.message.role === 'assistant') {
          currentGroup.autoMessages.push(msg)
        } else if (currentGroup && currentGroup.message.role === 'user') {
          messageGroups.push(currentGroup)
          currentGroup = {
            message: msg,
            autoMessages: [],
            isAutoAsBot: true
          }
        } else {
          currentGroup = {
            message: msg,
            autoMessages: [],
            isAutoAsBot: true
          }
        }
      } else {
        if (currentGroup) {
          messageGroups.push(currentGroup)
        }
        currentGroup = {
          message: msg,
          autoMessages: []
        }
      }
    }
    if (currentGroup) {
      messageGroups.push(currentGroup)
    }

    return messageGroups
  }

  return (
    <div className="chat-box">
      <div className="chat-messages" ref={messagesContainerRef}>
        {messages.length === 0 ? (
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
        ) : (
          getMessageGroups().map((group, index) => {
            const msg = group.message
            const followingAutoMessages = group.autoMessages
            const isStreamingBotMessage = msg.role === 'assistant' && msg.streaming
            const isUserMessage = msg.role === 'user'
            const isBotMessage = msg.role === 'assistant'
            const isAutoAsBot = group.isAutoAsBot

            return (
              <div
                key={msg.id || index}
                className={`message ${isUserMessage ? 'user' : 'bot'} ${msg.streaming ? 'streaming' : ''}`}
              >
                <div className="message-avatar">
                  {isUserMessage ? '👤' : '🤖'}
                </div>
                <div className="message-content">
                  {isAutoAsBot ? (
                    renderAutoMessageContent(msg)
                  ) : (
                    <div className="message-text">
                      {msg.streaming ? (
                        renderMarkdownContent(msg.content, true)
                      ) : isUserMessage ? (
                        msg.content
                      ) : (
                        renderMarkdownContent(msg.content, false)
                      )}
                      {isStreamingBotMessage && (
                        <div className="thinking-animation">
                          <span className="dot"></span>
                          <span className="dot"></span>
                          <span className="dot"></span>
                        </div>
                      )}
                    </div>
                  )}
                  {followingAutoMessages.length > 0 && (
                    <div className="nested-auto-messages">
                      {followingAutoMessages.map((autoMsg, autoIndex) => (
                        <div key={autoMsg.id || `auto-${autoIndex}`} className="auto-message-item">
                          {renderAutoMessageContent(autoMsg)}
                        </div>
                      ))}
                    </div>
                  )}
                  {!msg.streaming && isBotMessage && (
                    <div className="message-meta">
                      <span className="message-time">
                        {new Date(msg.timestamp).toLocaleTimeString('zh-CN', {
                          hour: '2-digit',
                          minute: '2-digit'
                        })}
                      </span>
                      <button
                        className="btn-copy"
                        onClick={() => handleCopy(msg.content, msg.id || index)}
                        title={t('common.copy')}
                      >
                        {copiedMessageId === (msg.id || index) ? (
                          <Check size={14} />
                        ) : (
                          <Copy size={14} />
                        )}
                      </button>
                    </div>
                  )}
                </div>
              </div>
            )
          })
        )}

        <div ref={messagesEndRef} />
      </div>

      <div className="chat-input-area">
        <div className="chat-input-container">
          <div className="chat-input-wrapper">
            <textarea
              ref={textareaRef}
              className="chat-input"
              value={input}
              onChange={handleInput}
              onKeyPress={handleKeyPress}
              placeholder={t('chat.inputPlaceholder')}
              disabled={loading || wsRef?.current?.readyState !== WebSocket.OPEN}
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
                  disabled={!input.trim() || loading || wsRef?.current?.readyState !== WebSocket.OPEN}
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
}

export default ChatBox
