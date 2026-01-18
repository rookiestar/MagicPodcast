/**
 * 图片加载队列管理器
 * 功能:
 * 1. 并发控制: 限制同时加载的图片数量
 * 2. 优先级队列: 高优先级图片优先加载
 * 3. 重试机制: 失败图片自动重试
 * 4. 预加载: 提前加载即将进入视口的图片
 */

interface LoadTask {
  id: string
  src: string
  imgElement: HTMLImageElement
  priority: 'high' | 'medium' | 'low'
  retryCount: number
  onSuccess: () => void
  onError: () => void
}

class ImageLoadQueue {
  private queue: LoadTask[] = []
  private loading = new Set<string>()
  private maxConcurrent = 3 // 最多同时加载3张图片
  private maxRetries = 1 // 最多重试1次（减少总等待时间）
  private loadingTimeout = 5000 // 5秒超时（减少等待时间）

  /**
   * 添加图片到加载队列
   */
  add(task: LoadTask) {
    // 检查是否已在队列中或正在加载
    const existing = this.queue.find(t => t.id === task.id)
    if (existing) {
      return // 已在队列中,不重复添加
    }

    if (this.loading.has(task.id)) {
      return // 正在加载中
    }

    this.queue.push(task)
    this.queue.sort((a, b) => this.getPriorityScore(b) - this.getPriorityScore(a))
    this.process()
  }

  /**
   * 获取优先级分数
   */
  private getPriorityScore(task: LoadTask): number {
    switch (task.priority) {
      case 'high': return 100
      case 'medium': return 50
      case 'low': return 10
      default: return 0
    }
  }

  /**
   * 处理队列
   */
  private process() {
    while (this.queue.length > 0 && this.loading.size < this.maxConcurrent) {
      const task = this.queue.shift()!
      this.load(task)
    }
  }

  /**
   * 加载单个图片
   */
  private load(task: LoadTask) {
    this.loading.add(task.id)

    // 设置超时
    const timeoutId = setTimeout(() => {
      this.handleTimeout(task)
    }, this.loadingTimeout)

    // 创建新的Image对象来预加载
    const tempImg = new Image()

    tempImg.onload = () => {
      clearTimeout(timeoutId)
      this.loading.delete(task.id)

      // 设置真实图片的src
      if (task.imgElement) {
        task.imgElement.src = task.src
      }
      task.onSuccess()
      this.process() // 继续处理队列
    }

    tempImg.onerror = () => {
      clearTimeout(timeoutId)
      this.handleError(task)
    }

    // 开始加载
    tempImg.src = task.src
  }

  /**
   * 处理加载错误
   */
  private handleError(task: LoadTask) {
    this.loading.delete(task.id)

    // 检查是否需要重试
    if (task.retryCount < this.maxRetries) {
      task.retryCount++
      console.log(`[ImageLoadQueue] 重试加载图片 (${task.retryCount}/${this.maxRetries}):`, task.src)
      this.queue.push(task) // 重新加入队列
      this.process()
    } else {
      console.error(`[ImageLoadQueue] 图片加载失败,已达到最大重试次数:`, task.src)
      task.onError()
      this.process() // 继续处理队列
    }
  }

  /**
   * 处理超时
   */
  private handleTimeout(task: LoadTask) {
    console.warn(`[ImageLoadQueue] 图片加载超时:`, task.src)
    this.loading.delete(task.id)
    this.handleError(task)
  }

  /**
   * 取消加载
   */
  cancel(id: string) {
    // 从队列中移除
    this.queue = this.queue.filter(t => t.id !== id)
    this.loading.delete(id)
  }

  /**
   * 清空队列
   */
  clear() {
    this.queue = []
    this.loading.clear()
  }

  /**
   * 获取队列状态
   */
  getStatus() {
    return {
      queue: this.queue.length,
      loading: this.loading.size,
      maxConcurrent: this.maxConcurrent
    }
  }
}

// 单例模式
export const imageLoadQueue = new ImageLoadQueue()

/**
 * React Hook: 使用图片加载队列
 */
export function useImageLoadQueue() {
  return {
    loadImage: (task: LoadTask) => imageLoadQueue.add(task),
    cancelLoad: (id: string) => imageLoadQueue.cancel(id),
    getStatus: () => imageLoadQueue.getStatus()
  }
}
