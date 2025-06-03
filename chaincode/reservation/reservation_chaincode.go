package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// ReservationAsset descreve os detalhes de uma reserva armazenada no ledger.
// Usamos strings para timestamps para garantir consistência na serialização JSON.
type ReservationAsset struct {
	TransactionID     string              `json:"transactionID"`     // ID da transação 2PC original, servirá como chave no ledger
	OriginalRequestID string              `json:"originalRequestID"` // ID da requisição de rota original do carro
	VehicleID         string              `json:"vehicleID"`
	Route             []RouteSegmentAsset `json:"route"`
	StatusOnChain     string              `json:"statusOnChain"` // Ex: "RESERVATION_RECORDED"
	RecordedAt        string              `json:"recordedAt"`    // Timestamp de quando foi gravado no blockchain (ISO8601)
}

// RouteSegmentAsset é a representação de um trecho de rota para o chaincode.
type RouteSegmentAsset struct {
	City         string `json:"city"`
	StartTimeUTC string `json:"startTimeUTC"` // Formato: "YYYY-MM-DDTHH:mm:ssZ"
	EndTimeUTC   string `json:"endTimeUTC"`   // Formato: "YYYY-MM-DDTHH:mm:ssZ"
}

// SmartContract fornece as funções para gerenciar as reservas.
type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) InitLedger(ctx contractapi.TransactionContextInterface) error {
	fmt.Println("Chaincode de Reserva Inicializado (InitLedger não faz nada neste exemplo)")
	return nil
}

// RecordReservation registra uma nova reserva no ledger.
// Args: transactionID, originalRequestID, vehicleID, routeJSON (string JSON de []RouteSegmentAsset), statusOnChain
func (s *SmartContract) RecordReservation(ctx contractapi.TransactionContextInterface, transactionID string, originalRequestID string, vehicleID string, routeJSON string, statusOnChain string) error {
	// Verificar se a reserva já existe
	exists, err := s.ReservationExists(ctx, transactionID)
	if err != nil {
		return fmt.Errorf("falha ao verificar existência da reserva: %v", err)
	}
	if exists {
		return fmt.Errorf("a reserva com ID %s já existe", transactionID)
	}

	var route []RouteSegmentAsset
	err = json.Unmarshal([]byte(routeJSON), &route)
	if err != nil {
		return fmt.Errorf("falha ao deserializar os segmentos da rota: %v. JSON recebido: %s", err, routeJSON)
	}

	reservation := ReservationAsset{
		TransactionID:     transactionID,
		OriginalRequestID: originalRequestID,
		VehicleID:         vehicleID,
		Route:             route,
		StatusOnChain:     statusOnChain,
		RecordedAt:        time.Now().UTC().Format(time.RFC3339), // Timestamp atual em UTC ISO8601
	}

	reservationBytes, err := json.Marshal(reservation)
	if err != nil {
		return fmt.Errorf("falha ao serializar a reserva: %v", err)
	}

	// Coloca o estado no ledger usando o transactionID como chave
	err = ctx.GetStub().PutState(transactionID, reservationBytes)
	if err != nil {
		return fmt.Errorf("falha ao registrar a reserva no ledger: %v", err)
	}
	fmt.Printf("Reserva %s registrada com sucesso.\n", transactionID)
	return nil
}

// QueryReservation retorna os detalhes de uma reserva armazenada no ledger.
// Args: transactionID
func (s *SmartContract) QueryReservation(ctx contractapi.TransactionContextInterface, transactionID string) (*ReservationAsset, error) {
	reservationBytes, err := ctx.GetStub().GetState(transactionID)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler a reserva %s do ledger: %v", transactionID, err)
	}
	if reservationBytes == nil {
		return nil, fmt.Errorf("a reserva com ID %s não existe", transactionID)
	}

	var reservation ReservationAsset
	err = json.Unmarshal(reservationBytes, &reservation)
	if err != nil {
		return nil, fmt.Errorf("falha ao deserializar a reserva %s: %v", transactionID, err)
	}

	return &reservation, nil
}

// ReservationExists verifica se uma reserva com o ID fornecido já existe no ledger.
// Args: transactionID
func (s *SmartContract) ReservationExists(ctx contractapi.TransactionContextInterface, transactionID string) (bool, error) {
	reservationBytes, err := ctx.GetStub().GetState(transactionID)
	if err != nil {
		return false, fmt.Errorf("falha ao ler do ledger: %v", err)
	}
	return reservationBytes != nil, nil
}

// Função main para iniciar o chaincode
func main() {
	chaincode, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		fmt.Printf("Erro ao criar chaincode de reserva: %s\n", err.Error())
		return
	}

	if err := chaincode.Start(); err != nil {
		fmt.Printf("Erro ao iniciar chaincode de reserva: %s\n", err.Error())
	}
}
