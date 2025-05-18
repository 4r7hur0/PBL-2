package main

import (
	"encoding/json" 
	"fmt"
	"log"      
	"net/http"      
	"os"
	"strconv"
	"sync"


	"github.com/4r7hur0/PBL-2/api/mqtt"
	"github.com/4r7hur0/PBL-2/api/router"
	"github.com/4r7hur0/PBL-2/schemas"   
	"github.com/google/uuid"      
	"github.com/gin-gonic/gin"
       
)

// Mapa de cidades para empresas e lista de todas as cidades

var allCities = []string{"Salvador", "Feira de Santana", "Ilheus"}

// Estruturas para gerenciamento de disponibilidade com 2PC
var cityAvailablePostsCount = make(map[string]int) 
var preparedTransactions = make(map[string][]string)
var postsMutex = &sync.Mutex{} 
var enterpriseName string 
var initialPostsPerCity int 

func main() {
	
	enterpriseName := os.Getenv("ENTERPRISE_NAME")
	enterprisePort := os.Getenv("ENTERPRISE_PORT")
	postsQuantityStr := os.Getenv("POSTS_QUANTITY")

	if enterpriseName == "" {
		fmt.Println("AVISO: ENTERPRISE_NAME não definido. Usando 'SolAtlantico'.")
		enterpriseName = "SolAtlantico"
	}
	if enterprisePort == "" {
		fmt.Println("AVISO: ENTERPRISE_PORT não definido. Usando '8080'.")
		enterprisePort = "8080"
	}
	if postsQuantityStr == "" {
		fmt.Println("AVISO: POSTS_QUANTITY não definido. Usando '5' por cidade.")
		initialPostsPerCity = 5
	} else {
		var err error
		initialPostsPerCity, err = strconv.Atoi(postsQuantityStr)
		if err != nil {
			log.Printf("Erro ao converter POSTS_QUANTITY: %v. Usando 5.", err)
			initialPostsPerCity = 5
		}
	}

	fmt.Printf("Iniciando API para a empresa: %s\n", enterpriseName)

	// Inicializar disponibilidade de postos

	postsMutex.Lock()
	for _, city := range allCities {
		cityAvailablePostsCount[city] = initialPostsPerCity
	}
	postsMutex.Unlock()

	// Inicializar MQTT


	mqtt.InitializeMQTT("tcp://localhost:1883") 
	messageChannel := mqtt.StartListening(enterpriseName, 10) 
	chosenRouteTopic := fmt.Sprintf("car/route/%s", enterpriseName)
	chosenRouteMessageChannel := mqtt.StartListening(chosenRouteTopic, 10)


	// Goroutine para processar os pedidos de rota e retornar as opções de rota
	go func() {
		for messagePayload := range messageChannel { 
			fmt.Printf("[%s] Mensagem de REQUISIÇÃO DE ROTA recebida: %s\n", enterpriseName, messagePayload)

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
			requestID := uuid.New().String()

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

			// 6. Publicar a resposta JSON para o tópico MQTT do carro (O carro escuta em um tópico que é o seu próprio ID)

			responseTopic := routeReq.VehicleID
			mqtt.Publish(responseTopic, string(responseBytes)) 

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
			transactionID := uuid.New().String() 

			fmt.Printf("[%s] Mensagem de ROTA ESCOLHIDA recebida no tópico '%s': %s\n", enterpriseName, chosenRouteTopic, messagePayload)
			fmt.Println("Iniciando 2PC...")

			// 1. Deserializar a mensagem recebida (payload) para ChosenRouteMsg
			var chosenRoute schemas.ChosenRouteMsg
			err := json.Unmarshal([]byte(messagePayload), &chosenRoute)
			if err != nil {
				log.Printf("[%s] Erro ao deserializar ChosenRouteMsg: %v. Mensagem original: %s", enterpriseName, err, messagePayload)
				continue 
			}
					if chosenRoute.VehicleID == "" || chosenRoute.RequestID == "" {
			log.Printf("[%s] TX[%s]: VehicleID ou RequestID ausente na ChosenRouteMsg. Payload: %s", enterpriseName, transactionID, messagePayload)
			continue
		}
		if len(chosenRoute.Route) == 0 {
			log.Printf("[%s] TX[%s]: Rota escolhida está vazia para VehicleID %s.", enterpriseName, transactionID, chosenRoute.VehicleID)
			publishReservationStatus(chosenRoute.VehicleID, transactionID, "REJECTED", "Rota escolhida estava vazia", nil)

			continue
		}
		// 2. Determinar cidades únicas que precisam de reserva de posto
		citiesInChosenRoute := make(map[string]bool)
		for _, segment := range chosenRoute.Route {
			citiesInChosenRoute[segment.City] = true 
		}

		var citiesToReserve []string
		for city := range citiesInChosenRoute {
			citiesToReserve = append(citiesToReserve, city)
		}

		if len(citiesToReserve) == 0 {
			log.Printf("[%s] TX[%s]: Nenhuma cidade identificada para reserva na rota escolhida para VehicleID %s.", enterpriseName, transactionID, chosenRoute.VehicleID)

			publishReservationStatus(chosenRoute.VehicleID, transactionID, "REJECTED", "Nenhuma cidade para reserva na rota", &chosenRoute)


		}
		// 3. Fase de Preparação (PREPARE)
		log.Printf("[%s] TX[%s]: Iniciando FASE DE PREPARAÇÃO para VehicleID %s. Cidades: %v", enterpriseName, transactionID, chosenRoute.VehicleID, citiesToReserve)
		preparedCitiesForThisTx := []string{}
		prepareSuccess := true

		postsMutex.Lock()
		for _, city := range citiesToReserve {
			if cityAvailablePostsCount[city] > 0 {
				cityAvailablePostsCount[city]--
				preparedCitiesForThisTx = append(preparedCitiesForThisTx, city)
				log.Printf("[%s] TX[%s]: SUCESSO na etapa PREPARE para cidade '%s'. Postos restantes: %d. VehicleID: %s", enterpriseName, transactionID, city, cityAvailablePostsCount[city], chosenRoute.VehicleID)
			} else {
				log.Printf("[%s] TX[%s]: FALHA na etapa PREPARE para cidade '%s'. Sem postos disponíveis. VehicleID: %s", enterpriseName, transactionID, city, chosenRoute.VehicleID)
				prepareSuccess = false
				break 
			}
		}

		if prepareSuccess {
			preparedTransactions[transactionID] = preparedCitiesForThisTx
			log.Printf("[%s] TX[%s]: FASE DE PREPARAÇÃO bem-sucedida para todas as cidades. VehicleID: %s", enterpriseName, transactionID, chosenRoute.VehicleID)
		}
		postsMutex.Unlock()

		// 4. Fase de EFETIVAÇÂO (COMMIT ou ABORT)

		if prepareSuccess {
			// COMMIT 
			log.Printf("[%s] TX[%s]: Iniciando FASE DE COMMIT para VehicleID %s.", enterpriseName, transactionID, chosenRoute.VehicleID)

			postsMutex.Lock()
			delete(preparedTransactions, transactionID) 
			postsMutex.Unlock()

			log.Printf("[%s] TX[%s]: COMMIT SUCESSO. Reserva confirmada para VehicleID %s.", enterpriseName, transactionID, chosenRoute.VehicleID)

			logChosenRouteDetails(transactionID, chosenRoute)
			publishReservationStatus(chosenRoute.VehicleID, transactionID, "CONFIRMED", "Reserva confirmada com sucesso", &chosenRoute)

		} else {
			// ABORT
			log.Printf("[%s] TX[%s]: Iniciando FASE DE ABORTO para VehicleID %s devido à falha na preparação.", enterpriseName, transactionID, chosenRoute.VehicleID)
			postsMutex.Lock()

			for _, cityToRollback := range preparedCitiesForThisTx {
				cityAvailablePostsCount[cityToRollback]++
				log.Printf("[%s] TX[%s]: ROLLBACK para cidade '%s'. Postos agora: %d. VehicleID: %s",
					enterpriseName, transactionID, cityToRollback, cityAvailablePostsCount[cityToRollback], chosenRoute.VehicleID)
			}
			delete(preparedTransactions, transactionID)
			postsMutex.Unlock()

			log.Printf("[%s] TX[%s]: ABORT SUCESSO. Reserva rejeitada para VehicleID %s.", enterpriseName, transactionID, chosenRoute.VehicleID)
			// Informar o veículo sobre a falha
			publishReservationStatus(chosenRoute.VehicleID, transactionID, "REJECTED", "Falha ao alocar postos em todas as cidades necessárias", &chosenRoute)
			fmt.Println()
		}	
		}
	}()


	router.InitRouter(enterprisePort)
}

