package booking

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/4r7hur0/PBL-2/registry"
	"github.com/4r7hur0/PBL-2/schemas"
	"github.com/google/uuid"
)

// BookingHandlerConfig como antes
type BookingHandlerConfig struct {
	LocalEnterpriseName      string
	LocalEnterpriseCity      string
	LocalPointsCapacity      int
	Host                     string
	Port                     int
	DefaultPreparationBuffer time.Duration
	DefaultTravelTime        time.Duration
	DefaultChargingDuration  int
	RemotePrepareEndpoint    string
	RemoteCommitEndpoint     string
	RemoteAbortEndpoint      string
}

// BookingHandler como antes, com localSlotUsageCount e provisionalSegments
type BookingHandler struct {
	registry            *registry.ServiceRegistry
	config              BookingHandlerConfig
	bookings            map[string]*BookingRequest          // Key: BookingID (TransactionID)
	localSlotUsageCount map[string]int                      // Key: TimeSlotKey, Value: Contagem
	provisionalSegments map[string]schemas.TransactionState // Key: SegmentID -> Estado do Segmento no 2PC
	lock                sync.RWMutex
}

func NewBookingHandler(reg *registry.ServiceRegistry, cfg BookingHandlerConfig) *BookingHandler {
	return &BookingHandler{
		registry:            reg,
		config:              cfg,
		bookings:            make(map[string]*BookingRequest),
		localSlotUsageCount: make(map[string]int),
		provisionalSegments: make(map[string]schemas.TransactionState),
	}
}

// calculateReservationWindow e buildCoordinatorCallbackURLsForRemote como antes
func (h *BookingHandler) calculateReservationWindow(startTime time.Time, durationMinutes int) schemas.ReservationWindow {
	endTime := startTime.Add(time.Duration(durationMinutes) * time.Minute)
	return schemas.ReservationWindow{
		StartTimeUTC: startTime.Format(schemas.ISOFormat), // Usa a constante do pacote schemas
		EndTimeUTC:   endTime.Format(schemas.ISOFormat),
	}
}
func (h *BookingHandler) buildCoordinatorCallbackURLsForRemote(transactionID string) schemas.CoordinatorCallbackURLs {
	baseCallbackURL := fmt.Sprintf("http://%s:%d", h.config.Host, h.config.Port)
	return schemas.CoordinatorCallbackURLs{
		CommitURL: fmt.Sprintf("%s%s/%s", baseCallbackURL, h.config.RemoteCommitEndpoint, transactionID), // Ajustar path se commit for por segmento
		AbortURL:  fmt.Sprintf("%s%s/%s", baseCallbackURL, h.config.RemoteAbortEndpoint, transactionID),  // Ajustar path se abort for por segmento
	}
}

// ETAPA 1 do Fluxo do Carro: Recebe schemas.ChargingResquest, gera opções de rota.
func (h *BookingHandler) HandleInitialCarRequest(carReqPayload []byte) (*schemas.RouteReservationResponse, error) {
	var carReq schemas.ChargingResquest
	if err := json.Unmarshal(carReqPayload, &carReq); err != nil {
		// Retorna uma resposta de erro para o carro, se possível
		return &schemas.RouteReservationResponse{Success: false, Message: "Formato de requisição inválido: " + err.Error()},
			fmt.Errorf("HandleInitialCarRequest: erro ao deserializar payload: %w", err)
	}

	h.lock.Lock()
	bookingID := uuid.New().String()   // Este será o RequestID para o carro
	initialBooking := &BookingRequest{ // Usa a struct interna booking.BookingRequest
		BookingID:     bookingID,
		CarID:         carReq.CarID,
		Origin:        carReq.OriginCity,
		Destination:   carReq.DestinationCity,
		BatteryLevel:  carReq.BatteryLevel,
		DischargeRate: carReq.DischargeRate,
		Status:        schemas.StatusBookingAwaitingRouteSelection, // Estado inicial
		CreatedAt:     time.Now(),
	}
	h.bookings[bookingID] = initialBooking
	h.lock.Unlock()

	// O `baseRequestTime` é importante para que todos os TimeSlots gerados sejam relativos a um ponto comum.
	routeOptions, err := h.generateRouteOptions(carReq.OriginCity, carReq.DestinationCity, time.Now())

	responseToCar := &schemas.RouteReservationResponse{
		RequestID: bookingID,
		VehicleID: carReq.CarID,
	}

	if err != nil {
		h.lock.Lock()
		initialBooking.Status = schemas.StatusBookingFailed // Falha no planejamento de opções
		h.lock.Unlock()
		responseToCar.Success = false
		responseToCar.Message = fmt.Sprintf("Nenhuma rota encontrada: %s", err.Error())
	} else {
		responseToCar.Success = true
		responseToCar.Options = routeOptions
		responseToCar.Message = fmt.Sprintf("%d opções de rota encontradas.", len(routeOptions))
		h.lock.Lock()
		initialBooking.Status = schemas.StatusBookingRouteOptionsSent
		h.lock.Unlock()
	}
	return responseToCar, nil // Retorna a resposta para ser enviada via MQTT
}

