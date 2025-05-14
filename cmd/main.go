package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"runtime"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

const TotalIterations = int64(100_000_000_00)
const uploadURI = "https://edvinbasil.com/bench"
const resultsURI = "https://baserow.io/public/grid/lukMbgm9bHh8FoHm9B6EFORHBayutvPxJCLY2bmiX7c"

type SysInfo struct {
	OS   string `json:"OS"`
	Arch string `json:"Arch"`

	CPUModel string `json:"CPUModel"`

	Cores   uint64 `json:"Cores"`
	Threads uint64 `json:"Threads"`
	CPUUsed int64  `json:"UsedCPUCount"`

	RAM uint64 `json:"RAM_MB"`
}

type BenchResults struct {
	TimeSingle float64 `json:"Time_S"`
	TimeMulti  float64 `json:"Time_M"`
	SysInfo
}

func computeRange(start, end int64) int64 {
	result := int64(0)
	for i := start; i < end; i++ {
		val := (i * 31) ^ (i + 17)
		result ^= val
	}
	return result
}

func singleThreaded() float64 {
	start := time.Now()
	result := computeRange(0, TotalIterations)
	end := time.Now()
	elapsed := end.Sub(start)
	elapsedSec := float64(elapsed.Microseconds()) / float64(1_000_000)
	fmt.Printf("Single-threaded: %.6f\n", elapsedSec)
	fmt.Printf("Result: %d\n", result)
	return elapsedSec
}

func multiThreaded(concurrency int64) float64 {
	// divide total iterations to chunks
	chunkSize := TotalIterations / concurrency

	resultChan := make(chan int64, concurrency)

	var wg sync.WaitGroup
	wg.Add(int(concurrency))

	startTime := time.Now()

	for i := int64(0); i < concurrency; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if i == concurrency-1 {
			end = TotalIterations
		}
		go func(start, end int64) {
			defer wg.Done()
			resultChan <- computeRange(start, end)
		}(start, end)
	}
	wg.Wait()

	endTime := time.Now()
	elapsed := endTime.Sub(startTime)

	result := int64(0)
	for i := int64(0); i < concurrency; i++ {
		result ^= <-resultChan
	}
	elapsedSec := float64(elapsed.Microseconds()) / float64(1_000_000)
	fmt.Printf("Concurrent: %.6f seconds\n", elapsedSec)
	fmt.Printf("Result: %d\n", result)

	return elapsedSec
}

func sysInfo() (*SysInfo, error) {

	info := &SysInfo{}

	// OS and architecture
	info.Arch = runtime.GOARCH
	info.OS = runtime.GOOS

	// CPU Model
	cpuInfo, err := cpu.Info()
	if err != nil {
		log.Fatalf("Error getting CPU info: %v", err)
		return info, err
	}

	if len(cpuInfo) > 0 {
		info.CPUModel = cpuInfo[0].ModelName
	}

	// Logical and Physical CPU count
	logicalCount, _ := cpu.Counts(true)
	physicalCount, _ := cpu.Counts(false)

	info.Cores = uint64(physicalCount)
	info.Threads = uint64(logicalCount)

	// Total RAM in MB
	vmStat, _ := mem.VirtualMemory()
	info.RAM = (vmStat.Total) / (1024 * 1024)

	return info, nil
}

func main() {
	// command-line flags
	concurrency := flag.Int64("concurrency", int64(runtime.NumCPU()), "Number of 'threads' to use")
	upload := flag.Bool("upload", false, fmt.Sprintf("Upload results to %s", resultsURI))

	flag.Parse()

	// system info
	info, err := sysInfo()
	if err != nil {
		log.Printf("[systeminfo]: failed to get system info: %v", err)
	}
	info.CPUUsed = *concurrency

	data := &BenchResults{}
	data.SysInfo = *info

	fmt.Printf("[system info]: %s/%s\n", info.OS, info.Arch)
	fmt.Printf("[system info]: CPU Model: %s\n", info.CPUModel)
	fmt.Printf("[system info]: %d Cores, %d Threads\n", info.Cores, info.Threads)

	// custom concurrency or not?
	if *concurrency != int64(runtime.NumCPU()) {
		fmt.Printf("[tinybench]: setting custom concurrency to %d\n", *concurrency)
	} else {
		fmt.Printf("[tinybench]: using default CPU concurrency of %d\n", *concurrency)
	}

	fmt.Printf("Running compute benchmark with %d iterations...\n", TotalIterations)
	data.TimeSingle = singleThreaded()

	fmt.Printf("Running multithreaded compute benchmark with %d iterations on concurrency %d...\n", TotalIterations, *concurrency)
	data.TimeMulti = multiThreaded(*concurrency)

	if *upload {

		req, _ := json.Marshal(data)
		fmt.Printf("%+s\n", req)
		resp, err := http.Post(uploadURI, "application/json", bytes.NewBuffer(req))
		if err != nil {
			log.Fatalf("Error posting results: %+v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			fmt.Printf("Posted results successfully!\n")
		} else {
			fmt.Printf("Got response code %d: ", resp.StatusCode)
			respbody, _ := io.ReadAll(resp.Body)
			fmt.Printf("%+s\n", respbody)
		}
	}
}
