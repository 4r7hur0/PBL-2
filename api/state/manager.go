// PBL-2/api/state/manager.go
package state

import (
	"fmt"
	"log"
	"sync"
	"time"
	"encoding/json"
	"net/url"
	"strings"

  "github.com/4r7hur0/PBL-2/api/mqtt"
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
	enterpriseName string // Nome da empresa que gerencia esta cidade
	cityDataMux *sync.Mutex
	myAPIURL string // URL da API que gerencia este estado
	cpWorkerIDs []string // IDs dos workers que estão processando reservas nesta cidade
}

func NewStateManager(ownedCity string, initialPosts int, myAPIURL string, workerIDs []string) *StateManager {
    log.Printf("[StateManager] Inicializando para a cidade: %s com %d postos.", ownedCity, initialPosts)

    // Extrai o nome da empresa da URL da API (ex: http://solatlantico:8080)
    // Esta é uma forma simples; pode ser melhorado se a URL for mais complexa.
    var entName string
    if u, err := url.Parse(myAPIURL); err == nil {
        entName = strings.Split(u.Hostname(), ".")[0]
    }

    return &StateManager{
        ownedCity:      ownedCity,
        enterpriseName: entName, // Adicionado
        myAPIURL:       myAPIURL,
        cpWorkerIDs:    workerIDs, // Adicionado
        cityData: &CityState{
            MaxPosts:           initialPosts,
            ActiveReservations: []schemas.ActiveReservation{},
        },
        cityDataMux: &sync.Mutex{},
    }
}

// PrepareReservation verifica e "pré-aloca" um posto na cidade gerenciada.
// Substitua a função PrepareReservation inteira por esta versão
func (m *StateManager) PrepareReservation(transactionID, vehicleID, requestID string, window schemas.ReservationWindow, coordinatorURL string) (bool, error) {
    m.cityDataMux.Lock()
    defer m.cityDataMux.Unlock()

    // 1. Verificar capacidade no StateManager (lógica existente)
    overlappingCount := 0
    for _, existingRes := range m.cityData.ActiveReservations {
        if (existingRes.Status == schemas.StatusReservationCommitted ||
            (existingRes.Status == schemas.StatusReservationPrepared && existingRes.TransactionID != transactionID)) &&
            windowsOverlap(existingRes.ReservationWindow, window) {
            overlappingCount++
        }
    }

    if overlappingCount >= m.cityData.MaxPosts {
        errMsg := fmt.Sprintf("conflito de horário ou capacidade máxima (%d/%d) atingida", overlappingCount, m.cityData.MaxPosts)
        log.Printf("[StateManager-%s] TX[%s]: FALHA PREPARE - %s", m.ownedCity, transactionID, errMsg)
        return false, fmt.Errorf("%s", errMsg)
    }

    // 2. Encontrar e preparar um worker disponível
    preparedWorkerID, err := m.findAndPrepareWorker(transactionID, window)
    if err != nil {
        log.Printf("[StateManager-%s] TX[%s]: FALHA PREPARE - Não foi possível preparar um worker: %v", m.ownedCity, transactionID, err)
        return false, err // Retorna o erro do worker
    }

    // 3. Sucesso! Adicionar a reserva como PREPARED no estado local
    newRes := schemas.ActiveReservation{
        TransactionID:     transactionID,
        VehicleID:         vehicleID,
        RequestID:         requestID,
        City:              m.ownedCity,
        ReservationWindow: window,
        Status:            schemas.StatusReservationPrepared,
        CoordinatorURL:    coordinatorURL,
        WorkerID:          preparedWorkerID, // Salva o worker que foi preparado
    }
    m.cityData.ActiveReservations = append(m.cityData.ActiveReservations, newRes)
    log.Printf("[StateManager-%s] TX[%s]: SUCESSO PREPARE. Worker '%s' alocado. Reserva: %+v", m.ownedCity, transactionID, preparedWorkerID, newRes)
    return true, nil
}

