/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        border: 'hsl(var(--border))',
        'border-subtle': 'hsl(var(--border-subtle))',
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        surface: {
          DEFAULT: 'hsl(var(--surface))',
          secondary: 'hsl(var(--surface-secondary))',
          elevated: 'hsl(var(--surface-elevated))',
        },
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          hover: 'hsl(var(--primary-hover))',
          active: 'hsl(var(--primary-active))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        success: {
          DEFAULT: 'hsl(var(--success))',
          foreground: 'hsl(var(--success-foreground))',
        },
        warning: {
          DEFAULT: 'hsl(var(--warning))',
          foreground: 'hsl(var(--warning-foreground))',
        },
        info: {
          DEFAULT: 'hsl(var(--info))',
          foreground: 'hsl(var(--info-foreground))',
        },
      },
      borderRadius: {
        '4xl': '1.75rem',
        '3xl': '1.25rem',
        '2xl': '1rem',
        xl: 'var(--radius)',
        lg: 'calc(var(--radius) - 2px)',
        md: 'calc(var(--radius) - 4px)',
        sm: 'calc(var(--radius) - 6px)',
        xs: '0.375rem',
      },
      fontFamily: {
        sans: [
          'Inter',
          '-apple-system',
          'BlinkMacSystemFont',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'Segoe UI',
          'sans-serif',
        ],
        mono: ['JetBrains Mono', 'SF Mono', 'Fira Code', 'Menlo', 'Consolas', 'monospace'],
      },
      boxShadow: {
        '2xs': '0 1px 2px 0 rgba(0, 0, 0, 0.02)',
        xs: '0 1px 2px 0 rgba(0, 0, 0, 0.04)',
        subtle: '0 1px 3px 0 rgba(0, 0, 0, 0.04), 0 1px 2px -1px rgba(0, 0, 0, 0.02)',
        card: '0 1px 3px 0 rgba(0, 0, 0, 0.04), 0 1px 2px -1px rgba(0, 0, 0, 0.02)',
        'card-hover': '0 10px 25px -5px rgba(0, 0, 0, 0.06), 0 8px 10px -6px rgba(0, 0, 0, 0.03)',
        elevated: '0 12px 30px -4px rgba(0, 0, 0, 0.08), 0 4px 10px -2px rgba(0, 0, 0, 0.03)',
        float: '0 24px 48px -12px rgba(0, 0, 0, 0.16), 0 6px 16px -4px rgba(0, 0, 0, 0.06)',
        glow: '0 0 20px -3px var(--primary-glow)',
        'glow-sm': '0 0 12px -2px var(--primary-glow)',
        'glow-lg': '0 0 36px -6px var(--primary-glow)',
        'glow-success': '0 0 20px -3px rgba(16, 185, 129, 0.22)',
        'glow-amber': '0 0 20px -3px rgba(245, 158, 11, 0.22)',
        'glow-danger': '0 0 20px -3px rgba(239, 68, 68, 0.22)',
        'inner-subtle': 'inset 0 1px 2px 0 rgba(0, 0, 0, 0.03)',
      },
      transitionTimingFunction: {
        natural: 'cubic-bezier(0.16, 1, 0.3, 1)',
        spring: 'cubic-bezier(0.34, 1.56, 0.64, 1)',
        smooth: 'cubic-bezier(0.4, 0, 0.2, 1)',
      },
      keyframes: {
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' },
        },
        'fade-in': {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        'scale-in': {
          '0%': { opacity: '0', transform: 'scale(0.97)' },
          '100%': { opacity: '1', transform: 'scale(1)' },
        },
        'slide-up': {
          '0%': { opacity: '0', transform: 'translateY(6px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        'slide-down': {
          '0%': { opacity: '0', transform: 'translateY(-6px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        pulseSubtle: {
          '0%, 100%': { opacity: '1', transform: 'scale(1)' },
          '50%': { opacity: '0.6', transform: 'scale(0.96)' },
        },
      },
      animation: {
        shimmer: 'shimmer 2s infinite linear',
        'fade-in': 'fade-in 160ms cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'scale-in': 'scale-in 180ms cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'slide-up': 'slide-up 200ms cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'slide-down': 'slide-down 200ms cubic-bezier(0.16, 1, 0.3, 1) forwards',
        'pulse-subtle': 'pulseSubtle 2.2s cubic-bezier(0.4, 0, 0.6, 1) infinite',
      },
    },
  },
  plugins: [],
}
