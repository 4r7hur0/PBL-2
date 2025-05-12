package booking

import (
	//"bytes"
	"encoding/json"
	"fmt"
	"net/http" // Para os handlers HTTP de participante
	"sync"
	"time"

	"github.com/4r7hur0/PBL-2/registry"
	"github.com/4r7hur0/PBL-2/schemas" // Agora usando as structs do seu company.go

	"github.com/google/uuid"
	"github.com/gin-gonic/gin" // Se for usar Gin para os endpoints HTTP do participante
)

// BookingHandlerConfig (como definido antes, com os caminhos dos endpoints 2PC)
type BookingHandlerConfig struct {
	LocalEnterpriseName       string
	Host                      string // Host DESTA empresa (para callback URLs)
	Port                      int    // Porta DESTA empresa (para callback URLs)
	DefaultPreparationBuffer  time.Duration
	DefaultTravelTime         time.Duration
	DefaultChargingDuration   int
	RemotePrepareEndpoint     string // Ex: "/api/2pc/prepare" (endpoint que as EMPRESAS REMOTAS expõem)
	RemoteCommitEndpoint      string // Ex: "/api/2pc/commit"  (endpoint que as EMPRESAS REMOTAS expõem)
	RemoteAbortEndpoint       string // Ex: "/api/2pc/abort"   (endpoint que as EMPRESAS REMOTAS expõem)
	// TransactionTimeout        time.Duration // Não implementado neste exemplo, mas importante
}

type BookingHandler struct {
	registry              *registry.ServiceRegistry
	config                BookingHandlerConfig
	bookings              map[string]*BookingRequest              // Key: BookingID (TransactionID)
	// pointTimeReservations armazena o estado da PREPARAÇÃO dos pontos LOCAIS.
	// O valor poderia ser schemas.ProvisionalReservation ou schemas.TransactionState se elas tiverem campos para expiração, etc.
	// Por simplicidade, vamos armazenar a schemas.PrepareRequestBody original que levou à preparação.
	localPreparedReservations map[string]map[string]schemas.PrepareRequestBody // Key1: ChargingPointID, Key2: TimeSlotKey -> Dados da Preparação
	lock                      sync.RWMutex
}

func NewBookingHandler(reg *registry.ServiceRegistry, cfg BookingHandlerConfig) *BookingHandler {
	return &BookingHandler{
		registry:                  reg,
		config:                    cfg,
		bookings:                  make(map[string]*BookingRequest),
		localPreparedReservations: make(map[string]map[string]schemas.PrepareRequestBody),
	}
}

func (h *BookingHandler) calculateReservationWindow(startTime time.Time, durationMinutes int) schemas.ReservationWindow {
	endTime := startTime.Add(time.Duration(durationMinutes) * time.Minute)
	return schemas.ReservationWindow{
		StartTimeUTC: startTime.Format(schemas.ISOFormat),
		EndTimeUTC:   endTime.Format(schemas.ISOFormat),
	}
}

// buildCoordinatorCallbackURLs constrói as URLs que este coordenador (Empresa A)
// exporá para as empresas participantes chamarem de volta (se o protocolo 2PC delas exigir).
// No entanto, o schemas/company.go já tem essas URLs no PrepareRequestBody,
// o que significa que o COORDENADOR (nosso BookingHandler) é quem DIZ para o participante
// quais URLs o participante DEVERIA chamar se o participante tivesse uma lógica de callback assíncrona.
// Para um 2PC síncrono onde o coordenador espera a resposta do prepare e depois envia commit/abort,
// estas URLs são mais para o participante saber quem é o coordenador.
// Se as chamadas de commit/abort são INICIADAS PELO COORDENADOR, estas URLs específicas
// podem não ser estritamente necessárias no PrepareRequestBody ENVIADO, mas sim
// os endpoints /commit e /abort do PARTICIPANTE que o coordenador irá chamar.
// O seu `schemas/company.go` -> `PrepareRequestBody` -> `CoordinatorCallbackURLs` sugere que
// o participante pode precisar delas. Vamos preenchê-las com os endpoints do nosso coordenador.
func (h *BookingHandler) buildCoordinatorCallbackURLsForRemote(transactionID string) schemas.CoordinatorCallbackURLs {
	// Estas são as URLs onde ESTE SERVIDOR (Empresa A, o coordenador)
	// esperaria receber uma notificação de commit/abort de um participante,
	// se o participante tivesse essa lógica de callback.
	// No nosso fluxo, o coordenador ativamente envia commit/abort.
	// Mas, para conformidade com o PrepareRequestBody, vamos preenchê-las.
	baseCallbackURL := fmt.Sprintf("http://%s:%d", h.config.Host, h.config.Port) // Host e porta da Empresa A
	return schemas.CoordinatorCallbackURLs{
		// Estes endpoints precisariam ser expostos pela API da Empresa A (coordenador)
		CommitURL: fmt.Sprintf("%s/api/coordinator/callback/commit/%s", baseCallbackURL, transactionID),
		AbortURL:  fmt.Sprintf("%s/api/coordinator/callback/abort/%s", baseCallbackURL, transactionID),
	}
}


