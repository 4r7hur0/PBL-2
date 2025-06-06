package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/4r7hur0/PBL-2/api/mqtt"
	"github.com/4r7hur0/PBL-2/api/router"
	"github.com/4r7hur0/PBL-2/api/state"
	rc "github.com/4r7hur0/PBL-2/registry/registry_client"
	"github.com/4r7hur0/PBL-2/schemas"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	// Variáveis globais para a configuração desta instância da API
	enterpriseName  string
	enterprisePort  string
	ownedCity       string
	postsQuantity   int
	stateMgr        *state.StateManager
	allSystemCities []string
	registryClient  *rc.RegistryClient // Cliente do Registry

)

func main() {

	enterpriseName := os.Getenv("ENTERPRISE_NAME")
	enterprisePort := os.Getenv("ENTERPRISE_PORT")
	postsQuantityStr := os.Getenv("POSTS_QUANTITY")
	ownedCity = os.Getenv("OWNED_CITY")
	registryURL := os.Getenv("REGISTRY_URL") // Ex: http://localhost:9000

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
		postsQuantity = 5
	} else {
		var err error
		postsQuantity, err = strconv.Atoi(postsQuantityStr)
		if err != nil {
			log.Printf("Erro ao converter POSTS_QUANTITY: %v. Usando 5.", err)
			postsQuantity = 5
		}
	}

	log.Printf("Iniciando API para a empresa: %s na porta %s, gerenciando a cidade: %s com %d postos.", enterpriseName, enterprisePort, ownedCity, postsQuantity)

	// Inicializar o StateManager APENAS para a cidade que esta API possui
	stateMgr = state.NewStateManager(ownedCity, postsQuantity)

	// Inicializar e usar o Registry Client
	registryClient := rc.NewRegistryClient(registryURL)
  
	myAPIURL := fmt.Sprintf("http://%v:%s", enterpriseName, enterprisePort) // Ajuste se estiver atrás de um proxy ou em rede Docker diferente

	err := registryClient.RegisterService(enterpriseName, ownedCity, myAPIURL)
	if err != nil {
		log.Fatalf("[%s] Falha ao registrar no Registry: %v", enterpriseName, err)
	} else {
		log.Printf("[%s] Registrado com sucesso no Registry como gerenciador de '%s' em %s", enterpriseName, ownedCity, myAPIURL)
	}

	allSystemCities = []string{"Salvador", "Feira de Santana", "Ilheus"}

	// Inicializar MQTT

	mqtt.InitializeMQTT("tcp://mosquitto:1883")
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
				possibleRoutes = router.GeneratePossibleRoutes(routeReq.Origin, routeReq.Destination, allSystemCities)
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
		}
	}()

	// Goroutine para processar a rota escolhida pelo carro
	go func() {
		for messagePayload := range chosenRouteMessageChannel {
			transactionID := uuid.New().String()

			fmt.Printf("[%s] TX[%s] Mensagem de ROTA ESCOLHIDA recebida no tópico '%s': %s\n", enterpriseName, transactionID, chosenRouteTopic, messagePayload)
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
				publishReservationStatus(chosenRoute.VehicleID, transactionID, "REJECTED", "Rota escolhida estava vazia", nil, enterpriseName)

				continue
			}
			// Fase de PREPARE
			preparedParticipants := make(map[string]string) // cidade -> "local" ou URL da API remota
			prepareOverallSuccess := true

			for _, segment := range chosenRoute.Route {
				cityToReserve := segment.City
				windowToReserve := segment.ReservationWindow

				if cityToReserve == ownedCity { // Reserva LOCAL
					log.Printf("[%s] TX[%s]: Iniciando PREPARE LOCAL para %s em %s", enterpriseName, transactionID, chosenRoute.VehicleID, cityToReserve)
					success, err := stateMgr.PrepareReservation(transactionID, chosenRoute.VehicleID, chosenRoute.RequestID, windowToReserve)
					if !success || err != nil {
						log.Printf("[%s] TX[%s]: FALHA PREPARE LOCAL para %s: %v", enterpriseName, transactionID, cityToReserve, err)
						prepareOverallSuccess = false
						break
					}
					log.Printf("[%s] TX[%s]: SUCESSO PREPARE LOCAL para %s", enterpriseName, transactionID, cityToReserve)
					preparedParticipants[cityToReserve] = "local"
				} else { // Reserva REMOTA
					log.Printf("[%s] TX[%s]: Descobrindo API para cidade remota '%s'", enterpriseName, transactionID, cityToReserve)
					discoveredService, err_discover := registryClient.DiscoverService(cityToReserve)
					if err_discover != nil || !discoveredService.Found {
						log.Printf("[%s] TX[%s]: FALHA ao descobrir API para cidade remota '%s': %v. Found: %v", enterpriseName, transactionID, cityToReserve, err_discover, discoveredService.Found)
						prepareOverallSuccess = false
						break
					}
					remoteAPIURL := discoveredService.ApiURL
					log.Printf("[%s] TX[%s]: Iniciando PREPARE REMOTO para %s em %s (API: %s)", enterpriseName, transactionID, chosenRoute.VehicleID, cityToReserve, remoteAPIURL)

					remoteReqPayload := schemas.RemotePrepareRequest{
						TransactionID:     transactionID,
						VehicleID:         chosenRoute.VehicleID,
						RequestID:         chosenRoute.RequestID,
						City:              cityToReserve, // Importante: enviar a cidade correta
						ReservationWindow: windowToReserve,
					}
					payloadBytes, _ := json.Marshal(remoteReqPayload)

					httpClient := &http.Client{Timeout: time.Second * 10} // Adicionar timeout
					resp, httpErr := httpClient.Post(fmt.Sprintf("%s/2pc_remote/prepare", remoteAPIURL), "application/json", bytes.NewBuffer(payloadBytes))

					if httpErr != nil {
						log.Printf("[%s] TX[%s]: ERRO HTTP no PREPARE REMOTO para %s: %v", enterpriseName, transactionID, cityToReserve, httpErr)
						prepareOverallSuccess = false
						break
					}

					var remoteResp schemas.RemotePrepareResponse
					bodyBytes, _ := io.ReadAll(resp.Body)
					resp.Body.Close() // Fechar o corpo

					if err := json.Unmarshal(bodyBytes, &remoteResp); err != nil {
						log.Printf("[%s] TX[%s]: Erro ao deserializar resposta PREPARE REMOTO de %s (Status: %s, Corpo: %s): %v", enterpriseName, transactionID, cityToReserve, resp.Status, string(bodyBytes), err)
						prepareOverallSuccess = false
						break
					}

					if resp.StatusCode == http.StatusOK && remoteResp.Status == schemas.StatusReservationPrepared {
						log.Printf("[%s] TX[%s]: SUCESSO PREPARE REMOTO para %s", enterpriseName, transactionID, cityToReserve)
						preparedParticipants[cityToReserve] = remoteAPIURL
					} else {
						log.Printf("[%s] TX[%s]: FALHA PREPARE REMOTO para %s. Status: %s, Resposta: %+v", enterpriseName, transactionID, cityToReserve, resp.Status, remoteResp)
						prepareOverallSuccess = false
						break
					}
				}
			}

			// Fase de COMMIT ou ABORT
			if prepareOverallSuccess {
				log.Printf("[%s] TX[%s]: FASE DE PREPARAÇÃO GLOBAL SUCESSO. Iniciando COMMIT.", enterpriseName, transactionID)
				for city, participantTypeOrURL := range preparedParticipants {
					if participantTypeOrURL == "local" {
						stateMgr.CommitReservation(transactionID)
						log.Printf("[%s] TX[%s]: COMMIT LOCAL para %s", enterpriseName, transactionID, city)
					} else {
						// Enviar COMMIT REMOTO
						log.Printf("[%s] TX[%s]: Enviando COMMIT REMOTO para %s (API: %s)", enterpriseName, transactionID, city, participantTypeOrURL)
						remoteCmdPayload := schemas.RemoteCommitAbortRequest{TransactionID: transactionID}
						payloadBytes, _ := json.Marshal(remoteCmdPayload)
						httpClient := &http.Client{Timeout: time.Second * 10}
						resp, httpErr := httpClient.Post(fmt.Sprintf("%s/2pc_remote/commit", participantTypeOrURL), "application/json", bytes.NewBuffer(payloadBytes))
						if httpErr != nil {
							log.Printf("[%s] TX[%s]: ERRO HTTP no COMMIT REMOTO para %s: %v. A transação pode ficar inconsistente.", enterpriseName, transactionID, city, httpErr)
						} else {
							if resp.StatusCode != http.StatusOK {
								bodyBytes, _ := io.ReadAll(resp.Body)
								log.Printf("[%s] TX[%s]: AVISO - COMMIT REMOTO para %s falhou. Status: %s, Corpo: %s. A transação pode ficar inconsistente.", enterpriseName, transactionID, city, resp.Status, string(bodyBytes))
								resp.Body.Close()
							} else {
								resp.Body.Close()
								log.Printf("[%s] TX[%s]: COMMIT REMOTO para %s enviado com sucesso.", enterpriseName, transactionID, city)
							}
						}
					}
				}
				publishReservationStatus(chosenRoute.VehicleID, transactionID, "CONFIRMED", "Reserva confirmada com sucesso", &chosenRoute, enterpriseName)
			} else {
				log.Printf("[%s] TX[%s]: FASE DE PREPARAÇÃO GLOBAL FALHOU. Iniciando ABORT.", enterpriseName, transactionID)
				for city, participantTypeOrURL := range preparedParticipants { // Abortar apenas os que foram preparados
					if participantTypeOrURL == "local" {
						stateMgr.AbortReservation(transactionID)
						log.Printf("[%s] TX[%s]: ABORT LOCAL para %s", enterpriseName, transactionID, city)
					} else {
						// Enviar ABORT REMOTO
						log.Printf("[%s] TX[%s]: Enviando ABORT REMOTO para %s (API: %s)", enterpriseName, transactionID, city, participantTypeOrURL)
						// ... (lógica de chamada HTTP POST para /2pc_remote/abort, similar ao commit) ...
						remoteCmdPayload := schemas.RemoteCommitAbortRequest{TransactionID: transactionID}
						payloadBytes, _ := json.Marshal(remoteCmdPayload)
						httpClient := &http.Client{Timeout: time.Second * 10}
						resp, httpErr := httpClient.Post(fmt.Sprintf("%s/2pc_remote/abort", participantTypeOrURL), "application/json", bytes.NewBuffer(payloadBytes))
						if httpErr != nil {
							log.Printf("[%s] TX[%s]: ERRO HTTP no ABORT REMOTO para %s: %v.", enterpriseName, transactionID, city, httpErr)
						} else {
							resp.Body.Close() // Sempre fechar
							log.Printf("[%s] TX[%s]: ABORT REMOTO para %s enviado. Status: %s", enterpriseName, transactionID, city, resp.Status)
						}
					}
				}
				publishReservationStatus(chosenRoute.VehicleID, transactionID, "REJECTED", "Falha ao alocar postos necessários ou conflito de reserva", &chosenRoute, enterpriseName)
			}
		}
	}()

	// Goroutine para verificar e encerrar reservas
		go func() {
				ticker := time.NewTicker(10 * time.Second) // Verificar a cada 10 segundos
				defer ticker.Stop()
				for range ticker.C {
						stateMgr.CheckAndEndReservations()
				}
		}()

	// Configurar e iniciar o servidor Gin (HTTP)
	r := gin.Default()
	setupRouter(r, stateMgr, enterpriseName) // Passar dependências
	log.Printf("[%s] Servidor HTTP escutando na porta %s", enterpriseName, enterprisePort)
	if err := r.Run(":" + enterprisePort); err != nil {
		log.Fatalf("Falha ao iniciar o servidor Gin: %v", err)
	}

}

