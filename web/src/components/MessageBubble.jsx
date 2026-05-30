import React, { memo, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy, Check } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeHighlight from 'rehype-highlight'
import rehypeRaw from 'rehype-raw'
import remarkMath from 'remark-math'
import rehypeKatex from 'rehype-katex'

/**
 * MarkdownRenderer - 纯渲染组件，仅在 content 变化时重渲染
 */
const MarkdownRenderer = memo(({ content, isStreaming }) => {
  const markdownComponents = useMemo(() => ({
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
              <span>{/* will use translation from parent */}Copy</span>
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
  }), [])

  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeRaw, rehypeHighlight, rehypeKatex]}
      components={markdownComponents}
    >
      {content || ''}
    </ReactMarkdown>
  )
})

MarkdownRenderer.displayName = 'MarkdownRenderer'

/**
 * AutoMessageContent - 思考框内容（auto 类型消息）
 */
const AutoMessageContent = memo(({ msg }) => (
  <div className="thought-box">
    <div className="thought-content">
      <MarkdownRenderer content={msg.content} isStreaming={msg.streaming} />
      {msg.streaming && (
        <div className="streaming-animation-indicator">
          <div className="thinking-animation">
            <span className="dot"></span>
            <span className="dot"></span>
            <span className="dot"></span>
          </div>
        </div>
      )}
    </div>
  </div>
))

AutoMessageContent.displayName = 'AutoMessageContent'

/**
 * MessageBubble - 单个消息气泡，使用 React.memo 避免不必要的重渲染
 *
 * 比较策略：
 * - 已完成的消息（streaming === false）只在 content 真正变化时重渲染
 * - 流式消息在 content 变化时重渲染
 */
const MessageBubble = memo(({
  msg,
  isStreaming,
  isUserMessage,
  isBotMessage,
  isAutoAsBot,
  followingAutoMessages,
  copiedMessageId,
  onCopy,
  t
}) => {
  const msgId = msg.id

  const messageMeta = useMemo(() => (
    !msg.streaming && isBotMessage ? (
      <div className="message-meta">
        <span className="message-time">
          {new Date(msg.timestamp).toLocaleTimeString('zh-CN', {
            hour: '2-digit',
            minute: '2-digit'
          })}
        </span>
        <button
          className="btn-copy"
          onClick={() => onCopy(msg.content, msgId)}
          title={t('common.copy')}
        >
          {copiedMessageId === msgId ? (
            <Check size={14} />
          ) : (
            <Copy size={14} />
          )}
        </button>
      </div>
    ) : null
  ), [msg.streaming, isBotMessage, msg.timestamp, msg.content, msgId, copiedMessageId, onCopy, t])

  return (
    <div
      className={`message ${isUserMessage ? 'user' : 'bot'} ${msg.streaming ? 'streaming' : ''}`}
    >
      <div className="message-avatar">
        {isUserMessage ? '👤' : '🤖'}
      </div>
      <div className="message-content">
        {isAutoAsBot ? (
          <AutoMessageContent msg={msg} />
        ) : (
          <>
            <div className="message-text">
              {msg.streaming ? (
                <MarkdownRenderer content={msg.content} isStreaming={true} />
              ) : isUserMessage ? (
                msg.content
              ) : (
                <MarkdownRenderer content={msg.content} isStreaming={false} />
              )}
            </div>
            {isStreaming && (
              <div className="streaming-animation-indicator">
                <div className="thinking-animation">
                  <span className="dot"></span>
                  <span className="dot"></span>
                  <span className="dot"></span>
                </div>
              </div>
            )}
          </>
        )}
        {followingAutoMessages.length > 0 && (
          <div className="nested-auto-messages">
            {followingAutoMessages.map((autoMsg, autoIndex) => (
              <div key={autoMsg.id || `auto-${autoIndex}`} className="auto-message-item">
                <AutoMessageContent msg={autoMsg} />
              </div>
            ))}
          </div>
        )}
        {messageMeta}
      </div>
    </div>
  )
}, (prevProps, nextProps) => {
  // 自定义比较：只在相关 props 变化时重渲染
  if (prevProps.msg.content !== nextProps.msg.content) return false
  if (prevProps.msg.streaming !== nextProps.msg.streaming) return false
  if (prevProps.isStreaming !== nextProps.isStreaming) return false
  if (prevProps.copiedMessageId !== nextProps.copiedMessageId) return false
  // followingAutoMessages 可能变化，浅比较
  if (prevProps.followingAutoMessages !== nextProps.followingAutoMessages) return false
  return true // 跳过重渲染
})

MessageBubble.displayName = 'MessageBubble'

export { MarkdownRenderer, AutoMessageContent }
export default MessageBubble