// ETAPA 2 do Fluxo do Carro: Recebe schemas.ChosenRouteMsg, inicia reserva atômica 2PC.
func (h *BookingHandler) HandleChosenRouteFromCar(chosenRoutePayload []byte) (*schemas.RouteReservationResponse, error) { // Alterado para retornar RouteReservationResponse
	var chosenRouteReq schemas.ChosenRouteMsg
	if err := json.Unmarshal(chosenRoutePayload, &chosenRouteReq); err != nil {
		// Retorna uma RouteReservationResponse indicando o erro de parsing
		return &schemas.RouteReservationResponse{
			RequestID: chosenRouteReq.RequestID, // Pode estar vazio se o payload for muito corrupto
			VehicleID: chosenRouteReq.VehicleID, // Pode estar vazio
			Success:   false,
			Message:   "Formato de rota escolhida inválido: " + err.Error(),
			Options:   nil,
		}, fmt.Errorf("HandleChosenRouteFromCar: erro ao deserializar payload: %w", err)
	}

	h.lock.Lock()
	bookingReq, exists := h.bookings[chosenRouteReq.RequestID]
	if !exists {
		h.lock.Unlock()
		return &schemas.RouteReservationResponse{
			RequestID: chosenRouteReq.RequestID,
			VehicleID: chosenRouteReq.VehicleID,
			Success:   false,
			Options:   nil,
			Message:   fmt.Sprintf("Booking original com RequestID %s não encontrado.", chosenRouteReq.RequestID),
		}, nil
	}

	if !(bookingReq.Status == schemas.StatusBookingAwaitingRouteSelection || bookingReq.Status == schemas.StatusBookingRouteOptionsSent) {
		h.lock.Unlock()
		return &schemas.RouteReservationResponse{
			RequestID: bookingReq.BookingID,
			VehicleID: bookingReq.CarID,
			Success:   false,
			Options:   nil,
			Message:   fmt.Sprintf("Booking %s não está aguardando seleção de rota (status atual: %s)", bookingReq.BookingID, bookingReq.Status),
		}, nil
	}

	var internalStops []ChargingStopRequest
	for _, chosenSeg := range chosenRouteReq.Route { // chosenRouteReq.Route é []schemas.RouteSegment
		startTime, _ := time.Parse(schemas.ISOFormat, chosenSeg.ReservationWindow.StartTimeUTC)

		internalStops = append(internalStops, ChargingStopRequest{
			SegmentID:       chosenSeg.SegmentID,
			EnterpriseName:  chosenSeg.EnterpriseName,
			City:            chosenSeg.City,
			TimeSlot:        startTime,
			DurationMinutes: chosenSeg.DurationMinutes,
			Status:          schemas.StatusStopPending,
		})
	}
	bookingReq.ChargingStops = internalStops
	bookingReq.Status = schemas.StatusBookingPending
	h.bookings[bookingReq.BookingID] = bookingReq
	h.lock.Unlock()

	finalSuccess, processErr := h.processAtomicReservation2PC(bookingReq) // Ou a versão simplificada

	h.lock.Lock()
	defer h.lock.Unlock()
	finalBookingState, _ := h.bookings[bookingReq.BookingID] // Pega o estado mais recente

	// Construir a RouteReservationResponse final
	responseToCar := &schemas.RouteReservationResponse{
		RequestID: finalBookingState.BookingID,
		VehicleID: finalBookingState.CarID,
		Success:   finalSuccess, // True se o 2PC foi COMMITTED, false caso contrário
	}

	if processErr != nil {
		responseToCar.Message = fmt.Sprintf("Erro durante a reserva: %s. Status do Booking: %s", processErr.Error(), finalBookingState.Status)
	} else if !finalSuccess {
		responseToCar.Message = fmt.Sprintf("Reserva da rota falhou ou foi abortada. Status do Booking: %s", finalBookingState.Status)
	} else { // finalSuccess == true
		responseToCar.Message = "Reserva da rota confirmada com sucesso!"
		// Se sucesso, preenchemos a rota confirmada como a única opção.
		var confirmedRouteSegmentsForClient []schemas.RouteSegment
		for _, confirmedStop := range finalBookingState.ChargingStops {
			// Inclui apenas as paradas que foram efetivamente confirmadas/comitadas
			if confirmedStop.Status == schemas.StatusCommitted || confirmedStop.Status == schemas.StatusConfirmed {
				confirmedRouteSegmentsForClient = append(confirmedRouteSegmentsForClient, schemas.RouteSegment{
					SegmentID:         confirmedStop.SegmentID,
					City:              confirmedStop.City,
					EnterpriseName:    confirmedStop.EnterpriseName,
					ReservationWindow: h.calculateReservationWindow(confirmedStop.TimeSlot, confirmedStop.DurationMinutes),
					DurationMinutes:   confirmedStop.DurationMinutes,
				})
			}
		}
		// A RouteReservationResponse espera uma lista de opções.
		// Para a resposta final de sucesso, enviamos uma única opção com a rota confirmada.
		if len(confirmedRouteSegmentsForClient) > 0 {
			responseToCar.Options = []schemas.RouteOptionForClient{
				{
					OptionID: "CONFIRMED_ROUTE",
					Segments: confirmedRouteSegmentsForClient,
				},
			}
		} else if finalSuccess {
			responseToCar.Message = "Reserva processada, mas nenhum segmento foi confirmado."
			responseToCar.Success = false
		}

	}
	return responseToCar, nil
}
