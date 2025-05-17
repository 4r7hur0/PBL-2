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

var allCities = []string{"Salvador", "Feira de Santana", "Ilheus"}

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

	chosenRouteTopic := fmt.Sprintf("car/route/%s", enterpriseName)
	chosenRouteMessageChannel := mqtt.StartListening(chosenRouteTopic, 10)


	// Goroutine para processar os pedidos de rota e retornar as opções de rota
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

			var formattedResp schemas.RouteReservationOptions
			_ = json.Unmarshal(responseBytes, &formattedResp)

			fmt.Printf("[%s] Resposta enviada para o tópico %s:\n", enterpriseName, responseTopic)
			fmt.Printf("Request ID: %s\n", formattedResp.RequestID)
			fmt.Printf("Vehicle ID: %s\n\n", formattedResp.VehicleID)

			for i, rota := range formattedResp.Routes {
				fmt.Printf("Rota %d:\n", i+1)
				for j, segment := range rota {
					start := segment.ReservationWindow.StartTimeUTC.Local().Format("15:04")
					end := segment.ReservationWindow.EndTimeUTC.Local().Format("15:04")
					date := segment.ReservationWindow.StartTimeUTC.Local().Format("02/01/2006")
					fmt.Printf("  Etapa %d: Cidade: %s | Janela: %s até %s - %s\n", j+1, segment.City, start, end, date)
				}
				fmt.Println()
			}
					}
				}()

	// Goroutine para processar a rota escolhida pelo carro 
		go func() {
		for messagePayload := range chosenRouteMessageChannel {
			fmt.Printf("[%s] Mensagem de ROTA ESCOLHIDA recebida no tópico '%s': %s\n", enterpriseName, chosenRouteTopic, messagePayload)

			// 1. Deserializar a mensagem recebida (payload) para ChosenRouteMsg
			var chosenRoute schemas.ChosenRouteMsg // Usando a struct definida em schemas
			err := json.Unmarshal([]byte(messagePayload), &chosenRoute)
			if err != nil {
				log.Printf("[%s] Erro ao deserializar ChosenRouteMsg: %v. Mensagem original: %s", enterpriseName, err, messagePayload)
				continue // Pula para a próxima mensagem em caso de erro
			}

			// 2. Processar a mensagem da rota escolhida
			// Exemplo: Logar os detalhes da rota escolhida
			log.Printf("[%s] Detalhes da Rota Escolhida pelo Veículo %s (Request ID: %s):\n",
				enterpriseName, chosenRoute.VehicleID, chosenRoute.RequestID)

			if len(chosenRoute.Route) == 0 {
				log.Printf("[%s] Rota escolhida para VehicleID %s está vazia.", enterpriseName, chosenRoute.VehicleID)
			} else {
				for i, segment := range chosenRoute.Route {
					// Assumindo que schemas.RouteSegment tem ReservationWindow com StartTimeUTC e EndTimeUTC
					// e que você quer formatá-los como no exemplo anterior.
					start := segment.ReservationWindow.StartTimeUTC.Local().Format("15:04")
					end := segment.ReservationWindow.EndTimeUTC.Local().Format("15:04")
					date := segment.ReservationWindow.StartTimeUTC.Local().Format("02/01/2006")
					log.Printf("  Segmento %d: Cidade: %s | Janela de Reserva: %s às %s do dia %s\n",
						i+1, segment.City, start, end, date)
				}
			}
			fmt.Println()
		}
	}()


	router.InitRouter(enterprisePort)
}