// setupRouter configura as rotas HTTP, incluindo os endpoints para 2PC remoto
func setupRouter(r *gin.Engine, sm *state.StateManager, entName string) {
	// Exemplo de endpoint de status da cidade gerenciada
	r.GET("/status", func(c *gin.Context) {
		cName, maxP, activeR := sm.GetCityAvailability()
		c.JSON(http.StatusOK, gin.H{
			"enterprise":          enterpriseName,
			"managed_city":        cName,
			"max_posts":           maxP,
			"active_reservations": activeR,
		})
	})

	// Endpoints para serem chamados por outras APIs (participantes remotos do 2PC)
	remoteGroup := r.Group("/2pc_remote")
	{
		remoteGroup.POST("/prepare", func(c *gin.Context) {
			handleRemotePrepare(c, sm, entName)
		})
		remoteGroup.POST("/commit", func(c *gin.Context) {
			handleRemoteCommit(c, sm, entName)
		})
		remoteGroup.POST("/abort", func(c *gin.Context) {
			handleRemoteAbort(c, sm, entName)
		})
	}
}

// Handlers para os endpoints /2pc_remote/* (podem ficar aqui ou em um arquivo separado)

func handleRemotePrepare(c *gin.Context, sm *state.StateManager, localEntName string) {
	var req schemas.RemotePrepareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, schemas.RemotePrepareResponse{Status: "REJECTED", TransactionID: req.TransactionID, Reason: "Payload inválido: " + err.Error()})
		return
	}
	// Validação importante: esta API deve ser a "dona" da req.City
	if req.City != ownedCity { // ownedCity é a variável global desta instância
		errMsg := fmt.Sprintf("Requisição de PREPARE REMOTO para cidade %s, mas esta API gerencia %s", req.City, ownedCity)
		log.Printf("[%s] TX[%s]: %s", localEntName, req.TransactionID, errMsg)
		c.JSON(http.StatusBadRequest, schemas.RemotePrepareResponse{Status: "REJECTED", TransactionID: req.TransactionID, Reason: errMsg})
		return
	}

	log.Printf("[%s] TX[%s]: Recebido PREPARE REMOTO para VehicleID %s na cidade %s", localEntName, req.TransactionID, req.VehicleID, req.City)
	success, err := sm.PrepareReservation(req.TransactionID, req.VehicleID, req.RequestID, req.ReservationWindow) // Passa a janela

	if !success || err != nil {
		log.Printf("[%s] TX[%s]: FALHA PREPARE REMOTO (interno): %v", localEntName, req.TransactionID, err)
		c.JSON(http.StatusConflict, schemas.RemotePrepareResponse{Status: "REJECTED", TransactionID: req.TransactionID, Reason: err.Error()})
		return
	}
	log.Printf("[%s] TX[%s]: SUCESSO PREPARE REMOTO (interno)", localEntName, req.TransactionID)
	c.JSON(http.StatusOK, schemas.RemotePrepareResponse{Status: schemas.StatusReservationPrepared, TransactionID: req.TransactionID})
}