// HandleBookingRequestFromMQTT (chamado pelo listener MQTT da Empresa A)
func (h *BookingHandler) HandleBookingRequestFromMQTT(mqttPayload []byte) (*BookingRequest, error) {
	h.lock.Lock()
	var carReq schemas.ChargingResquest // Struct do schemas/car.go
	if err := json.Unmarshal(mqttPayload, &carReq); err != nil {
		h.lock.Unlock()
		return nil, fmt.Errorf("erro ao deserializar payload MQTT: %w", err)
	}

	req := BookingRequest{ // Struct interna do booking/planning.go
		BookingID:     uuid.New().String(),      // TransactionID Global
		CarID:         carReq.CarID,
		Origin:        carReq.OriginCity,
		Destination:   carReq.DestinationCity,
		BatteryLevel:  carReq.BatteryLevel,
		DischargeRate: carReq.DischargeRate,
		Status:        schemas.StatusBookingPending,
		CreatedAt:     time.Now(),
		// req.RouteIdentifier = carReq.RouteIdentifier // Se você adicionar ao schemas.ChargingResquest
	}

	if err := h.planRoute(&req); err != nil { // planRoute preenche req.ChargingStops
		req.Status = schemas.StatusBookingFailed
		h.bookings[req.BookingID] = &req
		h.lock.Unlock()
		return &req, fmt.Errorf("erro ao planejar rota para BookingID %s: %w", req.BookingID, err)
	}
	if len(req.ChargingStops) == 0 {
		req.Status = schemas.StatusBookingFailed
		h.bookings[req.BookingID] = &req
		h.lock.Unlock()
		return &req, fmt.Errorf("planejamento de rota não resultou em paradas para BookingID %s", req.BookingID)
	}
	h.bookings[req.BookingID] = &req
	h.lock.Unlock()

	finalSuccess, finalError := h.processAtomicReservation2PC(&req)

	h.lock.Lock()
	defer h.lock.Unlock()
	currentBooking, _ := h.bookings[req.BookingID] // Pega a versão mais atual, caso tenha sido modificada por callbacks (improvável no fluxo síncrono)

	if finalError != nil {
		currentBooking.Status = schemas.StatusBookingFailed
	} else if finalSuccess {
		currentBooking.Status = schemas.StatusCommitted // Ou StatusConfirmed
	} else {
		// Se !finalSuccess && finalError == nil, o aborto foi "limpo"
		currentBooking.Status = schemas.StatusAborted // Ou StatusCancelled
	}
	return currentBooking, finalError
}


