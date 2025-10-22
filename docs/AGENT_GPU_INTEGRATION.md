# Agent 仓库 GPU 监控集成提示词

## 背景

Komari 后端已经完成了 Nvidia GPU 性能监控的支持，现在需要在 Agent 仓库中实现 GPU 数据的采集和上报功能。

## 任务描述

为 Komari Agent 添加 Nvidia GPU 监控功能，使用 `nvidia-smi` 命令获取 GPU 信息，并通过现有的上报机制发送到后端。

## 功能需求

### 1. GPU 信息采集

Agent 需要采集以下 GPU 信息：
- GPU 型号名称
- 显存总量（字节）
- 显存使用量（字节）
- GPU 使用率（百分比）
- GPU 温度（摄氏度）
- GPU 功耗（瓦特）

### 2. 数据上报格式

需要在 Report 结构中添加 `gpu` 字段，格式如下：

```json
{
  "uuid": "client-uuid",
  "cpu": {...},
  "ram": {...},
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

### 3. nvidia-smi 命令使用

使用以下命令获取 GPU 信息：

```bash
nvidia-smi --query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw --format=csv,noheader,nounits
```

输出示例：
```
0, NVIDIA GeForce RTX 4090, 24564, 12282, 75, 65, 350.50
1, NVIDIA GeForce RTX 4090, 24564, 8192, 45, 60, 280.30
```

**重要说明**：
- 显存单位是 MiB，需要转换为字节：`bytes = mib * 1024 * 1024`
- 功耗单位是瓦特（W），无需转换
- 温度单位是摄氏度（°C），无需转换

## 实现要点

### 1. 错误处理

- **nvidia-smi 不存在**：系统没有安装 Nvidia 驱动或没有 GPU
  - 不上报 GPU 字段（或设为 null）
  - 不影响其他监控数据的采集和上报
  - 不输出错误日志（这是正常情况）

- **命令执行失败**：可能是驱动问题或权限问题
  - 记录日志但不阻塞
  - 可以尝试重试（设置合理的重试策略）
  - 继续上报其他监控数据

- **数据解析失败**：CSV 输出格式异常
  - 记录日志
  - 跳过本次 GPU 数据上报
  - 不影响下次采集

### 2. 性能优化

- **命令超时**：设置 2-3 秒的超时时间
- **采样频率**：与其他监控数据保持一致（建议 3-5 秒）
- **缓存策略**：可以在命令失败时使用上次成功的数据（但要标记时间戳）

### 3. 兼容性处理

- 优雅降级：在没有 GPU 的系统上正常运行
- 多 GPU 支持：正确处理多块 GPU 的情况
- 跨平台：考虑 Windows/Linux 的路径和命令差异

## 代码实现参考

### Go 语言实现示例

```go
package gpu

import (
    "encoding/csv"
    "os/exec"
    "strconv"
    "strings"
    "time"
)

type GPUInfo struct {
    Name        string  `json:"name"`
    MemoryTotal int64   `json:"memory_total"`
    MemoryUsed  int64   `json:"memory_used"`
    Utilization float64 `json:"utilization"`
    Temperature int     `json:"temperature"`
    PowerUsage  float64 `json:"power_usage"`
}

type GPUReport struct {
    Count        int       `json:"count"`
    AverageUsage float64   `json:"average_usage"`
    DetailedInfo []GPUInfo `json:"detailed_info"`
}

// GetGPUInfo 获取 GPU 信息
func GetGPUInfo() (*GPUReport, error) {
    // 创建命令，设置超时
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    
    cmd := exec.CommandContext(ctx,
        "nvidia-smi",
        "--query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw",
        "--format=csv,noheader,nounits")
    
    output, err := cmd.Output()
    if err != nil {
        // nvidia-smi 不存在或执行失败
        return nil, err
    }

    // 解析 CSV 输出
    reader := csv.NewReader(strings.NewReader(string(output)))
    records, err := reader.ReadAll()
    if err != nil {
        return nil, err
    }

    if len(records) == 0 {
        return nil, fmt.Errorf("no GPU found")
    }

    gpus := make([]GPUInfo, 0, len(records))
    var totalUtil float64

    for _, record := range records {
        if len(record) < 7 {
            continue // 跳过格式错误的行
        }

        // 解析各字段（注意错误处理）
        memTotal, _ := strconv.ParseInt(strings.TrimSpace(record[2]), 10, 64)
        memUsed, _ := strconv.ParseInt(strings.TrimSpace(record[3]), 10, 64)
        util, _ := strconv.ParseFloat(strings.TrimSpace(record[4]), 64)
        temp, _ := strconv.Atoi(strings.TrimSpace(record[5]))
        power, _ := strconv.ParseFloat(strings.TrimSpace(record[6]), 64)

        totalUtil += util

        gpus = append(gpus, GPUInfo{
            Name:        strings.TrimSpace(record[1]),
            MemoryTotal: memTotal * 1024 * 1024, // MiB 转 bytes
            MemoryUsed:  memUsed * 1024 * 1024,  // MiB 转 bytes
            Utilization: util,
            Temperature: temp,
            PowerUsage:  power,
        })
    }

    avgUtil := totalUtil / float64(len(gpus))

    return &GPUReport{
        Count:        len(gpus),
        AverageUsage: avgUtil,
        DetailedInfo: gpus,
    }, nil
}

