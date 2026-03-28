// Package tools 工具加载器
// 工具不再自动注册，而是通过扫描 skills/*/scripts/ 目录动态加载
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	domain_model "aiProject/internal/domain/model"
	"aiProject/internal/domain/tool"
	"aiProject/internal/shared"
	"go.uber.org/zap"
)

const (
	// scriptExecTimeout 单次脚本执行最长超时时间
	scriptExecTimeout = 30 * time.Second
	// scriptMaxOutputBytes 脚本输出最大字节数（512KB），防止输出爆炸
	scriptMaxOutputBytes = 512 * 1024
)

// toolManifest scripts/ 目录下工具定义文件（tool.json）的结构
type toolManifest struct {
	Name        string                              `json:"name"`
	DisplayName string                              `json:"display_name"` // 展示给用户的中文名称（可选）
	Description string                              `json:"description"`
	Script      string                              `json:"script"`       // 脚本文件名，如 run.sh / run.py
	Parameters  domain_model.ToolParameters         `json:"parameters"`
}

// LoadToolsFromSkillsDir 扫描 skillsDir 下所有技能的 scripts/ 子目录，
// 读取 tool.json 并注册对应工具。
// 每个工具执行时会运行 scripts/ 目录下对应的脚本文件，并将参数以 JSON 形式通过 stdin 传入。
// baiduAK 为百度地图 API Key，用于天气查询和逆地理编码工具。
func LoadToolsFromSkillsDir(skillsDir string, baiduAK string) {
	logger := shared.GetLogger()

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		logger.Warn("读取 skills 目录失败，跳过工具加载", zap.String("dir", skillsDir), zap.Error(err))
		return
	}

	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		scriptsDir := filepath.Join(skillsDir, entry.Name(), "scripts")
		if _, err := os.Stat(scriptsDir); os.IsNotExist(err) {
			continue
		}
		n := loadToolsFromScriptsDir(scriptsDir, baiduAK, logger)
		loaded += n
	}

	logger.Info("工具加载完成", zap.Int("total", loaded))
}

// loadToolsFromScriptsDir 从单个 scripts/ 目录加载所有工具
func loadToolsFromScriptsDir(scriptsDir string, baiduAK string, logger *zap.Logger) int {
	manifestPath := filepath.Join(scriptsDir, "tool.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		logger.Warn("未找到 tool.json，跳过", zap.String("scripts_dir", scriptsDir))
		return 0
	}

	// 支持单个工具定义 {} 或多个工具定义 [{}]
	var manifests []toolManifest
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(data, &manifests); err != nil {
			logger.Warn("解析 tool.json 数组失败", zap.String("path", manifestPath), zap.Error(err))
			return 0
		}
	} else {
		var single toolManifest
		if err := json.Unmarshal(data, &single); err != nil {
			logger.Warn("解析 tool.json 失败", zap.String("path", manifestPath), zap.Error(err))
			return 0
		}
		manifests = []toolManifest{single}
	}

	count := 0
	for _, m := range manifests {
		if m.Name == "" {
			continue
		}
		registerScriptTool(m, scriptsDir, baiduAK, logger)
		count++
	}
	return count
}

// validateScriptPath 校验脚本路径是否在允许的 scriptsDir 白名单目录内，防止路径穿越攻击
func validateScriptPath(scriptsDir, scriptPath string) error {
	abs, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf("解析脚本路径失败: %v", err)
	}
	allowedBase, err := filepath.Abs(scriptsDir)
	if err != nil {
		return fmt.Errorf("解析白名单目录失败: %v", err)
	}
	if !strings.HasPrefix(abs, allowedBase+string(filepath.Separator)) {
		return fmt.Errorf("脚本路径 %q 不在允许的目录 %q 内，拒绝执行", abs, allowedBase)
	}
	return nil
}

