// PBL-2/api/state/manager.go
package state

import (
	"fmt"
	"log"
	"sync"

	"github.com/4r7hur0/PBL-2/schemas" 
)

func windowsOverlap(r1 schemas.ReservationWindow, r2 schemas.ReservationWindow) bool {
	return r1.StartTimeUTC.Before(r2.EndTimeUTC) && r1.EndTimeUTC.After(r2.StartTimeUTC)
}

type CityState struct {
	MaxPosts           int
	ActiveReservations []schemas.ActiveReservation
}

type StateManager struct {
	ownedCity   string 
	cityData    *CityState
	cityDataMux *sync.Mutex
}

func NewStateManager(ownedCity string, initialPostsForOwnedCity int) *StateManager {
	log.Printf("[StateManager] Inicializando para a cidade: %s com %d postos.", ownedCity, initialPostsForOwnedCity)
	return &StateManager{
		ownedCity: ownedCity,
		cityData: &CityState{
			MaxPosts:           initialPostsForOwnedCity,
			ActiveReservations: []schemas.ActiveReservation{},
		},
		cityDataMux: &sync.Mutex{},
	}
}

// PrepareReservation verifica e "pré-aloca" um posto na cidade gerenciada.
func (m *StateManager) PrepareReservation(transactionID, vehicleID, requestID string, window schemas.ReservationWindow) (bool, error) {
	m.cityDataMux.Lock()
	defer m.cityDataMux.Unlock()

	overlappingCount := 0
	for _, existingRes := range m.cityData.ActiveReservations {
		if existingRes.Status == schemas.StatusReservationCommitted ||
			(existingRes.Status == schemas.StatusReservationPrepared && existingRes.TransactionID != transactionID) {
			if windowsOverlap(existingRes.ReservationWindow, window) {
				overlappingCount++
			}
		}
	}

	if overlappingCount >= m.cityData.MaxPosts {
		errMsg := fmt.Sprintf("conflito de horário ou capacidade máxima (%d/%d) atingida para a cidade %s na janela solicitada", overlappingCount, m.cityData.MaxPosts, m.ownedCity)
		log.Printf("[StateManager-%s] TX[%s]: FALHA PREPARE - %s", m.ownedCity, transactionID, errMsg)
		return false, fmt.Errorf(errMsg)
	}

	// Adiciona a nova reserva como PREPARED
	newRes := schemas.ActiveReservation{
		TransactionID:     transactionID,
		VehicleID:         vehicleID,
		RequestID:         requestID,
		City:              m.ownedCity, // Sempre a cidade gerenciada
		ReservationWindow: window,
		Status:            schemas.StatusReservationPrepared,
	}
	m.cityData.ActiveReservations = append(m.cityData.ActiveReservations, newRes)
	log.Printf("[StateManager-%s] TX[%s]: SUCESSO PREPARE. %d postos ocupados na janela. Reserva: %+v", m.ownedCity, transactionID, overlappingCount+1, newRes)
	return true, nil
}

func (m *StateManager) CommitReservation(transactionID string) {
	m.cityDataMux.Lock()
	defer m.cityDataMux.Unlock()

	found := false
	for i, res := range m.cityData.ActiveReservations {
		if res.TransactionID == transactionID && res.Status == schemas.StatusReservationPrepared {
			m.cityData.ActiveReservations[i].Status = schemas.StatusReservationCommitted
			log.Printf("[StateManager-%s] TX[%s]: SUCESSO COMMIT. Reserva: %+v", m.ownedCity, transactionID, m.cityData.ActiveReservations[i])
			found = true
			// Não precisa retornar, pode haver múltiplos segmentos para a mesma TX (embora não neste modelo de cidade única por API)
		}
	}
	if !found {
		log.Printf("[StateManager-%s] TX[%s]: AVISO COMMIT - Nenhuma reserva PREPARED encontrada para este TransactionID.", m.ownedCity, transactionID)
	}
}

func (m *StateManager) AbortReservation(transactionID string) {
	m.cityDataMux.Lock()
	defer m.cityDataMux.Unlock()

	var keptReservations []schemas.ActiveReservation
	aborted := false
	for _, res := range m.cityData.ActiveReservations {
		if res.TransactionID == transactionID && res.Status == schemas.StatusReservationPrepared {
			log.Printf("[StateManager-%s] TX[%s]: SUCESSO ABORT. Removendo reserva: %+v", m.ownedCity, transactionID, res)
			aborted = true
		} else {
			keptReservations = append(keptReservations, res)
		}
	}
	m.cityData.ActiveReservations = keptReservations
	if !aborted {
		log.Printf("[StateManager-%s] TX[%s]: AVISO ABORT - Nenhuma reserva PREPARED encontrada para este TransactionID.", m.ownedCity, transactionID)
	}
}

// GetCityAvailability - pode ser útil para um endpoint de status
func (m *StateManager) GetCityAvailability() (string, int, []schemas.ActiveReservation) {
	m.cityDataMux.Lock()
	defer m.cityDataMux.Unlock()
	// Retorna uma cópia para evitar race conditions se o chamador modificar o slice
	reservationsCopy := make([]schemas.ActiveReservation, len(m.cityData.ActiveReservations))
	copy(reservationsCopy, m.cityData.ActiveReservations)
	return m.ownedCity, m.cityData.MaxPosts, reservationsCopy
}