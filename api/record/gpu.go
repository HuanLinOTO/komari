package record

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/ws"
)

// GetLatestGPUInfo 获取客户端最新的GPU信息
// GET /api/records/gpu/latest?uuid=xxx
func GetLatestGPUInfo(c *gin.Context) {
	uuid := c.Query("uuid")
	if uuid == "" {
		api.RespondError(c, 400, "UUID is required")
		return
	}

	// 登录状态检查
	isLogin := false
	session, _ := c.Cookie("session_token")
	_, err := accounts.GetUserBySession(session)
	if err == nil {
		isLogin = true
	}

	// 仅在未登录时需要 Hidden 信息做过滤
	if !isLogin {
		var hiddenClients []models.Client
		db := dbcore.GetDBInstance()
		_ = db.Select("uuid").Where("hidden = ?", true).Find(&hiddenClients).Error
		hiddenMap := make(map[string]bool)
		for _, cli := range hiddenClients {
			hiddenMap[cli.UUID] = true
		}

		if hiddenMap[uuid] {
			api.RespondError(c, 403, "Access denied")
			return
		}
	}

	// 首先尝试从最新的上报数据中获取GPU信息（实时数据）
	allReports := ws.GetLatestReport()
	latestReport, exists := allReports[uuid]
	if exists && latestReport != nil && latestReport.GPU != nil && len(latestReport.GPU.DetailedInfo) > 0 {
		// 构建返回数据结构
		gpuDevices := make([]gin.H, 0, len(latestReport.GPU.DetailedInfo))
		for idx, gpu := range latestReport.GPU.DetailedInfo {
			gpuDevices = append(gpuDevices, gin.H{
				"device_index":   idx,
				"device_name":    gpu.Name,
				"memory_total":   gpu.MemoryTotal,
				"memory_used":    gpu.MemoryUsed,
				"memory_percent": float64(gpu.MemoryUsed) / float64(gpu.MemoryTotal) * 100,
				"utilization":    gpu.Utilization,
				"temperature":    gpu.Temperature,
				"power_usage":    gpu.PowerUsage,
				"timestamp":      latestReport.UpdatedAt,
			})
		}

		api.RespondSuccess(c, gin.H{
			"uuid":        uuid,
			"gpu_count":   len(gpuDevices),
			"gpu_devices": gpuDevices,
			"source":      "realtime",
		})
		return
	}

	// 如果没有实时数据，从数据库获取最近的GPU记录
	db := dbcore.GetDBInstance()
	var gpuRecords []models.GPURecord
	
	// 获取最近5分钟的GPU记录
	err = db.Where("client = ? AND time >= ?", uuid, time.Now().Add(-5*time.Minute)).
		Order("time DESC, device_index ASC").
		Find(&gpuRecords).Error

	if err != nil {
		api.RespondError(c, 500, "Failed to fetch GPU records")
		return
	}

	if len(gpuRecords) == 0 {
		api.RespondSuccess(c, gin.H{
			"uuid":      uuid,
			"gpu_count": 0,
			"message":   "No GPU data available",
		})
		return
	}

	// 按device_index分组，取每个设备最新的记录
	deviceMap := make(map[int]models.GPURecord)
	for _, record := range gpuRecords {
		if existing, exists := deviceMap[record.DeviceIndex]; !exists || record.Time.ToTime().After(existing.Time.ToTime()) {
			deviceMap[record.DeviceIndex] = record
		}
	}

	// 构建返回数据
	gpuDevices := make([]gin.H, 0, len(deviceMap))
	var latestTime time.Time
	for _, record := range deviceMap {
		if record.Time.ToTime().After(latestTime) {
			latestTime = record.Time.ToTime()
		}
		
		memPercent := float64(0)
		if record.MemTotal > 0 {
			memPercent = float64(record.MemUsed) / float64(record.MemTotal) * 100
		}
		
		gpuDevices = append(gpuDevices, gin.H{
			"device_index":   record.DeviceIndex,
			"device_name":    record.DeviceName,
			"memory_total":   record.MemTotal,
			"memory_used":    record.MemUsed,
			"memory_percent": memPercent,
			"utilization":    record.Utilization,
			"temperature":    record.Temperature,
			"power_usage":    record.PowerUsage,
			"timestamp":      record.Time.ToTime(),
		})
	}

	api.RespondSuccess(c, gin.H{
		"uuid":        uuid,
		"gpu_count":   len(gpuDevices),
		"gpu_devices": gpuDevices,
		"source":      "database",
		"timestamp":   latestTime,
	})
}
