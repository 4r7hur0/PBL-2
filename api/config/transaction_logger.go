package config

import (

	"github.com/4r7hur0/PBL-2/schemas"
	"log"
	"sync"
	"time"
)

// TransactionLogger gerencia o log de transações para uma instância de empresa.
type TransactionLogger struct {
	mu             sync.Mutex
	transactionLog map[string]map[string]schemas.TransactionState // Chave: transactionID -> Chave: segmentID -> TransactionState
}

// NewTransactionLogger cria um novo TransactionLogger.
func NewTransactionLogger() *TransactionLogger {
	return &TransactionLogger{
		transactionLog: make(map[string]map[string]schemas.TransactionState),
	}
}

// LogState registra o estado de uma transação/segmento.
func (tl *TransactionLogger) LogState(txID, segID, status string, details interface{}, originalReq schemas.PrepareRequestBody) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if _, ok := tl.transactionLog[txID]; !ok {
		tl.transactionLog[txID] = make(map[string]schemas.TransactionState)
	}
	tl.transactionLog[txID][segID] = schemas.TransactionState{
		Status:          status,
		Details:         details,
		Timestamp:       time.Now().UTC(),
		ReservationData: originalReq,
	}
	// Usa GetCurrentCompany para obter o nome da empresa atual para o log
	log.Printf("[%s] TRANSACTION LOG: TX_ID=%s, SEG_ID=%s, STATUS=%s", GetCurrentCompany().Name, txID, segID, status)
}

// GetState retorna o estado de uma transação/segmento.
func (tl *TransactionLogger) GetState(txID, segID string) (schemas.TransactionState, bool) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	if segments, ok := tl.transactionLog[txID]; ok {
		state, found := segments[segID]
		return state, found
	}
	return schemas.TransactionState{}, false
}
