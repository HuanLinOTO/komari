# Nvidia GPU 监控集成指南

## 概述

本文档描述了 Komari 后端的 GPU 监控功能，以及 Agent 如何上报 GPU 数据。

## 后端 GPU 监控功能

### 数据模型

#### GPURecord (数据库模型)
```go
type GPURecord struct {
    Client      string    // 客户端UUID
    Time        LocalTime // 记录时间
    DeviceIndex int       // GPU设备索引 (0,1,2...)
    DeviceName  string    // GPU型号
    MemTotal    int64     // 显存总量(字节)
    MemUsed     int64     // 显存使用(字节)
    Utilization float32   // GPU使用率(%)
    Temperature int       // GPU温度(°C)
    PowerUsage  float32   // GPU功耗(瓦特W)
}
```

#### GPUDetailReport (Agent上报格式)
```go
type GPUDetailReport struct {
    Count          int             // GPU数量
    AverageUsage   float64         // 平均使用率
    DetailedInfo   []GPUDeviceInfo // 每个GPU的详细信息
}

type GPUDeviceInfo struct {
    Name         string  // GPU型号名称
    MemoryTotal  int64   // 显存总量(字节)
    MemoryUsed   int64   // 显存已用(字节)
    Utilization  float64 // GPU使用率(%)
    Temperature  int     // GPU温度(°C)
    PowerUsage   float64 // GPU功耗(瓦特W)
}
```

### API 端点

#### 1. 获取最新GPU信息
**端点**: `GET /api/records/gpu/latest`

**查询参数**:
- `uuid` (required): 客户端UUID

**响应示例**:
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
            },
            {
                "device_index": 1,
                "device_name": "NVIDIA GeForce RTX 4090",
                "memory_total": 25769803776,
                "memory_used": 8589934592,
                "memory_percent": 33.3,
                "utilization": 45.2,
                "temperature": 60,
                "power_usage": 280.3,
                "timestamp": "2025-10-22T15:30:00Z"
            }
        ],
        "source": "realtime"
    }
}
```

**说明**:
- `source` 可能是 "realtime" (来自实时WebSocket数据) 或 "database" (来自数据库最近记录)
- 如果客户端没有GPU或没有数据，`gpu_count` 将为 0

#### 2. 获取历史GPU数据
**端点**: `GET /api/records/load`

**查询参数**:
- `uuid` (required): 客户端UUID
- `hours` (optional): 获取多少小时的数据，默认4小时
- `load_type` (optional): 设置为 "gpu" 或 "all" 来获取GPU数据

**响应示例**:
```json
{
    "status": "success",
    "data": {
        "records": [...],
        "count": 240,
        "has_gpu_data": true,
        "gpu_devices": {
            "0": {
                "device_index": 0,
                "device_name": "NVIDIA GeForce RTX 4090",
                "records": [
                    {
                        "client": "client-uuid",
                        "time": "2025-10-22T15:30:00Z",
                        "device_index": 0,
                        "device_name": "NVIDIA GeForce RTX 4090",
                        "mem_total": 25769803776,
                        "mem_used": 12884901888,
                        "utilization": 75.5,
                        "temperature": 65,
                        "power_usage": 350.5
                    }
                ]
            }
        }
    }
}
```

## Agent 集成指南

### 获取 GPU 信息的方法

#### 使用 nvidia-smi 命令

Agent 应该使用以下 `nvidia-smi` 命令获取 GPU 信息：

```bash
nvidia-smi --query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw --format=csv,noheader,nounits
```

**输出示例**:
```
0, NVIDIA GeForce RTX 4090, 24564, 12282, 75, 65, 350.50
1, NVIDIA GeForce RTX 4090, 24564, 8192, 45, 60, 280.30
```

**字段说明**:
- `index`: GPU设备索引
- `name`: GPU型号名称
- `memory.total`: 显存总量 (MiB)
- `memory.used`: 显存使用 (MiB)
- `utilization.gpu`: GPU使用率 (%)
- `temperature.gpu`: GPU温度 (°C)
- `power.draw`: 功耗 (W)

### Agent 上报数据格式

Agent 应该在上报的 `Report` 结构中包含 `gpu` 字段：

```json
{
    "uuid": "client-uuid",
    "cpu": {...},
    "ram": {...},
    "gpu": {
        "count": 2,
        "average_usage": 60.35,
        "detailed_info": [
            {
                "name": "NVIDIA GeForce RTX 4090",
                "memory_total": 25769803776,
                "memory_used": 12884901888,
                "utilization": 75.5,
                "temperature": 65,
                "power_usage": 350.5
            },
            {
                "name": "NVIDIA GeForce RTX 4090",
                "memory_total": 25769803776,
                "memory_used": 8589934592,
                "utilization": 45.2,
                "temperature": 60,
                "power_usage": 280.3
            }
        ]
    }
}
```

**重要说明**:
1. 显存单位需要从 MiB 转换为字节：`memory_bytes = memory_mib * 1024 * 1024`
2. `count` 应该等于 `detailed_info` 数组的长度
3. `average_usage` 应该是所有GPU utilization的平均值
4. 如果系统没有GPU或获取失败，可以不包含 `gpu` 字段或设置为 `null`

### Agent 实现伪代码示例

#### Go 语言示例

```go
package main

