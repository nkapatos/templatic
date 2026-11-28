// Command load opens many live streams against a running ticker and reports
// what they receive: frames, bytes, latency, and the server's CPU and memory.
package main

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var tokenRE = regexp.MustCompile(`"token":"([^"]+)"`)

// stream is one viewer: one page load and its event stream.
type stream struct {
	frames, bytes, resyncs atomic.Int64

	mu        sync.Mutex
	latencies []time.Duration
	lastAsOf  time.Time
	backwards int
}

func main() {
	base := flag.String("url", "http://localhost:8080", "the ticker's address")
	n := flag.Int("n", 100, "streams to open")
	dur := flag.Duration("d", 20*time.Second, "how long to measure")
	warmup := flag.Duration("warmup", 3*time.Second, "time after the last connect before measuring")
	pid := flag.Int("pid", 0, "the server's process id, for CPU and memory (Linux /proc)")
	flag.Parse()

	client := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 0, MaxIdleConnsPerHost: *n}}
	var measuring atomic.Bool
	streams := make([]*stream, *n)
	sem := make(chan struct{}, 32)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range streams {
		streams[i] = &stream{}
		sem <- struct{}{}
		wg.Add(1)
		go func(s *stream) {
			defer wg.Done()
			body, err := open(client, *base)
			<-sem
			if err != nil {
				log.Fatalf("load: %v", err)
			}
			go s.read(body, &measuring)
		}(streams[i])
	}
	wg.Wait()
	log.Printf("load: %d streams open in %v", *n, time.Since(start).Round(time.Millisecond))
	time.Sleep(*warmup)

	for _, s := range streams {
		s.frames.Store(0)
		s.bytes.Store(0)
	}
	cpu0, self0 := procCPU(*pid), procCPU(os.Getpid())
	t0 := time.Now()
	measuring.Store(true)
	time.Sleep(*dur)
	measuring.Store(false)
	elapsed := time.Since(t0).Seconds()
	cpu1, self1 := procCPU(*pid), procCPU(os.Getpid())

	var frames, nbytes, resyncs int64
	var lat []time.Duration
	backwards := 0
	for _, s := range streams {
		frames += s.frames.Load()
		nbytes += s.bytes.Load()
		resyncs += s.resyncs.Load()
		s.mu.Lock()
		lat = append(lat, s.latencies...)
		backwards += s.backwards
		s.mu.Unlock()
	}
	slices.Sort(lat)
	fmt.Printf("streams              %d (%d cores, %s)\n", *n, runtime.NumCPU(), runtime.GOOS)
	fmt.Printf("frames/s             %.0f total, %.1f per stream\n", float64(frames)/elapsed, float64(frames)/elapsed/float64(*n))
	fmt.Printf("bytes/s              %.0f total, %.0f per stream\n", float64(nbytes)/elapsed, float64(nbytes)/elapsed/float64(*n))
	if len(lat) > 0 {
		fmt.Printf("latency publish→read p50 %v  p99 %v  max %v  (%d samples)\n",
			pct(lat, 50), pct(lat, 99), lat[len(lat)-1], len(lat))
	}
	fmt.Printf("resyncs              %d, as-of going backwards %d\n", resyncs, backwards)
	if *pid > 0 {
		fmt.Printf("server CPU           %.0f%% of one core, RSS %s\n", 100*(cpu1-cpu0)/elapsed, procRSS(*pid))
	}
	fmt.Printf("load CPU             %.0f%% of one core\n", 100*(self1-self0)/elapsed)
}

// open loads the page for its token and opens the page's event stream.
func open(client *http.Client, base string) (io.ReadCloser, error) {
	resp, err := client.Get(base + "/")
	if err != nil {
		return nil, err
	}
	page, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	m := tokenRE.FindSubmatch(page)
	if m == nil {
		return nil, errors.New("no page token in " + base + "/")
	}
	resp, err = client.Get(base + "/live/stream?t=" + string(m[1]))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("stream: %s", resp.Status)
	}
	return resp.Body, nil
}

// read counts every frame and times the board.asof ones against the
// stamp they carry (the server and this program share a clock).
func (s *stream) read(body io.ReadCloser, measuring *atomic.Bool) {
	defer body.Close()
	r := bufio.NewReaderSize(body, 64<<10)
	var data []byte
	for {
		line, err := r.ReadSlice('\n')
		if err != nil {
			log.Fatalf("load: stream ended: %v", err)
		}
		s.bytes.Add(int64(len(line)))
		switch {
		case bytes.HasPrefix(line, []byte("data: ")):
			data = append(data[:0], line[len("data: "):]...)
		case len(line) == 1 && len(data) > 0:
			s.frames.Add(1)
			s.frame(data, measuring.Load())
			data = data[:0]
		}
	}
}

func (s *stream) frame(data []byte, measuring bool) {
	if bytes.Contains(data, []byte(`"kind":"resync"`)) {
		s.resyncs.Add(1)
		return
	}
	if !bytes.Contains(data, []byte(`"topic":"board.asof"`)) {
		return
	}
	now := time.Now()
	var ev struct {
		V string `json:"v"`
	}
	if err := json.Unmarshal(data, &ev); err != nil || ev.V == "" {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, ev.V)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if at.Before(s.lastAsOf) {
		s.backwards++
	}
	s.lastAsOf = at
	if measuring {
		s.latencies = append(s.latencies, now.Sub(at))
	}
}

func pct(sorted []time.Duration, p int) time.Duration {
	return sorted[(len(sorted)-1)*p/100].Round(10 * time.Microsecond)
}

// procCPU returns a process's user+system CPU seconds, assuming the usual
// 100 clock ticks a second; 0 when /proc cannot tell.
func procCPU(pid int) float64 {
	if pid <= 0 {
		return 0
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	// Fields after the parenthesized command name: utime and stime are the 12th and 13th.
	f := strings.Fields(string(b[bytes.LastIndexByte(b, ')')+2:]))
	if len(f) < 13 {
		return 0
	}
	u, _ := strconv.ParseFloat(f[11], 64)
	st, _ := strconv.ParseFloat(f[12], 64)
	return (u + st) / 100
}

func procRSS(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return "?"
	}
	for line := range strings.Lines(string(b)) {
		if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return "?"
}