// getAvailabilityHandler responde com a disponibilidade de postos em todas as cidades.
func getAvailabilityHandler(c *gin.Context) {
	postsMutex.Lock()
	defer postsMutex.Unlock()

	// Retorna apenas os postos que estão realmente disponíveis (não em fase de PREPARE)
	currentAvailability := make(map[string]int)
	for city, count := range cityAvailablePostsCount {
		currentAvailability[city] = count
	}

	response := gin.H{
		"enterprise":              enterpriseName,
		"all_cities_availability": currentAvailability,
	}
	c.JSON(http.StatusOK, response)
}

// getCityAvailabilityHandler responde com a disponibilidade de postos para uma cidade específica.
func getCityAvailabilityHandler(c *gin.Context) {
	targetCity := c.Param("city") // Pega o parâmetro da URL (ex: /availability/Salvador)

	postsMutex.Lock()
	defer postsMutex.Unlock()

	if availability, ok := cityAvailablePostsCount[targetCity]; ok {
		response := gin.H{
			"enterprise":      enterpriseName,
			"city":            targetCity,
			"available_posts": availability,
		}
		c.JSON(http.StatusOK, response)
	} else {
		c.JSON(http.StatusNotFound, gin.H{
			"enterprise": enterpriseName,
			"error":      fmt.Sprintf("Cidade '%s' não encontrada ou não gerenciada.", targetCity),
		})
	}
}