import (
    "encoding/csv"
    "os/exec"
    "strconv"
    "strings"
)

type GPUInfo struct {
    Name        string
    MemoryTotal int64
    MemoryUsed  int64
    Utilization float64
    Temperature int
    PowerUsage  float64
}

func GetGPUInfo() ([]GPUInfo, error) {
    // 执行 nvidia-smi 命令
    cmd := exec.Command("nvidia-smi",
        "--query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw",
        "--format=csv,noheader,nounits")
    
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    // 解析CSV输出
    reader := csv.NewReader(strings.NewReader(string(output)))
    records, err := reader.ReadAll()
    if err != nil {
        return nil, err
    }

    gpus := make([]GPUInfo, 0, len(records))
    for _, record := range records {
        if len(record) < 7 {
            continue
        }

        memTotal, _ := strconv.ParseInt(strings.TrimSpace(record[2]), 10, 64)
        memUsed, _ := strconv.ParseInt(strings.TrimSpace(record[3]), 10, 64)
        util, _ := strconv.ParseFloat(strings.TrimSpace(record[4]), 64)
        temp, _ := strconv.Atoi(strings.TrimSpace(record[5]))
        power, _ := strconv.ParseFloat(strings.TrimSpace(record[6]), 64)

        gpus = append(gpus, GPUInfo{
            Name:        strings.TrimSpace(record[1]),
            MemoryTotal: memTotal * 1024 * 1024, // 转换为字节
            MemoryUsed:  memUsed * 1024 * 1024,  // 转换为字节
            Utilization: util,
            Temperature: temp,
            PowerUsage:  power,
        })
    }

    return gpus, nil
}

func BuildGPUReport(gpus []GPUInfo) map[string]interface{} {
    if len(gpus) == 0 {
        return nil
    }

    // 计算平均使用率
    var totalUtil float64
    for _, gpu := range gpus {
        totalUtil += gpu.Utilization
    }
    avgUtil := totalUtil / float64(len(gpus))

    // 构建详细信息
    detailedInfo := make([]map[string]interface{}, len(gpus))
    for i, gpu := range gpus {
        detailedInfo[i] = map[string]interface{}{
            "name":         gpu.Name,
            "memory_total": gpu.MemoryTotal,
            "memory_used":  gpu.MemoryUsed,
            "utilization":  gpu.Utilization,
            "temperature":  gpu.Temperature,
            "power_usage":  gpu.PowerUsage,
        }
    }

    return map[string]interface{}{
        "count":          len(gpus),
        "average_usage":  avgUtil,
        "detailed_info":  detailedInfo,
    }
}
```

#### Python 语言示例

```python
import subprocess
import csv
from typing import List, Dict, Optional

class GPUInfo:
    def __init__(self, name: str, memory_total: int, memory_used: int,
                 utilization: float, temperature: int, power_usage: float):
        self.name = name
        self.memory_total = memory_total
        self.memory_used = memory_used
        self.utilization = utilization
        self.temperature = temperature
        self.power_usage = power_usage

def get_gpu_info() -> Optional[List[GPUInfo]]:
    """获取GPU信息"""
    try:
        # 执行 nvidia-smi 命令
        cmd = [
            "nvidia-smi",
            "--query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw",
            "--format=csv,noheader,nounits"
        ]
        
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        
        # 解析CSV输出
        reader = csv.reader(result.stdout.strip().split('\n'))
        gpus = []
        
        for row in reader:
            if len(row) < 7:
                continue
            
            gpus.append(GPUInfo(
                name=row[1].strip(),
                memory_total=int(row[2].strip()) * 1024 * 1024,  # MiB转字节
                memory_used=int(row[3].strip()) * 1024 * 1024,   # MiB转字节
                utilization=float(row[4].strip()),
                temperature=int(row[5].strip()),
                power_usage=float(row[6].strip())
            ))
        
        return gpus if gpus else None
        
    except (subprocess.CalledProcessError, FileNotFoundError):
        # nvidia-smi不存在或执行失败
        return None

