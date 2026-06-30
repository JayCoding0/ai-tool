// Package tools 工具加载器
// builtin_tools_executors.go — 内置工具执行器实现（从 builtin_tools.go 拆分）
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

	"aiProject/internal/domain/tool"
)

// ─── 目录列表工具 ──────────────────────────────────────────────────────────────

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
		Name string `json:"name"`
		Type string `json:"type"` // "file" 或 "dir"
		Size int64  `json:"size,omitempty"`
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

// ─── 公网 IP 查询工具 ──────────────────────────────────────────────────────────

// ipAPIResponse ip-api.com 响应结构
type ipAPIResponse struct {
	Status     string  `json:"status"`
	Message    string  `json:"message"`
	Country    string  `json:"country"`
	RegionName string  `json:"regionName"`
	City       string  `json:"city"`
	District   string  `json:"district"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Query      string  `json:"query"`
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

// makePublicIPExecutor get_public_ip 工具的 Go 原生实现：获取公网 IP 及归属地，并反查 district_id
func makePublicIPExecutor(baiduAK string) tool.ExecuteFunc {
	return func(ctx context.Context, _ map[string]interface{}) (string, error) {
		return executeGetPublicIPWithAK(ctx, baiduAK)
	}
}

func executeGetPublicIPWithAK(ctx context.Context, baiduAK string) (string, error) {
	// 1. 获取公网 IP 及归属地（含主源 ip-api.com + 兜底源 ipapi.co）
	ipInfo, err := fetchPublicIPInfo(ctx)
	if err != nil {
		return "", err
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

// ipQueryClient 公网 IP 查询专用客户端（带超时，避免请求长时间挂起）
var ipQueryClient = &http.Client{Timeout: 8 * time.Second}

// fetchPublicIPInfo 获取公网 IP 及归属地。
// 优先使用 ip-api.com（HTTP，含中文归属地与经纬度），失败时回退到 ipapi.co（HTTPS）。
func fetchPublicIPInfo(ctx context.Context) (ipAPIResponse, error) {
	info, primaryErr := fetchFromIPAPI(ctx)
	if primaryErr == nil {
		return info, nil
	}
	info, fallbackErr := fetchFromIPAPICo(ctx)
	if fallbackErr == nil {
		return info, nil
	}
	return ipAPIResponse{}, fmt.Errorf("公网 IP 查询失败（主源: %v；兜底源: %v）", primaryErr, fallbackErr)
}

// fetchFromIPAPI 主源 ip-api.com（免费版 HTTP-only，限流 45 次/分钟）
func fetchFromIPAPI(ctx context.Context) (ipAPIResponse, error) {
	const url = "http://ip-api.com/json/?lang=zh-CN&fields=status,message,country,regionName,city,district,lat,lon,query"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ipAPIResponse{}, fmt.Errorf("构建 IP 查询请求失败: %v", err)
	}
	resp, err := ipQueryClient.Do(req)
	if err != nil {
		return ipAPIResponse{}, fmt.Errorf("查询公网 IP 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ipAPIResponse{}, fmt.Errorf("读取 IP 响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ipAPIResponse{}, fmt.Errorf("ip-api.com 返回状态码 %d（可能被限流）: %s", resp.StatusCode, snippet(body))
	}

	var ipInfo ipAPIResponse
	if err := json.Unmarshal(body, &ipInfo); err != nil {
		return ipAPIResponse{}, fmt.Errorf("解析 IP JSON 失败: %v（原始响应: %s）", err, snippet(body))
	}
	if ipInfo.Status != "success" {
		reason := ipInfo.Message
		if reason == "" {
			reason = snippet(body)
		}
		return ipAPIResponse{}, fmt.Errorf("ip-api.com 查询未成功: %s", reason)
	}
	return ipInfo, nil
}

// ipAPICoResponse ipapi.co/json 响应结构（兜底源，HTTPS）
type ipAPICoResponse struct {
	IP          string  `json:"ip"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	CountryName string  `json:"country_name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Error       bool    `json:"error"`
	Reason      string  `json:"reason"`
}

// fetchFromIPAPICo 兜底源 ipapi.co（HTTPS，无 district 字段）
func fetchFromIPAPICo(ctx context.Context) (ipAPIResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipapi.co/json/", nil)
	if err != nil {
		return ipAPIResponse{}, fmt.Errorf("构建兜底 IP 查询请求失败: %v", err)
	}
	resp, err := ipQueryClient.Do(req)
	if err != nil {
		return ipAPIResponse{}, fmt.Errorf("兜底查询公网 IP 失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ipAPIResponse{}, fmt.Errorf("读取兜底 IP 响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ipAPIResponse{}, fmt.Errorf("ipapi.co 返回状态码 %d: %s", resp.StatusCode, snippet(body))
	}

	var co ipAPICoResponse
	if err := json.Unmarshal(body, &co); err != nil {
		return ipAPIResponse{}, fmt.Errorf("解析兜底 IP JSON 失败: %v（原始响应: %s）", err, snippet(body))
	}
	if co.Error || co.IP == "" {
		return ipAPIResponse{}, fmt.Errorf("ipapi.co 查询失败: %s", co.Reason)
	}
	return ipAPIResponse{
		Status:     "success",
		Country:    co.CountryName,
		RegionName: co.Region,
		City:       co.City,
		Lat:        co.Latitude,
		Lon:        co.Longitude,
		Query:      co.IP,
	}, nil
}

// snippet 截取响应体片段用于错误日志，避免过长
func snippet(b []byte) string {
	const max = 200
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(空响应)"
	}
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
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

// makeWeatherExecutor get_weather 工具的 Go 原生实现：查询百度天气 API
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

// ─── 通用脚本执行器 ──────────────────────────────────────────────────────────

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
