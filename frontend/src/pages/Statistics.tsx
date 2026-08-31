import { useEffect, useRef, useState } from 'react'
import client from '@/api/client'
import { Card, CardHeader, PageHeader, Skeleton, StatCard } from '@/components/ui'
import * as echarts from 'echarts'
import { useUIStore } from '@/store/ui'
import type { ChapterMastery, StatOverview } from '@/types'
import { BarChart3, Flame, Target, TrendingUp, Zap } from 'lucide-react'

function useChart(option: echarts.EChartsOption | null, deps: unknown[]) {
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

export default function Statistics() {
  const [overview, setOverview] = useState<StatOverview | null>(null)
  const [trend, setTrend] = useState<{ date: string; question_count: number; correct_count: number; wrong_count: number }[]>([])
  const [wrongTrend, setWrongTrend] = useState<{ date: string; wrong_count: number }[]>([])
  const [chapterMastery, setChapterMastery] = useState<ChapterMastery[]>([])
  const [loading, setLoading] = useState(true)
  const darkMode = useUIStore((s) => s.darkMode)

  useEffect(() => {
    const load = async () => {
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
    load()
  }, [])

  const textColor = darkMode ? '#94a3b8' : '#64748b'
  const borderColor = darkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'
  const tooltipBg = darkMode ? '#1e293b' : '#ffffff'
  const tooltipBorder = darkMode ? '#334155' : '#e2e8f0'
  const tooltipText = darkMode ? '#f8fafc' : '#0f172a'

  // 30 Days Learning Trend
  const trendOption: echarts.EChartsOption | null = trend.length
    ? {
        backgroundColor: 'transparent',
        tooltip: {
          trigger: 'axis',
          backgroundColor: tooltipBg,
          borderColor: tooltipBorder,
          textStyle: { color: tooltipText, fontSize: 12 },
          padding: [8, 12],
          extraCssText: 'box-shadow: 0 8px 24px -4px rgba(0,0,0,0.12); border-radius: 12px;',
        },
        legend: {
          data: ['做题总量', '正确题数', '错题数'],
          bottom: 0,
          textStyle: { color: textColor, fontSize: 11 },
          icon: 'roundRect',
        },
        grid: { left: 35, right: 15, top: 20, bottom: 40 },
        xAxis: {
          type: 'category',
          data: trend.map((t) => t.date.slice(5)),
          axisLine: { lineStyle: { color: borderColor } },
          axisLabel: { color: textColor, fontSize: 10 },
          axisTick: { show: false },
        },
        yAxis: {
          type: 'value',
          minInterval: 1,
          splitLine: { lineStyle: { color: borderColor, type: 'dashed' } },
          axisLabel: { color: textColor, fontSize: 10 },
        },
        series: [
          {
            name: '做题总量',
            type: 'bar',
            data: trend.map((t) => t.question_count),
            itemStyle: { color: '#6366f1', borderRadius: [6, 6, 0, 0] },
            barMaxWidth: 16,
          },
          {
            name: '正确题数',
            type: 'line',
            smooth: true,
            data: trend.map((t) => t.correct_count),
            itemStyle: { color: '#10b981' },
            lineStyle: { width: 2.5 },
            showSymbol: false,
          },
          {
            name: '错题数',
            type: 'line',
            smooth: true,
            data: trend.map((t) => t.wrong_count),
            itemStyle: { color: '#ef4444' },
            lineStyle: { width: 2, type: 'dashed' },
            showSymbol: false,
          },
        ],
      }
    : null

  // Wrong Trend Chart
  const wrongOption: echarts.EChartsOption | null = wrongTrend.length
    ? {
        backgroundColor: 'transparent',
        tooltip: {
          trigger: 'axis',
          backgroundColor: tooltipBg,
          borderColor: tooltipBorder,
          textStyle: { color: tooltipText, fontSize: 12 },
          extraCssText: 'box-shadow: 0 8px 24px -4px rgba(0,0,0,0.12); border-radius: 12px;',
        },
        grid: { left: 35, right: 15, top: 20, bottom: 25 },
        xAxis: {
          type: 'category',
          data: wrongTrend.map((t) => t.date.slice(5)),
          axisLine: { lineStyle: { color: borderColor } },
          axisLabel: { color: textColor, fontSize: 10 },
          axisTick: { show: false },
        },
        yAxis: {
          type: 'value',
          minInterval: 1,
          splitLine: { lineStyle: { color: borderColor, type: 'dashed' } },
          axisLabel: { color: textColor, fontSize: 10 },
        },
        series: [
          {
            name: '每日错题数',
            type: 'bar',
            data: wrongTrend.map((t) => t.wrong_count),
            itemStyle: { color: '#ef4444', borderRadius: [6, 6, 0, 0] },
            barMaxWidth: 16,
          },
        ],
      }
    : null

  // Chapter Mastery
  const masteryOption: echarts.EChartsOption | null = chapterMastery.length
    ? {
        backgroundColor: 'transparent',
        tooltip: {
          trigger: 'axis',
          backgroundColor: tooltipBg,
          borderColor: tooltipBorder,
          textStyle: { color: tooltipText, fontSize: 12 },
          extraCssText: 'box-shadow: 0 8px 24px -4px rgba(0,0,0,0.12); border-radius: 12px;',
        },
        legend: {
          data: ['掌握度分', '作答正确率 %'],
          bottom: 0,
          textStyle: { color: textColor, fontSize: 11 },
        },
        grid: { left: 40, right: 20, top: 20, bottom: 85 },
        xAxis: {
          type: 'category',
          data: chapterMastery.map((c) => c.chapter_name),
          axisLine: { lineStyle: { color: borderColor } },
          axisLabel: {
            color: textColor,
            fontSize: 10,
            interval: 0,
            rotate: 32,
            width: 80,
            overflow: 'truncate',
          },
          axisTick: { show: false },
        },
        yAxis: {
          type: 'value',
          max: 100,
          splitLine: { lineStyle: { color: borderColor, type: 'dashed' } },
          axisLabel: { color: textColor, fontSize: 10 },
        },
        series: [
          {
            name: '掌握度分',
            type: 'bar',
            data: chapterMastery.map((c) => c.mastery_score),
            itemStyle: { color: '#6366f1', borderRadius: [6, 6, 0, 0] },
            barMaxWidth: 20,
          },
          {
            name: '作答正确率 %',
            type: 'line',
            smooth: true,
            data: chapterMastery.map((c) => c.accuracy),
            itemStyle: { color: '#10b981' },
            lineStyle: { width: 2.5 },
            showSymbol: false,
          },
        ],
      }
    : null

  const trendRef = useChart(trendOption, [trend])
  const wrongRef = useChart(wrongOption, [wrongTrend])
  const masteryRef = useChart(masteryOption, [chapterMastery])

  return (
    <div className="space-y-6">
      <PageHeader
        title="学习数据分析"
        subtitle="全景式透视答题轨迹、正确率趋势与各学科章节掌握度。"
      />

      {/* KPI Cards */}
      {loading ? (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-28 rounded-3xl" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <StatCard
            icon={Target}
            label="累计答题总量"
            value={overview?.total_question_count ?? 0}
            unit="道"
            sub="全周期作答"
            accent="primary"
          />
          <StatCard
            icon={TrendingUp}
            label="全周期总正确率"
            value={`${overview?.accuracy ?? 0}`}
            unit="%"
            sub="综合正答表现"
            accent="success"
          />
          <StatCard
            icon={Zap}
            label="错题收录量"
            value={overview?.wrong_count ?? 0}
            unit="道"
            sub="错题本管理总量"
            accent="warning"
          />
          <StatCard
            icon={Flame}
            label="今日已刷题目"
            value={overview?.today.question_count ?? 0}
            unit="道"
            sub={`今日已答 ${overview?.today.question_count ?? 0} 题`}
            accent="danger"
          />
        </div>
      )}

      {/* Charts Layout */}
      <div className="grid gap-6 lg:grid-cols-2">
        <Card className="rounded-4xl p-2 sm:p-4">
          <CardHeader
            title="30 天学习动态"
            desc="做题量与正误分布趋势"
            icon={<TrendingUp className="h-4 w-4" />}
          />
          <div ref={trendRef} className="h-72 p-2" />
        </Card>

        <Card className="rounded-4xl p-2 sm:p-4">
          <CardHeader
            title="错题产生频次"
            desc="近期答错考点与错误波峰追踪"
            icon={<Zap className="h-4 w-4" />}
          />
          <div ref={wrongRef} className="h-72 p-2" />
        </Card>
      </div>

      <Card className="rounded-4xl p-2 sm:p-4">
        <CardHeader
          title="各章节知识掌握度分析"
          desc="结合答题次数、遗忘曲线与正答率综合计算"
          icon={<BarChart3 className="h-4 w-4" />}
        />
        <div ref={masteryRef} className="h-80 p-2" />
      </Card>
    </div>
  )
}
