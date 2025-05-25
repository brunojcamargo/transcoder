package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type Flavor struct {
	Name      string
	Scale     string
	Bitrate   string
	OutputDir string
}

var flavors = []Flavor{
	{"2160p", "3840:2160", "14000k", "output/2160p"},
	{"1080p", "1920:1080", "6000k", "output/1080p"},
	{"720p", "1280:720", "3000k", "output/720p"},
	{"480p", "854:480", "1000k", "output/480p"},
	{"360p", "640:360", "600k", "output/360p"},
	{"180p", "320:180", "300k", "output/180p"},
}

// Progresso global, thread-safe
var (
	progress     = make(map[string]float64) // flavor.Name -> %
	progressLock sync.Mutex
)

// Helpers para progresso
func setProgress(flavor string, p float64) {
	progressLock.Lock()
	defer progressLock.Unlock()
	progress[flavor] = p
}
func getProgress() map[string]float64 {
	progressLock.Lock()
	defer progressLock.Unlock()
	cp := make(map[string]float64)
	for k, v := range progress {
		cp[k] = v
	}
	return cp
}

// Converter "00:01:23.45" para segundos float64
func timeToSeconds(ts string) float64 {
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	s, _ := strconv.ParseFloat(parts[2], 64)
	return h*3600 + m*60 + s
}

// Obter duração total do vídeo
func getDuration(input string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", input)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
}

// Verifica se tem áudio
func hasAudio(input string) bool {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "a",
		"-show_entries", "stream=index", "-of", "csv=p=0", input)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false
	}
	return strings.TrimSpace(out.String()) != ""
}

// Transcodifica 1 flavor
func transcodeFlavor(input string, flavor Flavor, includeAudio bool, duration float64, wg *sync.WaitGroup) {
	defer wg.Done()

	os.MkdirAll(flavor.OutputDir, os.ModePerm)

	args := []string{
		"-y", "-i", input,
		"-vf", fmt.Sprintf("scale=%s", flavor.Scale),
		"-c:v", "libx264", "-b:v", flavor.Bitrate,
	}
	if includeAudio {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	} else {
		args = append(args, "-an") // remove audio
	}
	args = append(args,
		"-f", "hls",
		"-hls_time", "6", "-hls_list_size", "0",
		"-hls_segment_filename", fmt.Sprintf("%s/file_%%03d.ts", flavor.OutputDir),
		fmt.Sprintf("%s/prog.m3u8", flavor.OutputDir),
	)

	cmd := exec.Command("ffmpeg", args...)
	stderr, _ := cmd.StderrPipe()

	log.Printf("Transcodificando %s...\n", flavor.Name)
	if err := cmd.Start(); err != nil {
		log.Printf("Erro em %s: %v\n", flavor.Name, err)
		setProgress(flavor.Name, 100)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "time=") {
				// time=00:00:33.84
				timeStr := ""
				if idx := strings.Index(line, "time="); idx != -1 {
					timeStr = line[idx+5:]
					if i := strings.Index(timeStr, " "); i != -1 {
						timeStr = timeStr[:i]
					}
				}
				if timeStr != "" {
					sec := timeToSeconds(timeStr)
					percent := (sec / duration) * 100
					if percent > 100 {
						percent = 100
					}
					setProgress(flavor.Name, percent)
				}
			}
		}
	}()
	err := cmd.Wait()
	setProgress(flavor.Name, 100) // Garante 100% ao finalizar
	if err != nil {
		log.Printf("Erro em %s: %v\n", flavor.Name, err)
	} else {
		log.Printf("%s concluído.\n", flavor.Name)
	}
}

// Cria o master manifest m3u8
func generateMasterManifest() {
	log.Println("Gerando master.m3u8...")
	f, err := os.Create("output/master.m3u8")
	if err != nil {
		log.Println("Erro ao criar master.m3u8:", err)
		return
	}
	defer f.Close()

	f.WriteString("#EXTM3U\n")
	for _, flavor := range flavors {
		bandwidth := flavor.Bitrate[:len(flavor.Bitrate)-1] + "000"
		line := fmt.Sprintf(
			"#EXT-X-STREAM-INF:BANDWIDTH=%s,RESOLUTION=%s\n/output/%s/prog.m3u8\n",
			bandwidth, flavor.Scale, flavor.Name,
		)
		f.WriteString(line)
	}
	log.Println("master.m3u8 gerado corretamente.")
}

// Detecta arquivo de entrada
func detectInputFile() string {
	paths := []string{
		"input/input.mp4",
		"input/input.ts",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			log.Println("Arquivo de entrada detectado:", path)
			return path
		}
	}
	log.Fatal("Nenhum arquivo de entrada encontrado em /input (input.mp4 ou input.ts)")
	return ""
}

// Endpoint para transcodificar
func handleTranscode(w http.ResponseWriter, r *http.Request) {
	input := detectInputFile()
	duration, err := getDuration(input)
	if err != nil || duration == 0 {
		log.Println("Erro ao obter duração do vídeo:", err)
		http.Error(w, "Erro ao obter duração do vídeo", http.StatusInternalServerError)
		return
	}

	log.Println("Verificando faixa de áudio...")
	includeAudio := hasAudio(input)
	if includeAudio {
		log.Println("Áudio encontrado.")
	} else {
		log.Println("Sem áudio, transcodificando apenas vídeo.")
	}

	// Resetar progresso
	progressLock.Lock()
	for _, flavor := range flavors {
		progress[flavor.Name] = 0
	}
	progressLock.Unlock()

	var wg sync.WaitGroup
	for _, flavor := range flavors {
		wg.Add(1)
		go transcodeFlavor(input, flavor, includeAudio, duration, &wg)
	}
	wg.Wait()

	generateMasterManifest()
	fmt.Fprintln(w, "Transcodificação finalizada com sucesso.")
}

// Endpoint para progresso
func handleProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	p := getProgress()

	// Exemplo de estrutura de resposta:
	type Progress struct {
		Flavor  string  `json:"flavor"`
		Percent float64 `json:"percent"`
	}
	var resp []Progress
	for _, flavor := range flavors {
		resp = append(resp, Progress{
			Flavor:  flavor.Name,
			Percent: p[flavor.Name],
		})
	}

	json.NewEncoder(w).Encode(resp)
}

// Servir arquivos e conteúdos HLS
func handleRoot(w http.ResponseWriter, r *http.Request) {
	path := "." + r.URL.Path

	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	switch {
	case strings.HasSuffix(path, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case strings.HasSuffix(path, ".ts"):
		w.Header().Set("Content-Type", "video/MP2T")
	case strings.HasSuffix(path, ".html"):
		w.Header().Set("Content-Type", "text/html")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	stat, _ := f.Stat()
	http.ServeContent(w, r, path, stat.ModTime(), f)
}

func main() {
	http.HandleFunc("/transcode", handleTranscode)
	http.HandleFunc("/progress", handleProgress)
	http.HandleFunc("/", handleRoot)

	log.Println("Servidor em pé! Acesse: http://localhost:8080/transcode")
	log.Println("Acompanhe o progresso em: http://localhost:8080/progress")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
