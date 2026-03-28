import { create } from 'zustand'
import type { AIReport, AIConversation, AIMessage } from '../api/ai'

interface AIState {
  // Reports
  reports: AIReport[]
  reportsTotal: number
  generatingReportIds: number[]

  // Chat
  conversations: AIConversation[]
  activeConversationId: number | null
  messages: AIMessage[]
  streamingContent: string
  streamingMessageId: number | null
  chatOpen: boolean
  chatListOpen: boolean

  // Report actions
  setReports: (reports: AIReport[], total: number) => void
  addGeneratingReport: (id: number) => void
  removeGeneratingReport: (id: number) => void

  // Chat actions
  setChatOpen: (open: boolean) => void
  setChatListOpen: (open: boolean) => void
  setActiveConversation: (id: number | null) => void
  setMessages: (msgs: AIMessage[]) => void
  setConversations: (convs: AIConversation[]) => void
  appendStreamChunk: (content: string) => void
  startStreaming: (messageId: number) => void
  finalizeStream: () => void
  setStreamError: (messageId: number, error: string) => void
  resetStreaming: () => void
}

export const useAIStore = create<AIState>((set) => ({
  reports: [],
  reportsTotal: 0,
  generatingReportIds: [],

  conversations: [],
  activeConversationId: null,
  messages: [],
  streamingContent: '',
  streamingMessageId: null,
  chatOpen: false,
  chatListOpen: true,

  setReports: (reports, total) => set({ reports, reportsTotal: total }),
  addGeneratingReport: (id) =>
    set((s) => ({ generatingReportIds: [...s.generatingReportIds, id] })),
  removeGeneratingReport: (id) =>
    set((s) => ({ generatingReportIds: s.generatingReportIds.filter((x) => x !== id) })),

  setChatOpen: (open) => set({ chatOpen: open }),
  setChatListOpen: (open) => set({ chatListOpen: open }),
  setActiveConversation: (id) => set({ activeConversationId: id }),
  setMessages: (msgs) => set({ messages: msgs }),
  setConversations: (convs) => set({ conversations: convs }),
  appendStreamChunk: (content) =>
    set((s) => ({ streamingContent: s.streamingContent + content })),
  startStreaming: (messageId) =>
    set({ streamingMessageId: messageId, streamingContent: '' }),
  finalizeStream: () =>
    set((s) => {
      // Update the assistant message in messages array with accumulated content
      const updated = s.messages.map((m) =>
        m.id === s.streamingMessageId
          ? { ...m, content: s.streamingContent, status: 'done' }
          : m
      )
      return { messages: updated, streamingMessageId: null, streamingContent: '' }
    }),
  setStreamError: (messageId, error) =>
    set((s) => {
      const updated = s.messages.map((m) =>
        m.id === messageId ? { ...m, status: 'failed', error_message: error } : m
      )
      return { messages: updated, streamingMessageId: null, streamingContent: '' }
    }),
  resetStreaming: () => set({ streamingMessageId: null, streamingContent: '' }),
}))
