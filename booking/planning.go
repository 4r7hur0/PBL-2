package booking

import (
	"fmt"
	"time"
	"github.com/google/uuid"
	"github.com/4r7hur0/PBL-2/schemas"
)

// BookingRequest (estrutura interna do BookingHandler)
type BookingRequest struct {
	BookingID     string    `json:"booking_id"`
	CarID         string    `json:"car_id"`
	Origin        string    `json:"origin"`
	Destination   string    `json:"destination"`
	BatteryLevel  int       `json:"battery_level"` // Mantido caso precise para alguma lógica futura ou log
	DischargeRate string    `json:"discharge_rate"`// Mantido caso precise

	ChargingStops []ChargingStopRequest `json:"charging_stops"`
	Status        string                `json:"status"` 
	CreatedAt     time.Time             `json:"created_at"`
}

// ChargingStopRequest é a estrutura interna para detalhar cada parada de recarga.
type ChargingStopRequest struct {
	SegmentID       string    `json:"segment_id"`
	EnterpriseName  string    `json:"enterprise_name"`
	City            string    `json:"city"`
	ChargingPointID string    `json:"charging_point_id,omitempty"` // Pode ser preenchido pela empresa que confirma
	TimeSlot        time.Time `json:"time_slot"`                   // Horário de início da reserva
	DurationMinutes int       `json:"duration_minutes"`            // Duração da reserva (padronizada para testes)
	Status          string    `json:"status"`                      // Usará schemas.StatusStop...
}

// RouteStopInfo para predefinedRoutesDB - não precisa de ChargingPointID aqui
type RouteStopInfo struct {
	EnterpriseName string
	City           string
}

var predefinedRoutesDB = map[string][]RouteStopInfo{

	// --- Rotas Iniciadas via Servidor da SolAtlantico (Salvador) ---
	"Salvador_Feira de Santana": { 
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
	},
	"Salvador_Lençóis": { 
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
	},
	"Salvador_Ilhéus": { 
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
	},
	"Salvador_Juazeiro": { 
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
	},

	// --- Rotas Iniciadas via Servidor da SertaoCarga (Feira de Santana) ---
	"Feira de Santana_Salvador": { 
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
	},
	"Feira de Santana_Lençóis": { 
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
	},
	"Feira de Santana_Ilhéus": { 
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
	},
	"Feira de Santana_Juazeiro": { 
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
	},

	// --- Rotas Iniciadas via Servidor da ChapadaEletric (Lençóis) ---
	"Lençóis_Salvador": { 
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
	},
	"Lençóis_Feira de Santana": { 
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
	},
	"Lençóis_Ilhéus": { 
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
	},
	"Lençóis_Juazeiro": { 
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
	},

	// --- Rotas Iniciadas via Servidor da CacauPower (Ilhéus) ---
	"Ilhéus_Salvador": { 
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
	},
	"Ilhéus_Feira de Santana": { 
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
	},
	"Ilhéus_Lençóis": { 
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
	},
	"Ilhéus_Juazeiro": { 
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
	},

	// --- Rotas Iniciadas via Servidor da VelhoChicoEnergia (Juazeiro) ---
	"Juazeiro_Salvador": { 
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
	},
	"Juazeiro_Feira de Santana": { 
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
	},
	"Juazeiro_Lençóis": { 
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
		{EnterpriseName: "ChapadaEletric", City: "Lençóis"},
	},
	"Juazeiro_Ilhéus": { 
		{EnterpriseName: "VelhoChicoEnergia", City: "Juazeiro"},
		{EnterpriseName: "SertaoCarga", City: "Feira de Santana"},
		{EnterpriseName: "SolAtlantico", City: "Salvador"},
		{EnterpriseName: "CacauPower", City: "Ilhéus"},
	},
}

func GetPredefinedRoute(originCity, destinationCity, routeIdentifier string) ([]RouteStopInfo, bool) {
	key := originCity + "_" + destinationCity
	if routeIdentifier != "" {
		key = routeIdentifier 
	}
	route, exists := predefinedRoutesDB[key]
	return route, exists
}

func (h *BookingHandler) planRoute(req *BookingRequest) error {
	routeIdentifierFromRequest := "" 

	routeInfo, exists := GetPredefinedRoute(req.Origin, req.Destination, routeIdentifierFromRequest)
	if !exists {
		return fmt.Errorf("nenhuma rota predefinida encontrada de %s para %s (Identificador: '%s')", req.Origin, req.Destination, routeIdentifierFromRequest)
	}
	if len(routeInfo) == 0 {
		return fmt.Errorf("a rota predefinida de %s para %s está vazia", req.Origin, req.Destination)
	}

	var stops []ChargingStopRequest
	currentTime := time.Now()
	// TimeSlot para a primeira parada: hora atual + um buffer de preparação
	// Este buffer permite um tempo mínimo antes da primeira reserva.
	estimatedArrivalTimeAtStop := currentTime.Add(h.config.DefaultPreparationBuffer)

	// Validação básica da primeira parada da rota
	firstStopDefinition := routeInfo[0]
	if !(firstStopDefinition.City == req.Origin) {
		return fmt.Errorf("a rota predefinida para %s->%s tem uma cidade de primeira parada inválida: %s. Deveria começar em %s",
			req.Origin, req.Destination, firstStopDefinition.City, req.Origin)
	}
	// Opcional: Validar se a primeira empresa da rota é a empresa local (h.config.LocalEnterpriseName)
	// Isso depende se a primeira parada é sempre gerenciada pela empresa que recebe a requisição MQTT.
	// Se a primeira parada da rota predefinida for da empresa local, ela será tratada por `prepareLocalReservation`.
	// Se for de outra empresa, será tratada por `prepareRemoteReservation`.


	for i, stopInfo := range routeInfo {
		// Para paradas subsequentes (i > 0), calcula-se o horário de chegada
		// com base no horário de término da parada anterior mais o tempo de viagem.
		if i > 0 {
			prevStopDuration := time.Duration(stops[i-1].DurationMinutes) * time.Minute
			estimatedTravelTime := h.config.DefaultTravelTime // Tempo de viagem placeholder
			// O próximo TimeSlot começa após o término da recarga anterior + tempo de viagem
			estimatedArrivalTimeAtStop = stops[i-1].TimeSlot.Add(prevStopDuration + estimatedTravelTime)
		}

		// Usa a duração de recarga padrão definida na configuração do BookingHandler.
		// Para testes, você pode definir um valor pequeno em DefaultChargingDuration.
		durationMinutes := h.config.DefaultChargingDuration

		stop := ChargingStopRequest{
			SegmentID:       uuid.New().String(),        // ID único para esta parada/segmento
			EnterpriseName:  stopInfo.EnterpriseName,    // Empresa da parada
			City:            stopInfo.City,              // Cidade da parada
			// ChargingPointID não é preenchido aqui. Será determinado pela empresa que confirma.
			TimeSlot:        estimatedArrivalTimeAtStop.Truncate(time.Minute), // Horário de início da reserva (arredondado para o minuto)
			DurationMinutes: durationMinutes,            // Duração padronizada
			Status:          schemas.StatusStopPending,  // Estado inicial da parada
		}
		stops = append(stops, stop)
	}

	if len(stops) == 0 { // Deve ser redundante se a validação de len(routeInfo) == 0 já foi feita.
		return fmt.Errorf("o planejamento da rota não resultou em paradas de recarga para %s -> %s", req.Origin, req.Destination)
	}

	req.ChargingStops = stops
	return nil
}