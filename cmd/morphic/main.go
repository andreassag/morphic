package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"github.com/andreassag/morphic/internal/shared"
	"github.com/andreassag/morphic/web"
)

func main() {
	host := flag.String("host", "127.0.0.1", "Host to bind to")
	port := flag.Int("port", 8000, "Port to listen on")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	url := fmt.Sprintf("http://%s", addr)

	if !*noBrowser {
		go openBrowser(url)
	}

	log.Printf("Morphic %s starting on %s", shared.Version, url)

	router := web.SetupRouter()
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}
