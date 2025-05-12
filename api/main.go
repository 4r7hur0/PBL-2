package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	// Imports atualizados
	"github.com/4r7hur0/PBL-2/api/config"
	"github.com/4r7hur0/PBL-2/api/coordinator"
	"github.com/4r7hur0/PBL-2/api/mqtt"
	"github.com/4r7hur0/PBL-2/api/router"
)

func main() {
	// Flags para configurar a empresa e a porta
	companyNameFlag := flag.String("company", "", "Name of the company this instance represents (e.g., SolAtlantico, SertaoCarga)")
	httpPortFlag := flag.String("port", "", "HTTP port for the API server (e.g., 8081)")
	mqttBrokerFlag := flag.String("mqttbroker", "tcp://localhost:1883", "MQTT broker URL")

	flag.Parse()

	if *companyNameFlag == "" || *httpPortFlag == "" {
		fmt.Println("Usage: go run main.go -company <CompanyName> -port <PortNumber>")
		fmt.Println("Example: go run main.go -company SolAtlantico -port 8081")
		os.Exit(1)
	}

	companyKey := config.NormalizeCompanyName(*companyNameFlag)
	currentCompany, ok := config.Companies[companyKey]
	if !ok {
		log.Fatalf("Error: Company configuration for '%s' not found. Ensure it's defined in config.Companies.", *companyNameFlag)
	}
	currentCompany.HTTPPort = *httpPortFlag // Sobrescreve a porta com a flag

	log.Printf("Starting server for %s on port %s", currentCompany.Name, currentCompany.HTTPPort)
	log.Printf("Managing Charging Points: %v", currentCompany.ChargingPoints)

	// Inicializa o cliente MQTT
	mqttClient := mqtt.InitializeMQTT(*mqttBrokerFlag, fmt.Sprintf("server-%s", companyKey))
	defer mqttClient.Disconnect(250)

	// Passa o cliente MQTT para a configuração para que o coordenador possa usá-lo
	currentCompany.MQTTClient = mqttClient

	// Inicializa a configuração global APÓS ter o cliente MQTT
	config.SetCurrentCompany(currentCompany)


	// O tópico de escuta pode ser específico da empresa
	mqttRequestTopic := fmt.Sprintf("reservations/request/%s", companyKey)
	messageChannel := mqtt.StartListening(mqttClient, mqttRequestTopic, 10) // QoS 1

	// Goroutine para processar mensagens MQTT recebidas (requisições de reserva de rota)
	go func() {
		for msg := range messageChannel {
			// Obtém a configuração atualizada que inclui o cliente MQTT
			currentCompanyConfig := config.GetCurrentCompany()
			log.Printf("[%s] Received MQTT message on topic %s: %s\n", currentCompanyConfig.Name, msg.Topic(), string(msg.Payload()))
			// Passa o cliente MQTT para o handler do coordenador
			go coordinator.HandleRouteReservationRequest(msg.Payload(), currentCompanyConfig.MQTTClient)
		}
	}()

	// Inicia o roteador da API HTTP
	log.Printf("[%s] Starting HTTP API server on port %s", currentCompany.Name, currentCompany.HTTPPort)
	router.InitRouter(currentCompany.HTTPPort) // Passa a porta configurada
}