// Adicione esta nova função de busca de worker ao manager.go
func (m *StateManager) findAndPrepareWorker(transactionID string, window schemas.ReservationWindow) (string, error) {
    for _, workerID := range m.cpWorkerIDs {
        log.Printf("[StateManager-%s] TX[%s]: Tentando preparar worker '%s'", m.ownedCity, transactionID, workerID)

        // Lógica de comunicação com o worker (anteriormente em `reserveWindowWithWorker`)
        prepareMsg := map[string]interface{}{
            "command":        "PREPARE_RESERVE_WINDOW",
            "window":         window,
            "transaction_id": transactionID,
        }
        prepareBytes, _ := json.Marshal(prepareMsg)
        commandTopic := fmt.Sprintf("enterprise/%s/cp/%s/command", m.enterpriseName, workerID)

        responseTopic := fmt.Sprintf("enterprise/%s/cp/%s/response", m.enterpriseName, workerID)
        respChan := mqtt.StartListening(responseTopic, 1) // Escuta por uma resposta

        mqtt.Publish(commandTopic, string(prepareBytes))

        select {
        case respPayload := <-respChan:
            var resp map[string]interface{}
            if err := json.Unmarshal([]byte(respPayload), &resp); err != nil {
                log.Printf("[StateManager-%s] TX[%s]: Erro ao decodificar resposta do worker '%s': %v", m.ownedCity, transactionID, workerID, err)
                continue // Tenta o próximo worker
            }

            // Verifica se a resposta é para o comando de PREPARE e se foi bem-sucedida
            if resp["command"] == "PREPARE_RESPONSE" {
                success, _ := resp["success"].(bool)
                if success {
                    log.Printf("[StateManager-%s] TX[%s]: Sucesso! Worker '%s' preparado.", m.ownedCity, transactionID, workerID)
                    return workerID, nil // Sucesso, retorna o ID do worker
                }
            }

        case <-time.After(5 * time.Second):
            log.Printf("[StateManager-%s] TX[%s]: Timeout esperando resposta do worker '%s'", m.ownedCity, transactionID, workerID)
            continue // Tenta o próximo worker
        }
    }
    // Se o loop terminar, nenhum worker foi preparado com sucesso.
    return "", fmt.Errorf("nenhum charging point worker disponível na cidade %s para a janela solicitada", m.ownedCity)
}

func (m *StateManager) CommitReservation(transactionID string) {
	m.cityDataMux.Lock()
	defer m.cityDataMux.Unlock()

	found := false
	for i, res := range m.cityData.ActiveReservations {
		if res.TransactionID == transactionID && res.Status == schemas.StatusReservationPrepared {
			m.cityData.ActiveReservations[i].Status = schemas.StatusReservationCommitted

			// Notifica o worker específico que foi reservado!
            if res.WorkerID != "" {
                m.sendCommandToWorker(res.WorkerID, transactionID, "COMMIT")
            }

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

			if res.WorkerID != "" {
                m.sendCommandToWorker(res.WorkerID, transactionID, "ABORT")
            }

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

// GetCoordinatorURL encontra e retorna a URL da API coordenadora para uma dada transação.
func (m *StateManager) GetCoordinatorURL(transactionID string) (string, bool) {
	m.cityDataMux.Lock()
	defer m.cityDataMux.Unlock()

	for _, res := range m.cityData.ActiveReservations {
		if res.TransactionID == transactionID {
			// Retorna a URL e um booleano indicando que foi encontrada.
			return res.CoordinatorURL, true
		}
	}

	// Retorna uma string vazia e false se a transação não for encontrada.
	return "", false
}

// IsCoordinator retorna true se esta instância é a coordenadora da transação.
func (m *StateManager) IsCoordinator(transactionID string) bool {
    m.cityDataMux.Lock()
    defer m.cityDataMux.Unlock()

    for _, res := range m.cityData.ActiveReservations {
        if res.TransactionID == transactionID {
            // Se a URL do coordenador for vazia ou "localhost" ou igual à URL desta instância, considere coordenador.
            // Adapte conforme sua lógica de identificação.
            return res.CoordinatorURL == m.myAPIURL // ou compare com sua URL real
        }
    }
    return false
}

// CheckAndEndReservations verifica as reservas e envia notificações MQTT se necessário.
func (m *StateManager) CheckAndEndReservations() {
    m.cityDataMux.Lock()
    defer m.cityDataMux.Unlock()

    now := time.Now().UTC()
    var keptReservations []schemas.ActiveReservation

    for _, res := range m.cityData.ActiveReservations {
        if res.Status == schemas.StatusReservationCommitted && now.After(res.ReservationWindow.EndTimeUTC) {
            // Reserva expirou! Enviar notificação MQTT
            endMessage := schemas.ReservationEndMessage{
                VehicleID:     res.VehicleID,
                TransactionID: res.TransactionID,
                EndTimeUTC:    res.ReservationWindow.EndTimeUTC,
                Message:       "Reserva encerrada",
            }
            payloadBytes, _ := json.Marshal(endMessage)
            mqtt.Publish(fmt.Sprintf("car/reservation/end/%s", res.VehicleID), string(payloadBytes)) // Tópico específico para fim de reserva
            log.Printf("[StateManager-%s] TX[%s]: Reserva para veículo %s encerrada. Notificação MQTT enviada.", m.ownedCity, res.TransactionID, res.VehicleID)
        } else {
            keptReservations = append(keptReservations, res) // Manter reservas não expiradas
        }
    }

    m.cityData.ActiveReservations = keptReservations // Atualizar a lista de reservas
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

func (m *StateManager) sendCommandToWorker(workerID, transactionID, command string) {
    msg := map[string]interface{}{
        "command":        command,
        "transaction_id": transactionID,
    }
    msgBytes, _ := json.Marshal(msg)
    topic := fmt.Sprintf("enterprise/%s/cp/%s/command", m.enterpriseName, workerID)
    mqtt.Publish(topic, string(msgBytes))
    log.Printf("[StateManager-%s] TX[%s]: Comando '%s' enviado para worker '%s'", m.ownedCity, transactionID, command, workerID)
}