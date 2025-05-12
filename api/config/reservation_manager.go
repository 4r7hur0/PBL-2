package config

import (
	"github.com/4r7hur0/PBL-2/schemas"

	"fmt"
	"log"
	"sync"
	"time"
)

// ReservationManager gerencia as reservas para os pontos de uma empresa.
type ReservationManager struct {
	mu                    sync.Mutex
	provisionalReservations map[string][]schemas.ProvisionalReservation // Chave: chargingPointID -> Lista
	managedChargingPoints map[string]bool
}

// NewReservationManager cria um novo ReservationManager.
func NewReservationManager(managedCPs map[string]bool) *ReservationManager {
	return &ReservationManager{
		provisionalReservations: make(map[string][]schemas.ProvisionalReservation),
		managedChargingPoints:   managedCPs,
	}
}

// IsManaged verifica se um ponto de recarga é gerenciado por esta instância.
func (rm *ReservationManager) IsManaged(chargingPointID string) bool {
	// Não precisa de lock aqui, pois managedChargingPoints é definido na inicialização
	// e não é modificado concorrentemente depois disso.
	_, ok := rm.managedChargingPoints[chargingPointID]
	return ok
}

// MakeProvisionalReservation tenta criar uma reserva provisória.
// Retorna true se bem-sucedido, false e uma razão em caso de falha.
func (rm *ReservationManager) MakeProvisionalReservation(req schemas.PrepareRequestBody) (bool, string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	startTime, errStart := time.Parse(schemas.ISOFormat, req.ReservationWindow.StartTimeUTC)
	endTime, errEnd := time.Parse(schemas.ISOFormat, req.ReservationWindow.EndTimeUTC)
	if errStart != nil || errEnd != nil {
		return false, "Invalid time format in reservation window"
	}

	// Verificar disponibilidade
	reservations := rm.provisionalReservations[req.ChargingPointID]
	for _, res := range reservations {
		// Ignorar reservas canceladas ou se já está preparado para esta mesma transação/segmento (idempotência)
		if res.Status == schemas.StatusCancelled {
			continue
		}
		// Verifica idempotência: Se a mesma requisição já está como preparada.
		if res.TransactionID == req.TransactionID && res.SegmentID == req.SegmentID && res.Status == schemas.StatusPreparedPendingCommit {
			log.Printf("[%s] Idempotency: Provisional reservation for TX_ID %s, SEG_ID %s already exists.", GetCurrentCompany().Name, req.TransactionID, req.SegmentID)
			return true, "Already provisionally reserved (idempotency)"
		}

		// Verificar sobreposição de tempo apenas com reservas confirmadas ou pendentes de outras transações
		if res.TransactionID != req.TransactionID || res.SegmentID != req.SegmentID {
			if res.StartTime.Before(endTime) && res.EndTime.After(startTime) {
				// Conflito se a reserva existente está confirmada ou pendente de commit
				if res.Status == schemas.StatusPreparedPendingCommit || res.Status == schemas.StatusConfirmed {
					reason := fmt.Sprintf("Conflict with existing reservation for TX_ID %s, SEG_ID %s from %s to %s",
						res.TransactionID, res.SegmentID, res.StartTime.Format(schemas.ISOFormat), res.EndTime.Format(schemas.ISOFormat))
					log.Printf("[%s] Conflict for %s: %s", GetCurrentCompany().Name, req.ChargingPointID, reason)
					return false, reason
				}
			}
		}
	}

	// Criar reserva provisória
	provisionalRes := schemas.ProvisionalReservation{
		TransactionID:     req.TransactionID,
		SegmentID:         req.SegmentID,
		ChargingPointID:   req.ChargingPointID,
		VehicleID:         req.VehicleID,
		StartTime:         startTime,
		EndTime:           endTime,
		Status:            schemas.StatusPreparedPendingCommit,
		ReservationWindow: req.ReservationWindow,
	}
	rm.provisionalReservations[req.ChargingPointID] = append(rm.provisionalReservations[req.ChargingPointID], provisionalRes)
	log.Printf("[%s] Provisionally reserved %s for TX_ID: %s, SEG_ID: %s", GetCurrentCompany().Name, req.ChargingPointID, req.TransactionID, req.SegmentID)
	return true, "Provisionally reserved"
}

// UpdateReservationStatus atualiza o status de uma reserva.
// Usado para Commit ou Abort.
func (rm *ReservationManager) UpdateReservationStatus(txID, segID, newStatus string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	found := false
	for cpID, reservations := range rm.provisionalReservations {
		for i, res := range reservations {
			if res.TransactionID == txID && res.SegmentID == segID {
				// Verifica se a transição de estado é válida (ex: não mudar de CONFIRMED para CANCELLED diretamente aqui)
				// A lógica de commit/abort no handler deve garantir que só chamamos isso nos estados corretos.
				log.Printf("[%s] Updating reservation for TX_ID: %s, SEG_ID: %s on CP: %s from %s to %s",
					GetCurrentCompany().Name, txID, segID, cpID, res.Status, newStatus)
				rm.provisionalReservations[cpID][i].Status = newStatus
				found = true
				// Não sair do loop interno, pode haver duplicatas acidentais (embora não devesse)
				// break // Comentado para garantir que todas as correspondências sejam atualizadas (embora deva ser apenas uma)
			}
		}
		// Não sair do loop externo ainda, a reserva pode estar em outro CP (embora não devesse)
		// if found {
		// 	break
		// }
	}
	if !found {
		log.Printf("[%s] Could not find reservation to update status for TX_ID: %s, SEG_ID: %s", GetCurrentCompany().Name, txID, segID)
	}
	return found
}