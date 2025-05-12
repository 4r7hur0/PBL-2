package config

import (
	"log"
	"strings"
	"sync"

	// Importa o cliente MQTT Paho
	mqtt_lib "github.com/eclipse/paho.mqtt.golang"
)

// CompanyInfo define a estrutura para informações da empresa.
type CompanyInfo struct {
	Name               string
	Key                string // Chave normalizada para uso em mapas e tópicos MQTT
	HTTPPort           string
	APIBaseURL         string            // Ex: "http://localhost:8081"
	ChargingPoints     map[string]bool   // Pontos de recarga gerenciados por esta empresa
	TransactionLogger  *TransactionLogger // Logger de transações específico da instância
	ReservationManager *ReservationManager // Gerenciador de reservas específico da instância
	MQTTClient         mqtt_lib.Client    // Cliente MQTT para esta instância (usado pelo coordenador)
}

var (
	// Companies é o mapa de configuração para todas as empresas.
	// As chaves devem ser normalizadas (lowercase, sem espaços) para fácil lookup.
	Companies = map[string]CompanyInfo{
		"solatlantico": {
			Name:           "Sol Atlântico",
			Key:            "solatlantico",
			HTTPPort:       "8081", // Default, pode ser sobrescrito pela flag
			APIBaseURL:     "http://localhost:8081",
			ChargingPoints: map[string]bool{"SAP1": true, "SAP2": true, "SAP3": true, "SAP4": true, "SAP5": true},
		},
		"sertaocarga": {
			Name:           "Sertão Carga",
			Key:            "sertaocarga",
			HTTPPort:       "8082",
			APIBaseURL:     "http://localhost:8082",
			ChargingPoints: map[string]bool{"FSP1": true, "FSP2": true, "FSP3": true, "FSP4": true},
		},
		"chapadaeletric": {
			Name:           "Chapada Eletric",
			Key:            "chapadaeletric",
			HTTPPort:       "8083",
			APIBaseURL:     "http://localhost:8083",
			ChargingPoints: map[string]bool{"LCP1": true, "LCP2": true},
		},
		"cacaupower": {
			Name:           "Cacau Power",
			Key:            "cacaupower",
			HTTPPort:       "8084",
			APIBaseURL:     "http://localhost:8084",
			ChargingPoints: map[string]bool{"ILP1": true, "ILP2": true, "ILP3": true},
		},
		"velhochicoenergia": {
			Name:           "Velho Chico Energia",
			Key:            "velhochicoenergia",
			HTTPPort:       "8085",
			APIBaseURL:     "http://localhost:8085",
			ChargingPoints: map[string]bool{"JZP1": true, "JZP2": true, "JZP3": true},
		},
	}

	// currentCompanyInfo armazena a configuração da empresa para a instância atual.
	currentCompanyInfo CompanyInfo
	once               sync.Once
)

// NormalizeCompanyName converte o nome da empresa para uma chave padronizada.
func NormalizeCompanyName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", ""))
}

// SetCurrentCompany inicializa a configuração da empresa atual.
// Isso deve ser chamado uma vez no início, após a criação do cliente MQTT.
func SetCurrentCompany(company CompanyInfo) {
	once.Do(func() {
		// Atualiza a APIBaseURL com a porta correta, caso tenha sido sobrescrita pela flag
		company.APIBaseURL = "http://localhost:" + company.HTTPPort

		// Inicializa o logger de transações e o gerenciador de reservas para esta instância
		company.TransactionLogger = NewTransactionLogger()
		company.ReservationManager = NewReservationManager(company.ChargingPoints)

		// Armazena a configuração completa, incluindo o cliente MQTT
		currentCompanyInfo = company
		log.Printf("[Config] Set current company to: %s, API Base URL: %s", currentCompanyInfo.Name, currentCompanyInfo.APIBaseURL)

		// Atualiza o mapa global Companies também, se a porta foi sobrescrita
		// Isso é importante para que o coordenador use as URLs corretas
		if globalCompany, ok := Companies[company.Key]; ok {
			globalCompany.APIBaseURL = company.APIBaseURL // Garante que o mapa global tenha a URL correta
			globalCompany.HTTPPort = company.HTTPPort
			// NÃO copia o MQTTClient para o mapa global, pois cada instância tem o seu
			Companies[company.Key] = globalCompany
		}
	})
}

// GetCurrentCompany retorna a configuração da empresa atual.
func GetCurrentCompany() CompanyInfo {
	if currentCompanyInfo.Name == "" {
		// Considerar usar um mutex aqui se houver chance de race condition na inicialização,
		// mas a flag 'once' já protege a escrita inicial. A leitura pode ocorrer antes da escrita
		// se chamada muito cedo.
		log.Fatal("Current company configuration not set. Call SetCurrentCompany first.")
	}
	return currentCompanyInfo
}

// GetCompanyAPIURL retorna a URL base da API para uma determinada empresa pelo seu charging point ID.
func GetCompanyAPIURL(chargingPointID string) (string, bool) {
	prefix := ""
	if len(chargingPointID) >= 2 {
		prefix = chargingPointID[0:2] // SAP, FSP, LCP, ILP, JZP
	} else {
		return "", false // ID inválido
	}

	// Acessa o mapa global atualizado
	for _, company := range Companies {
		// Verifica se algum dos charging points da empresa começa com o prefixo
		for cp := range company.ChargingPoints {
			if strings.HasPrefix(cp, prefix) {
				// Retorna a APIBaseURL que foi potencialmente atualizada pela flag de porta
				return company.APIBaseURL, true
			}
		}
	}
	return "", false
}

// GetCompanyInfoByKey retorna a struct CompanyInfo completa pelo Key normalizado.
// Usado pelo coordenador para obter informações do participante.
func GetCompanyInfoByKey(key string) (CompanyInfo, bool) {
	// Acessa o mapa global atualizado
	company, found := Companies[key]
	return company, found
}

// GetCompanyInfoByChargingPoint retorna a struct CompanyInfo completa pelo Charging Point ID.
func GetCompanyInfoByChargingPoint(chargingPointID string) (CompanyInfo, bool) {
	prefix := ""
	if len(chargingPointID) >= 2 {
		prefix = chargingPointID[0:2]
	} else {
		return CompanyInfo{}, false
	}

	for key, company := range Companies {
		for cp := range company.ChargingPoints {
			if strings.HasPrefix(cp, prefix) {
				// Retorna a cópia do mapa global (sem o MQTTClient específico da instância)
				return Companies[key], true
			}
		}
	}
	return CompanyInfo{}, false
}