// processAtomicReservation2PC orquestra o Two-Phase Commit.
func (h *BookingHandler) processAtomicReservation2PC(bookingReq *BookingRequest) (bool, error) {
	h.lock.Lock()
	bookingReq.Status = schemas.StatusBookingProcessing // Coordenador está iniciando a fase de PREPARE
	h.lock.Unlock()

	fmt.Printf("BookingID %s: Iniciando Fase 1 (Prepare) para %d paradas.\n", bookingReq.BookingID, len(bookingReq.ChargingStops))
	preparedSegments := make([]*ChargingStopRequest, 0, len(bookingReq.ChargingStops))
	allPreparedSuccessfully := true
	var firstPrepareError error

	// Fase 1: Prepare
	for i := range bookingReq.ChargingStops {
		stop := &bookingReq.ChargingStops[i]
		h.lock.Lock()
		stop.Status = schemas.StatusBookingProcessing// Usar uma constante se tiver, senão pode ser a mesma de BookingProcessing
		h.lock.Unlock()

		var prepared bool
		var err error

		if stop.EnterpriseName == h.config.LocalEnterpriseName {
			prepared, err = h.prepareLocalReservation(bookingReq.BookingID, bookingReq.CarID, stop)
		} else {
			remoteEnterprise, exists := h.registry.GetEnterpriseByName(stop.EnterpriseName)
			if !exists {
				err = fmt.Errorf("empresa remota '%s' não encontrada no registro para SegmentID %s", stop.EnterpriseName, stop.SegmentID)
			} else {
				prepared, err = h.prepareRemoteReservation(remoteEnterprise, bookingReq.BookingID, bookingReq.CarID, stop)
			}
		}

		h.lock.Lock()
		if err != nil {
			stop.Status = schemas.StatusBookingFailed // Ou um status mais específico como "PREPARE_FAILED"
			allPreparedSuccessfully = false
			if firstPrepareError == nil { // Guarda o primeiro erro que causou a falha no prepare
				firstPrepareError = fmt.Errorf("erro ao preparar SegmentID %s (%s at %s): %w", stop.SegmentID, stop.ChargingPointID, stop.EnterpriseName, err)
			}
			h.lock.Unlock()
			break // Interrompe a fase de preparação no primeiro erro
		}
		if !prepared { // Preparação recusada sem erro explícito (ex: ponto não disponível)
			stop.Status = schemas.StatusAborted // Ou "PREPARE_REJECTED"
			allPreparedSuccessfully = false
			if firstPrepareError == nil {
				firstPrepareError = fmt.Errorf("preparação recusada para SegmentID %s (%s at %s)", stop.SegmentID, stop.ChargingPointID, stop.EnterpriseName)
			}
			h.lock.Unlock()
			break
		}
		stop.Status = schemas.StatusPrepared // Segmento preparado com sucesso
		preparedSegments = append(preparedSegments, stop)
		h.lock.Unlock()
		fmt.Printf("BookingID %s: SegmentID %s (%s at %s) PREPARED.\n", bookingReq.BookingID, stop.SegmentID, stop.ChargingPointID, stop.EnterpriseName)
	}

	// Fase 2: Commit ou Abort
	if allPreparedSuccessfully {
		fmt.Printf("BookingID %s: Todos os %d segmentos PREPARED. Iniciando Fase 2 (Commit).\n", bookingReq.BookingID, len(preparedSegments))
		h.lock.Lock()
		//bookingReq.Status = schemas.StatusCommitted // Usar uma constante se tiver
		h.lock.Unlock()
		
		commitOverallSuccess := h.executePhase2Commit(bookingReq.BookingID, preparedSegments)
		if commitOverallSuccess {
			fmt.Printf("BookingID %s: Todos os segmentos COMMITTED.\n", bookingReq.BookingID)
			return true, nil
		}
		// Se o commit falhou para algum segmento após todos terem sido preparados,
		// esta é uma situação problemática que pode requerer intervenção manual ou compensação.
		// O protocolo 2PC padrão não lida bem com falhas na fase de commit do coordenador após os prepares.
		// Para simulação, podemos marcar como falha.
		fmt.Printf("BookingID %s: FALHA CRÍTICA - Nem todos os segmentos puderam ser COMMITTED.\n", bookingReq.BookingID)
		// Aqui você pode tentar reverter os commits bem-sucedidos (compensação) ou apenas logar.
		// Por simplicidade, vamos apenas marcar como falha.
		h.lock.Lock()
		bookingReq.Status = schemas.StatusBookingFailed // Ou um "INCONSISTENT_COMMIT_STATE"
		h.lock.Unlock()
		return false, fmt.Errorf("falha crítica durante a fase de commit para BookingID %s", bookingReq.BookingID)

	}

	// Se nem todos foram preparados ou houve erro na fase de prepare, então Abort
	// Abortar apenas os 'preparedSegments'
	fmt.Printf("BookingID %s: Falha na fase Prepare (allPrepared: %v, error: %v). Iniciando Fase 2 (Abort) para %d segmentos.\n",
		bookingReq.BookingID, allPreparedSuccessfully, firstPrepareError, len(preparedSegments))
	h.lock.Lock()
	bookingReq.Status = schemas.StatusAborted // Usar uma constante se tiver
	h.lock.Unlock()

	h.executePhase2Abort(bookingReq.BookingID, preparedSegments) // Tenta abortar o que foi preparado
	fmt.Printf("BookingID %s: Abort executado para segmentos preparados.\n", bookingReq.BookingID)
	if firstPrepareError != nil {
		return false, firstPrepareError
	}
	return false, nil // Abortado "limpamente" porque um dos prepares não foi aceito
}


