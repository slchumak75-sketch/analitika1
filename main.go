// Программа «Точка безубыточности и оптимизация накладных расходов
// розничного магазина».
//
// Архитектура: один исполняемый файл без внешних зависимостей.
// HTML-интерфейс встроен в бинарник (go:embed) и открывается в браузере,
// все расчёты выполняет Go-сервер на 127.0.0.1 (эндпоинт /api/calc).
// Интернет для работы программы не нужен.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

const version = "1.0.0"

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(indexHTML)
	})

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok", "version": version})
	})

	// Основной расчётный эндпоинт: принимает Input, возвращает Result.
	mux.HandleFunc("/api/calc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "нужен POST", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // не больше 1 МБ
		var in Input
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "некорректный JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, Compute(in))
	})

	// Импорт таблиц (.xls/.xlsx/.csv): разбор файла и анализ отчёта.
	// См. spreadsheet.go, report.go и docs/SPREADSHEET_IMPORT.md.
	mux.HandleFunc("/api/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "нужен POST", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			http.Error(w, "некорректная форма: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "файл не передан (поле file)", http.StatusBadRequest)
			return
		}
		defer file.Close()
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, io.LimitReader(file, 16<<20)); err != nil {
			http.Error(w, "не удалось прочитать файл", http.StatusBadRequest)
			return
		}
		wb, err := ParseSpreadsheet(header.Filename, buf.Bytes())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, AnalyzeWorkbook(wb))
	})

	// Корректное завершение программы из интерфейса.
	mux.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "нужен POST", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]string{"status": "bye"})
		go func() {
			time.Sleep(300 * time.Millisecond)
			log.Println("завершение по запросу из интерфейса")
			os.Exit(0)
		}()
	})

	// Свободный порт на локальном интерфейсе — конфликтов не будет.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("не удалось открыть порт: %v", err)
	}
	url := "http://" + ln.Addr().String() + "/"

	fmt.Println("Точка безубыточности магазина, версия", version)
	fmt.Println("Интерфейс открыт в браузере:", url)
	fmt.Println("Для завершения нажмите кнопку «Завершить программу» в интерфейсе или Ctrl+C.")

	go openBrowser(url)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.Serve(ln))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func openBrowser(url string) {
	time.Sleep(200 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("не удалось открыть браузер автоматически, откройте вручную: %s (%v)", url, err)
	}
}
