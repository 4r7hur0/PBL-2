package booking

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/4r7hur0/PBL-2/schemas"
)

// BookingRequest (estrutura interna do BookingHandler para representar a requisição de reserva da rota inteira)
type BookingRequest struct {
	BookingID     string    `json:"booking_id"`     // ID único para toda a transação de reserva da rota
	CarID         string    `json:"car_id"`         // ID do carro
	Origin        string    `json:"origin"`         // Cidade de origem da viagem
	Destination   string    `json:"destination"`    // Cidade de destino da viagem
	BatteryLevel  int       `json:"battery_level"`  // Nível de bateria do carro
	DischargeRate string    `json:"discharge_rate"` // Taxa de descarga
	// RouteIdentifier string `json:"route_identifier,omitempty"` // Se o carro enviar um identificador de rota específico

	ChargingStops []ChargingStopRequest `json:"charging_stops"` // Lista detalhada das paradas planejadas
	Status        string                `json:"status"`         // Status geral do BookingRequest (ex: PENDING, PREPARING, CONFIRMED, FAILED, ABORTED)
	CreatedAt     time.Time             `json:"created_at"`
	// Adicionar campos para URLs de callback se o BookingHandler precisar expô-los para si mesmo
	// ou se as URLs de callback forem dinâmicas por transação.
	// Para simplificar, assumiremos que as URLs de callback são fixas para esta instância do BookingHandler.
}

// ChargingStopRequest é a estrutura interna para detalhar cada parada de recarga.
// Ela é preenchida pelo planRoute com base nas rotas predefinidas.
type ChargingStopRequest struct {
	SegmentID       string    `json:"segment_id"`       // ID único para esta parada/segmento da transação (pode ser o ReservationID anterior)
	EnterpriseName  string    `json:"enterprise_name"`  // Nome da empresa do ponto de recarga
	City            string    `json:"city"`             // Cidade do ponto de recarga
	ChargingPointID string    `json:"charging_point_id"`// ID do ponto de recarga específico
	TimeSlot        time.Time `json:"time_slot"`        // Horário de início da reserva para esta parada
	DurationMinutes int       `json:"duration_minutes"` // Duração da reserva em minutos
	Status          string    `json:"status"`           // Status desta parada (ex: PENDING, PREPARED, COMMITTED, ABORTED)
	// PreparedUntilUTC time.Time `json:"prepared_until_utc,omitempty"` // Se a preparação tiver um tempo de expiração
	// ErrorMessage string    `json:"error_message,omitempty"`        // Mensagem de erro se esta parada falhar
}

// --- Estruturas para o Banco de Dados de Rotas Predefinidas ---

type RouteStopInfo struct {
	EnterpriseName  string
	City            string
	ChargingPointID string
	// DefaultDurationMinutes int // Opcional: Duração padrão para esta parada na rota
}

// predefinedRoutesDB armazena as rotas predefinidas.
// A chave é "Origem_Destino" ou um identificador de rota mais específico.
// Os nomes das empresas e ChargingPointID devem corresponder ao PDF.
var predefinedRoutesDB = map[string][]RouteStopInfo{
	"Salvador_Feira de Santana": {
		{EnterpriseName: "Empresa Sol Atlântico", City: "Salvador", ChargingPointID: "SA-P1"},
		{EnterpriseName: "Empresa Chapada Eletric", City: "Lençóis", ChargingPointID: "LC-P1"},
		{EnterpriseName: "Empresa Sertão Carga", City: "Feira de Santana", ChargingPointID: "FS-P1"},
	},
	"Salvador_Lençóis": {
		{EnterpriseName: "Empresa Sol Atlântico", City: "Salvador", ChargingPointID: "SA-P2"},
		{EnterpriseName: "Empresa Sertão Carga", City: "Feira de Santana", ChargingPointID: "FS-P2"},
		{EnterpriseName: "Empresa Chapada Eletric", City: "Lençóis", ChargingPointID: "LC-P2"},
	},
	"Salvador_Ilhéus": {
		{EnterpriseName: "Empresa Sol Atlântico", City: "Salvador", ChargingPointID: "SA-P3"},
		{EnterpriseName: "Empresa Chapada Eletric", City: "Lençóis", ChargingPointID: "LC-P1"},
		{EnterpriseName: "Empresa Velho Chico Energia", City: "Juazeiro", ChargingPointID: "JZ-P1"},
		{EnterpriseName: "Empresa Cacau Power", City: "Ilhéus", ChargingPointID: "IL-P1"},
	},
	// ... COLOQUE TODAS AS 20 ROTAS DO SEU PDF AQUI ...
	// Exemplo de rota iniciada por outra empresa, para referência, mas o DB é global.
	"Feira de Santana_Salvador": {
		{EnterpriseName: "Empresa Sertão Carga", City: "Feira de Santana", ChargingPointID: "FS-P3"},
		{EnterpriseName: "Empresa Cacau Power", City: "Ilhéus", ChargingPointID: "IL-P3"},
		{EnterpriseName: "Empresa Sol Atlântico", City: "Salvador", ChargingPointID: "SA-P1"},
	},
}