func (h *BookingHandler) executePhase2Commit(transactionID string, segmentsToCommit []*ChargingStopRequest) bool {
	allCommitted := true
	for _, stop := range segmentsToCommit {
		h.lock.Lock()
		stop.Status = schemas.StatusCommitted // Ou constante similar
		h.lock.Unlock()

		var committed bool
		var err error
		if stop.EnterpriseName == h.config.LocalEnterpriseName {
			committed, err = h.commitLocalSegment(transactionID, stop.SegmentID)
		} else {
			remoteEnterprise, _ := h.registry.GetEnterpriseByName(stop.EnterpriseName) // Já deve existir
			committed, err = h.commitRemoteSegment(remoteEnterprise, transactionID, stop.SegmentID)
		}

		h.lock.Lock()
		if err != nil || !committed {
			stop.Status = schemas.StatusBookingFailed // Ou "COMMIT_FAILED"
			allCommitted = false
			fmt.Printf("BookingID %s: Falha ao COMMIT SegmentID %s. Erro: %v\n", transactionID, stop.SegmentID, err)
			// Em um 2PC real, uma falha de commit aqui é problemática.
		} else {
			stop.Status = schemas.StatusCommitted
			fmt.Printf("BookingID %s: SegmentID %s COMMITTED.\n", transactionID, stop.SegmentID)
		}
		h.lock.Unlock()
	}
	return allCommitted
}

func (h *BookingHandler) executePhase2Abort(transactionID string, segmentsToAbort []*ChargingStopRequest) {
	for _, stop := range segmentsToAbort { // segmentsToAbort são os que retornaram PREPARED
		h.lock.Lock()
		stop.Status = schemas.StatusAborted // Ou constante similar
		h.lock.Unlock()

		var aborted bool
		var err error
		if stop.EnterpriseName == h.config.LocalEnterpriseName {
			aborted, err = h.abortLocalSegment(transactionID, stop.SegmentID)
		} else {
			remoteEnterprise, _ := h.registry.GetEnterpriseByName(stop.EnterpriseName)
			aborted, err = h.abortRemoteSegment(remoteEnterprise, transactionID, stop.SegmentID)
		}
		h.lock.Lock()
		if err != nil || !aborted {
			stop.Status = schemas.StatusAborted // Ou "ABORT_FAILED"
			fmt.Printf("BookingID %s: Falha ao ABORT SegmentID %s. Erro: %v\n", transactionID, stop.SegmentID, err)
			// Logar, mas o objetivo do abort é liberar recursos, então o estado final do stop deve ser ABORTED/CANCELLED.
		} else {
			stop.Status = schemas.StatusAborted
			fmt.Printf("BookingID %s: SegmentID %s ABORTED.\n", transactionID, stop.SegmentID)
		}
		h.lock.Unlock()
	}
}


// --- Funções de Preparação, Commit, Abort (Local e Remoto) ---

func (h *BookingHandler) prepareLocalReservation(transactionID string, vehicleID string, stop *ChargingStopRequest) (bool, error) {
	fmt.Printf("BookingID %s: Preparando LOCALMENTE SegmentID %s (Ponto: %s)\n", transactionID, stop.SegmentID, stop.ChargingPointID)
	h.lock.Lock()
	defer h.lock.Unlock()

	slotKey := stop.TimeSlot.Format(schemas.ISOFormat)
	if _, pointExists := h.localPreparedReservations[stop.ChargingPointID]; !pointExists {
		h.localPreparedReservations[stop.ChargingPointID] = make(map[string]schemas.PrepareRequestBody)
	}

	if existingPrep, exists := h.localPreparedReservations[stop.ChargingPointID][slotKey]; exists {
		// Se já existe uma preparação para OUTRA transação, então o slot não está disponível.
		// Se for para a MESMA transação e segmento (retentativa), pode ser OK.
		if existingPrep.TransactionID != transactionID {
			fmt.Printf("BookingID %s: Ponto LOCAL %s no horário %s JÁ PREPARADO para TxID %s.\n",
				transactionID, stop.ChargingPointID, slotKey, existingPrep.TransactionID)
			return false, nil // Não disponível
		}
		// Se é uma retentativa para o mesmo segmento da mesma transação, consideramos já preparado.
		fmt.Printf("BookingID %s: Ponto LOCAL %s no horário %s já estava PREPARADO para este TxID/SegID %s.\n",
			transactionID, stop.ChargingPointID, slotKey, stop.SegmentID)
		return true, nil
	}

	// Armazena os detalhes da preparação local
	// A ProvisionalReservation ou TransactionState do seu schemas/company.go seria mais completa aqui.
	// Por simplicidade, vamos armazenar o PrepareRequestBody que seria enviado se fosse remoto.
	localPrepData := schemas.PrepareRequestBody{
		TransactionID:     transactionID,
		SegmentID:         stop.SegmentID,
		ChargingPointID:   stop.ChargingPointID,
		VehicleID:         vehicleID,
		ReservationWindow: h.calculateReservationWindow(stop.TimeSlot, stop.DurationMinutes),
		// CoordinatorCallbackURLs não são necessárias para a lógica interna do prepare local,
		// mas manteria a estrutura de dados consistente se usássemos TransactionState.
	}
	h.localPreparedReservations[stop.ChargingPointID][slotKey] = localPrepData
	fmt.Printf("BookingID %s: Ponto LOCAL %s no horário %s PREPARADO para TxID/SegID %s.\n",
		transactionID, stop.ChargingPointID, slotKey, stop.SegmentID)
	return true, nil
}

