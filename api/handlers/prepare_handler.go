package handlers

import (
	"github.com/4r7hur0/PBL-2/api/config"
	"github.com/4r7hur0/PBL-2/schemas"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func sendJSONResponse(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		// Não tentar escrever novamente no header aqui, pois já foi escrito.
	}
}

// PrepareHandler lida com as requisições /prepare.
func PrepareHandler(w http.ResponseWriter, r *http.Request) {
	currentCompany := config.GetCurrentCompany()
	log.Printf("[%s] Received /prepare request", currentCompany.Name)

	if r.Method != http.MethodPost {
		sendJSONResponse(w, http.StatusMethodNotAllowed, schemas.ErrorResponse{Reason: "Only POST method is allowed"})
		return
	}

	var reqBody schemas.PrepareRequestBody
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reqBody); err != nil {
		log.Printf("[%s] Error decoding /prepare request body: %v", currentCompany.Name, err)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Invalid request body: " + err.Error()})
		return
	}

	// 1. Validação da Requisição
	if reqBody.TransactionID == "" || reqBody.SegmentID == "" || reqBody.ChargingPointID == "" ||
		reqBody.VehicleID == "" || reqBody.ReservationWindow.StartTimeUTC == "" ||
		reqBody.ReservationWindow.EndTimeUTC == "" || reqBody.CoordinatorCallbackURLs.CommitURL == "" ||
		reqBody.CoordinatorCallbackURLs.AbortURL == "" {
		log.Printf("[%s] Missing required fields in /prepare request for TX_ID: %s, SEG_ID: %s", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{
			Status:        schemas.StatusAborted,
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        "Missing required fields.",
		})
		return
	}

	// Validar se este servidor gerencia o charging_point_id
	if !currentCompany.ReservationManager.IsManaged(reqBody.ChargingPointID) {
		reason := fmt.Sprintf("Charging point %s not managed by this server (%s).", reqBody.ChargingPointID, currentCompany.Name)
		log.Printf("[%s] %s TX_ID: %s, SEG_ID: %s", currentCompany.Name, reason, reqBody.TransactionID, reqBody.SegmentID)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{
			Status:        schemas.StatusAborted,
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        reason,
		})
		return
	}

	startTime, errStart := time.Parse(schemas.ISOFormat, reqBody.ReservationWindow.StartTimeUTC)
	endTime, errEnd := time.Parse(schemas.ISOFormat, reqBody.ReservationWindow.EndTimeUTC)

	if errStart != nil || errEnd != nil {
		log.Printf("[%s] Invalid datetime format for TX_ID: %s, SEG_ID: %s. StartErr: %v, EndErr: %v", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID, errStart, errEnd)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{
			Status:        schemas.StatusAborted,
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        "Invalid datetime format. Use ISO 8601 UTC (YYYY-MM-DDTHH:mm:ssZ).",
		})
		return
	}
	if !endTime.After(startTime) {
		log.Printf("[%s] End time must be after start time for TX_ID: %s, SEG_ID: %s", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{
			Status:        schemas.StatusAborted,
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        "End time must be after start time.",
		})
		return
	}


	// Idempotência: Verificar se já estamos preparados para esta transação/segmento
	// O TransactionLogger e ReservationManager agora lidam com a lógica de lock e idempotência internamente.
	if txState, found := currentCompany.TransactionLogger.GetState(reqBody.TransactionID, reqBody.SegmentID); found {
		if txState.Status == schemas.StatusPrepared {
			log.Printf("[%s] Idempotency: Already prepared for TX_ID: %s, SEG_ID: %s. Responding PREPARED.", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
			preparedUntil := time.Now().Add(5 * time.Minute).UTC().Format(schemas.ISOFormat)
			sendJSONResponse(w, http.StatusOK, schemas.PrepareSuccessResponse{
				Status:           schemas.StatusPrepared,
				TransactionID:    reqBody.TransactionID,
				SegmentID:        reqBody.SegmentID,
				PreparedUntilUTC: preparedUntil,
			})
			return
		}
		// Se o estado for COMMITTED ou ABORTED, não podemos preparar novamente.
		if txState.Status == schemas.StatusCommitted || txState.Status == schemas.StatusAborted {
			log.Printf("[%s] Idempotency: Cannot prepare TX_ID: %s, SEG_ID: %s. Already in terminal state: %s.", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID, txState.Status)
			// Retorna um erro indicando que a transação já foi finalizada.
			sendJSONResponse(w, http.StatusConflict, schemas.ErrorResponse{
				Status:        txState.Status, // Retorna o estado final atual
				TransactionID: reqBody.TransactionID,
				SegmentID:     reqBody.SegmentID,
				Reason:        fmt.Sprintf("Transaction segment already finalized with status: %s", txState.Status),
			})
			return
		}
	}

	// 2. Verificação de Disponibilidade e Regras de Negócio
	// MakeProvisionalReservation agora também verifica idempotência internamente
	available, reason := currentCompany.ReservationManager.MakeProvisionalReservation(reqBody)
	if !available {
		log.Printf("[%s] Resource %s unavailable or conflict for TX_ID: %s, SEG_ID: %s. Reason: %s", currentCompany.Name, reqBody.ChargingPointID, reqBody.TransactionID, reqBody.SegmentID, reason)
		// Se a razão for de idempotência, o status code pode ser OK, mas aqui tratamos como conflito genérico.
		// A lógica no ReservationManager já retorna true para idempotência, então este bloco só é atingido por conflitos reais.
		sendJSONResponse(w, http.StatusConflict, schemas.ErrorResponse{
			Status:        schemas.StatusAborted,
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        reason,
		})
		return
	}

	// 3. Se puder reservar (VOTO_COMMIT simulado)
	// Persistência Durável (Log de Transação do Participante)
	// Loga o estado PREPARED apenas se a reserva provisória foi criada com sucesso (available=true)
	currentCompany.TransactionLogger.LogState(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusPrepared, reqBody, reqBody)

	preparedUntil := time.Now().Add(5 * time.Minute).UTC().Format(schemas.ISOFormat)
	successResp := schemas.PrepareSuccessResponse{
		Status:           schemas.StatusPrepared,
		TransactionID:    reqBody.TransactionID,
		SegmentID:        reqBody.SegmentID,
		PreparedUntilUTC: preparedUntil,
	}
	log.Printf("[%s] Responding PREPARED for TX_ID: %s, SEG_ID: %s", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
	sendJSONResponse(w, http.StatusOK, successResp)
}
