package record

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/common"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/ws"
	"github.com/stretchr/testify/assert"
)

func TestGetLatestGPUInfo_NoData(t *testing.T) {
	// 设置测试环境
	gin.SetMode(gin.TestMode)
	
	// 创建测试路由
	router := gin.New()
	router.GET("/api/records/gpu/latest", GetLatestGPUInfo)
	
	// 创建测试请求
	req, _ := http.NewRequest("GET", "/api/records/gpu/latest?uuid=test-uuid", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	data := response["data"].(map[string]interface{})
	assert.Equal(t, "test-uuid", data["uuid"])
	assert.Equal(t, float64(0), data["gpu_count"])
}

func TestGetLatestGPUInfo_WithRealtimeData(t *testing.T) {
	// 设置测试环境
	gin.SetMode(gin.TestMode)
	
	// 模拟实时GPU数据
	testUUID := "test-uuid-with-gpu"
	testReport := &common.Report{
		UUID: testUUID,
		GPU: &common.GPUDetailReport{
			Count:        2,
			AverageUsage: 60.5,
			DetailedInfo: []common.GPUDeviceInfo{
				{
					Name:         "NVIDIA GeForce RTX 4090",
					MemoryTotal:  25769803776,
					MemoryUsed:   12884901888,
					Utilization:  75.5,
					Temperature:  65,
					PowerUsage:   350.5,
				},
				{
					Name:         "NVIDIA GeForce RTX 4090",
					MemoryTotal:  25769803776,
					MemoryUsed:   8589934592,
					Utilization:  45.5,
					Temperature:  60,
					PowerUsage:   280.3,
				},
			},
		},
		UpdatedAt: time.Now(),
	}
	
	// 设置最新报告
	ws.SetLatestReport(testUUID, testReport)
	defer func() {
		// 清理
		ws.SetLatestReport(testUUID, nil)
	}()
	
	// 创建测试路由
	router := gin.New()
	router.GET("/api/records/gpu/latest", GetLatestGPUInfo)
	
	// 创建测试请求
	req, _ := http.NewRequest("GET", "/api/records/gpu/latest?uuid="+testUUID, nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "success", response["status"])
	
	data := response["data"].(map[string]interface{})
	assert.Equal(t, testUUID, data["uuid"])
	assert.Equal(t, float64(2), data["gpu_count"])
	assert.Equal(t, "realtime", data["source"])
	
	// 验证GPU设备信息
	gpuDevices := data["gpu_devices"].([]interface{})
	assert.Len(t, gpuDevices, 2)
	
	gpu0 := gpuDevices[0].(map[string]interface{})
	assert.Equal(t, "NVIDIA GeForce RTX 4090", gpu0["device_name"])
	assert.Equal(t, float64(75.5), gpu0["utilization"])
	assert.Equal(t, float64(350.5), gpu0["power_usage"])
	assert.Equal(t, float64(65), gpu0["temperature"])
}

func TestGetLatestGPUInfo_MissingUUID(t *testing.T) {
	// 设置测试环境
	gin.SetMode(gin.TestMode)
	
	// 创建测试路由
	router := gin.New()
	router.GET("/api/records/gpu/latest", GetLatestGPUInfo)
	
	// 创建测试请求（缺少uuid参数）
	req, _ := http.NewRequest("GET", "/api/records/gpu/latest", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	// 验证响应
	assert.Equal(t, http.StatusBadRequest, w.Code)
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	
	assert.Equal(t, "error", response["status"])
	assert.Equal(t, "UUID is required", response["message"])
}

func TestGPURecordPowerUsage(t *testing.T) {
	// 测试GPURecord模型是否正确包含PowerUsage字段
	record := models.GPURecord{
		Client:      "test-client",
		Time:        models.FromTime(time.Now()),
		DeviceIndex: 0,
		DeviceName:  "NVIDIA GeForce RTX 4090",
		MemTotal:    25769803776,
		MemUsed:     12884901888,
		Utilization: 75.5,
		Temperature: 65,
		PowerUsage:  350.5,
	}
	
	// 验证字段赋值
	assert.Equal(t, float32(350.5), record.PowerUsage)
	assert.Equal(t, "NVIDIA GeForce RTX 4090", record.DeviceName)
	assert.Equal(t, float32(75.5), record.Utilization)
}

func TestGPUDeviceInfoPowerUsage(t *testing.T) {
	// 测试GPUDeviceInfo结构是否正确包含PowerUsage字段
	info := common.GPUDeviceInfo{
		Name:         "NVIDIA GeForce RTX 4090",
		MemoryTotal:  25769803776,
		MemoryUsed:   12884901888,
		Utilization:  75.5,
		Temperature:  65,
		PowerUsage:   350.5,
	}
	
	// 验证字段赋值
	assert.Equal(t, 350.5, info.PowerUsage)
	assert.Equal(t, "NVIDIA GeForce RTX 4090", info.Name)
	assert.Equal(t, 75.5, info.Utilization)
}
