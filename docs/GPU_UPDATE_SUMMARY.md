# GPU 监控功能更新说明

## 概述

Komari 后端已完成 Nvidia GPU 性能监控支持。现在可以监控 GPU 型号、显存、使用率、温度和功耗信息。

## ✅ 后端已完成的功能

### 1. 数据模型扩展
- ✅ `GPURecord` 模型增加了 `PowerUsage` 字段（GPU功耗，单位：瓦特）
- ✅ `GPUDeviceInfo` 结构增加了 `PowerUsage` 字段
- ✅ 数据库自动迁移支持新字段

### 2. 数据处理增强
- ✅ GPU 数据聚合处理（支持功耗数据）
- ✅ GPU 数据压缩存储（长期数据保存）
- ✅ 多 GPU 系统支持（通过 device_index 区分）

### 3. 新增 API 端点
- ✅ `/api/records/gpu/latest` - 获取最新 GPU 信息
- ✅ `/api/records/load` - 支持 GPU 历史数据查询（设置 load_type=all 或 gpu）

### 4. 完整文档
- ✅ `docs/GPU_MONITORING.md` - API 文档和集成指南
- ✅ `docs/AGENT_GPU_INTEGRATION.md` - Agent 开发指南

### 5. 测试覆盖
- ✅ 数据模型测试
- ✅ API 端点测试
- ✅ 实时数据和数据库数据测试

## 📋 待 Agent 仓库实现

Agent 需要实现 GPU 数据采集和上报功能：

### 使用 nvidia-smi 命令采集数据

```bash
nvidia-smi --query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw --format=csv,noheader,nounits
```

### 上报数据格式

```json
{
  "gpu": {
    "count": 2,
    "average_usage": 60.5,
    "detailed_info": [
      {
        "name": "NVIDIA GeForce RTX 4090",
        "memory_total": 25769803776,
        "memory_used": 12884901888,
        "utilization": 75.5,
        "temperature": 65,
        "power_usage": 350.5
      }
    ]
  }
}
```

**重要**：显存单位需要从 MiB 转换为字节（* 1024 * 1024）

## 🔧 API 使用示例

### 获取最新 GPU 信息
```bash
curl "http://localhost:25774/api/records/gpu/latest?uuid=client-uuid"
```

响应示例：
```json
{
  "status": "success",
  "data": {
    "uuid": "client-uuid",
    "gpu_count": 2,
    "gpu_devices": [
      {
        "device_index": 0,
        "device_name": "NVIDIA GeForce RTX 4090",
        "memory_total": 25769803776,
        "memory_used": 12884901888,
        "memory_percent": 50.0,
        "utilization": 75.5,
        "temperature": 65,
        "power_usage": 350.5,
        "timestamp": "2025-10-22T15:30:00Z"
      }
    ],
    "source": "realtime"
  }
}
```

### 获取历史 GPU 数据
```bash
curl "http://localhost:25774/api/records/load?uuid=client-uuid&hours=4&load_type=all"
```

## 📝 Agent 集成步骤

1. **实现 GPU 数据采集**
   - 执行 nvidia-smi 命令
   - 解析 CSV 输出
   - 转换单位（MiB -> bytes）

2. **错误处理**
   - nvidia-smi 不存在时优雅降级
   - 命令执行失败不阻塞其他数据上报
   - 设置合理的超时时间（2-3秒）

3. **集成到上报流程**
   - 在现有 Report 结构中添加 GPU 字段
   - 如果 GPU 采集失败，不添加该字段
   - 保持与其他监控数据相同的采样频率

## 📚 详细文档

- **Agent 开发指南**：`docs/AGENT_GPU_INTEGRATION.md`
  - 包含完整的 Go 和 Python 代码示例
  - 错误处理最佳实践
  - 测试和验证方法

- **API 文档**：`docs/GPU_MONITORING.md`
  - API 端点详细说明
  - 数据格式规范
  - 前端集成建议

## 🎯 前端展示建议

前端可以展示以下 GPU 信息：

1. **基础信息**
   - GPU 型号（device_name）
   - GPU 数量（gpu_count）

2. **实时状态**
   - 显存使用：显示为百分比或 "已用/总量 GB"
   - GPU 使用率：百分比，可用进度条展示
   - 温度：摄氏度，可设置告警阈值（如 >80°C 显示警告）
   - 功耗：瓦特，可显示为 "当前/额定 W"

3. **历史趋势**
   - 使用 `/api/records/load` 获取历史数据
   - 绘制使用率、温度、功耗的时间序列图表
   - 支持按设备索引查看多 GPU 系统

4. **告警建议**
   - 温度过高（>85°C）
   - 显存使用率过高（>95%）
   - GPU 使用率异常（长时间100%或0%）

## 🔍 兼容性说明

- ✅ 仅支持 Nvidia GPU
- ✅ 需要系统安装 nvidia-smi 工具
- ✅ 支持多 GPU 系统
- ✅ 向后兼容：无 GPU 系统不受影响
- ✅ 优雅降级：Agent 不上报 GPU 数据时，后端正常运行

## 🚀 部署说明

### 后端
- 无需特殊配置，更新代码后重启即可
- 数据库会自动添加新的 power_usage 字段
- 现有数据不受影响

### Agent
- 需要实现 GPU 数据采集功能
- 在有 GPU 的机器上测试
- 在无 GPU 的机器上验证不会报错

## 📊 数据保留策略

- **实时数据**：最近 4 小时的原始 GPU 数据
- **长期数据**：超过 4 小时的数据按 15 分钟压缩保存
- **数据清理**：根据系统配置的 RecordPreserveTime 自动清理

## ⚠️ 注意事项

1. **nvidia-smi 权限**：确保 Agent 有权限执行 nvidia-smi 命令
2. **功耗数据**：某些 GPU 或驱动版本可能不支持功耗查询，这是正常的
3. **显存单位**：nvidia-smi 输出的显存单位是 MiB，需要转换为字节
4. **采样频率**：建议与其他监控数据保持一致（3-5秒一次）

## 🤝 下一步行动

### 对于 Agent 开发者
1. 阅读 `docs/AGENT_GPU_INTEGRATION.md`
2. 实现 GPU 数据采集功能
3. 进行测试验证
4. 提交 PR

### 对于前端开发者
1. 阅读 `docs/GPU_MONITORING.md`
2. 设计 GPU 监控界面
3. 集成新的 API 端点
4. 测试多 GPU 场景

## 📮 问题反馈

如有问题或建议，请在对应的 GitHub Issues 中反馈。