func (h *BookingHandler) prepareRemoteReservation(
	remoteEnterprise registry.EnterpriseService,
	transactionID string,
	vehicleID string,
	stop *ChargingStopRequest) (bool, error) {

	fmt.Printf("BookingID %s: Enviando PREPARE para %s - SegmentID %s (Ponto: %s)\n",
		transactionID, remoteEnterprise.Name, stop.SegmentID, stop.ChargingPointID)

	prepareBody := schemas.PrepareRequestBody{
		TransactionID:           transactionID,
		SegmentID:               stop.SegmentID,
		ChargingPointID:         stop.ChargingPointID,
		VehicleID:               vehicleID,
		ReservationWindow:       h.calculateReservationWindow(stop.TimeSlot, stop.DurationMinutes),
		CoordinatorCallbackURLs: h.buildCoordinatorCallbackURLsForRemote(transactionID), // URLs da Empresa A (este coordenador)
	}

	payloadBytes, err := json.Marshal(prepareBody)
	if err != nil {
		return false, fmt.Errorf("erro ao serializar PrepareRequestBody para %s: %w", remoteEnterprise.Name, err)
	}

	responseBody, err := registry.ContactEnterprise(remoteEnterprise, h.config.RemotePrepareEndpoint, "POST", payloadBytes)
	if err != nil {
		var errorResp schemas.ErrorResponse
		if json.Unmarshal(responseBody, &errorResp) == nil {
			return false, fmt.Errorf("prepare em %s falhou (SegID %s): %s (TxID: %s)",
				remoteEnterprise.Name, stop.SegmentID, errorResp.Reason, errorResp.TransactionID)
		}
		return false, fmt.Errorf("erro ao contatar %s para prepare (SegID %s): %w", remoteEnterprise.Name, stop.SegmentID, err)
	}

	var successResp schemas.PrepareSuccessResponse
	if err := json.Unmarshal(responseBody, &successResp); err != nil {
		return false, fmt.Errorf("erro ao deserializar PrepareSuccessResponse de %s (SegID %s): %w. Corpo: %s",
			remoteEnterprise.Name, stop.SegmentID, err, string(responseBody))
	}

	if successResp.Status == schemas.StatusPrepared {
		// Opcional: armazenar successResp.PreparedUntilUTC se for usado para timeouts
		return true, nil
	}
	return false, fmt.Errorf("prepare em %s (SegID %s) retornou status '%s' em vez de '%s'",
		remoteEnterprise.Name, stop.SegmentID, successResp.Status, schemas.StatusPrepared)
}


func (h *BookingHandler) commitLocalSegment(transactionID string, segmentID string) (bool, error) {
	fmt.Printf("BookingID %s: Comitando LOCALMENTE SegmentID %s\n", transactionID, segmentID)
	h.lock.Lock()
	defer h.lock.Unlock()

	// Achar a preparação pelos dados do segmento (ex: ponto e horário)
	// A forma como você armazena o estado "PREPARED" localmente determinará como você o "commita".
	// Se localPreparedReservations armazena o PrepareRequestBody, precisamos iterar ou ter um índice melhor.
	// Por simplicidade, vamos assumir que o commit local é uma mudança de estado em algum lugar.
	// Idealmente, localPreparedReservations guardaria um objeto com um campo Status.
	// Para este exemplo, vamos apenas logar. A reserva já está "feita" no mapa ao ser preparada.
	// A remoção aconteceria no abort.
	
	// Procurar o segmento preparado e mudar seu estado para COMMITTED
	// (Esta parte precisa de uma estrutura de dados melhor para o estado dos segmentos locais)
	// Por ora, se chegou aqui, consideramos o commit local bem-sucedido conceitualmente.
	// Se localPreparedReservations[pointID][slotKey] existe e pertence a esta tx/seg,
	// não precisamos fazer nada se a preparação já significa "reservado até abortar".
	// Se precisássemos de um estado explícito, atualizaríamos aqui.

	fmt.Printf("BookingID %s: Commit LOCAL para SegmentID %s efetivado (recurso já estava PREPARADO).\n", transactionID, segmentID)
	return true, nil
}

