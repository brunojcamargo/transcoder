package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
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

func transcodeFlavor(input string, flavor Flavor, includeAudio bool, wg *sync.WaitGroup) {
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
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	log.Printf("Transcodificando %s...\n", flavor.Name)
	err := cmd.Run()
	if err != nil {
		log.Printf("Erro em %s: %v\n", flavor.Name, err)
	} else {
		log.Printf("%s concluído.\n", flavor.Name)
	}
}

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
		bandwidth := flavor.Bitrate[:len(flavor.Bitrate)-1] + "000" // "14000k" -> "14000000"
		line := fmt.Sprintf(
			"#EXT-X-STREAM-INF:BANDWIDTH=%s,RESOLUTION=%s\n/output/%s/prog.m3u8\n",
			bandwidth, flavor.Scale, flavor.Name,
		)
		f.WriteString(line)
	}

	log.Println("master.m3u8 gerado corretamente.")
}

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

func handleTranscode(w http.ResponseWriter, r *http.Request) {
	input := detectInputFile()

	log.Println("Verificando faixa de áudio...")
	includeAudio := hasAudio(input)
	if includeAudio {
		log.Println("Áudio encontrado.")
	} else {
		log.Println("Sem áudio, transcodificando apenas vídeo.")
	}

	var wg sync.WaitGroup
	for _, flavor := range flavors {
		wg.Add(1)
		go transcodeFlavor(input, flavor, includeAudio, &wg)
	}
	wg.Wait()

	generateMasterManifest()

	fmt.Fprintln(w, "Transcodificação finalizada com sucesso.")
}

func main() {
	http.HandleFunc("/transcode", handleTranscode)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := "." + r.URL.Path

		//O arquivo existe?
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		// Define Content-Type correto manualmente
		// TO DO: Deve ter uma forma melhor ...
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

		// Serve o conteúdo
		stat, _ := f.Stat()
		http.ServeContent(w, r, path, stat.ModTime(), f)
	})

	log.Println("ADEServidor em pé! Acesse: http://localhost:8080/transcode")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
