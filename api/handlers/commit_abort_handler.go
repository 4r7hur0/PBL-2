package handlers

import (
	"github.com/4r7hur0/PBL-2/api/config"
	"github.com/4r7hur0/PBL-2/schemas"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// CommitHandler lida com as requisições /commit.
func CommitHandler(w http.ResponseWriter, r *http.Request) {
	currentCompany := config.GetCurrentCompany()
	log.Printf("[%s] Received /commit request", currentCompany.Name)

	if r.Method != http.MethodPost {
		sendJSONResponse(w, http.StatusMethodNotAllowed, schemas.ErrorResponse{Reason: "Only POST method is allowed"})
		return
	}

	var reqBody struct { // Corpo da requisição simples para commit/abort
		TransactionID string `json:"transaction_id"`
		SegmentID     string `json:"segment_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Printf("[%s] Error decoding /commit request body: %v", currentCompany.Name, err)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Invalid request body"})
		return
	}

	if reqBody.TransactionID == "" || reqBody.SegmentID == "" {
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Missing transaction_id or segment_id"})
		return
	}

	// --- Início Seção Crítica (simplificada com lock no logger/manager) ---
	txState, found := currentCompany.TransactionLogger.GetState(reqBody.TransactionID, reqBody.SegmentID)

	// Lógica de Idempotência e Estado
	if found && txState.Status == schemas.StatusCommitted {
		log.Printf("[%s] Idempotency: Segment TX_ID %s, SEG_ID %s already committed.", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
		sendJSONResponse(w, http.StatusOK, map[string]string{"status": schemas.StatusCommitted, "transaction_id": reqBody.TransactionID, "segment_id": reqBody.SegmentID})
		return
	}

	if !found || txState.Status != schemas.StatusPrepared {
		reason := fmt.Sprintf("Transaction segment TX_ID %s, SEG_ID %s not found or not in PREPARED state. Current state: %s", reqBody.TransactionID, reqBody.SegmentID, txState.Status)
		log.Printf("[%s] %s", currentCompany.Name, reason)
		// Se já foi abortado, não pode comitar.
		statusCode := http.StatusConflict
		respStatus := schemas.StatusError
		if found {
			respStatus = txState.Status // Retorna o estado atual se encontrado
		}
		sendJSONResponse(w, statusCode, schemas.ErrorResponse{Status: respStatus, Reason: reason, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID})
		return
	}

	// Efetivar a reserva (Mudar status para CONFIRMED)
	updated := currentCompany.ReservationManager.UpdateReservationStatus(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusConfirmed)
	if !updated {
		// Isso indica uma inconsistência entre o log de transação e o gerenciador de reservas.
		reason := fmt.Sprintf("Inconsistency: Failed to find provisional reservation to update status to CONFIRMED for TX_ID %s, SEG_ID %s, but transaction log was PREPARED.", reqBody.TransactionID, reqBody.SegmentID)
		log.Printf("[%s] CRITICAL: %s", currentCompany.Name, reason)
		// Logar o erro, mas ainda tentar marcar como COMMITTED no log de transação para refletir a intenção.
		currentCompany.TransactionLogger.LogState(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusCommitted, reason, txState.ReservationData) // Logar como COMMITTED mesmo com erro na reserva
		sendJSONResponse(w, http.StatusInternalServerError, schemas.ErrorResponse{Status: schemas.StatusError, Reason: reason, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID})
		return
	}

	// Logar o estado final COMMITTED
	currentCompany.TransactionLogger.LogState(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusCommitted, "Reservation confirmed", txState.ReservationData)
	log.Printf("[%s] COMMITTED segment TX_ID: %s, SEG_ID: %s", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
	sendJSONResponse(w, http.StatusOK, map[string]string{"status": schemas.StatusCommitted, "transaction_id": reqBody.TransactionID, "segment_id": reqBody.SegmentID})
	// --- Fim Seção Crítica ---
}

// AbortHandler lida com as requisições /abort.
func AbortHandler(w http.ResponseWriter, r *http.Request) {
	currentCompany := config.GetCurrentCompany()
	log.Printf("[%s] Received /abort request", currentCompany.Name)

	if r.Method != http.MethodPost {
		sendJSONResponse(w, http.StatusMethodNotAllowed, schemas.ErrorResponse{Reason: "Only POST method is allowed"})
		return
	}
	var reqBody struct {
		TransactionID string `json:"transaction_id"`
		SegmentID     string `json:"segment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Printf("[%s] Error decoding /abort request body: %v", currentCompany.Name, err)
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Invalid request body"})
		return
	}
	if reqBody.TransactionID == "" || reqBody.SegmentID == "" {
		sendJSONResponse(w, http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Missing transaction_id or segment_id"})
		return
	}

	// --- Início Seção Crítica ---
	txState, found := currentCompany.TransactionLogger.GetState(reqBody.TransactionID, reqBody.SegmentID)

	// Lógica de Idempotência e Estado
	if found && txState.Status == schemas.StatusAborted {
		log.Printf("[%s] Idempotency: Segment TX_ID %s, SEG_ID %s already aborted.", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
		sendJSONResponse(w, http.StatusOK, map[string]string{"status": schemas.StatusAborted, "transaction_id": reqBody.TransactionID, "segment_id": reqBody.SegmentID})
		return
	}

	// Se não encontrado, considera como abortado (idempotência)
	if !found {
		log.Printf("[%s] Idempotency: Segment TX_ID %s, SEG_ID %s not found. Considering as aborted.", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
		// Loga um estado ABORTED se não existia
		currentCompany.TransactionLogger.LogState(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusAborted, "Aborted because transaction segment not found", schemas.PrepareRequestBody{})
		sendJSONResponse(w, http.StatusOK, map[string]string{"status": schemas.StatusAborted, "transaction_id": reqBody.TransactionID, "segment_id": reqBody.SegmentID})
		return
	}

	// Só podemos abortar explicitamente se estiver PREPARADO.
	if txState.Status != schemas.StatusPrepared {
		reason := fmt.Sprintf("Cannot abort TX_ID %s, SEG_ID %s. Current state: %s. Only PREPARED segments can be aborted.", reqBody.TransactionID, reqBody.SegmentID, txState.Status)
		log.Printf("[%s] %s", currentCompany.Name, reason)
		// Se já está COMMITTED, retorna erro.
		statusCode := http.StatusConflict
		respStatus := txState.Status
		sendJSONResponse(w, statusCode, schemas.ErrorResponse{Status: respStatus, Reason: reason, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID})
		return
	}

	// Cancelar a reserva provisória (Mudar status para CANCELLED)
	updated := currentCompany.ReservationManager.UpdateReservationStatus(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusCancelled)
	if !updated {
		// Logar, mas ainda assim marcar como abortado no log de transação
		log.Printf("[%s] Warning: Could not find provisional reservation to cancel for TX_ID %s, SEG_ID %s (status PREPARED), but proceeding to log ABORTED.", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
	}

	// Logar o estado final ABORTED
	currentCompany.TransactionLogger.LogState(reqBody.TransactionID, reqBody.SegmentID, schemas.StatusAborted, "Reservation aborted by coordinator", txState.ReservationData)
	log.Printf("[%s] ABORTED segment TX_ID: %s, SEG_ID: %s", currentCompany.Name, reqBody.TransactionID, reqBody.SegmentID)
	sendJSONResponse(w, http.StatusOK, map[string]string{"status": schemas.StatusAborted, "transaction_id": reqBody.TransactionID, "segment_id": reqBody.SegmentID})
	// --- Fim Seção Crítica ---
}