// Função auxiliar para publicar o status da reserva
func publishReservationStatus(vehicleID, transactionID, status, message string, chosenRoute *schemas.ChosenRouteMsg) {
	topic := fmt.Sprintf("car/reservation/status/%s", vehicleID)
	
	// Criar um payload mais estruturado
	statusPayload := schemas.ReservationStatus{
		TransactionID: transactionID,
		VehicleID:     vehicleID,
		RequestID:     "", // Preencher se disponível no chosenRoute
		Status:        status,
		Message:       message,
	}
	if chosenRoute != nil {
		statusPayload.RequestID = chosenRoute.RequestID
		if status == "CONFIRMED" { // Apenas enviar a rota se confirmada
			statusPayload.ConfirmedRoute = chosenRoute.Route
		}
	}

	payloadBytes, err := json.Marshal(statusPayload)
	if err != nil {
		log.Printf("[%s] TX[%s]: Erro ao serializar ReservationStatus para VehicleID %s: %v", enterpriseName, transactionID, vehicleID, err)
		return
	}
	mqtt.Publish(topic, string(payloadBytes))
	log.Printf("[%s] TX[%s]: Status da reserva '%s' publicado para VehicleID %s no tópico %s.", enterpriseName, transactionID, status, vehicleID, topic)
}

// Função auxiliar para logar detalhes da rota escolhida
func logChosenRouteDetails(transactionID string, chosenRoute schemas.ChosenRouteMsg) {
	log.Printf("[%s] TX[%s]: Detalhes da Rota Escolhida e CONFIRMADA pelo Veículo %s (Request ID: %s):\n",
		enterpriseName, transactionID, chosenRoute.VehicleID, chosenRoute.RequestID)
	for i, segment := range chosenRoute.Route {
		start := segment.ReservationWindow.StartTimeUTC.Local().Format("15:04")
		end := segment.ReservationWindow.EndTimeUTC.Local().Format("15:04")
		date := segment.ReservationWindow.StartTimeUTC.Local().Format("02/01/2006")
		log.Printf("  Segmento %d: Cidade: %s | Janela: %s às %s do dia %s\n",
			i+1, segment.City, start, end, date)
	}
}
