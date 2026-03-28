import { useAIStore } from '../../stores/aiStore'
import { ChatPanel } from './ChatPanel'

export function ChatButton() {
  const { chatOpen, setChatOpen } = useAIStore()

  return (
    <>
      {!chatOpen && (
        <button
          onClick={() => setChatOpen(true)}
          className="fixed bottom-6 right-6 z-50 w-14 h-14 rounded-full
                     bg-gradient-to-br from-[var(--color-primary)] to-[var(--color-primary-container)]
                     text-white shadow-lg hover:shadow-xl transition-all
                     flex items-center justify-center hover:scale-105"
          title="AI 助手"
        >
          <span className="material-symbols-outlined text-2xl">smart_toy</span>
        </button>
      )}
      {chatOpen && <ChatPanel />}
    </>
  )
}