func handleRemoteCommit(c *gin.Context, sm *state.StateManager, localEntName string) {
	var req schemas.RemoteCommitAbortRequest
	if err := c.ShouldBindJSON(&req); err != nil { /* ... erro ... */
		return
	}
	log.Printf("[%s] TX[%s]: Recebido COMMIT REMOTO", localEntName, req.TransactionID)
	sm.CommitReservation(req.TransactionID)
	c.JSON(http.StatusOK, gin.H{"status": schemas.StatusReservationCommitted, "transaction_id": req.TransactionID})
}

func handleRemoteAbort(c *gin.Context, sm *state.StateManager, localEntName string) {
	var req schemas.RemoteCommitAbortRequest
	if err := c.ShouldBindJSON(&req); err != nil { /* ... erro ... */
		return
	}
	log.Printf("[%s] TX[%s]: Recebido ABORT REMOTO", localEntName, req.TransactionID)
	sm.AbortReservation(req.TransactionID)
	c.JSON(http.StatusOK, gin.H{"status": "ABORTED", "transaction_id": req.TransactionID})
}

// Função auxiliar para publicar o status da reserva (ajustada para incluir enterpriseName nos logs)
func publishReservationStatus(vehicleID, transactionID, status, message string, chosenRoute *schemas.ChosenRouteMsg, pubEnterpriseName string) {
	topic := fmt.Sprintf("car/reservation/status/%s", vehicleID)
	statusPayload := schemas.ReservationStatus{
		TransactionID: transactionID,
		VehicleID:     vehicleID,
		Status:        status,
		Message:       message,
	}
	if chosenRoute != nil {
		statusPayload.RequestID = chosenRoute.RequestID
		if status == schemas.StatusConfirmed { // Usar constante
			statusPayload.ConfirmedRoute = chosenRoute.Route
		}
	}
	payloadBytes, _ := json.Marshal(statusPayload)
	mqtt.Publish(topic, string(payloadBytes))
	log.Printf("[%s] TX[%s]: Status da reserva '%s' publicado para VehicleID %s no tópico %s.", pubEnterpriseName, transactionID, status, vehicleID, topic)
}
