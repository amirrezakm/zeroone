import type { Config } from 'tailwindcss';

const config: Config = {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        bg: { DEFAULT: '#f5f6f8', dark: '#0f1115' },
        panel: { DEFAULT: '#ffffff', dark: '#17191e' },
        border: { DEFAULT: '#e5e7eb', dark: '#262931' },
        text: { DEFAULT: '#111827', dark: '#e7e9ee' },
        muted: { DEFAULT: '#6b7280', dark: '#9ba1ac' },
        accent: { DEFAULT: '#f38020', dark: '#f38020' },
        ok: { DEFAULT: '#0a8050', dark: '#22c08a' },
        warn: { DEFAULT: '#b45309', dark: '#facc15' },
        bad: { DEFAULT: '#b91c1c', dark: '#f87171' },
      },
      fontFamily: {
        sans: ['Inter', 'ui-sans-serif', 'system-ui', '-apple-system', 'BlinkMacSystemFont', 'Segoe UI', 'sans-serif'],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      boxShadow: {
        card: '0 1px 2px rgba(16, 24, 40, .04)',
        elev: '0 8px 24px rgba(16, 24, 40, .08)',
      },
      borderRadius: {
        xl: '10px',
      },
    },
  },
  plugins: [],
};

export default config;