// GetPredefinedRoute busca uma rota no "banco de dados".
// routeIdentifier é opcional e pode ser usado para variantes da mesma O/D.
func GetPredefinedRoute(originCity, destinationCity, routeIdentifier string) ([]RouteStopInfo, bool) {
	key := originCity + "_" + destinationCity
	if routeIdentifier != "" {
		// Poderia ser uma chave como "Origin_Destination_RouteID" ou apenas RouteID
		// Depende de como você quer identificar as rotas no predefinedRoutesDB
		key = routeIdentifier // Assumindo que RouteIdentifier é a chave completa se fornecido
	}
	route, exists := predefinedRoutesDB[key]
	return route, exists
}

// planRoute usa a Origin/Destination da BookingRequest para preencher ChargingStops detalhadas.
func (h *BookingHandler) planRoute(req *BookingRequest /*carReq *schemas.ChargingResquest - não precisa mais, req já tem os dados*/) error {
	// req.Origin e req.Destination vêm do schemas.ChargingResquest.OriginCity/DestinationCity
	// req.RouteIdentifier vem do schemas.ChargingResquest.RouteIdentifier (se existir)
	routeInfo, exists := GetPredefinedRoute(req.Origin, req.Destination, "") // Passar req.RouteIdentifier se ele for usado como chave
	if !exists {
		return fmt.Errorf("no predefined route found from %s to %s", req.Origin, req.Destination)
	}
	if len(routeInfo) == 0 {
		return fmt.Errorf("predefined route from %s to %s is empty", req.Origin, req.Destination)
	}

	var stops []ChargingStopRequest
	currentTime := time.Now()
	// TimeSlot da primeira parada. Pode precisar de ajuste se a primeira parada não for imediata.
	estimatedStartTime := currentTime.Add(h.config.DefaultPreparationBuffer) // Ex: 15 minutos de buffer

	// Validar se a primeira parada da rota corresponde à origem da requisição
	// e se a empresa da primeira parada é a empresa local (se este for um requisito)
	firstStopDefinition := routeInfo[0]
	if !(firstStopDefinition.City == req.Origin) {
		return fmt.Errorf("predefined route for %s->%s has an invalid first stop city: %s. It should start in %s",
			req.Origin, req.Destination, firstStopDefinition.City, req.Origin)
	}
	// Se a primeira parada da rota predefinida não for da empresa local,
	// a empresa local está apenas orquestrando. Isso é um cenário válido.

	currentEndTime := estimatedStartTime
	for i, stopInfo := range routeInfo {
		var slotToUse time.Time
		if i == 0 {
			slotToUse = currentEndTime // Para a primeira parada, é o estimatedStartTime
		} else {
			travelTimeToNextStop := h.config.DefaultTravelTime // Ex: 2 horas (placeholder)
			slotToUse = currentEndTime.Add(travelTimeToNextStop)
		}

		// Duração padrão da recarga, pode vir da stopInfo se definido lá
		durationMinutes := h.config.DefaultChargingDuration

		stop := ChargingStopRequest{
			SegmentID:       uuid.New().String(), // ID único para este segmento/parada da reserva
			EnterpriseName:  stopInfo.EnterpriseName,
			City:            stopInfo.City,
			ChargingPointID: stopInfo.ChargingPointID,
			TimeSlot:        slotToUse.Truncate(time.Minute), // Trunca para minutos
			DurationMinutes: durationMinutes,
			Status:          schemas.StatusStopPending, // Usando uma constante de status (definir em schemas ou booking)
		}
		stops = append(stops, stop)
		currentEndTime = slotToUse.Add(time.Duration(durationMinutes) * time.Minute)
	}

	if len(stops) == 0 {
		return fmt.Errorf("route planning resulted in no charging stops for %s to %s", req.Origin, req.Destination)
	}

	req.ChargingStops = stops
	return nil
}