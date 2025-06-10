package schemas

import (
	"time"
)

// ReservationWindow define o início e o fim de uma reserva.
type ReservationWindow struct {
	StartTimeUTC time.Time `json:"start_time_utc"` // Formato: "YYYY-MM-DDTHH:mm:ssZ"
	EndTimeUTC   time.Time `json:"end_time_utc"`   // Formato: "YYYY-MM-DDTHH:mm:ssZ"
}

// CoordinatorCallbackURLs URLs para o coordenador ser chamado de volta.
type CoordinatorCallbackURLs struct {
	CommitURL string `json:"commit_url"`
	AbortURL  string `json:"abort_url"`
}

type ActiveReservation struct {
	TransactionID     string            `json:"transaction_id"`
	VehicleID         string            `json:"vehicle_id"`
	RequestID         string            `json:"request_id"` // ID da requisição de rota original
	City              string            `json:"city"`
	ReservationWindow ReservationWindow `json:"reservation_window"`
	Status            string            `json:"status"` // Ex: "PREPARED", "COMMITTED"
	CoordinatorURL    string            `json:"coordinator_url,omitempty"` // URL da API que iniciou o 2PC

}
type ReservationEndMessage struct {
	VehicleID     string    `json:"vehicle_id"`
	TransactionID string    `json:"transaction_id"`
	EndTimeUTC    time.Time `json:"end_time_utc"`
	Message       string    `json:"message"` // Ex: "Reserva encerrada"
}

type RemotePrepareResponse struct {
	Status        string `json:"status"` // "PREPARED" ou "REJECTED"
	TransactionID string `json:"transaction_id"`
	Reason        string `json:"reason,omitempty"`
}

// Novas structs para comunicação inter-APIs (para os Passos 3 e 4)
type RemotePrepareRequest struct {
	TransactionID     string            `json:"transaction_id"`
	VehicleID         string            `json:"vehicle_id"`
	RequestID         string            `json:"request_id"`
	City              string            `json:"city"` // A cidade para preparar
	ReservationWindow ReservationWindow `json:"reservation_window"`
}

type RemoteCommitAbortRequest struct {
	TransactionID string `json:"transaction_id"`
}

// ReservationStatus informa o veículo sobre o resultado da tentativa de reserva.
type ReservationStatus struct {
	TransactionID  string         `json:"transaction_id"`
	VehicleID      string         `json:"vehicle_id"`
	RequestID      string         `json:"request_id"` // ID da requisição de rota original
	Status         string         `json:"status"`     // Ex: "CONFIRMED", "REJECTED"
	Message        string         `json:"message"`
	ConfirmedRoute []RouteSegment `json:"confirmed_route,omitempty"` // Rota confirmada, se aplicável
}

// Constantes para Status da Reserva Ativa
const (
	StatusReservationPrepared  = "PREPARED"
	StatusReservationCommitted = "COMMITTED"
)

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
	City              string            `json:"city"`
	ReservationWindow ReservationWindow `json:"reservation_window"`
}

// RouteReservationResponse é a estrutura da mensagem MQTT para enviar uma resposta para o carro.
type RouteReservationResponse struct {
	RequestID string         `json:"request_id"` // ID único para esta requisição de rota
	VehicleID string         `json:"vehicle_id"`
	Route     []RouteSegment `json:"route"`
}

type RouteReservationOptions struct {
	RequestID string           `json:"request_id"` // ID único para esta requisição de rota
	VehicleID string           `json:"vehicle_id"`
	Routes    [][]RouteSegment `json:"route"`
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

type ChosenRouteMsg struct {
	RequestID string         `json:"request_id"` // ID único para esta requisição de rota
	VehicleID string         `json:"vehicle_id"`
	Route     []RouteSegment `json:"route"`
}

// RegisterRequest é o payload para registrar uma API de cidade.
type RegisterRequest struct {
	CityManaged    string `json:"city_managed"`    // A cidade que esta API gerencia
	ApiURL         string `json:"api_url"`         // A URL base da API (ex: http://localhost:8080)
	EnterpriseName string `json:"enterprise_name"` // Nome da empresa/API
}

// DiscoverResponse é o payload da resposta de descoberta.
type DiscoverResponse struct {
	CityName       string `json:"city_name"`
	ApiURL         string `json:"api_url"`
	EnterpriseName string `json:"enterprise_name,omitempty"`
	Found          bool   `json:"found"`
}
