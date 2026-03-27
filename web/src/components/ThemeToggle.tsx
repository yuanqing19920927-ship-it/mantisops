import { useThemeStore } from '../stores/themeStore'

export function ThemeToggle() {
  const { theme, toggle } = useThemeStore()
  return (
    <button
      onClick={toggle}
      title={theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
      className="w-8 h-8 flex items-center justify-center rounded-lg text-[#878a99] hover:text-[#495057] hover:bg-[#eff2f7] transition-colors"
    >
      <span className="material-symbols-outlined text-[18px]">
        {theme === 'dark' ? 'light_mode' : 'dark_mode'}
      </span>
    </button>
  )
}
