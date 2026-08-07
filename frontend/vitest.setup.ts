import React from 'react'
import { vi } from 'vitest'
import '@testing-library/jest-dom'

// Mock Next.js router
vi.mock('next/navigation', () => ({
  useRouter() {
    return {
      push: vi.fn(),
      replace: vi.fn(),
      prefetch: vi.fn(),
      back: vi.fn(),
      pathname: '/',
      query: {},
      asPath: '/',
    }
  },
  useSearchParams() {
    return new URLSearchParams()
  },
  usePathname() {
    return '/'
  },
}))

// Mock Next.js Image component
vi.mock('next/image', () => ({
  __esModule: true,
  default: vi.fn().mockImplementation(({
    src,
    alt,
    fill,
    priority,
    unoptimized,
    ...props
  }: any) =>
    React.createElement('img', {
      src: typeof src === 'string' ? src : src?.src,
      alt,
      'data-optimized': unoptimized ? 'false' : 'true',
      ...props,
    }),
  ),
}))

// Mock window.matchMedia
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation(query => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})