func (h *BookingHandler) abortLocalSegment(transactionID string, segmentID string) (bool, error) {
	fmt.Printf("BookingID %s: Abortando LOCALMENTE SegmentID %s\n", transactionID, segmentID)
	h.lock.Lock()
	defer h.lock.Unlock()

	// Iterar sobre localPreparedReservations para encontrar e remover a preparação
	// correspondente ao transactionID e segmentID.
	var pointToClean string
	var slotToClean string

	found := false
	for pointID, slots := range h.localPreparedReservations {
		for slotKey, prepData := range slots {
			if prepData.TransactionID == transactionID && prepData.SegmentID == segmentID {
				pointToClean = pointID
				slotToClean = slotKey
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if found {
		delete(h.localPreparedReservations[pointToClean], slotToClean)
		if len(h.localPreparedReservations[pointToClean]) == 0 {
			delete(h.localPreparedReservations, pointToClean)
		}
		fmt.Printf("BookingID %s: Abort LOCAL para SegmentID %s (Ponto: %s, Slot: %s) efetivado, preparação removida.\n",
			transactionID, segmentID, pointToClean, slotToClean)
		return true, nil
	}
	fmt.Printf("BookingID %s: Abort LOCAL para SegmentID %s não encontrou preparação correspondente.\n", transactionID, segmentID)
	return false, fmt.Errorf("preparação local para SegmentID %s (TxID %s) não encontrada para abortar", segmentID, transactionID)
}


func (h *BookingHandler) commitRemoteSegment(remoteEnterprise registry.EnterpriseService, transactionID string, segmentID string) (bool, error) {
	fmt.Printf("BookingID %s: Enviando COMMIT para %s - SegmentID %s\n", transactionID, remoteEnterprise.Name, segmentID)
	commitBody := schemas.CommitRequestBody{ // Usando a nova struct de schemas/company.go
		TransactionID: transactionID,
		SegmentID:     segmentID,
	}
	payloadBytes, err := json.Marshal(commitBody)
	if err != nil {
		return false, fmt.Errorf("erro ao serializar CommitRequestBody para %s: %w", remoteEnterprise.Name, err)
	}

	// O endpoint para commit pode ser /api/2pc/commit/{transactionID} ou /api/2pc/commit (com TxID/SegID no corpo)
	// O CoordinatorCallbackURLs sugere que o TransactionID já está no path.
	// Se o SegmentID também for no path, o endpoint precisa ser ajustado.
	// Por simplicidade, enviaremos no corpo por enquanto, mas o endpoint pode ser mais específico.
	// O endpoint na config é genérico, ex: h.config.RemoteCommitEndpoint = "/api/2pc/commit"
	// Se o endpoint for /api/2pc/commit/{transactionID}, então o path seria:
	// commitPath := fmt.Sprintf("%s/%s", h.config.RemoteCommitEndpoint, transactionID)
	// Mas as CoordinatorCallbackURLs são para o *coordenador* ser chamado de volta.
	// Para o *participante*, é mais provável um endpoint fixo que recebe os IDs no corpo.
	
	responseBody, err := registry.ContactEnterprise(remoteEnterprise, h.config.RemoteCommitEndpoint, "POST", payloadBytes)
	if err != nil {
		var errorResp schemas.ErrorResponse
		if json.Unmarshal(responseBody, &errorResp) == nil {
			return false, fmt.Errorf("commit em %s falhou (SegID %s): %s", remoteEnterprise.Name, segmentID, errorResp.Reason)
		}
		return false, fmt.Errorf("erro ao contatar %s para commit (SegID %s): %w", remoteEnterprise.Name, segmentID, err)
	}
	var successResp schemas.CommitSuccessResponse
	if err := json.Unmarshal(responseBody, &successResp); err != nil {
		return false, fmt.Errorf("erro ao deserializar CommitSuccessResponse de %s (SegID %s): %w. Corpo: %s",
			remoteEnterprise.Name, segmentID, err, string(responseBody))
	}
	if successResp.Status == schemas.StatusCommitted {
		return true, nil
	}
	return false, fmt.Errorf("commit em %s (SegID %s) retornou status '%s'", remoteEnterprise.Name, segmentID, successResp.Status)
}

func (h *BookingHandler) abortRemoteSegment(remoteEnterprise registry.EnterpriseService, transactionID string, segmentID string) (bool, error) {
	fmt.Printf("BookingID %s: Enviando ABORT para %s - SegmentID %s\n", transactionID, remoteEnterprise.Name, segmentID)
	abortBody := schemas.AbortRequestBody{ // Usando a nova struct de schemas/company.go
		TransactionID: transactionID,
		SegmentID:     segmentID,
		Reason:        "Transação da rota geral abortada pelo coordenador.",
	}
	payloadBytes, err := json.Marshal(abortBody)
	if err != nil {
		return false, fmt.Errorf("erro ao serializar AbortRequestBody para %s: %w", remoteEnterprise.Name, err)
	}
	responseBody, err := registry.ContactEnterprise(remoteEnterprise, h.config.RemoteAbortEndpoint, "POST", payloadBytes)
	if err != nil {
		var errorResp schemas.ErrorResponse
		if json.Unmarshal(responseBody, &errorResp) == nil {
			return false, fmt.Errorf("abort em %s falhou (SegID %s): %s", remoteEnterprise.Name, segmentID, errorResp.Reason)
		}
		return false, fmt.Errorf("erro ao contatar %s para abort (SegID %s): %w", remoteEnterprise.Name, segmentID, err)
	}
	var successResp schemas.AbortSuccessResponse
	if err := json.Unmarshal(responseBody, &successResp); err != nil {
		return false, fmt.Errorf("erro ao deserializar AbortSuccessResponse de %s (SegID %s): %w. Corpo: %s",
			remoteEnterprise.Name, segmentID, err, string(responseBody))
	}
	if successResp.Status == schemas.StatusAborted {
		return true, nil
	}
	return false, fmt.Errorf("abort em %s (SegID %s) retornou status '%s'", remoteEnterprise.Name, segmentID, successResp.Status)
}


// --- Implementação dos Endpoints HTTP para o BookingHandler ATUAR COMO PARTICIPANTE ---
// Estes seriam registrados no seu router (ex: Gin) na inicialização da API da Empresa A.

// HandleRemotePrepare é chamado por um COORDENADOR EXTERNO para preparar um recurso LOCAL desta empresa.
func (h *BookingHandler) HandleRemotePrepare(c *gin.Context) {
	var reqBody schemas.PrepareRequestBody
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID, Reason: "Payload inválido: " + err.Error()})
		return
	}

	fmt.Printf("[%s como PARTICIPANTE]: Recebido PREPARE para TxID %s, SegID %s, Ponto %s\n",
		h.config.LocalEnterpriseName, reqBody.TransactionID, reqBody.SegmentID, reqBody.ChargingPointID)

	// Converter ReservationWindow para time.Time e duration
	startTime, errStart := time.Parse(schemas.ISOFormat, reqBody.ReservationWindow.StartTimeUTC)
	endTime, errEnd := time.Parse(schemas.ISOFormat, reqBody.ReservationWindow.EndTimeUTC)
	if errStart != nil || errEnd != nil {
		c.JSON(http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID, Reason: "Formato de ReservationWindow inválido"})
		return
	}
	durationMinutes := int(endTime.Sub(startTime).Minutes())

	// Cria um ChargingStopRequest temporário para usar com a lógica de prepareLocalReservation
	tempStop := ChargingStopRequest{
		SegmentID:       reqBody.SegmentID,
		EnterpriseName:  h.config.LocalEnterpriseName, // É para um recurso local
		City:            h.getLocalCityForPoint(reqBody.ChargingPointID), // Precisa de uma forma de obter a cidade do ponto
		ChargingPointID: reqBody.ChargingPointID,
		TimeSlot:        startTime,
		DurationMinutes: durationMinutes,
	}

	prepared, err := h.prepareLocalReservation(reqBody.TransactionID, reqBody.VehicleID, &tempStop)

	if err != nil {
		c.JSON(http.StatusInternalServerError, schemas.ErrorResponse{
			Status:        schemas.StatusError,
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        "Erro interno ao preparar localmente: " + err.Error(),
		})
		return
	}
	if !prepared {
		c.JSON(http.StatusConflict, schemas.ErrorResponse{ // 409 Conflict se não disponível
			Status:        schemas.StatusAborted, // Ou um status como POINT_UNAVAILABLE
			TransactionID: reqBody.TransactionID,
			SegmentID:     reqBody.SegmentID,
			Reason:        "Ponto de recarga não disponível para o horário solicitado ou preparação recusada.",
		})
		return
	}

	// Calcular PreparedUntilUTC (ex: agora + algum tempo razoável para o coordenador decidir)
	// Este tempo deve ser gerenciado; se expirar, a preparação local deve ser abortada automaticamente.
	preparedUntil := time.Now().Add(5 * time.Minute).Format(schemas.ISOFormat) // Placeholder

	c.JSON(http.StatusOK, schemas.PrepareSuccessResponse{
		Status:           schemas.StatusPrepared,
		TransactionID:    reqBody.TransactionID,
		SegmentID:        reqBody.SegmentID,
		PreparedUntilUTC: preparedUntil,
	})
}

