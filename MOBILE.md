# MagicPodcast 移动端开发指南

## 📱 iPhone/iPad 访问开发服务器

### 快速开始

1. **确保设备和Mac在同一WiFi网络**

2. **在iPhone Safari中访问：**
   ```
   http://192.168.3.58:3000
   ```

3. **如果遇到Network Error：**
   - 重启前端服务：`cd frontend && npm run dev`
   - 确认环境变量配置正确（见下方）

---

## 🔧 环境变量配置

### 开发环境（本地 + 移动设备）

文件：`frontend/.env.local`

```bash
# 局域网访问，支持iPhone/iPad
NEXT_PUBLIC_API_URL=http://192.168.3.58:8080
```

**注意**：
- ❌ 不要使用 `localhost:8080`（移动设备会指向自己）
- ✅ 使用局域网IP `192.168.3.58:8080`
- ✅ 本机和移动设备都可以访问

### 生产环境（部署后）

```bash
# 使用相对路径或环境域名
NEXT_PUBLIC_API_URL=/api/v1
# 或
NEXT_PUBLIC_API_URL=https://api.yourdomain.com
```

---

## 🛠️ 网络诊断工具

### 运行诊断脚本

```bash
cd frontend
./test-mobile.sh
```

该脚本会检查：
- ✅ 后端API健康状态
- ✅ 前端服务状态
- ✅ 端口监听状态
- ✅ 环境变量配置

### 手动测试API

```bash
# 测试后端健康检查
curl http://192.168.3.58:8080/health

# 预期响应
# {"database":"ok","service":"magicpodcast-backend","status":"ok"}
```

---

## 🐛 常见问题排查

### 1. "Network Error" 在播客列表页

**原因**：iPhone无法访问localhost

**解决**：
```bash
# 1. 编辑环境变量
vim frontend/.env.local

# 2. 修改为
NEXT_PUBLIC_API_URL=http://192.168.3.58:8080

# 3. 重启前端
cd frontend && npm run dev
```

### 2. 防火墙阻止连接

**检查**：系统偏好设置 → 安全性与隐私 → 防火墙

**临时解决**：关闭防火墙测试

**永久解决**：允许传入连接
```bash
# macOS允许特定端口
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add /usr/local/bin/node
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp "Node"
```

### 3. VPN干扰

**症状**：连接不稳定或无法访问

**解决**：暂时关闭VPN测试

### 4. IP地址变化

**原因**：路由器DHCP分配了新IP

**解决**：
```bash
# 查看当前IP
ifconfig | grep "inet " | grep -v 127.0.0.1

# 更新.env.local中的IP地址
# 重启前端服务
```

**永久解决**：在路由器设置中为Mac保留静态IP

---

## 📲 iPhone Safari调试

### 启用Web检查器

1. iPhone：设置 → Safari → 高级 → Web检查器 ✅
2. Mac：Safari → 偏好设置 → 高级 → "在菜单栏中显示开发菜单" ✅
3. USB连接iPhone到Mac
4. Mac Safari：开发 → [你的iPhone] → 选择页面

### 查看Console

1. 在iPhone Safari中打开网页
2. Mac Safari → 开发 → [你的iPhone] → [当前页面]
3. 点击"Console"标签查看错误

---

## 📊 移动端测试清单

### 布局测试
- [ ] 播客列表在iPhone上显示为单列
- [ ] 卡片内容清晰可读，无横向滚动
- [ ] 图片加载正常
- [ ] 标签筛选可滚动

### 交互测试
- [ ] 所有按钮触摸目标≥44px
- [ ] 点击有视觉反馈
- [ ] 模态框正常打开/关闭
- [ ] 表单输入正常工作

### 功能测试
- [ ] 播客列表加载
- [ ] 标签筛选
- [ ] 搜索功能
- [ ] 工作流管理
- [ ] 同步功能

---

## 🌐 局域网访问总结

| 组件 | 本机访问 | 移动设备访问 |
|------|---------|-------------|
| 前端 | http://localhost:3000 | http://192.168.3.58:3000 |
| 后端 | http://localhost:8080 | http://192.168.3.58:8080 |
| API_URL | localhost或192.168.3.58 | **必须是** 192.168.3.58 |

---

## 🔗 相关文件

- `.env.local` - 前端环境变量配置
- `test-mobile.sh` - 网络诊断脚本
- `backend/configs/config.yaml` - 后端CORS配置
- `CLAUDE.md` - 项目文档

---

## 📞 需要帮助？

如果遇到问题：
1. 运行 `./test-mobile.sh` 诊断
2. 检查Console错误信息
3. 确认iPhone和Mac在同一WiFi
4. 尝试关闭VPN和防火墙

---

**最后更新**：2025-02-10
**版本**：v1.0
