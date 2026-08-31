import React, { useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  BarChart3,
  BookX,
  ChevronLeft,
  FileSearch,
  ImagePlus,
  LayoutDashboard,
  Library,
  ListFilter,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Settings as SettingsIcon,
  ShieldCheck,
  Sparkles,
  Sun,
  UploadCloud,
  User as UserIcon,
  X,
} from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { useUIStore } from '@/store/ui'
import { MobileToastContainer } from './MobileToast'
import { MobileAiSheet } from './MobileAiSheet'

const bottomNavItems = [
  { to: '/', label: '学习', icon: LayoutDashboard, end: true },
  { to: '/question-banks', label: '题库', icon: Library },
  { to: '/wrong-book', label: '错题', icon: BookX },
  { to: '/statistics', label: '数据', icon: BarChart3 },
  { to: '/settings', label: '我的', icon: SettingsIcon },
]

const drawerAdminItems = [
  { to: '/wrong-import', label: '错题图片 AI 导入', icon: ImagePlus, desc: '拍照/截图提取试题与答案' },
  { to: '/admin/imports', label: 'PDF 导入 Pipeline', icon: UploadCloud, desc: '自动化多文档解析工程' },
  { to: '/admin/candidates', label: '候选题目人工审核', icon: FileSearch, desc: '低置信度题目快速核准' },
  { to: '/admin/questions', label: '全量题库管理', icon: ListFilter, desc: '全量题目检索与向量重索' },
]

