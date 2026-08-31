import { useState, useCallback, useRef, useEffect } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  LayoutDashboard,
  Library,
  BookX,
  ImagePlus,
  BarChart3,
  UploadCloud,
  FileSearch,
  ListFilter,
  Settings as SettingsIcon,
  LogOut,
  Sun,
  Moon,
  PanelLeftClose,
  PanelLeft,
  Sparkles,
  User as UserIcon,
  ShieldCheck,
  Smartphone,
} from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import { Button, IconButton } from '@/components/ui'
import AiSidebar from '@/components/AiSidebar'

const mainNavItems = [
  { to: '/', label: '学习中心', icon: LayoutDashboard, end: true },
  { to: '/question-banks', label: '题库空间', icon: Library },
  { to: '/wrong-import', label: '错题导入', icon: ImagePlus },
  { to: '/wrong-book', label: '错题复习', icon: BookX },
  { to: '/statistics', label: '数据分析', icon: BarChart3 },
]

const adminNavItems = [
  { to: '/admin/imports', label: 'PDF 导入 Pipeline', icon: UploadCloud },
  { to: '/admin/candidates', label: '候选题目审核', icon: FileSearch },
  { to: '/admin/questions', label: '全量题库管理', icon: ListFilter },
]

export default function Layout() {
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const { sidebarOpen, toggleSidebar, darkMode, toggleDark, aiPanelOpen, setAiPanel, aiPanelWidth, setAiPanelWidth, toggleViewMode } = useUIStore()
  const navigate = useNavigate()
  const location = useLocation()
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const draggingRef = useRef(false)

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    draggingRef.current = true
    setIsDragging(true)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    const onMouseMove = (moveEvent: MouseEvent) => {
      if (!draggingRef.current) return
      const newWidth = window.innerWidth - moveEvent.clientX
      setAiPanelWidth(newWidth)
    }

    const onMouseUp = () => {
      draggingRef.current = false
      setIsDragging(false)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('mousemove', onMouseMove)
      window.removeEventListener('mouseup', onMouseUp)
    }

    window.addEventListener('mousemove', onMouseMove)
    window.addEventListener('mouseup', onMouseUp)
  }, [setAiPanelWidth])

  useEffect(() => {
    return () => {
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
  }, [])

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const getPageMeta = () => {
    const p = location.pathname
    if (p === '/') return { title: '学习中心', parent: null }
    if (p.startsWith('/question-bank/')) return { title: '题库详情', parent: { title: '题库空间', to: '/question-banks' } }
    if (p.startsWith('/question-banks')) return { title: '题库空间', parent: null }
    if (p.startsWith('/practice/')) return { title: '智能刷题', parent: { title: '题库空间', to: '/question-banks' } }
    if (p.startsWith('/wrong-import')) return { title: '错题导入', parent: null }
    if (p.startsWith('/wrong-book')) return { title: '错题复习', parent: null }
    if (p.startsWith('/statistics')) return { title: '数据分析', parent: null }
    if (p.startsWith('/admin/imports')) return { title: 'PDF 导入 Pipeline', parent: null }
    if (p.startsWith('/admin/candidates')) return { title: '候选题目审核', parent: null }
    if (p.startsWith('/admin/questions')) return { title: '全量题库管理', parent: null }
    if (p.startsWith('/settings')) return { title: '系统与模型设置', parent: null }
    return { title: '学习工作台', parent: null }
  }

  const isWorkbenchPage =
    location.pathname.startsWith('/practice/') ||
    location.pathname.startsWith('/admin/imports') ||
    location.pathname.startsWith('/wrong-import')
  const isPracticePage = location.pathname.startsWith('/practice/')
  const isDashboardPage = location.pathname === '/'
  const pageMeta = getPageMeta()

  return (
    <div className="flex h-screen w-full overflow-hidden bg-background text-foreground">
      {/* Mobile Drawer Backdrop */}
      {mobileDrawerOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 backdrop-blur-xs lg:hidden transition-opacity"
          onClick={() => setMobileDrawerOpen(false)}
        />
      )}

      {/* Modern Collapsible Sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 flex flex-col border-r border-border/80 bg-surface/95 backdrop-blur-xl transition-all duration-200 ease-natural lg:static ${
          sidebarOpen ? 'w-64' : 'w-16'
        } ${mobileDrawerOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
      >
        {/* Brand Header */}
        <div className="flex h-15 items-center justify-between px-4 border-b border-border/60">
          <Link to="/" className="flex items-center gap-3 min-w-0 group">
            <div className="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-xl overflow-hidden shadow-glow ring-1 ring-primary/30 group-hover:scale-105 transition-transform bg-surface-secondary">
              <img src="/logo.jpg" alt="QuizTrace" className="h-full w-full object-cover" />
              <span className="absolute -top-0.5 -right-0.5 flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary/40 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-primary"></span>
              </span>
            </div>
            {sidebarOpen && (
              <div className="flex flex-col min-w-0">
                <span className="text-sm font-bold tracking-tight leading-none text-foreground flex items-center gap-1.5">
                  QuizTrace
                  <span className="rounded-md bg-primary/10 px-1.5 py-0.5 text-[9px] font-mono font-bold text-primary">AI</span>
                </span>
                <span className="text-[10px] text-muted-foreground mt-0.5 tracking-tight truncate">自适应智能题库</span>
              </div>
            )}
          </Link>
          {sidebarOpen && (
            <IconButton
              variant="ghost"
              size="xs"
              onClick={toggleSidebar}
              className="text-muted-foreground hover:text-foreground hidden lg:flex"
              title="折叠侧栏"
            >
              <PanelLeftClose className="h-4 w-4" />
            </IconButton>
          )}
        </div>

        {/* Navigation Content */}
        <div className="flex-1 space-y-5 overflow-y-auto px-3 py-4">
          {/* Main Space */}
          <div className="space-y-1">
            {sidebarOpen && (
              <div className="px-2.5 pb-1 text-[10px] font-bold uppercase tracking-wider text-muted-foreground/60">
                学习空间
              </div>
            )}
            {mainNavItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                onClick={() => setMobileDrawerOpen(false)}
                className={({ isActive }) =>
                  `group relative flex items-center gap-3 rounded-2xl px-3 py-2.5 text-xs font-medium transition-all duration-150 ${
                    isActive
                      ? 'bg-primary/10 text-primary font-bold shadow-2xs'
                      : 'text-muted-foreground hover:bg-surface-secondary hover:text-foreground hover:translate-x-0.5'
                  } ${!sidebarOpen ? 'justify-center px-0' : ''}`
                }
                title={!sidebarOpen ? item.label : undefined}
              >
                {({ isActive }) => (
                  <>
                    <item.icon
                      className={`h-4 w-4 shrink-0 transition-transform group-hover:scale-110 ${
                        isActive ? 'text-primary' : 'text-muted-foreground group-hover:text-foreground'
                      }`}
                    />
                    {sidebarOpen && <span className="truncate">{item.label}</span>}
                    {isActive && (
                      <span className="absolute left-0 top-1/2 -translate-y-1/2 h-5 w-1 rounded-r-full bg-primary shadow-glow" />
                    )}
                  </>
                )}
              </NavLink>
            ))}
          </div>

          {/* AI / Automation / Admin section */}
          <div className="space-y-1 pt-1">
            {sidebarOpen && (
              <div className="flex items-center justify-between px-2.5 pb-1 text-[10px] font-bold uppercase tracking-wider text-muted-foreground/60">
                <span>自动化与工程（开发中）</span>
                {user?.role === 'admin' && (
                  <span className="flex items-center gap-1 text-[9px] text-primary/80 font-semibold bg-primary/10 px-1.5 py-0.2 rounded-md">
                    <ShieldCheck className="h-3 w-3" />
                    管理
                  </span>
                )}
              </div>
            )}
            {adminNavItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                onClick={() => setMobileDrawerOpen(false)}
                className={({ isActive }) =>
                  `group relative flex items-center gap-3 rounded-2xl px-3 py-2.5 text-xs font-medium transition-all duration-150 ${
                    isActive
                      ? 'bg-primary/10 text-primary font-bold shadow-2xs'
                      : 'text-muted-foreground hover:bg-surface-secondary hover:text-foreground hover:translate-x-0.5'
                  } ${!sidebarOpen ? 'justify-center px-0' : ''}`
                }
                title={!sidebarOpen ? item.label : undefined}
              >
                {({ isActive }) => (
                  <>
                    <item.icon
                      className={`h-4 w-4 shrink-0 transition-transform group-hover:scale-110 ${
                        isActive ? 'text-primary' : 'text-muted-foreground group-hover:text-foreground'
                      }`}
                    />
                    {sidebarOpen && <span className="truncate">{item.label}</span>}
                    {isActive && (
                      <span className="absolute left-0 top-1/2 -translate-y-1/2 h-5 w-1 rounded-r-full bg-primary shadow-glow" />
                    )}
                  </>
                )}
              </NavLink>
            ))}
          </div>
        </div>

        {/* Sidebar Footer / User Profile */}
        <div className="border-t border-border/60 p-3 space-y-2 bg-surface-secondary/40">
          {/* Mobile View Switcher Button */}
          <button
            type="button"
            onClick={toggleViewMode}
            className={`flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-xs font-semibold transition-all border border-primary/25 bg-primary/10 text-primary hover:bg-primary/15 active:scale-[0.98] shadow-2xs cursor-pointer ${
              !sidebarOpen ? 'justify-center px-0' : ''
            }`}
            title="切换至手机端模式"
          >
            <Smartphone className="h-4 w-4 shrink-0 text-primary animate-pulse-subtle" />
            {sidebarOpen && <span>切换手机端视图</span>}
          </button>

          <NavLink
            to="/settings"
            onClick={() => setMobileDrawerOpen(false)}
            className={({ isActive }) =>
              `flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-xs font-medium transition-colors ${
                isActive
                  ? 'bg-accent text-foreground font-bold'
                  : 'text-muted-foreground hover:bg-surface-secondary hover:text-foreground'
              } ${!sidebarOpen ? 'justify-center px-0' : ''}`
            }
            title="系统与模型设置"
          >
            <SettingsIcon className="h-4 w-4 shrink-0 text-muted-foreground" />
            {sidebarOpen && <span>系统与模型设置</span>}
          </NavLink>

          <div
            className={`flex items-center gap-2.5 rounded-2xl p-2 transition-all ${
              sidebarOpen ? 'bg-surface border border-border/70 shadow-subtle' : 'justify-center'
            }`}
          >
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-primary/15 to-primary/5 text-primary font-bold text-xs border border-primary/20">
              {user?.avatar ? (
                <img src={user.avatar} alt="Avatar" className="h-full w-full rounded-xl object-cover" />
              ) : (
                <UserIcon className="h-4 w-4" />
              )}
            </div>

            {sidebarOpen && (
              <div className="flex flex-1 flex-col min-w-0">
                <span className="truncate text-xs font-bold text-foreground leading-tight">
                  {user?.username ?? '学习者'}
                </span>
                <span className="truncate text-[10px] text-muted-foreground">
                  {user?.role === 'admin' ? '系统管理员' : '标准学习账号'}
                </span>
              </div>
            )}

            {sidebarOpen && (
              <IconButton
                variant="ghost"
                size="xs"
                onClick={handleLogout}
                className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                title="退出登录"
              >
                <LogOut className="h-3.5 w-3.5" />
              </IconButton>
            )}
          </div>
        </div>
      </aside>

      {/* Main Workspace Area */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {/* Top App Header */}
        <header className="flex h-15 shrink-0 items-center justify-between gap-3 border-b border-border/60 bg-surface/80 px-4 sm:px-6 backdrop-blur-xl z-10">
          <div className="flex items-center gap-3 min-w-0">
            {/* Mobile menu trigger */}
            <IconButton
              variant="ghost"
              size="sm"
              onClick={() => setMobileDrawerOpen(true)}
              className="lg:hidden text-muted-foreground hover:text-foreground"
              title="打开导航菜单"
            >
              <PanelLeft className="h-4 w-4" />
            </IconButton>

            {/* Desktop collapse restore button if sidebar closed */}
            {!sidebarOpen && (
              <IconButton
                variant="ghost"
                size="sm"
                onClick={toggleSidebar}
                className="hidden lg:flex text-muted-foreground hover:text-foreground hover:bg-surface-secondary"
                title="展开侧栏"
              >
                <PanelLeft className="h-4 w-4" />
              </IconButton>
            )}

            {/* Breadcrumb Info */}
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground truncate font-medium">
              <Link to="/" className="hover:text-foreground transition-colors hidden sm:inline text-muted-foreground/80">
                题迹
              </Link>
              {pageMeta.parent && (
                <>
                  <span className="hidden sm:inline text-muted-foreground/40">/</span>
                  <Link to={pageMeta.parent.to} className="hover:text-foreground transition-colors hidden sm:inline text-muted-foreground/80">
                    {pageMeta.parent.title}
                  </Link>
                </>
              )}
              <span className="text-muted-foreground/40">/</span>
              <span className="font-bold text-foreground tracking-tight">{pageMeta.title}</span>
            </div>
          </div>

          {/* Right Action Bar */}
          <div className="flex items-center gap-2">
            {/* AI Assistant Quick Toggle if not in practice mode */}
            {!isPracticePage && (
              <button
                type="button"
                onClick={() => setAiPanel(!aiPanelOpen)}
                className={`inline-flex items-center gap-2 rounded-xl px-3 py-1.5 text-xs font-semibold transition-all duration-200 cursor-pointer ${
                  aiPanelOpen
                    ? 'bg-primary/15 text-primary border border-primary/30 shadow-glow'
                    : 'border border-border/80 bg-surface text-foreground hover:bg-surface-secondary hover:border-primary/40 shadow-2xs'
                }`}
              >
                <Sparkles className={`h-3.5 w-3.5 ${aiPanelOpen ? 'text-primary animate-pulse-subtle' : 'text-primary'}`} />
                <span>AI Copilot</span>
                {aiPanelOpen && <span className="h-1.5 w-1.5 rounded-full bg-primary" />}
              </button>
            )}

            {/* Theme Toggle with smooth transition */}
            <IconButton
              variant="ghost"
              size="sm"
              onClick={toggleDark}
              className="text-muted-foreground hover:text-foreground transition-transform hover:scale-105"
              title={darkMode ? '切换浅色模式' : '切换深色模式'}
            >
              {darkMode ? <Sun className="h-4 w-4 text-amber-400" /> : <Moon className="h-4 w-4" />}
            </IconButton>
          </div>
        </header>

        {/* Scrollable Main Content & Docked AI Copilot Sidebar Area */}
        <div className="flex flex-1 min-h-0 min-w-0 overflow-hidden relative">
          {/* Squeezable Main Window */}
          <main className={`flex-1 min-w-0 ${isDashboardPage ? 'overflow-hidden flex flex-col' : 'overflow-y-auto'} bg-background/50 transition-all duration-300`}>
            <div className={`h-full ${isWorkbenchPage ? 'w-full p-2 sm:p-3' : isDashboardPage ? 'max-w-[1600px] w-full mx-auto p-3 sm:p-3.5 lg:p-4 2xl:p-5.5 animate-fade-in flex flex-col min-h-0' : 'max-w-[1600px] w-full mx-auto p-4 sm:p-5 lg:p-6 animate-fade-in'}`}>
              <Outlet />
            </div>
          </main>

          {/* Docked AI Copilot Sidebar with Resizer */}
          {!isPracticePage && (
            <aside
              style={{ width: aiPanelOpen ? `${aiPanelWidth}px` : '0px' }}
              className={`relative shrink-0 flex flex-col h-full bg-surface border-l ${
                aiPanelOpen ? 'border-border/80 opacity-100' : 'border-transparent opacity-0 pointer-events-none'
              } ${isDragging ? '' : 'transition-all duration-300 ease-in-out'} overflow-hidden z-20`}
            >
              {/* Draggable Resize Handle on Left Edge */}
              {aiPanelOpen && (
                <div
                  onMouseDown={handleMouseDown}
                  className="absolute left-0 top-0 bottom-0 w-2 -ml-1 cursor-col-resize hover:bg-primary/40 active:bg-primary transition-colors z-30 group flex items-center justify-center select-none"
                  title="拖拽调整侧边栏宽度"
                >
                  <div className="w-0.5 h-8 rounded-full bg-border group-hover:bg-primary group-active:bg-primary transition-colors shadow-glow" />
                </div>
              )}

              {/* Inner Fixed-Width Viewport for Smooth Slide & Clip Transition */}
              <div
                style={{ width: `${aiPanelWidth}px` }}
                className="h-full flex flex-col shrink-0 overflow-hidden"
              >
                <AiSidebar
                  question={null}
                  result={null}
                  session={null}
                  onClose={() => setAiPanel(false)}
                />
              </div>
            </aside>
          )}
        </div>
      </div>
    </div>
  )
}