// HandleRemoteCommit é chamado por um COORDENADOR EXTERNO para comitar um recurso local preparado.
// A URL para este handler no router seria algo como "/api/2pc/commit" ou "/api/2pc/commit/:transactionID/:segmentID"
// O schemas/company.go -> CoordinatorCallbackURLs sugere que transactionID está no path.
// Por simplicidade, vamos assumir que o corpo contém TransactionID e SegmentID.
func (h *BookingHandler) HandleRemoteCommit(c *gin.Context) {
	var reqBody schemas.CommitRequestBody
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Payload de commit inválido: " + err.Error()})
		return
	}

	fmt.Printf("[%s como PARTICIPANTE]: Recebido COMMIT para TxID %s, SegID %s\n",
		h.config.LocalEnterpriseName, reqBody.TransactionID, reqBody.SegmentID)

	committed, err := h.commitLocalSegment(reqBody.TransactionID, reqBody.SegmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, schemas.ErrorResponse{Status: schemas.StatusError, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID, Reason: "Erro ao comitar localmente: " + err.Error()})
		return
	}
	if !committed {
		// Isso pode acontecer se o segmento não estava PREPARED ou não foi encontrado.
		c.JSON(http.StatusNotFound, schemas.ErrorResponse{Status: schemas.StatusError, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID, Reason: "Segmento não encontrado ou não estava preparado para commit."})
		return
	}

	c.JSON(http.StatusOK, schemas.CommitSuccessResponse{
		Status:        schemas.StatusCommitted,
		TransactionID: reqBody.TransactionID,
		SegmentID:     reqBody.SegmentID,
	})
}

