// PBL-2/api/api_config.go
package main

import (
	"log"
	"os"
	"strings"
)

// EnterpriseAPIInfo contém a URL base para a API de uma empresa/cidade.
type EnterpriseAPIInfo struct {
	CityName string
	BaseURL  string // Ex: "http://localhost:8081"
}

var cityToAPIURL map[string]string


func LoadRemoteAPIConfig() {
	cityToAPIURL = make(map[string]string)
	remoteAPIsEnv := os.Getenv("REMOTE_APIS")

	if remoteAPIsEnv == "" {
		log.Println("[APIConfig] AVISO: REMOTE_APIS não definido. Comunicação inter-APIs para cidades estrangeiras não funcionará.")
		return
	}

	configs := strings.Split(remoteAPIsEnv, ",")
	for _, conf := range configs {
		parts := strings.Split(strings.TrimSpace(conf), "=")
		if len(parts) == 2 {
			cityName := strings.TrimSpace(parts[0])
			baseURL := strings.TrimSpace(parts[1])
			cityToAPIURL[cityName] = baseURL
			log.Printf("[APIConfig] API remota configurada para %s: %s", cityName, baseURL)
		} else {
			log.Printf("[APIConfig] AVISO: Configuração de API remota inválida: %s", conf)
		}
	}
}

func GetAPIURLForCity(cityName string) (string, bool) {
	url, found := cityToAPIURL[cityName]
	return url, found
}