// registerScriptTool 注册一个脚本驱动的工具
func registerScriptTool(m toolManifest, scriptsDir string, baiduAK string, logger *zap.Logger) {
	scriptPath := filepath.Join(scriptsDir, m.Script)

	// 根据内置工具名分发到对应的 Go 实现
	var execFunc tool.ExecuteFunc
	switch m.Name {
	case "list_directory":
		execFunc = executeListDirectory
	case "get_weather":
		execFunc = makeWeatherExecutor(baiduAK)
	case "get_public_ip":
		execFunc = makePublicIPExecutor(baiduAK)
	default:
		// 通用：执行脚本文件，参数通过 stdin 以 JSON 传入
		// 先做路径白名单校验，防止 tool.json 中配置了恶意路径
		if err := validateScriptPath(scriptsDir, scriptPath); err != nil {
			logger.Error("脚本路径校验失败，跳过注册", zap.String("name", m.Name), zap.Error(err))
			return
		}
		execFunc = makeScriptExecutor(scriptPath, scriptsDir)
	}

	tool.Register(&tool.Tool{
		Definition: domain_model.ToolDefinition{
			Name:        m.Name,
			DisplayName: m.DisplayName,
			Description: m.Description,
			Parameters:  m.Parameters,
		},
		Execute: execFunc,
	})
	logger.Info("工具已注册", zap.String("name", m.Name), zap.String("scripts_dir", scriptsDir))
}

