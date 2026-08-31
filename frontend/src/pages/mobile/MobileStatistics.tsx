import React, { useEffect, useRef, useState } from 'react'
import client from '@/api/client'
import { MasteryIndicator, Skeleton } from '@/components/ui'
import { MobilePullToRefresh } from '@/components/mobile/MobilePullToRefresh'
import { useUIStore } from '@/store/ui'
import * as echarts from 'echarts'
import type { ChapterMastery, StatOverview } from '@/types'
import {
  BarChart3,
  Calendar,
  CheckCircle2,
  Clock,
  Flame,
  GraduationCap,
  Target,
  TrendingUp,
  Zap,
} from 'lucide-react'

function useMobileChart(option: echarts.EChartsOption | null, deps: unknown[]) {
  const ref = useRef<HTMLDivElement>(null)
  const darkMode = useUIStore((s) => s.darkMode)

  useEffect(() => {
    if (!ref.current || !option) return
    const chart = echarts.init(ref.current, darkMode ? 'dark' : undefined)
    chart.setOption(option)
    const onResize = () => chart.resize()
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chart.dispose()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, darkMode])
  return ref
}

export default function MobileStatistics() {
  const [overview, setOverview] = useState<StatOverview | null>(null)
  const [trend, setTrend] = useState<{ date: string; question_count: number; correct_count: number; wrong_count: number }[]>([])
  const [wrongTrend, setWrongTrend] = useState<{ date: string; wrong_count: number }[]>([])
  const [chapterMastery, setChapterMastery] = useState<ChapterMastery[]>([])
  const [loading, setLoading] = useState(true)
  const darkMode = useUIStore((s) => s.darkMode)

  const loadData = async () => {
    try {
      const [ov, tr, wt, cm] = await Promise.all([
        client.get('/statistics/overview'),
        client.get('/statistics/trend', { params: { days: 30 } }),
        client.get('/statistics/wrong-trend', { params: { days: 30 } }),
        client.get('/statistics/chapter-mastery'),
      ])
      setOverview(ov.data.data)
      setTrend(tr.data.data)
      setWrongTrend(wt.data.data)
      setChapterMastery(cm.data.data)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  const textColor = darkMode ? '#94a3b8' : '#64748b'
  const borderColor = darkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'
  const tooltipBg = darkMode ? '#1e293b' : '#ffffff'
  const tooltipBorder = darkMode ? '#334155' : '#e2e8f0'
  const tooltipText = darkMode ? '#f8fafc' : '#0f172a'

  // Mobile 30 Days Trend Option
  const trendOption: echarts.EChartsOption | null = trend.length
    ? {
        backgroundColor: 'transparent',
        tooltip: {
          trigger: 'axis',
          backgroundColor: tooltipBg,
          borderColor: tooltipBorder,
          textStyle: { color: tooltipText, fontSize: 11 },
          padding: [6, 10],
          extraCssText: 'box-shadow: 0 4px 16px rgba(0,0,0,0.12); border-radius: 12px;',
        },
        legend: {
          data: ['做题', '正确', '错题'],
          bottom: 0,
          textStyle: { color: textColor, fontSize: 10 },
          icon: 'roundRect',
          itemWidth: 10,
          itemHeight: 10,
        },
        grid: { left: 30, right: 10, top: 15, bottom: 35 },
        xAxis: {
          type: 'category',
          data: trend.slice(-14).map((t) => t.date.slice(5)),
          axisLine: { lineStyle: { color: borderColor } },
          axisLabel: { color: textColor, fontSize: 9 },
          axisTick: { show: false },
        },
        yAxis: {
          type: 'value',
          minInterval: 1,
          splitLine: { lineStyle: { color: borderColor, type: 'dashed' } },
          axisLabel: { color: textColor, fontSize: 9 },
        },
        series: [
          {
            name: '做题',
            type: 'bar',
            data: trend.slice(-14).map((t) => t.question_count),
            itemStyle: { color: '#6366f1', borderRadius: [4, 4, 0, 0] },
            barMaxWidth: 12,
          },
          {
            name: '正确',
            type: 'line',
            smooth: true,
            data: trend.slice(-14).map((t) => t.correct_count),
            itemStyle: { color: '#10b981' },
            lineStyle: { width: 2 },
            showSymbol: false,
          },
          {
            name: '错题',
            type: 'line',
            smooth: true,
            data: trend.slice(-14).map((t) => t.wrong_count),
            itemStyle: { color: '#ef4444' },
            lineStyle: { width: 2, type: 'dashed' },
            showSymbol: false,
          },
        ],
      }
    : null

  const trendChartRef = useMobileChart(trendOption, [trend])

  return (
    <MobilePullToRefresh onRefresh={loadData} className="p-4 space-y-4 pb-20">
      {/* Core KPI strip */}
      {loading ? (
        <div className="grid grid-cols-2 gap-2.5">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-20 rounded-2xl" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-2.5">
          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>今日做题</span>
              <Target className="h-3.5 w-3.5 text-primary" />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-foreground">
                {overview?.today.question_count ?? 0}
              </span>
              <span className="text-[10px] text-muted-foreground">题</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5">
              正确 {overview?.today.correct_count ?? 0} · 错 {overview?.today.wrong_count ?? 0}
            </div>
          </div>

          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>综合正确率</span>
              <TrendingUp className="h-3.5 w-3.5 text-emerald-500" />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-foreground">
                {overview?.accuracy ?? 0}
              </span>
              <span className="text-[10px] text-muted-foreground">%</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5">
              累计作答 {overview?.total_question_count ?? 0} 题
            </div>
          </div>

          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>待复习错题</span>
              <Clock className="h-3.5 w-3.5 text-amber-500" />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-amber-500">
                {overview?.due_review_count ?? 0}
              </span>
              <span className="text-[10px] text-muted-foreground">题</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5">艾宾浩斯队列</div>
          </div>

          <div className="rounded-2xl border border-border/80 bg-surface p-3 shadow-card">
            <div className="flex items-center justify-between text-[10px] font-bold text-muted-foreground uppercase">
              <span>连续打卡</span>
              <Flame className="h-3.5 w-3.5 text-rose-500" />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-xl font-bold font-mono text-foreground">
                {overview?.streak_days ?? 0}
              </span>
              <span className="text-[10px] text-muted-foreground">天</span>
            </div>
            <div className="text-[10px] text-muted-foreground mt-0.5">保持每日刷题</div>
          </div>
        </div>
      )}

      {/* 30-Day Trend Chart Card */}
      <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1.5">
            <TrendingUp className="h-4 w-4 text-primary" />
            <h3 className="text-xs font-bold text-foreground">近期做题走势 (近14天)</h3>
          </div>
        </div>
        <div ref={trendChartRef} className="h-52 w-full" />
      </div>

      {/* Chapter Mastery Rankings */}
      <div className="rounded-3xl border border-border/80 bg-surface p-4 shadow-card space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1.5">
            <GraduationCap className="h-4 w-4 text-emerald-500" />
            <h3 className="text-xs font-bold text-foreground">章节掌握度排行</h3>
          </div>
          <span className="text-[10px] text-muted-foreground font-mono">{chapterMastery.length} 个章节</span>
        </div>

        {chapterMastery.length === 0 ? (
          <div className="py-4 text-center text-xs text-muted-foreground">暂无章节掌握度统计</div>
        ) : (
          <div className="space-y-2.5">
            {chapterMastery.slice(0, 8).map((cm) => (
              <div
                key={cm.chapter_id}
                className="flex items-center justify-between rounded-2xl bg-surface-secondary/50 p-2.5 text-xs"
              >
                <div className="min-w-0 pr-2">
                  <div className="font-semibold text-foreground truncate max-w-[170px]">{cm.chapter_name}</div>
                  <div className="text-[10px] text-muted-foreground mt-0.5">
                    做题 {cm.attempts} 次 · 正确率 {Math.round(cm.accuracy * 100)}%
                  </div>
                </div>
                <MasteryIndicator score={cm.mastery_score} size="md" />
              </div>
            ))}
          </div>
        )}
      </div>
    </MobilePullToRefresh>
  )
}
