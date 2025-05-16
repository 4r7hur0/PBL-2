package booking

import (
	"fmt"
	"time"

	"github.com/4r7hur0/PBL-2/schemas"
	"github.com/google/uuid"
)

// cityEnterpriseMap e allCities como antes
var cityEnterpriseMap = map[string]string{
	"Salvador":         "SolAtlantico",
	"Feira de Santana": "SertãoCarga",
	"Ilhéus":           "CacauPower",
}
var allCities = []string{"Salvador", "Feira de Santana", "Ilhéus"}

type BookingRequest struct {
	BookingID     string                `json:"booking_id"`
	CarID         string                `json:"car_id"`
	Origin        string                `json:"origin"`
	Destination   string                `json:"destination"`
	BatteryLevel  int                   `json:"battery_level"`
	DischargeRate string                `json:"discharge_rate"`
	ChargingStops []ChargingStopRequest `json:"charging_stops"` // Paradas escolhidas pelo cliente
	Status        string                `json:"status"`
	CreatedAt     time.Time             `json:"created_at"`
}

// ChargingStopRequest (interno, para o processo de reserva 2PC) - permanece o mesmo
type ChargingStopRequest struct {
	SegmentID       string    `json:"segment_id"` // Este é o ID que vai no PrepareRequestBody
	EnterpriseName  string    `json:"enterprise_name"`
	City            string    `json:"city"`
	ChargingPointID string    `json:"charging_point_id,omitempty"` // Pode ser preenchido pela empresa que confirma
	TimeSlot        time.Time `json:"time_slot"`
	DurationMinutes int       `json:"duration_minutes"`
	Status          string    `json:"status"` // Status interno do coordenador para esta parada
}

// buildSegmentsForOption agora constrói []schemas.RouteSegment para enviar ao cliente
func (h *BookingHandler) buildSegmentsForOption(path []string, baseRequestTime time.Time) []schemas.RouteSegment {
	var segments []schemas.RouteSegment
	estimatedArrivalTimeAtStop := baseRequestTime.Add(h.config.DefaultPreparationBuffer)

	for i, cityName := range path {
		enterpriseName, _ := cityEnterpriseMap[cityName]
		durationMinutes := h.config.DefaultChargingDuration

		if i > 0 {
			prevSegment := segments[i-1] // É um schemas.RouteSegment
			prevStartTime, _ := time.Parse(schemas.ISOFormat, prevSegment.ReservationWindow.StartTimeUTC)
			// A DurationMinutes está em RouteSegment agora
			prevActualDuration := time.Duration(prevSegment.DurationMinutes) * time.Minute
			estimatedArrivalTimeAtStop = prevStartTime.Add(prevActualDuration + h.config.DefaultTravelTime)
		}

		segment := schemas.RouteSegment{
			SegmentID:         uuid.New().String(), // ID único para esta parada NA PROPOSTA DE ROTA
			EnterpriseName:    enterpriseName,
			City:              cityName,
			ReservationWindow: h.calculateReservationWindow(estimatedArrivalTimeAtStop.Truncate(time.Minute), durationMinutes),
			DurationMinutes:   durationMinutes,
		}
		segments = append(segments, segment)
	}
	return segments
}

// generateRouteOptions retorna []schemas.RouteOptionForClient
func (h *BookingHandler) generateRouteOptions(originCity, destinationCity string, baseRequestTime time.Time) ([]schemas.RouteOptionForClient, error) {
	if _, ok := cityEnterpriseMap[originCity]; !ok {
		return nil, fmt.Errorf("cidade de origem desconhecida: %s", originCity)
	}
	if _, ok := cityEnterpriseMap[destinationCity]; !ok {
		return nil, fmt.Errorf("cidade de destino desconhecida: %s", destinationCity)
	}
	if originCity == destinationCity {
		return nil, fmt.Errorf("origem e destino não podem ser a mesma cidade")
	}

	var allRouteOptions []schemas.RouteOptionForClient
	optionCounter := 0
	routeKeyBase := fmt.Sprintf("%s_%s", originCity, destinationCity)

	// 1. Rota Direta
	directPath := []string{originCity, destinationCity}
	directSegments := h.buildSegmentsForOption(directPath, baseRequestTime)
	if len(directSegments) > 0 {
		optionCounter++
		allRouteOptions = append(allRouteOptions, schemas.RouteOptionForClient{
			OptionID: fmt.Sprintf("OPT_D_%s_%d", routeKeyBase, optionCounter),
			Segments: directSegments,
		})
	}

	// 2. Rota Indireta
	var intermediateCity string
	for _, city := range allCities {
		if city != originCity && city != destinationCity {
			intermediateCity = city
			break
		}
	}
	if intermediateCity != "" {
		indirectPath := []string{originCity, intermediateCity, destinationCity}
		indirectSegments := h.buildSegmentsForOption(indirectPath, baseRequestTime)
		if len(indirectSegments) > 0 {
			optionCounter++
			allRouteOptions = append(allRouteOptions, schemas.RouteOptionForClient{
				OptionID: fmt.Sprintf("OPT_I_%s_%d", routeKeyBase, optionCounter),
				Segments: indirectSegments,
			})
		}
	}

	if len(allRouteOptions) == 0 {
		return nil, fmt.Errorf("nenhuma opção de rota pôde ser gerada de %s para %s", originCity, destinationCity)
	}
	return allRouteOptions, nil
}
