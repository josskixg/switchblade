/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    './index.html',
    './src/**/*.{js,jsx}',
  ],
  theme: {
    extend: {
      colors: {
        // Every token is a bare `R G B` triplet in src/index.css.
        primary: 'rgb(var(--color-primary) / <alpha-value>)',
        'primary-hover': 'rgb(var(--color-primary-hover) / <alpha-value>)',
        'primary-dim': 'rgb(var(--color-primary-dim) / <alpha-value>)',
        'primary-foreground': 'rgb(var(--color-primary-foreground) / <alpha-value>)',
        'primary-wash': 'rgb(var(--color-primary-wash) / <alpha-value>)',
        focus: 'rgb(var(--color-focus) / <alpha-value>)',

        background: 'rgb(var(--color-background) / <alpha-value>)',
        shell: 'rgb(var(--color-shell) / <alpha-value>)',
        surface: 'rgb(var(--color-surface) / <alpha-value>)',
        'surface-elevated': 'rgb(var(--color-surface-elevated) / <alpha-value>)',
        'surface-hover': 'rgb(var(--color-surface-hover) / <alpha-value>)',
        'surface-sunken': 'rgb(var(--color-surface-sunken) / <alpha-value>)',

        border: 'rgb(var(--color-border) / <alpha-value>)',
        'border-subtle': 'rgb(var(--color-border-subtle) / <alpha-value>)',
        'border-strong': 'rgb(var(--color-border-strong) / <alpha-value>)',

        text: 'rgb(var(--color-text) / <alpha-value>)',
        'text-secondary': 'rgb(var(--color-text-secondary) / <alpha-value>)',
        muted: 'rgb(var(--color-muted) / <alpha-value>)',
        'text-disabled': 'rgb(var(--color-text-disabled) / <alpha-value>)',

        success: 'rgb(var(--color-success) / <alpha-value>)',
        warning: 'rgb(var(--color-warning) / <alpha-value>)',
        danger: 'rgb(var(--color-danger) / <alpha-value>)',
        'danger-foreground': 'rgb(var(--color-danger-foreground) / <alpha-value>)',

        skeleton: 'rgb(var(--color-skeleton) / <alpha-value>)',
        overlay: 'rgb(var(--color-overlay) / <alpha-value>)',

        'series-1': 'rgb(var(--color-series-1) / <alpha-value>)',
        'series-2': 'rgb(var(--color-series-2) / <alpha-value>)',
        'series-3': 'rgb(var(--color-series-3) / <alpha-value>)',
        'series-4': 'rgb(var(--color-series-4) / <alpha-value>)',
        'series-5': 'rgb(var(--color-series-5) / <alpha-value>)',
        'series-6': 'rgb(var(--color-series-6) / <alpha-value>)',
      },

      fontFamily: {
        sans: ['"Instrument Sans"', 'ui-sans-serif', 'system-ui', '-apple-system', '"Segoe UI"', 'sans-serif'],
        display: ['"Instrument Serif"', 'Georgia', '"Times New Roman"', 'serif'],
        mono: ['"IBM Plex Mono"', 'ui-monospace', '"SF Mono"', 'Menlo', 'Consolas', 'monospace'],
      },

      // Semantic scale. Arbitrary sizes (text-[10px], text-[11px]) are banned.
      fontSize: {
        'display-1': ['2.5rem', { lineHeight: '2.75rem', fontWeight: '400', letterSpacing: '-0.02em' }],
        'display-2': ['2rem', { lineHeight: '2.25rem', fontWeight: '400', letterSpacing: '-0.018em' }],
        'page-title': ['1.5rem', { lineHeight: '1.875rem', fontWeight: '600', letterSpacing: '-0.014em' }],
        section: ['1.125rem', { lineHeight: '1.625rem', fontWeight: '600', letterSpacing: '-0.008em' }],
        'card-title': ['0.9375rem', { lineHeight: '1.375rem', fontWeight: '600', letterSpacing: '-0.004em' }],
        body: ['0.875rem', { lineHeight: '1.375rem', fontWeight: '400' }],
        label: ['0.8125rem', { lineHeight: '1.125rem', fontWeight: '500' }],
        caption: ['0.75rem', { lineHeight: '1.125rem', fontWeight: '400' }],
        micro: ['0.6875rem', { lineHeight: '0.875rem', fontWeight: '600', letterSpacing: '0.04em' }],
        'metric-sm': ['1.25rem', { lineHeight: '1.625rem', fontWeight: '600', letterSpacing: '-0.01em' }],
        'metric-md': ['1.75rem', { lineHeight: '2rem', fontWeight: '600', letterSpacing: '-0.018em' }],
        code: ['0.78125rem', { lineHeight: '1.25rem', fontWeight: '400' }],
      },

      // A nested container steps DOWN one level from its parent.
      borderRadius: {
        none: '0px',
        xs: 'var(--r-xs)',
        sm: 'var(--r-sm)',
        DEFAULT: 'var(--r-sm)',
        md: 'var(--r-md)',
        lg: 'var(--r-lg)',
        xl: 'var(--r-xl)',
        '2xl': 'var(--r-xl)',
        '3xl': '24px',
        full: 'var(--r-full)',
      },

      boxShadow: {
        1: 'var(--shadow-1)',
        2: 'var(--shadow-2)',
        3: 'var(--shadow-3)',
        // Deprecated aliases kept so pages mid-migration do not lose elevation.
        // Do not use in new code — reach for shadow-1|2|3.
        card: 'var(--shadow-1)',
        'card-hover': 'var(--shadow-2)',
        none: 'none',
      },

      transitionTimingFunction: {
        swift: 'cubic-bezier(.2, 0, .38, .9)',
      },

      transitionDuration: {
        120: '120ms',
        160: '160ms',
        400: '400ms',
      },

      animation: {
        'fade-in': 'fadeIn 0.16s cubic-bezier(.2, 0, .38, .9)',
        'slide-up': 'slideUp 0.16s cubic-bezier(.2, 0, .38, .9)',
        'slide-in-right': 'slideInRight 0.16s cubic-bezier(.2, 0, .38, .9)',
        'scale-in': 'scaleIn 0.16s cubic-bezier(.2, 0, .38, .9)',
        shimmer: 'shimmer 1.4s linear infinite',
      },

      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' },
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(8px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(8px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' },
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.98)' },
          '100%': { opacity: '1', transform: 'scale(1)' },
        },
        shimmer: {
          '0%': { backgroundPosition: '-160% 0' },
          '100%': { backgroundPosition: '260% 0' },
        },
      },
    },
  },
  plugins: [
    require('@tailwindcss/forms'),
  ],
};
