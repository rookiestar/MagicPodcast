# 性能测试指南

**文档版本**: v1.0
**创建日期**: 2026-02-01

---

## 一、后端性能测试

### 1.1 运行基准测试

已创建自定义基准测试工具 `cmd/benchmark/main.go`。

**运行步骤**：

```bash
# 1. 确保API服务器正在运行
cd backend
./bin/api

# 2. 在另一个终端运行基准测试
go build -o bin/benchmark ./cmd/benchmark/main.go
./bin/benchmark
```

**测试配置**（可修改）：
- `BENCHMARK_BASE_URL`: API服务器地址（默认: http://localhost:8080）
- `BENCHMARK_WORKERS`: 并发worker数（默认: 10）
- `BENCHMARK_DURATION`: 测试时长（默认: 30s）

**示例**：
```bash
# 使用20个并发worker，测试60秒
BENCHMARK_WORKERS=20 BENCHMARK_DURATION=60s ./bin/benchmark
```

### 1.2 当前性能基线

| 端点 | 吞吐量(req/s) | P95延迟 | 状态 |
|------|--------------|---------|------|
| 健康检查 | 1088 | 19ms | ⚠️ 需优化 |
| 播客列表 | 1627 | 11ms | ✅ 优秀 |
| 全文搜索 | 1.7 | 9091ms | 🔴 需紧急优化 |
| 标签列表 | 1089 | 20ms | ✅ 良好 |
| 工作流列表 | 1984 | 7ms | ✅ 优秀 |

### 1.3 性能优化目标

- **API响应时间 (P95)**：降低20%
- **吞吐量**：提升20%
- **成功率**：保持>99%
- **全文搜索**：从9秒降低到200ms以下

---

## 二、前端性能测试

### 2.1 打包分析

**脚本位置**：`frontend/scripts/analyze-bundle.sh`

**运行步骤**：
```bash
cd frontend
chmod +x scripts/analyze-bundle.sh
./scripts/analyze-bundle.sh
```

**注意**：当前有TypeScript编译错误需要修复：
```
src/app/import/page.tsx
Type error: Cannot find name 'Function'.
```

### 2.2 Lighthouse 性能审计

#### 安装 Lighthouse CI

```bash
cd frontend
npm install --save-dev @lhci/cli
```

#### 配置 Lighthouse CI

创建 `lighthouserc.js`：

```javascript
module.exports = {
  ci: {
    collect: {
      url: [
        'http://localhost:3000',
        'http://localhost:3000/podcasts',
        'http://localhost:3000/workflows',
      ],
      numberOfRuns: 3,
    },
    assert: {
      assertions: {
        'categories:performance': ['error', { minScore: 0.9 }],
        'categories:accessibility': ['warn', { minScore: 0.9 }],
        'categories:best-practices': ['warn', { minScore: 0.9 }],
        'categories:seo': ['warn', { minScore: 0.9 }],
      },
    },
    upload: {
      target: 'temporary-public-storage',
    },
  },
};
```

#### 运行 Lighthouse 审计

```bash
# 1. 启动开发服务器
npm run dev

# 2. 在另一个终端运行 Lighthouse
npx lhci autorun
```

### 2.3 Chrome DevTools 手动审计

1. 打开 Chrome DevTools（F12）
2. 切换到 "Lighthouse" 标签
3. 选择要审计的类别：
   - Performance
   - Accessibility
   - Best Practices
   - SEO
4. 点击 "Analyze page load"
5. 等待审计完成并查看报告

### 2.4 WebPageTest 在线测试

访问 https://www.webptests.org/
1. 输入网站URL
2. 配置测试选项（位置、浏览器、连接速度）
3. 运行测试
4. 查看详细报告

---

## 三、持续性能监控

### 3.1 集成到 CI/CD

**GitHub Actions 示例**：

```yaml
name: Performance Tests

on:
  pull_request:
    branches: [main]

jobs:
  lighthouse:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Use Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: npm ci
      - run: npm run build
      - name: Run Lighthouse CI
        run: npx lhci autorun
```

### 3.2 性能回归检测

设置性能预算（`package.json`）：

```json
{
  "scripts": {
    "lighthouse": "lhci autorun",
    "lighthouse:server": "lhci collect --numberOfRuns=5"
  }
}
```

---

## 四、性能优化建议

### 4.1 后端优化

#### 1. 全文搜索优化（紧急）
- **问题**：P95延迟 9秒
- **解决方案**：
  - 添加FTS索引优化
  - 实现查询结果缓存（Redis）
  - 限制搜索结果数量
  - 考虑使用Elasticsearch

#### 2. 数据库查询优化
- 使用EXPLAIN QUERY PLAN分析慢查询
- 添加适当的索引
- 使用预加载（Preload）减少N+1查询

#### 3. 并发优化
- 增加worker数量
- 实现连接池
- 使用goroutine池

### 4.2 前端优化

#### 1. 代码分割
- 使用Next.js动态导入
- 按路由分割代码
- 延迟加载非关键组件

#### 2. 图片优化
- 使用Next.js Image组件
- 实现懒加载
- 使用WebP格式

#### 3. 缓存策略
- 实现Service Worker
- 使用HTTP缓存头
- 实现本地存储缓存

#### 4. 打包优化
- Tree shaking移除未使用代码
- 压缩JavaScript和CSS
- 使用bundle analyzer分析打包产物

---

## 五、性能基准目标

### 5.1 后端目标

| 指标 | 当前 | 目标 | 提升 |
|------|------|------|------|
| API P95延迟 | 19ms | 15ms | -21% |
| 搜索P95延迟 | 9091ms | 200ms | -98% |
| 吞吐量 | 1627 req/s | 2000 req/s | +23% |
| 成功率 | 97.5% | 99% | +1.5% |

### 5.2 前端目标

| 指标 | 目标 |
|------|------|
| Lighthouse 性能分数 | >90 |
| 首次内容绘制(FCP) | <1.5s |
| 最大内容绘制(LCP) | <2.5s |
| 交互时间(TTI) | <3.0s |
| 累积布局偏移(CLS) | <0.1 |

---

## 六、故障排查

### 6.1 基准测试失败

**问题**：`API服务不可用`
- 检查API服务器是否正在运行
- 检查端口是否正确（默认8080）
- 检查防火墙设置

**问题**：`请求失败`
- 检查数据库连接
- 检查日志文件
- 使用curl手动测试端点

### 6.2 Lighthouse 失败

**问题**：无法连接到服务器
- 确保开发服务器正在运行
- 检查端口（默认3000）
- 尝试使用localhost而不是127.0.0.1

**问题**：分数很低
- 检查JavaScript错误
- 优化图片大小
- 启用压缩
- 减少重定向

---

## 七、参考资料

- [Google Web Fundamentals](https://developers.google.com/web/fundamentals)
- [Lighthouse Documentation](https://github.com/GoogleChrome/lighthouse)
- [WebPageTest Documentation](https://docs.webptests.org/)
- [Next.js Performance](https://nextjs.org/docs/advanced-features/measuring-performance)
- [Go Performance](https://go.dev/doc/diagnostics)

---

**最后更新**: 2026-02-01
**维护者**: Development Team