// 在主上报循环中调用
func collectReport() Report {
    report := Report{
        UUID: getClientUUID(),
        CPU:  collectCPU(),
        RAM:  collectRAM(),
        // ... 其他数据
    }

    // 尝试采集 GPU 信息
    gpuReport, err := GetGPUInfo()
    if err == nil {
        report.GPU = gpuReport
    }
    // 如果失败，GPU 字段为 nil，不影响其他数据

    return report
}
```

### Python 语言实现示例

```python
import subprocess
import csv
from typing import Optional, Dict, List

def get_gpu_info() -> Optional[Dict]:
    """获取 GPU 信息"""
    try:
        # 执行 nvidia-smi 命令，设置超时
        result = subprocess.run(
            [
                "nvidia-smi",
                "--query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw",
                "--format=csv,noheader,nounits"
            ],
            capture_output=True,
            text=True,
            timeout=3,
            check=True
        )
        
        # 解析 CSV 输出
        reader = csv.reader(result.stdout.strip().split('\n'))
        gpus = []
        total_util = 0.0
        
        for row in reader:
            if len(row) < 7:
                continue  # 跳过格式错误的行
            
            try:
                mem_total = int(row[2].strip()) * 1024 * 1024  # MiB 转 bytes
                mem_used = int(row[3].strip()) * 1024 * 1024   # MiB 转 bytes
                utilization = float(row[4].strip())
                temperature = int(row[5].strip())
                power_usage = float(row[6].strip())
                
                total_util += utilization
                
                gpus.append({
                    "name": row[1].strip(),
                    "memory_total": mem_total,
                    "memory_used": mem_used,
                    "utilization": utilization,
                    "temperature": temperature,
                    "power_usage": power_usage
                })
            except (ValueError, IndexError) as e:
                # 解析某行数据失败，跳过
                continue
        
        if not gpus:
            return None
        
        avg_util = total_util / len(gpus)
        
        return {
            "count": len(gpus),
            "average_usage": avg_util,
            "detailed_info": gpus
        }
        
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError):
        # nvidia-smi 不存在或执行失败
        return None
    except Exception as e:
        # 其他异常
        print(f"Error collecting GPU info: {e}")
        return None

def collect_report():
    """采集所有监控数据"""
    report = {
        "uuid": get_client_uuid(),
        "cpu": collect_cpu(),
        "ram": collect_ram(),
        # ... 其他数据
    }
    
    # 尝试采集 GPU 信息
    gpu_info = get_gpu_info()
    if gpu_info is not None:
        report["gpu"] = gpu_info
    # 如果采集失败，不添加 gpu 字段
    
    return report
```

## 测试建议

### 1. 有 GPU 的环境
- 验证数据采集正确
- 验证数据格式符合要求
- 验证多 GPU 情况
- 验证数据能正确上报到后端

### 2. 无 GPU 的环境
- 验证不会报错
- 验证不影响其他数据采集
- 验证 agent 能正常启动和运行

### 3. 异常情况
- nvidia-smi 命令不存在
- nvidia-smi 命令执行超时
- CSV 输出格式异常
- 权限不足

## 验证方法

### 1. 本地验证

```bash
# 手动测试 nvidia-smi 命令
nvidia-smi --query-gpu=index,name,memory.total,memory.used,utilization.gpu,temperature.gpu,power.draw --format=csv,noheader,nounits

# 启动 agent 并查看日志
./agent

# 检查后端是否收到 GPU 数据
curl "http://backend:25774/api/records/gpu/latest?uuid=your-uuid"
```

### 2. API 验证

```bash
# 获取最新 GPU 信息
curl "http://localhost:25774/api/records/gpu/latest?uuid=client-uuid"

# 获取历史 GPU 数据
curl "http://localhost:25774/api/records/load?uuid=client-uuid&hours=4&load_type=all"
```

## 配置选项（可选）

建议在配置文件中添加以下选项：

```yaml
gpu:
  enabled: true              # 是否启用 GPU 监控
  interval: 5                # 采样间隔（秒）
  timeout: 3                 # nvidia-smi 超时时间（秒）
  retry_on_error: true       # 失败时是否重试
  max_retries: 3             # 最大重试次数
```

## 注意事项

1. **不要阻塞主上报流程**：GPU 数据采集失败不应影响其他数据的上报
2. **合理设置超时**：避免 nvidia-smi 命令hang住
3. **注意单位转换**：显存从 MiB 转换为 bytes
4. **考虑跨平台**：Windows 和 Linux 的命令路径可能不同
5. **日志记录**：采集失败时记录详细日志便于调试

## 相关文档

详细的后端 API 文档和集成指南请参考：
- 后端仓库：`docs/GPU_MONITORING.md`
- API 端点：`/api/records/gpu/latest` 和 `/api/records/load`

## 问题排查

### Q: nvidia-smi 命令找不到
A: 检查是否安装了 Nvidia 驱动，或者在 PATH 中添加 nvidia-smi 路径

### Q: 数据采集成功但后端收不到
A: 检查数据格式是否正确，特别是显存单位转换

### Q: GPU 功耗数据为 0
A: 某些 GPU 或驱动版本不支持功耗查询，这是正常的

### Q: 多 GPU 时数据混乱
A: 确保按照 device_index 正确排序和区分每个 GPU

## 示例提交信息

```
feat: Add Nvidia GPU monitoring support

- Implement GPU data collection using nvidia-smi
- Add GPU report structure with device info, memory, utilization, temperature, and power
- Handle errors gracefully when GPU is not available
- Support multi-GPU systems
- Add configuration options for GPU monitoring

Ref: komari backend GPU monitoring support
```
