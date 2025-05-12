package coordinator

import (
	"github.com/4r7hur0/PBL-2/api/config"
	"github.com/4r7hur0/PBL-2/api/mqtt" 
	"github.com/4r7hur0/PBL-2/schemas"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	mqtt_lib "github.com/eclipse/paho.mqtt.golang" // Importando o pacote MQTT
	)

// SegmentResult armazena o resultado da fase de prepare para um segmento.
type SegmentResult struct {
	SegmentID string
	Success   bool
	Response  schemas.PrepareSuccessResponse // Usa schema
	Error     error
	Company   config.CompanyInfo // Informação da empresa participante
}

// HandleRouteReservationRequest é chamado quando uma mensagem MQTT de reserva de rota é recebida.
// Esta função atua como o início da lógica do Coordenador.
// Recebe o cliente MQTT como argumento.
func HandleRouteReservationRequest(payload []byte, mqttClient mqtt_lib.Client) {
	currentCompany := config.GetCurrentCompany() // A empresa que recebeu a mensagem MQTT e atua como coordenador
	log.Printf("[%s Coordinator] Handling route reservation request...", currentCompany.Name)

	var request schemas.RouteReservationRequest // Usa schema
	if err := json.Unmarshal(payload, &request); err != nil {
		log.Printf("[%s Coordinator] Error unmarshalling route reservation request: %v. Payload: %s", currentCompany.Name, err, string(payload))
		// Enviar resposta de erro via MQTT
		responseTopic := fmt.Sprintf("reservations/response/%s/%s", request.VehicleID, request.RequestID)
		errorPayload := fmt.Sprintf(`{"request_id": "%s", "status": "ROUTE_FAILED", "reason": "Invalid request format"}`, request.RequestID)
		_ = mqtt.PublishMessage(mqttClient, responseTopic, errorPayload)
		return
	}

	if len(request.Route) == 0 {
		log.Printf("[%s Coordinator] Empty route in request ID: %s", currentCompany.Name, request.RequestID)
		// Enviar resposta de erro via MQTT
		responseTopic := fmt.Sprintf("reservations/response/%s/%s", request.VehicleID, request.RequestID)
		errorPayload := fmt.Sprintf(`{"request_id": "%s", "status": "ROUTE_FAILED", "reason": "Empty route provided"}`, request.RequestID)
		_ = mqtt.PublishMessage(mqttClient, responseTopic, errorPayload)
		return
	}

	transactionID := "tx-" + uuid.New().String()
	log.Printf("[%s Coordinator] Starting 2PC for TX_ID: %s, RequestID: %s, Vehicle: %s, Route Segments: %d",
		currentCompany.Name, transactionID, request.RequestID, request.VehicleID, len(request.Route))

	var wg sync.WaitGroup
	// Usar um mapa para resultados para facilitar o acesso pelo segmentID
	resultsMap := make(map[string]SegmentResult)
	var mapMutex sync.Mutex // Mutex para proteger o mapa de resultados

	allPrepared := true // Assume sucesso inicialmente

	// Fase 1: Prepare
	for i, segment := range request.Route {
		wg.Add(1)
		// Usar um ID de segmento mais previsível se necessário, mas UUID é mais único
		segmentID := fmt.Sprintf("seg-%s-%d", transactionID, i+1)

		// Determinar qual empresa gerencia este charging_point_id e obter sua URL base
		participantCompanyInfo, found := config.GetCompanyInfoByChargingPoint(segment.ChargingPointID)
		if !found {
			err := fmt.Errorf("API URL/Info not found for charging point %s", segment.ChargingPointID)
			log.Printf("[%s Coordinator] %v. Aborting TX_ID: %s", currentCompany.Name, err, transactionID)
			mapMutex.Lock()
			resultsMap[segmentID] = SegmentResult{SegmentID: segmentID, Success: false, Error: err}
			allPrepared = false // Marca falha
			mapMutex.Unlock()
			wg.Done()
			continue // Pula para o próximo segmento
		}
		participantAPIURL := participantCompanyInfo.APIBaseURL


		preparePayload := schemas.PrepareRequestBody{ // Usa schema
			TransactionID:     transactionID,
			SegmentID:         segmentID,
			ChargingPointID:   segment.ChargingPointID,
			VehicleID:         request.VehicleID,
			ReservationWindow: segment.ReservationWindow,
			CoordinatorCallbackURLs: schemas.CoordinatorCallbackURLs{ // Usa schema
				// Os callbacks devem apontar para endpoints no Coordenador (esta instância)
				// Estes endpoints precisam ser implementados no router.go e handlers.
				CommitURL: fmt.Sprintf("%s/transaction/%s/segment/%s/commit_ack", currentCompany.APIBaseURL, transactionID, segmentID),
				AbortURL:  fmt.Sprintf("%s/transaction/%s/segment/%s/abort_ack", currentCompany.APIBaseURL, transactionID, segmentID),
			},
			// Adicionar detalhes de prioridade se necessário
			// PriorityDetails: schemas.PriorityDetails{...},
		}

		// Executa a chamada prepare em uma goroutine separada
		go func(pAPIURL string, pPayload schemas.PrepareRequestBody, pCompany config.CompanyInfo) {
			defer wg.Done()
			log.Printf("[%s Coordinator] Sending /prepare to %s (%s) for CP: %s (TX_ID: %s, SEG_ID: %s)",
				currentCompany.Name, pCompany.Name, pAPIURL, pPayload.ChargingPointID, transactionID, pPayload.SegmentID)

			jsonData, err := json.Marshal(pPayload)
			if err != nil {
				err = fmt.Errorf("failed to marshal prepare payload for %s: %w", pPayload.SegmentID, err)
				mapMutex.Lock()
				resultsMap[pPayload.SegmentID] = SegmentResult{SegmentID: pPayload.SegmentID, Success: false, Error: err, Company: pCompany}
				allPrepared = false
				mapMutex.Unlock()
				return
			}

			// Adicionar timeout à requisição HTTP
			client := http.Client{Timeout: 15 * time.Second} // Timeout de 15 segundos
			resp, err := client.Post(pAPIURL+"/prepare", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				err = fmt.Errorf("failed to call /prepare on %s (%s): %w", pCompany.Name, pAPIURL, err)
				mapMutex.Lock()
				resultsMap[pPayload.SegmentID] = SegmentResult{SegmentID: pPayload.SegmentID, Success: false, Error: err, Company: pCompany}
				allPrepared = false
				mapMutex.Unlock()
				return
			}
			defer resp.Body.Close()

			var result SegmentResult
			result.SegmentID = pPayload.SegmentID
			result.Company = pCompany

			if resp.StatusCode == http.StatusOK {
				var prepResp schemas.PrepareSuccessResponse // Usa schema
				if err := json.NewDecoder(resp.Body).Decode(&prepResp); err != nil {
					result.Success = false
					result.Error = fmt.Errorf("/prepare decode error from %s (%s): %w", pCompany.Name, pAPIURL, err)
					allPrepared = false
				} else {
					result.Success = true
					result.Response = prepResp
					log.Printf("[%s Coordinator] PREPARE successful for SEG_ID: %s from %s", currentCompany.Name, pPayload.SegmentID, pCompany.Name)
				}
			} else {
				var errResp schemas.ErrorResponse // Usa schema
				_ = json.NewDecoder(resp.Body).Decode(&errResp) // Tenta decodificar, mas o erro principal é o status code
				errMsg := fmt.Sprintf("/prepare failed on %s (%s) with status %d. Reason: %s", pCompany.Name, pAPIURL, resp.StatusCode, errResp.Reason)
				log.Printf("[%s Coordinator] %s (TX_ID: %s, SEG_ID: %s)", currentCompany.Name, errMsg, transactionID, pPayload.SegmentID)
				result.Success = false
				result.Error = fmt.Errorf(errMsg)
				allPrepared = false
			}
			mapMutex.Lock()
			resultsMap[result.SegmentID] = result
			mapMutex.Unlock()

		}(participantAPIURL, preparePayload, participantCompanyInfo)
	}

	wg.Wait() // Espera todas as goroutines /prepare terminarem

	// Coleta os resultados (já estão no mapa)
	preparedSegmentsList := []SegmentResult{}
	mapMutex.Lock() // Trava para leitura segura do mapa
	for _, result := range resultsMap {
		preparedSegmentsList = append(preparedSegmentsList, result)
		if !result.Success {
			// Log adicional de falha na preparação
			log.Printf("[%s Coordinator] Segment %s (Company: %s) failed prepare phase. Error: %v. TX_ID: %s",
				currentCompany.Name, result.SegmentID, result.Company.Name, result.Error, transactionID)
		}
	}
	mapMutex.Unlock() // Libera o lock

	// Fase 2: Commit ou Abort
	finalStatus := ""
	reason := ""

	if allPrepared && len(request.Route) > 0 {
		log.Printf("[%s Coordinator] All %d segments PREPARED for TX_ID: %s. Proceeding to COMMIT.", currentCompany.Name, len(preparedSegmentsList), transactionID)
		finalStatus = "ROUTE_CONFIRMED"
		// Enviar /commit para todos os participantes que retornaram sucesso no prepare
		// (neste ponto, todos em preparedSegmentsList deveriam ter Success = true)
		for _, segResult := range preparedSegmentsList {
			// Não precisa verificar Success aqui se allPrepared é true
			go sendCommitOrAbort(segResult.Company.APIBaseURL, transactionID, segResult.SegmentID, "commit", currentCompany.Name)
		}

	} else {
		log.Printf("[%s Coordinator] Not all segments prepared for TX_ID: %s. Proceeding to ABORT.", currentCompany.Name, transactionID)
		finalStatus = "ROUTE_FAILED"
		// Coleta as razões das falhas
		var errorReasons []string
		for _, segResult := range preparedSegmentsList {
			if !segResult.Success && segResult.Error != nil {
				errorReasons = append(errorReasons, fmt.Sprintf("Segment %s (%s): %s", segResult.SegmentID, segResult.Company.Name, segResult.Error.Error()))
			}
		}
		if len(errorReasons) > 0 {
			reason = strings.Join(errorReasons, "; ")
		} else {
			reason = "Unknown reason for abort (possibly empty route or initial error)."
		}


		// Enviar /abort para todos os participantes que foram contatados e podem ter se preparado
		// (ou seja, aqueles para os quais a chamada /prepare foi enviada, mesmo que tenha falhado depois)
		for _, segResult := range preparedSegmentsList {
			// Envia abort mesmo se o prepare falhou, para garantir limpeza no participante se ele chegou a preparar e falhou ao responder.
			// Uma lógica mais complexa poderia verificar o erro específico.
			// Só não envia abort se não conseguimos nem encontrar a URL da API.
			if segResult.Company.APIBaseURL != "" {
				go sendCommitOrAbort(segResult.Company.APIBaseURL, transactionID, segResult.SegmentID, "abort", currentCompany.Name)
			}
		}
	}

	// Enviar resposta final para o cliente via MQTT
	responseTopic := fmt.Sprintf("reservations/response/%s/%s", request.VehicleID, request.RequestID)
	responsePayloadMap := map[string]string{
		"request_id":    request.RequestID,
		"transaction_id": transactionID,
		"status":         finalStatus,
	}
	if reason != "" {
		responsePayloadMap["reason"] = reason
	}
	responsePayloadBytes, _ := json.Marshal(responsePayloadMap)
	err := mqtt.PublishMessage(mqttClient, responseTopic, string(responsePayloadBytes))
	if err != nil {
		log.Printf("[%s Coordinator] Failed to publish final response to MQTT topic %s: %v", currentCompany.Name, responseTopic, err)
	} else {
		log.Printf("[%s Coordinator] Published final response to %s: %s", currentCompany.Name, responseTopic, string(responsePayloadBytes))
	}
}

