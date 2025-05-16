package main

import (
	"encoding/json" 
	"fmt"
	"log"            
	"os" 

	"github.com/4r7hur0/PBL-2/api/mqtt"
	"github.com/4r7hur0/PBL-2/api/router"
	"github.com/4r7hur0/PBL-2/schemas"   
	"github.com/google/uuid"             
)

// Mapa de cidades para empresas e lista de todas as cidades

var allCities = []string{"Salvador", "Feira de Santana", "Ilhéus"}

func main() {
	
	enterpriseName := os.Getenv("ENTERPRISE_NAME")
	enterprisePort := os.Getenv("ENTERPRISE_PORT")
	if enterpriseName == "" {
		fmt.Println("AVISO: A variável de ambiente ENTERPRISE_NAME não está definida.")
		fmt.Println("Usando 'SolAtlantico' como padrão. Configure para a empresa desejada.")
		enterpriseName = "SolAtlantico"
	}

	fmt.Printf("Iniciando API para a empresa: %s\n", enterpriseName)

	mqtt.InitializeMQTT("tcp://localhost:1883") // 

	messageChannel := mqtt.StartListening(enterpriseName, 10) 

	// Goroutine para processar as mensagens recebidas
	go func() {
		for messagePayload := range messageChannel { 
			fmt.Printf("Mensagem recebida pela empresa %s: %s\n", enterpriseName, messagePayload)

			// 1. Deserializar a mensagem recebida (payload) para schemas.RouteRequest
			var routeReq schemas.RouteRequest
	
			err := json.Unmarshal([]byte(messagePayload), &routeReq)
			if err != nil {
				log.Printf("[%s] Erro ao deserializar RouteRequest: %v. Mensagem original: %s", enterpriseName, err, messagePayload)
				continue 
			}

			// Validar se o VehicleID foi recebido
			if routeReq.VehicleID == "" {
				log.Printf("[%s] VehicleID está vazio na requisição. Mensagem: %s", enterpriseName, messagePayload)
				continue
			}

			// 3. Gerar um RequestID único
			requestID := uuid.New().String() // Gera um novo UUID v4 como string

			var possibleRoutes [][]schemas.RouteSegment

			if routeReq.Origin != "" && routeReq.Destination != "" {
				// Chamar a função do pacote 'router'
				possibleRoutes = router.GeneratePossibleRoutes(routeReq.Origin, routeReq.Destination, allCities)
				if len(possibleRoutes) == 0 {
					log.Printf("[%s] Nenhuma rota retornada pelo módulo de roteamento para '%s' -> '%s'.", enterpriseName, routeReq.Origin, routeReq.Destination)
				}
			} else {
				log.Printf("[%s] Origem ou destino não especificados na requisição. Mensagem: %s", enterpriseName, messagePayload)
			}


			// 4. Construir o objeto de resposta schemas.RouteReservationResponse
			response := schemas.RouteReservationOptions{
				RequestID: requestID,
				VehicleID: routeReq.VehicleID,
				Routes:    possibleRoutes,
			}

			// 5. Serializar o objeto de resposta para JSON
			responseBytes, err := json.Marshal(response)
			if err != nil {
				log.Printf("[%s] Erro ao serializar RouteReservationRespose para VehicleID %s: %v", enterpriseName, routeReq.VehicleID, err)
				continue
			}

			// 6. Publicar a resposta JSON para o tópico MQTT do carro
			//    O carro escuta em um tópico que é o seu próprio ID (routeReq.VehicleID)
			responseTopic := routeReq.VehicleID
			mqtt.Publish(responseTopic, string(responseBytes)) // A função Publish espera uma string

			fmt.Printf("[%s] Resposta enviada para o tópico %s: %s\n", enterpriseName, responseTopic, string(responseBytes))
		}
	}()

	router.InitRouter(enterprisePort)
}