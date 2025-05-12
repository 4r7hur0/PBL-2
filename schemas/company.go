package schemas

import "time"

// ReservationWindow define o início e o fim de uma reserva.
type ReservationWindow struct {
	StartTimeUTC string `json:"start_time_utc"` // Formato: "YYYY-MM-DDTHH:mm:ssZ"
	EndTimeUTC   string `json:"end_time_utc"`   // Formato: "YYYY-MM-DDTHH:mm:ssZ"
}

// CoordinatorCallbackURLs URLs para o coordenador ser chamado de volta.
type CoordinatorCallbackURLs struct {
	CommitURL string `json:"commit_url"`
	AbortURL  string `json:"abort_url"`
}

// PrepareRequestBody é a estrutura para a requisição /prepare.
type PrepareRequestBody struct {
	TransactionID           string                  `json:"transaction_id"`
	SegmentID               string                  `json:"segment_id"`
	ChargingPointID         string                  `json:"charging_point_id"`
	VehicleID               string                  `json:"vehicle_id"`
	ReservationWindow       ReservationWindow       `json:"reservation_window"`
	CoordinatorCallbackURLs CoordinatorCallbackURLs `json:"coordinator_callback_urls"`
}

// PrepareSuccessResponse é a estrutura para uma resposta /prepare bem-sucedida.
type PrepareSuccessResponse struct {
	Status           string `json:"status"` // "PREPARED"
	TransactionID    string `json:"transaction_id"`
	SegmentID        string `json:"segment_id"`
	PreparedUntilUTC string `json:"prepared_until_utc,omitempty"`
}

// ErrorResponse é uma resposta de erro genérica.
type ErrorResponse struct {
	Status        string `json:"status"`
	TransactionID string `json:"transaction_id,omitempty"`
	SegmentID     string `json:"segment_id,omitempty"`
	Reason        string `json:"reason"`
}

// TransactionState representa o estado de um segmento de transação.
type TransactionState struct {
	Status          string      // PREPARED, COMMITTED, ABORTED
	Details         interface{} // Detalhes da reserva
	Timestamp       time.Time
	ReservationData PrepareRequestBody // Mantém os dados da reserva original
}

// ProvisionalReservation representa uma reserva pendente.
type ProvisionalReservation struct {
	TransactionID     string
	SegmentID         string
	ChargingPointID   string
	VehicleID         string
	StartTime         time.Time
	EndTime           time.Time
	Status            string // PREPARED_PENDING_COMMIT, CONFIRMED, CANCELLED
	ReservationWindow ReservationWindow
}

// Constantes de Status
const (
	StatusPrepared              = "PREPARED"
	StatusAborted               = "ABORTED"
	StatusError                 = "ERROR"
	StatusCommitted             = "COMMITTED"
	StatusPreparedPendingCommit = "PREPARED_PENDING_COMMIT"
	StatusConfirmed             = "CONFIRMED"
	StatusCancelled             = "CANCELLED"
	ISOFormat                   = "2006-01-02T15:04:05Z"
)

// RouteSegment define um trecho da rota a ser reservado.
type RouteSegment struct {
	ChargingPointID   string            `json:"charging_point_id"`
	ReservationWindow ReservationWindow `json:"reservation_window"`
}

// RouteReservationRequest é a estrutura da mensagem MQTT para solicitar uma reserva de rota.
type RouteReservationRequest struct {
	RequestID string         `json:"request_id"` // ID único para esta requisição de rota
	VehicleID string         `json:"vehicle_id"`
	Route     []RouteSegment `json:"route"`
}

type RouteRequest struct {
	VehicleID   string `json:"vehicle_id"`
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

type Enterprises struct {
	Name string `json:"name"`
	City string `json:"city"`
}
