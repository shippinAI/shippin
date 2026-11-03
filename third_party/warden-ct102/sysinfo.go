package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type SysInfoData struct {
	HeaderData
	Hostname       string
	OS             string
	Kernel         string
	Arch           string
	CPUModel       string
	CPUCores       int
	MemTotal       uint64
	MemUsed        uint64
	MemUsedPerc    float64
	SwapTotal      uint64
	SwapUsed       uint64
	SwapUsedPerc   float64
	Uptime         time.Duration
	BootTime       string
	LoadAvg        LoadAverage
	Network        []NetInfo
	Disks          []DiskInfo
	Temps          []TempSensor
	ProcessCount   int
	GoVersion      string
	Timezone       string
	Virtualization string
	InitSystem     string
	Shell          string
}

func handleSysInfo(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "System Info"

	hostname, _ := os.Hostname()
	osName := readFileString("/etc/os-release", "PRETTY_NAME")
	kernel := readFileTrimmed("/proc/version")
	if idx := strings.Index(kernel, "("); idx > 0 {
		kernel = strings.TrimSpace(kernel[:idx])
	}

	cpuInfo := getCPUInfoStatic()
	memory := getMemInfo("MemTotal", "MemFree", "MemAvailable", "Buffers", "Cached")
	swap := getSwapInfo()
	uptime := getUptime()
	bootTime := time.Now().Add(-uptime).Format("2006-01-02 15:04:05")

	// Timezone
	tz := time.Now().Location().String()
	if tzFile := readFileTrimmed("/etc/timezone"); tzFile != "N/A" {
		tz = tzFile
	}

	// Virtualization
	virt := "None (bare metal)"
	if out, err := execCommand("systemd-detect-virt"); err == nil {
		v := strings.TrimSpace(out)
		if v != "" && v != "none" {
			virt = v
		}
	}

	// Init system
	initSys := readFileTrimmed("/proc/1/comm")

	// Shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "N/A"
	}

	data := SysInfoData{
		HeaderData:     hd,
		Hostname:       hostname,
		OS:             osName,
		Kernel:         kernel,
		Arch:           runtime.GOARCH,
		CPUModel:       cpuInfo.ModelName,
		CPUCores:       cpuInfo.Cores,
		MemTotal:       memory.Total,
		MemUsed:        memory.Used,
		MemUsedPerc:    memory.UsedPerc,
		SwapTotal:      swap.Total,
		SwapUsed:       swap.Used,
		SwapUsedPerc:   swap.UsedPerc,
		Uptime:         uptime,
		BootTime:       bootTime,
		LoadAvg:        getLoadAvg(),
		Network:        getNetInfo(),
		Disks:          getDiskInfo(),
		Temps:          getTemperatures(),
		ProcessCount:   getProcessCount(),
		GoVersion:      runtime.Version(),
		Timezone:       tz,
		Virtualization: virt,
		InitSystem:     initSys,
		Shell:          shell,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "sysinfo.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

// getCPUInfoStatic returns CPU model and core count without measuring usage (no 200ms sleep).
func getCPUInfoStatic() CPUInfo {
	info := CPUInfo{Cores: runtime.NumCPU()}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return info
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.ModelName = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	return info
}
