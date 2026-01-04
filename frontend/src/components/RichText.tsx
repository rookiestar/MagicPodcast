'use client'

import { useMemo } from 'react'
import DOMPurify from 'dompurify'

interface RichTextProps {
  html: string
  className?: string
}

/**
 * 富文本渲染组件
 * 使用DOMPurify净化HTML，防止XSS攻击
 */
export default function RichText({ html, className = '' }: RichTextProps) {
  // 净化HTML，移除危险的标签和属性
  const cleanHtml = useMemo(() => {
    return DOMPurify.sanitize(html, {
      // 允许的标签
      ALLOWED_TAGS: [
        'p',
        'br',
        'span',
        'strong',
        'b',
        'em',
        'i',
        'u',
        'a',
        'ul',
        'ol',
        'li',
        'h1',
        'h2',
        'h3',
        'h4',
        'h5',
        'h6',
        'blockquote',
        'code',
        'pre',
        'div',
      ],
      // 允许的属性
      ALLOWED_ATTR: [
        'href',
        'title',
        'alt',
        'class',
        'style',
        'target',
        'data-*',
      ],
      // 允许的URI协议
      ALLOWED_URI_REGEXP: /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp|data):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
    })
  }, [html])

  return (
    <div
      className={`rich-text prose prose-slate max-w-none dark:prose-invert ${className}`}
      dangerouslySetInnerHTML={{ __html: cleanHtml }}
    />
  )
}