def build_gpu_report(gpus: Optional[List[GPUInfo]]) -> Optional[Dict]:
    """构建GPU上报数据"""
    if not gpus:
        return None
    
    # 计算平均使用率
    avg_util = sum(gpu.utilization for gpu in gpus) / len(gpus)
    
    # 构建详细信息
    detailed_info = [
        {
            "name": gpu.name,
            "memory_total": gpu.memory_total,
            "memory_used": gpu.memory_used,
            "utilization": gpu.utilization,
            "temperature": gpu.temperature,
            "power_usage": gpu.power_usage
        }
        for gpu in gpus
    ]
    
    return {
        "count": len(gpus),
        "average_usage": avg_util,
        "detailed_info": detailed_info
    }

# 使用示例
gpus = get_gpu_info()
if gpus:
    gpu_report = build_gpu_report(gpus)
    # 将 gpu_report 添加到上报的 Report 结构中
    report["gpu"] = gpu_report
```

### 错误处理

1. **nvidia-smi 不存在**: Agent 应该优雅地处理这种情况，不上报 GPU 数据
2. **命令执行失败**: 记录日志但不影响其他数据的上报
3. **数据解析失败**: 可以尝试重试或跳过本次GPU数据上报

### 性能考虑

1. **采样频率**: 建议与其他监控数据保持一致的采样频率（如3-5秒一次）
2. **命令超时**: 设置合理的超时时间（如2-3秒）避免阻塞
3. **缓存策略**: 如果 nvidia-smi 调用失败，可以使用上一次的成功数据（标记为过期）

## 数据存储和压缩

后端会自动处理GPU数据的存储和压缩：

1. **实时数据**: 最近4小时的原始数据存储在 `gpu_records` 表
2. **长期数据**: 超过4小时的数据会被压缩（取15分钟窗口的70百分位数）存储在 `gpu_records_long_term` 表
3. **数据保留**: 根据系统配置的 `RecordPreserveTime` 自动清理旧数据

## 前端集成建议

前端可以通过以下方式展示GPU信息：

1. **实时监控**: 使用 `/api/records/gpu/latest?uuid=xxx` 获取最新GPU状态
2. **历史趋势**: 使用 `/api/records/load?uuid=xxx&hours=24&load_type=all` 获取历史数据并绘制图表
3. **多GPU展示**: 根据 `device_index` 区分不同GPU，支持多GPU系统
4. **告警阈值**: 可以基于温度、利用率、显存使用率设置告警

### 推荐的显示指标

- **GPU型号**: `device_name`
- **显存使用**: `memory_used / memory_total` (显示为百分比或 GB/GB 格式)
- **GPU使用率**: `utilization` (百分比)
- **温度**: `temperature` (°C)
- **功耗**: `power_usage` (W)

## 兼容性说明

- 本功能仅支持 Nvidia GPU
- 需要系统安装 nvidia-smi 工具
- 支持多GPU系统（通过 device_index 区分）
- 向后兼容：如果Agent不上报GPU数据，不会影响其他功能

## 测试建议

### Agent端测试
1. 在有GPU的机器上测试数据采集
2. 在无GPU的机器上测试优雅降级
3. 测试多GPU场景
4. 测试nvidia-smi命令失败的情况

### API测试
```bash
# 获取最新GPU信息
curl "http://localhost:25774/api/records/gpu/latest?uuid=your-uuid"

# 获取历史GPU数据
curl "http://localhost:25774/api/records/load?uuid=your-uuid&hours=4&load_type=all"
```

## 常见问题

**Q: 如果服务器没有GPU会怎样？**
A: Agent 不会上报GPU数据，后端API会返回 `gpu_count: 0`，不影响其他功能。

**Q: 功耗数据不准确怎么办？**
A: nvidia-smi 的功耗数据来自GPU硬件传感器，如果不准确可能是驱动问题。可以考虑将其设为可选字段。

**Q: 支持AMD GPU吗？**
A: 当前版本仅支持Nvidia GPU。AMD GPU需要使用不同的工具（如 rocm-smi）。

**Q: 数据库迁移会自动进行吗？**
A: 是的，GORM会自动添加新的 `power_usage` 字段到现有的 `gpu_records` 表。
