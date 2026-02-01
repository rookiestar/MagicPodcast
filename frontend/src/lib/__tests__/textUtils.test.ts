import { describe, it, expect } from 'vitest'

// 简单的文本处理工具函数测试
describe('文本工具函数', () => {
  describe('字符串处理', () => {
    it('应该正确连接字符串', () => {
      const str1 = 'Hello'
      const str2 = 'World'
      const result = `${str1} ${str2}`
      expect(result).toBe('Hello World')
    })

    it('应该正确截断字符串', () => {
      const longText = 'This is a very long text that needs to be truncated'
      const maxLength = 20
      const truncated = longText.substring(0, maxLength) + '...'
      expect(truncated.length).toBeLessThanOrEqual(maxLength + 3)
    })

    it('应该正确判断空字符串', () => {
      expect(''.trim()).toBe('')
      expect('  '.trim()).toBe('')
    })
  })

  describe('数组处理', () => {
    it('应该正确过滤数组', () => {
      const arr = [1, 2, 3, 4, 5]
      const filtered = arr.filter(x => x > 2)
      expect(filtered).toEqual([3, 4, 5])
    })

    it('应该正确映射数组', () => {
      const arr = [1, 2, 3]
      const mapped = arr.map(x => x * 2)
      expect(mapped).toEqual([2, 4, 6])
    })
  })

  describe('数字处理', () => {
    it('应该正确格式化数字', () => {
      const num = 1234.5678
      const formatted = num.toFixed(2)
      expect(formatted).toBe('1234.57')
    })

    it('应该正确四舍五入', () => {
      expect(Math.round(1.4)).toBe(1)
      expect(Math.round(1.5)).toBe(2)
    })
  })
})