export function MobileLayout() {
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const { darkMode, toggleDark, toggleViewMode } = useUIStore()
  const navigate = useNavigate()
  const location = useLocation()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [aiSheetOpen, setAiSheetOpen] = useState(false)

  const isPracticeMode = location.pathname.startsWith('/practice/')
  const isWrongImportMode = location.pathname.startsWith('/wrong-import')
  const isAdminImportsMode = location.pathname.startsWith('/admin/imports')
  const isImmersive = isPracticeMode || isWrongImportMode || isAdminImportsMode

  // Calculate page title and whether to show back button
  const getPageInfo = () => {
    const p = location.pathname
    if (p === '/') return { title: '学习中心', isRoot: true }
    if (p === '/question-banks') return { title: '题库空间', isRoot: true }
    if (p.startsWith('/question-bank/')) return { title: '题库详情', isRoot: false, backTo: '/question-banks' }
    if (p.startsWith('/practice/')) return { title: '智能刷题', isRoot: false }
    if (p === '/wrong-book') return { title: '错题复习本', isRoot: true }
    if (p === '/wrong-import') return { title: '错题图片 AI 导入', isRoot: false, backTo: '/' }
    if (p === '/statistics') return { title: '学习数据分析', isRoot: true }
    if (p === '/settings') return { title: '设置与配置', isRoot: true }
    if (p === '/admin/imports') return { title: 'PDF 导入流水线', isRoot: false, backTo: '/' }
    if (p === '/admin/candidates') return { title: '候选题审核', isRoot: false, backTo: '/' }
    if (p === '/admin/questions') return { title: '全量题库管理', isRoot: false, backTo: '/' }
    return { title: '题迹 QuizTrace', isRoot: true }
  }

  const pageInfo = getPageInfo()

  return (
    <div className="flex flex-col h-screen w-full overflow-hidden bg-background text-foreground select-none">
      <MobileToastContainer />
      <MobileAiSheet open={aiSheetOpen} onClose={() => setAiSheetOpen(false)} />

      {/* Slide-over Drawer for Admin/Tools */}
      {drawerOpen && (
        <div className="fixed inset-0 z-50 flex">
          <div
            className="fixed inset-0 bg-black/50 backdrop-blur-xs transition-opacity"
            onClick={() => setDrawerOpen(false)}
          />
          <div className="relative z-10 flex flex-col w-72 max-w-[80vw] h-full bg-surface border-r border-border/80 shadow-float pt-safe pb-safe animate-in slide-in-from-left duration-200">
            {/* Drawer Header */}
            <div className="flex items-center justify-between p-4 border-b border-border/60">
              <div className="flex items-center gap-2.5">
                <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-subtle">
                  <ShieldCheck className="h-4 w-4" />
                </div>
                <div>
                  <span className="text-xs font-bold text-foreground block">工程与辅助工具</span>
                  <span className="text-[10px] text-muted-foreground">QuizTrace AI Engine</span>
                </div>
              </div>
              <button
                onClick={() => setDrawerOpen(false)}
                className="flex h-7 w-7 items-center justify-center rounded-full bg-surface-secondary text-muted-foreground"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>

            {/* Drawer Links */}
            <div className="flex-1 overflow-y-auto p-3 space-y-1.5 touch-pan-y">
              <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground block px-2 py-1">
                自动化与工程（开发中）
              </span>
              {drawerAdminItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  onClick={() => setDrawerOpen(false)}
                  className={({ isActive }) =>
                    `flex items-start gap-3 rounded-2xl p-3 text-xs transition-all ${
                      isActive
                        ? 'bg-primary/10 text-primary font-bold shadow-2xs'
                        : 'text-foreground/80 hover:bg-surface-secondary active:scale-[0.98]'
                    }`
                  }
                >
                  <item.icon className="h-4 w-4 text-primary shrink-0 mt-0.5" />
                  <div className="min-w-0">
                    <div className="font-semibold">{item.label}</div>
                    <div className="text-[10px] text-muted-foreground leading-tight mt-0.5">{item.desc}</div>
                  </div>
                </NavLink>
              ))}
            </div>

            {/* Drawer Footer */}
            <div className="p-3 border-t border-border/60 bg-surface-secondary/30 space-y-2.5">
              <button
                type="button"
                onClick={() => {
                  setDrawerOpen(false)
                  toggleViewMode()
                }}
                className="w-full flex items-center justify-center gap-2 rounded-2xl border border-primary/25 bg-primary/10 py-2.5 text-xs font-bold text-primary active:scale-[0.98] shadow-2xs cursor-pointer"
              >
                <Monitor className="h-4 w-4" />
                <span>切换至电脑端视图</span>
              </button>

              <div className="flex items-center justify-between px-2 py-1 text-xs">
                <span className="text-muted-foreground truncate">{user?.username}</span>
                <button
                  onClick={() => {
                    setDrawerOpen(false)
                    logout()
                    navigate('/login')
                  }}
                  className="flex items-center gap-1 text-destructive hover:opacity-80 text-xs font-medium"
                >
                  <LogOut className="h-3.5 w-3.5" />
                  <span>退出</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Top Sticky App Header */}
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between px-3.5 border-b border-border/60 bg-surface/85 backdrop-blur-xl pt-safe">
        {/* Left: Back button OR Menu toggle + Brand */}
        <div className="flex items-center gap-2 min-w-0">
          {!pageInfo.isRoot ? (
            <button
              onClick={() => {
                if (pageInfo.backTo) navigate(pageInfo.backTo)
                else navigate(-1)
              }}
              className="flex h-8 w-8 items-center justify-center rounded-xl bg-surface-secondary text-foreground hover:bg-accent active:scale-90 transition-transform"
              aria-label="返回"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
          ) : (
            <button
              onClick={() => setDrawerOpen(true)}
              className="flex h-8 w-8 items-center justify-center rounded-xl bg-surface-secondary text-foreground hover:bg-accent active:scale-90 transition-transform"
              aria-label="打开工具菜单"
            >
              <Menu className="h-4 w-4" />
            </button>
          )}

          <div className="min-w-0">
            <h1 className="text-sm font-bold tracking-tight text-foreground truncate">{pageInfo.title}</h1>
          </div>
        </div>

        {/* Right: AI Copilot & Theme Toggle */}
        <div className="flex items-center gap-1.5 shrink-0">
          {!isPracticeMode && (
            <button
              onClick={() => setAiSheetOpen(true)}
              className="flex items-center gap-1.5 rounded-xl border border-primary/25 bg-primary/10 px-2.5 py-1 text-xs font-bold text-primary active:scale-95 shadow-glow"
            >
              <Sparkles className="h-3.5 w-3.5 animate-pulse-subtle" />
              <span>AI 助教</span>
            </button>
          )}

          <button
            onClick={toggleDark}
            className="flex h-8 w-8 items-center justify-center rounded-xl text-muted-foreground hover:text-foreground active:scale-90 transition-transform"
            aria-label="切换深色模式"
          >
            {darkMode ? <Sun className="h-4 w-4 text-amber-400" /> : <Moon className="h-4 w-4" />}
          </button>
        </div>
      </header>

      {/* Main Scrollable Viewport */}
      <main
        className={`flex-1 min-h-0 overflow-y-auto bg-background/50 touch-pan-y ${
          isImmersive ? '' : 'pb-[calc(env(safe-area-inset-bottom)+3.5rem)]'
        }`}
      >
        <Outlet />
      </main>

      {/* Bottom Floating/Fixed TabBar */}
      {!isImmersive && (
        <nav className="fixed bottom-0 inset-x-0 z-40 flex items-center justify-around border-t border-border/70 bg-surface/90 backdrop-blur-xl px-2 py-1.5 pb-[calc(env(safe-area-inset-bottom)+0.35rem)] shadow-card">
          {bottomNavItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex flex-col items-center justify-center py-1 px-3 rounded-2xl transition-all duration-150 active:scale-90 ${
                  isActive ? 'text-primary font-bold' : 'text-muted-foreground hover:text-foreground'
                }`
              }
            >
              {({ isActive }) => (
                <>
                  <div className="relative">
                    <item.icon className={`h-5 w-5 transition-transform ${isActive ? 'scale-110' : ''}`} />
                    {isActive && (
                      <span className="absolute -bottom-1 left-1/2 -translate-x-1/2 h-1 w-1 rounded-full bg-primary" />
                    )}
                  </div>
                  <span className="text-[10px] mt-0.5 tracking-tight">{item.label}</span>
                </>
              )}
            </NavLink>
          ))}
        </nav>
      )}
    </div>
  )
}