// sendCommitOrAbort envia a requisição de commit ou abort para um participante.
func sendCommitOrAbort(participantAPIURL, transactionID, segmentID, action, coordinatorName string) {
	// Pequeno delay aleatório para evitar thundering herd
	// time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

	if participantAPIURL == "" {
		log.Printf("[%s Coordinator] Cannot send /%s for TX_ID: %s, SEG_ID: %s. Participant API URL is empty.", coordinatorName, action, transactionID, segmentID)
		return
	}

	endpoint := fmt.Sprintf("%s/%s", participantAPIURL, action) // action é "commit" ou "abort"
	payload := map[string]string{
		"transaction_id": transactionID,
		"segment_id":     segmentID,
	}
	jsonData, _ := json.Marshal(payload)

	log.Printf("[%s Coordinator] Sending /%s to %s for TX_ID: %s, SEG_ID: %s", coordinatorName, action, participantAPIURL, transactionID, segmentID)

	// Adicionar timeout e retentativas simples pode ser útil aqui
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))

	if err != nil {
		log.Printf("[%s Coordinator] Error sending /%s to %s for TX_ID: %s, SEG_ID: %s. Error: %v", coordinatorName, action, participantAPIURL, transactionID, segmentID, err)
		// TODO: Implementar lógica de retentativa para commit/abort é crucial em produção
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("[%s Coordinator] Successfully sent /%s to %s for TX_ID: %s, SEG_ID: %s. Participant Status: %d", coordinatorName, action, participantAPIURL, transactionID, segmentID, resp.StatusCode)
	} else {
		// Ler corpo da resposta de erro, se houver
		var errResp schemas.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		log.Printf("[%s Coordinator] Failed to send /%s to %s for TX_ID: %s, SEG_ID: %s. Participant Status: %d, Reason: %s", coordinatorName, action, participantAPIURL, transactionID, segmentID, resp.StatusCode, errResp.Reason)
		// TODO: Lidar com falhas no commit/abort (retentativas, logs de erro críticos, mecanismos de compensação se necessário)
	}
}