// executeListDirectory list_directory 工具的 Go 原生实现：列出指定目录下的文件和子目录
func executeListDirectory(ctx context.Context, args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	// 安全限制：不允许访问绝对路径或包含 .. 的路径
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", fmt.Errorf("不允许访问绝对路径或上级目录，请使用相对路径")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("读取目录失败: %v", err)
	}

	type fileInfo struct {
		Name  string `json:"name"`
		Type  string `json:"type"`  // "file" 或 "dir"
		Size  int64  `json:"size,omitempty"`
	}

	var items []fileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		item := fileInfo{Name: entry.Name()}
		if entry.IsDir() {
			item.Type = "dir"
		} else {
			item.Type = "file"
			item.Size = info.Size()
		}
		items = append(items, item)
	}

	result, err := json.Marshal(map[string]interface{}{
		"path":  path,
		"count": len(items),
		"items": items,
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// ipAPIResponse ip-api.com 响应结构
type ipAPIResponse struct {
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Country     string  `json:"country"`
	RegionName  string  `json:"regionName"`
	City        string  `json:"city"`
	District    string  `json:"district"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Query       string  `json:"query"`
}

// baiduGeocodeResponse 百度地理编码 API 响应结构
type baiduGeocodeResponse struct {
	Status int `json:"status"`
	Result struct {
		AddressComponents struct {
			Country  string `json:"country"`
			Province string `json:"province"`
			City     string `json:"city"`
			District string `json:"district"`
		} `json:"addressComponent"`
		Adcode string `json:"adcode"` // 行政区划编码，即 district_id
	} `json:"result"`
}

// executeGetPublicIP get_public_ip 工具的 Go 原生实现：获取公网 IP 及归属地，并反查 district_id
func makePublicIPExecutor(baiduAK string) tool.ExecuteFunc {
	return func(ctx context.Context, _ map[string]interface{}) (string, error) {
		return executeGetPublicIPWithAK(ctx, baiduAK)
	}
}

func executeGetPublicIPWithAK(ctx context.Context, baiduAK string) (string, error) {
	// 1. 通过 ip-api.com 获取公网 IP 及归属地（lang=zh-CN 返回中文）
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://ip-api.com/json/?lang=zh-CN&fields=status,message,country,regionName,city,district,lat,lon,query", nil)
	if err != nil {
		return "", fmt.Errorf("构建 IP 查询请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询公网 IP 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 IP 响应失败: %v", err)
	}

	var ipInfo ipAPIResponse
	if err := json.Unmarshal(body, &ipInfo); err != nil {
		return "", fmt.Errorf("解析 IP JSON 失败: %v", err)
	}
	if ipInfo.Status != "success" {
		return "", fmt.Errorf("IP 查询失败: %s", ipInfo.Message)
	}

	// 2. 通过百度地图逆地理编码 API，用经纬度查询 district_id（adcode）
	if baiduAK == "" {
		// 未配置 AK 时，跳过逆地理编码
		result := map[string]interface{}{
			"ip":       ipInfo.Query,
			"country":  ipInfo.Country,
			"province": ipInfo.RegionName,
			"city":     ipInfo.City,
			"district": ipInfo.District,
			"lat":      ipInfo.Lat,
			"lon":      ipInfo.Lon,
		}
		out, err := json.Marshal(result)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	geoURL := fmt.Sprintf(
		"https://api.map.baidu.com/reverse_geocoding/v3/?ak=%s&output=json&coordtype=wgs84ll&location=%f,%f",
		baiduAK, ipInfo.Lat, ipInfo.Lon,
	)

	geoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, geoURL, nil)
	if err != nil {
		return "", fmt.Errorf("构建逆地理编码请求失败: %v", err)
	}
	geoResp, err := http.DefaultClient.Do(geoReq)
	if err != nil {
		return "", fmt.Errorf("逆地理编码请求失败: %v", err)
	}
	defer geoResp.Body.Close()

	geoBody, err := io.ReadAll(geoResp.Body)
	if err != nil {
		return "", fmt.Errorf("读取逆地理编码响应失败: %v", err)
	}

	var geoInfo baiduGeocodeResponse
	if err := json.Unmarshal(geoBody, &geoInfo); err != nil {
		return "", fmt.Errorf("解析逆地理编码 JSON 失败: %v", err)
	}

	districtID := ""
	if geoInfo.Status == 0 {
		districtID = geoInfo.Result.Adcode
	}

	// 3. 组装结果
	result := map[string]interface{}{
		"ip":          ipInfo.Query,
		"country":     ipInfo.Country,
		"province":    ipInfo.RegionName,
		"city":        ipInfo.City,
		"district":    ipInfo.District,
		"lat":         ipInfo.Lat,
		"lon":         ipInfo.Lon,
		"district_id": districtID,
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// baiduWeatherResponse 百度天气 API 响应结构
type baiduWeatherResponse struct {
	Status int `json:"status"`
	Result struct {
		Now struct {
			Text      string `json:"text"`
			Temp      int    `json:"temp"`
			WindClass string `json:"wind_class"`
			WindDir   string `json:"wind_dir"`
		} `json:"now"`
		Forecasts []struct {
			Date      string `json:"date"`
			Week      string `json:"week"`
			TextDay   string `json:"text_day"`
			TempHigh  int    `json:"high"`
			TempLow   int    `json:"low"`
			WindClass string `json:"wd_day"`
			WindDir   string `json:"wc_night"`
		} `json:"forecasts"`
	} `json:"result"`
}

// executeGetWeather get_weather 工具的 Go 原生实现：查询百度天气 API
func makeWeatherExecutor(baiduAK string) tool.ExecuteFunc {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		return executeGetWeatherWithAK(ctx, args, baiduAK)
	}
}

func executeGetWeatherWithAK(ctx context.Context, args map[string]interface{}, baiduAK string) (string, error) {
	districtID, _ := args["district_id"].(string)
	const defaultDistrictID = "610402" // 默认：和市秦都区
	if districtID == "" {
		districtID = defaultDistrictID
	}
	if baiduAK == "" {
		return "", fmt.Errorf("百度天气工具未配置 baidu_ak，请在 trpc_go.yaml 的 custom.tools.baidu_ak 中配置")
	}
	apiURL := fmt.Sprintf(
		"https://api.map.baidu.com/weather/v1/?district_id=%s&data_type=all&ak=%s",
		districtID, baiduAK,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("构建天气请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求百度天气 API 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取天气响应失败: %v", err)
	}

	var weather baiduWeatherResponse
	if err := json.Unmarshal(body, &weather); err != nil {
		return "", fmt.Errorf("解析天气 JSON 失败: %v", err)
	}
	if weather.Status != 0 {
		return "", fmt.Errorf("百度天气 API 返回异常状态: %d", weather.Status)
	}

	now := weather.Result.Now
	result := map[string]interface{}{
		"district_id": districtID,
		"current": map[string]interface{}{
			"text":       now.Text,
			"temp":       fmt.Sprintf("%d°C", now.Temp),
			"wind_class": now.WindClass,
			"wind_dir":   now.WindDir,
		},
	}

	if len(weather.Result.Forecasts) > 0 {
		f := weather.Result.Forecasts[0]
		weekMap := map[string]string{
			"1": "星期一", "2": "星期二", "3": "星期三",
			"4": "星期四", "5": "星期五", "6": "星期六", "7": "星期日",
		}
		weekStr := f.Week
		if v, ok := weekMap[f.Week]; ok {
			weekStr = v
		}
		result["forecast"] = map[string]interface{}{
			"date":       f.Date,
			"week":       weekStr,
			"text":       f.TextDay,
			"temp_high":  fmt.Sprintf("%d°C", f.TempHigh),
			"temp_low":   fmt.Sprintf("%d°C", f.TempLow),
			"wind_class": f.WindClass,
			"wind_dir":   f.WindDir,
		}
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// makeScriptExecutor 创建通用脚本执行器（参数通过 stdin 以 JSON 传入，stdout 为结果）
// 安全措施：
//  1. 强制 scriptExecTimeout 超时（覆盖上层 ctx）
//  2. 进程组隔离（Setpgid），超时后 kill 整个进程组
//  3. 输出大小限制（scriptMaxOutputBytes），防止输出爆炸
//  4. 脚本路径已在注册阶段通过 validateScriptPath 白名单校验
func makeScriptExecutor(scriptPath, scriptsDir string) tool.ExecuteFunc {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return "", fmt.Errorf("序列化参数失败: %v", err)
		}

		// 使用独立超时 ctx，防止上层 ctx 超时过长
		execCtx, cancel := context.WithTimeout(ctx, scriptExecTimeout)
		defer cancel()

		var cmd *exec.Cmd
		switch {
		case strings.HasSuffix(scriptPath, ".py"):
			cmd = exec.CommandContext(execCtx, "python3", scriptPath)
		case strings.HasSuffix(scriptPath, ".sh"):
			cmd = exec.CommandContext(execCtx, "bash", scriptPath)
		case strings.HasSuffix(scriptPath, ".js"):
			cmd = exec.CommandContext(execCtx, "node", scriptPath)
		default:
			cmd = exec.CommandContext(execCtx, scriptPath)
		}

		// 进程组隔离：超时时 kill 整个子进程树，防止僵尸进程
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Stdin = strings.NewReader(string(argsJSON))

		// 限制输出大小，防止脚本输出过大耗尽内存
		var stdoutBuf bytes.Buffer
		limitedWriter := &limitedWriter{w: &stdoutBuf, limit: scriptMaxOutputBytes}
		cmd.Stdout = limitedWriter

		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Run(); err != nil {
			if execCtx.Err() == context.DeadlineExceeded {
				// 超时后 kill 整个进程组
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				return "", fmt.Errorf("脚本执行超时（超过 %s）", scriptExecTimeout)
			}
			// 优先从 stdout 提取脚本自身输出的 JSON 错误（如安全检查拒绝等）
			stdoutStr := strings.TrimSpace(stdoutBuf.String())
			if stdoutStr != "" {
				var errObj map[string]interface{}
				if json.Unmarshal([]byte(stdoutStr), &errObj) == nil {
					if errMsg, ok := errObj["error"].(string); ok && errMsg != "" {
						return "", fmt.Errorf("%s", errMsg)
					}
				}
				// stdout 有内容但不是 JSON 错误格式，也返回
				return "", fmt.Errorf("脚本执行失败: %v\nstdout: %s", err, stdoutStr)
			}
			stderrStr := strings.TrimSpace(stderrBuf.String())
			if stderrStr != "" {
				return "", fmt.Errorf("脚本执行失败: %v\nstderr: %s", err, stderrStr)
			}
			return "", fmt.Errorf("脚本执行失败: %v", err)
		}
		if limitedWriter.exceeded {
			return "", fmt.Errorf("脚本输出超过最大限制 %d 字节", scriptMaxOutputBytes)
		}
		return strings.TrimSpace(stdoutBuf.String()), nil
	}
}

// limitedWriter 限制最大写入字节数的 io.Writer
type limitedWriter struct {
	w        io.Writer
	limit    int
	written  int
	exceeded bool
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.exceeded {
		return 0, fmt.Errorf("输出已超过限制")
	}
	remaining := lw.limit - lw.written
	if len(p) > remaining {
		lw.exceeded = true
		p = p[:remaining]
	}
	n, err := lw.w.Write(p)
	lw.written += n
	return n, err
}