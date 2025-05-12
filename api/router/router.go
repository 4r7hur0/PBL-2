package router

import (
	"github.com/4r7hur0/PBL-2/api/handlers"
	"log"
	"net/http"
)

// InitRouter inicializa e inicia o servidor HTTP com as rotas da API.
func InitRouter(port string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/prepare", handlers.PrepareHandler)
	mux.HandleFunc("/commit", handlers.CommitHandler) // Endpoint de commit
	mux.HandleFunc("/abort", handlers.AbortHandler)   // Endpoint de abort

	// Endpoint de health check simples
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("HTTP server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
