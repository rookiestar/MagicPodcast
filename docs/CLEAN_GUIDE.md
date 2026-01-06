# MagicPodcast 清理脚本使用指南

本项目提供了三个清理脚本，用于解决 Next.js 缓存错误、清理构建产物和优化数据库。

## 📁 脚本列表

### 1. `frontend/clean.sh` - 前端清理脚本
清理 Next.js 缓存和前端构建产物。

**清理内容：**
- `.next/` - Next.js 构建缓存
- `out/` - 静态导出输出
- `*.tsbuildinfo` - TypeScript 缓存
- `.eslintcache` - ESLint 缓存
- `node_modules/.cache` - 依赖缓存（仅 `--deep` 模式）

### 2. `backend/clean.sh` - 后端清理脚本
清理 Go 构建缓存、日志文件和数据库临时文件。

**清理内容：**
- `bin/*` - Go 编译的二进制文件
- `api.log.backup_*` - 7 天前的日志备份
- `*.db-wal` / `*.db-shm` - SQLite 临时文件
- `temp/` - 空的临时目录
- 数据库 VACUUM 优化（仅 `--vacuum` 模式）

### 3. `clean-all.sh` - 统一清理脚本
一键清理前端和后端的所有缓存。

## 🚀 使用方法

### 方式一：使用 npm 命令（仅前端）

```bash
# 进入前端目录
cd frontend

# 常规清理
npm run clean

# 深度清理（包括 node_modules/.cache）
npm run clean:deep
```

### 方式二：直接运行脚本

```bash
# 前端常规清理
cd frontend
./clean.sh

# 前端深度清理
./clean.sh --deep

# 后端常规清理
cd ../backend
./clean.sh

# 后端清理 + 数据库优化
./clean.sh --vacuum
```

### 方式三：使用统一清理脚本（推荐）

```bash
# 在项目根目录执行

# 常规清理（前端 + 后端）
./clean-all.sh

# 深度清理（包括 node_modules/.cache）
./clean-all.sh --deep

# 清理 + 数据库优化（回收空间）
./clean-all.sh --vacuum

# 深度清理 + 数据库优化
./clean-all.sh --deep --vacuum
```

## 🔧 何时使用清理脚本

### 遇到以下情况时需要清理：

#### 前端问题
1. **Next.js 构建错误**
   - 错误信息：`Failed to compile` 或 `Module not found`
   - 页面显示异常或样式丢失

2. **TypeScript 类型错误**
   - 明明正确的类型却报错
   - `tsc` 编译失败

3. **ESLint 缓存问题**
   - 修复了 lint 错误但仍报错

4. **依赖问题**
   - `node_modules` 损坏
   - 包版本冲突

#### 后端问题
1. **Go 编译问题**
   - 二进制文件无法运行
   - 构建缓存损坏

2. **数据库性能下降**
   - 查询变慢
   - 数据库文件过大

3. **日志文件堆积**
   - 磁盘空间不足

## 📊 清理前后对比

### 前端清理前后
```bash
# 清理前
$ du -sh frontend/.next
150M    frontend/.next

$ npm run clean
🧹 开始清理前端缓存...
✓ 已删除 .next 目录
✅ 前端清理完成！

# 清理后
$ du -sh frontend/.next
frontend/.next: No such file or directory
```

### 后端数据库优化前后
```bash
# 优化前
$ ls -lh backend/data/magicpodcast.db
-rw-r--r-- 1 user staff 788K Jan 6 08:21 magicpodcast.db

$ cd backend && ./clean.sh --vacuum
🧹 开始清理后端缓存...
执行数据库 VACUUM（回收空间）...
✓ 已优化 magicpodcast.db
✅ 后端清理完成！

# 优化后
$ ls -lh backend/data/magicpodcast.db
-rw-r--r-- 1 user staff 136K Jan 6 08:35 magicpodcast.db
```

## ⚠️ 注意事项

1. **清理会删除编译缓存**
   - 清理后首次启动会重新编译，可能稍慢
   - 这是正常现象，后续启动会恢复速度

2. **数据库优化会锁定数据库**
   - VACUUM 期间会锁定数据库
   - 建议在停止服务后执行

3. **深度清理**
   - `--deep` 会删除 `node_modules/.cache`
   - 可能导致某些包需要重新下载资源
   - 仅在必要时使用

4. **日志清理**
   - 后端清理脚本只删除 7 天前的日志备份
   - 当前的 `api.log` 不会被删除

## 🔄 定期清理建议

### 开发环境
- **每日**：无需清理
- **每周**：前端 `npm run clean`
- **每月**：`./clean-all.sh --vacuum`

### 遇到问题时
- 立即执行对应的清理脚本
- 如果问题仍存在，尝试深度清理 `--deep`

## 💡 常见问题

### Q: 清理后还是报错怎么办？
A: 尝试以下步骤：
```bash
# 1. 深度清理
./clean-all.sh --deep

# 2. 重新安装依赖（前端）
cd frontend
rm -rf node_modules package-lock.json
npm install

# 3. 重新构建
npm run build
```

### Q: 清理会删除我的代码吗？
A: 不会。清理脚本只删除：
- 构建缓存（.next, bin/）
- 临时文件（.cache, .db-wal）
- 旧的日志备份

### Q: 可以在开发服务器运行时清理吗？
A:
- **前端**：可以，但建议先停止 `npm run dev`
- **后端**：可以，但 `--vacuum` 模式需要停止服务
- **推荐**：先停止所有服务，清理后再启动

### Q: 清理脚本失败怎么办？
A: 检查文件权限：
```bash
chmod +x frontend/clean.sh backend/clean.sh clean-all.sh
```

## 📝 更新日志

- **2026-01-06**: 创建清理脚本
  - 前端清理：支持 `--deep` 模式
  - 后端清理：支持 `--vacuum` 模式
  - 统一清理：支持组合参数
