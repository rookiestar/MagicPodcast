import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import React from 'react'

describe('Vitest 配置验证', () => {
  it('应该能够渲染一个简单的 React 组件', () => {
    const TestComponent = () => <div>Hello World</div>
    render(<TestComponent />)
    expect(screen.getByText('Hello World')).toBeInTheDocument()
  })

  it('应该能够进行基本的断言', () => {
    expect(1 + 1).toBe(2)
    expect(true).toBe(true)
  })
})
