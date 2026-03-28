import { useState, useEffect, useRef, useCallback } from 'react'
import { useAIStore } from '../../stores/aiStore'
import {
  listConversations,
  createConversation,
  getConversation,
  deleteConversation,
  sendMessage,
} from '../../api/ai'
import { sendWsMessage } from '../../hooks/useWebSocket'
import type { AIConversation, AIMessage } from '../../api/ai'

export function ChatPanel() {
  const {
    chatOpen,
    setChatOpen,
    chatListOpen,
    setChatListOpen,
    conversations,
    setConversations,
    activeConversationId,
    setActiveConversation,
    messages,
    setMessages,
    streamingContent,
    streamingMessageId,
    startStreaming,
  } = useAIStore()

  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)

  // Auto-scroll to bottom
  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  useEffect(() => {
    scrollToBottom()
  }, [messages, streamingContent, scrollToBottom])

  // Fetch conversations on mount
  useEffect(() => {
    loadConversations()
  }, [])

  async function loadConversations() {
    try {
      const data = await listConversations({ limit: 50 })
      setConversations(data.items || [])
    } catch {
      // ignore
    }
  }

  async function handleSelectConversation(conv: AIConversation) {
    setActiveConversation(conv.id)
    try {
      const data = await getConversation(conv.id)
      setMessages(data.messages || [])
    } catch {
      setMessages([])
    }
  }

  async function handleNewConversation() {
    try {
      const data = await createConversation()
      setActiveConversation(data.id)
      setMessages([])
      await loadConversations()
    } catch {
      // ignore
    }
  }

  async function handleDeleteConversation(e: React.MouseEvent, id: number) {
    e.stopPropagation()
    try {
      await deleteConversation(id)
      if (activeConversationId === id) {
        setActiveConversation(null)
        setMessages([])
      }
      await loadConversations()
    } catch {
      // ignore
    }
  }

  async function handleSend() {
    const content = input.trim()
    if (!content || sending || streamingMessageId) return

    let convId = activeConversationId

    // Create conversation if none active
    if (!convId) {
      try {
        const data = await createConversation()
        convId = data.id
        setActiveConversation(convId)
        await loadConversations()
      } catch {
        return
      }
    }

    const requestId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    const now = new Date().toISOString()

    // Optimistic user message
    const userMsg: AIMessage = {
      id: -Date.now(),
      conversation_id: convId,
      role: 'user',
      content,
      status: 'done',
      request_id: requestId,
      created_at: now,
    }

    // Placeholder assistant message
    const assistantMsgId = -(Date.now() + 1)
    const assistantMsg: AIMessage = {
      id: assistantMsgId,
      conversation_id: convId,
      role: 'assistant',
      content: '',
      status: 'streaming',
      request_id: requestId,
      created_at: now,
    }

    setMessages([...messages, userMsg, assistantMsg])
    setInput('')
    setSending(true)

    // Reset textarea height
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }

    try {
      const resp = await sendMessage(convId, { content, request_id: requestId })
      const streamId = resp.stream_id

      // Update assistant message with real ID from response
      const realAssistantId = resp.message_id || assistantMsgId
      setMessages(
        useAIStore.getState().messages.map((m) =>
          m.id === assistantMsgId ? { ...m, id: realAssistantId } : m
        )
      )

      // Subscribe to stream via WebSocket
      if (streamId) {
        sendWsMessage({ type: 'ai_stream_subscribe', stream_id: streamId })
      }

      startStreaming(realAssistantId)
    } catch (err: unknown) {
      // Mark assistant message as failed
      const errorMsg = err instanceof Error ? err.message : 'Failed to send'
      setMessages(
        useAIStore.getState().messages.map((m) =>
          m.id === assistantMsgId
            ? { ...m, status: 'failed', error_message: errorMsg }
            : m
        )
      )
    } finally {
      setSending(false)
    }
  }

  async function handleRetry(msg: AIMessage) {
    // Find the user message before this failed assistant message
    const idx = messages.findIndex((m) => m.id === msg.id)
    if (idx < 1) return
    const userMsg = messages[idx - 1]
    if (userMsg.role !== 'user') return

    // Remove the failed message and re-send
    setMessages(messages.filter((m) => m.id !== msg.id))
    setInput(userMsg.content)
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  function handleInputChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    setInput(e.target.value)
    // Auto-resize textarea
    const el = e.target
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 96) + 'px' // max ~4 rows
  }

  if (!chatOpen) return null

  return (
    <div
      ref={panelRef}
      className="fixed z-50 flex flex-col
                 bottom-0 right-0 md:bottom-6 md:right-6
                 w-full h-full md:w-[400px] md:h-[600px]
                 md:rounded-2xl overflow-hidden
                 bg-[var(--color-surface-container)] border border-[var(--color-outline-variant)]/30
                 shadow-2xl backdrop-blur-xl
                 animate-in fade-in slide-in-from-bottom-4 duration-200"
    >
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--color-outline-variant)]/20
                      bg-[var(--color-surface-container-high)]/80 backdrop-blur-sm shrink-0">
        <div className="flex items-center gap-2">
          <button
            onClick={() => setChatListOpen(!chatListOpen)}
            className="p-1 rounded-lg hover:bg-[var(--color-surface-container-highest)] transition-colors"
            title={chatListOpen ? '收起对话列表' : '展开对话列表'}
          >
            <span className="material-symbols-outlined text-lg text-[var(--color-on-surface-variant)]">
              {chatListOpen ? 'side_navigation' : 'menu'}
            </span>
          </button>
          <span className="material-symbols-outlined text-lg text-[var(--color-primary)]">smart_toy</span>
          <span className="text-sm font-semibold text-[var(--color-on-surface)]">AI 助手</span>
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={() => setChatOpen(false)}
            className="p-1 rounded-lg hover:bg-[var(--color-surface-container-highest)] transition-colors"
            title="最小化"
          >
            <span className="material-symbols-outlined text-lg text-[var(--color-on-surface-variant)]">
              minimize
            </span>
          </button>
          <button
            onClick={() => {
              setChatOpen(false)
              setActiveConversation(null)
              setMessages([])
            }}
            className="p-1 rounded-lg hover:bg-[var(--color-error-container)] transition-colors"
            title="关闭"
          >
            <span className="material-symbols-outlined text-lg text-[var(--color-on-surface-variant)]">
              close
            </span>
          </button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Conversation List Sidebar */}
        {chatListOpen && (
          <div className="w-[140px] shrink-0 border-r border-[var(--color-outline-variant)]/20
                          bg-[var(--color-surface-container)]/60 flex flex-col overflow-hidden">
            <button
              onClick={handleNewConversation}
              className="flex items-center gap-1.5 px-3 py-2 m-2 rounded-lg text-xs font-medium
                         bg-[var(--color-primary)]/15 text-[var(--color-primary)]
                         hover:bg-[var(--color-primary)]/25 transition-colors"
            >
              <span className="material-symbols-outlined text-sm">add</span>
              新对话
            </button>
            <div className="flex-1 overflow-y-auto">
              {conversations.map((conv) => (
                <div
                  key={conv.id}
                  onClick={() => handleSelectConversation(conv)}
                  className={`group flex items-center justify-between px-3 py-2 mx-1 rounded-lg cursor-pointer
                              text-xs transition-colors
                              ${activeConversationId === conv.id
                                ? 'bg-[var(--color-primary)]/15 text-[var(--color-primary)]'
                                : 'text-[var(--color-on-surface-variant)] hover:bg-[var(--color-surface-container-highest)]'
                              }`}
                >
                  <span className="truncate flex-1">{conv.title || '未命名对话'}</span>
                  <button
                    onClick={(e) => handleDeleteConversation(e, conv.id)}
                    className="opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-[var(--color-error-container)]
                               transition-opacity shrink-0 ml-1"
                    title="删除对话"
                  >
                    <span className="material-symbols-outlined text-xs text-[var(--color-error)]">delete</span>
                  </button>
                </div>
              ))}
              {conversations.length === 0 && (
                <div className="px-3 py-4 text-xs text-[var(--color-on-surface-variant)]/60 text-center">
                  暂无对话
                </div>
              )}
            </div>
          </div>
        )}

        {/* Chat Area */}
        <div className="flex-1 flex flex-col overflow-hidden">
          {/* Messages */}
          <div className="flex-1 overflow-y-auto px-3 py-3 space-y-3">
            {messages.length === 0 && !streamingMessageId && (
              <div className="flex flex-col items-center justify-center h-full text-center gap-3 py-8">
                <span className="material-symbols-outlined text-4xl text-[var(--color-primary)]/40">
                  smart_toy
                </span>
                <div className="text-sm text-[var(--color-on-surface-variant)]/60">
                  你好！我是 MantisOps AI 助手。
                  <br />
                  有什么可以帮助你的？
                </div>
              </div>
            )}

            {messages.map((msg) => {
              const isUser = msg.role === 'user'
              const isStreaming = msg.id === streamingMessageId
              const isFailed = msg.status === 'failed'

              return (
                <div
                  key={msg.id}
                  className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}
                >
                  <div
                    className={`max-w-[85%] rounded-2xl px-3 py-2 text-sm leading-relaxed
                                ${isUser
                                  ? 'bg-[var(--color-primary)] text-[var(--color-on-primary)] rounded-br-md'
                                  : 'bg-[var(--color-surface-container-highest)] text-[var(--color-on-surface)] rounded-bl-md'
                                }
                                ${isFailed ? 'border border-[var(--color-error)]/40' : ''}`}
                  >
                    {isStreaming ? (
                      <div className="whitespace-pre-wrap break-words">
                        {streamingContent || ''}
                        <span className="inline-block w-1.5 h-4 ml-0.5 bg-[var(--color-primary)] animate-pulse align-middle" />
                      </div>
                    ) : (
                      <div className="whitespace-pre-wrap break-words">{msg.content}</div>
                    )}

                    {isFailed && (
                      <div className="mt-1.5 flex items-center gap-2">
                        <span className="text-xs text-[var(--color-error)]">
                          {msg.error_message || '发送失败'}
                        </span>
                        <button
                          onClick={() => handleRetry(msg)}
                          className="text-xs text-[var(--color-primary)] hover:underline"
                        >
                          重试
                        </button>
                      </div>
                    )}
                  </div>
                </div>
              )
            })}

            <div ref={messagesEndRef} />
          </div>

          {/* Input Area */}
          <div className="shrink-0 border-t border-[var(--color-outline-variant)]/20 p-3
                          bg-[var(--color-surface-container)]/80 backdrop-blur-sm">
            <div className="flex items-end gap-2">
              <textarea
                ref={textareaRef}
                value={input}
                onChange={handleInputChange}
                onKeyDown={handleKeyDown}
                placeholder="输入消息..."
                rows={1}
                className="flex-1 resize-none rounded-xl px-3 py-2 text-sm
                           bg-[var(--color-surface-container-highest)] text-[var(--color-on-surface)]
                           placeholder:text-[var(--color-on-surface-variant)]/50
                           border border-[var(--color-outline-variant)]/30
                           focus:border-[var(--color-primary)]/60 focus:outline-none
                           transition-colors"
                style={{ maxHeight: '96px' }}
              />
              <button
                onClick={handleSend}
                disabled={!input.trim() || sending || !!streamingMessageId}
                className="shrink-0 w-9 h-9 rounded-xl flex items-center justify-center
                           bg-[var(--color-primary)] text-[var(--color-on-primary)]
                           hover:bg-[var(--color-primary)]/90
                           disabled:opacity-40 disabled:cursor-not-allowed
                           transition-all"
                title="发送"
              >
                <span className="material-symbols-outlined text-lg">send</span>
              </button>
            </div>
            <div className="text-[10px] text-[var(--color-on-surface-variant)]/40 mt-1 text-right">
              Enter 发送 · Shift+Enter 换行
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
