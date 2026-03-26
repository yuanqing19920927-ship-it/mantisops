import { useThemeStore } from '../stores/themeStore'

export function ThemeToggle() {
  const { theme, toggle } = useThemeStore()
  return (
    <button
      onClick={toggle}
      className="p-2 rounded-lg transition-colors"
      style={{ color: 'var(--text-secondary)' }}
      title={theme === 'dark' ? '切换浅色' : '切换深色'}
    >
      {theme === 'dark' ? '\u2600\uFE0F' : '\uD83C\uDF19'}
    </button>
  )
}