// HandleRemoteAbort é chamado por um COORDENADOR EXTERNO para abortar um recurso local preparado.
func (h *BookingHandler) HandleRemoteAbort(c *gin.Context) {
	var reqBody schemas.AbortRequestBody
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, schemas.ErrorResponse{Status: schemas.StatusError, Reason: "Payload de abort inválido: " + err.Error()})
		return
	}
	fmt.Printf("[%s como PARTICIPANTE]: Recebido ABORT para TxID %s, SegID %s. Razão: %s\n",
		h.config.LocalEnterpriseName, reqBody.TransactionID, reqBody.SegmentID, reqBody.Reason)

	aborted, err := h.abortLocalSegment(reqBody.TransactionID, reqBody.SegmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, schemas.ErrorResponse{Status: schemas.StatusError, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID, Reason: "Erro ao abortar localmente: " + err.Error()})
		return
	}
	if !aborted {
		c.JSON(http.StatusNotFound, schemas.ErrorResponse{Status: schemas.StatusError, TransactionID: reqBody.TransactionID, SegmentID: reqBody.SegmentID, Reason: "Segmento não encontrado ou não estava preparado para abort."})
		return
	}

	c.JSON(http.StatusOK, schemas.AbortSuccessResponse{
		Status:        schemas.StatusAborted,
		TransactionID: reqBody.TransactionID,
		SegmentID:     reqBody.SegmentID,
	})
}

// getLocalCityForPoint é uma função auxiliar placeholder.
// Você precisaria de uma forma de mapear um ChargingPointID local para sua cidade,
// talvez consultando o ServiceRegistry ou uma configuração interna.
func (h *BookingHandler) getLocalCityForPoint(pointID string) string {
    // Implementação de placeholder
    if details, exists := h.registry.GetEnterpriseByName(h.config.LocalEnterpriseName); exists {
        for _, p := range details.ChargingPoints {
            if p == pointID {
                return details.City
            }
        }
    }
    return "UnknownCity" // Ou buscar de uma lista de pontos locais
}