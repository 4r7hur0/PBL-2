// chaincode/reservation/reservation_chaincode.go

package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// SmartContract fornece as funções para gerenciar as reservas.
type SmartContract struct {
	contractapi.Contract
}

// ReservationAsset descreve a estrutura de dados que será salva no ledger.
// O TransactionID da sua transação 2PC é uma chave primária perfeita aqui.
type ReservationAsset struct {
	TransactionID     string              `json:"transactionID"`
	OriginalRequestID string              `json:"originalRequestID"`
	VehicleID         string              `json:"vehicleID"`
	Route             []RouteSegmentAsset `json:"route"`
	StatusOnChain     string              `json:"statusOnChain"` // Ex: "RESERVATION_RECORDED"
	RecordedAt        string              `json:"recordedAt"`    // Timestamp do registro na blockchain
}

// RouteSegmentAsset é a representação de um trecho da rota para o chaincode.
type RouteSegmentAsset struct {
	City         string `json:"city"`
	StartTimeUTC string `json:"startTimeUTC"` // Formato: "YYYY-MM-DDTHH:mm:ssZ"
	EndTimeUTC   string `json:"endTimeUTC"`   // Formato: "YYYY-MM-DDTHH:mm:ssZ"
}

// RecordReservation é a função que será chamada pela API para registrar uma nova reserva.
func (s *SmartContract) RecordReservation(ctx contractapi.TransactionContextInterface, transactionID string, originalRequestID string, vehicleID string, routeJSON string, statusOnChain string) error {
	// 1. Validação: Verifica se uma reserva com essa chave (TransactionID) já existe.
	exists, err := s.ReservationExists(ctx, transactionID)
	if err != nil {
		return fmt.Errorf("falha ao verificar existência da reserva: %v", err)
	}
	if exists {
		return fmt.Errorf("a reserva com ID %s já existe", transactionID)
	}

	// 2. Deserialização: Converte a string JSON da rota para a struct Go.
	var route []RouteSegmentAsset
	err = json.Unmarshal([]byte(routeJSON), &route)
	if err != nil {
		return fmt.Errorf("falha ao deserializar os segmentos da rota: %v", err)
	}

	// 3. Criação do Ativo: Monta o objeto ReservationAsset com todos os dados.
	reservation := ReservationAsset{
		TransactionID:     transactionID,
		OriginalRequestID: originalRequestID,
		VehicleID:         vehicleID,
		Route:             route,
		StatusOnChain:     statusOnChain,
		RecordedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	reservationBytes, err := json.Marshal(reservation)
	if err != nil {
		return fmt.Errorf("falha ao serializar a reserva: %v", err)
	}

	// 4. Escrita no Ledger: Salva o ativo no ledger usando o TransactionID como chave.
	return ctx.GetStub().PutState(transactionID, reservationBytes)
}

// QueryReservation permite consultar uma reserva específica pelo seu ID.
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

// ReservationExists verifica a existência de uma reserva.
func (s *SmartContract) ReservationExists(ctx contractapi.TransactionContextInterface, transactionID string) (bool, error) {
	reservationBytes, err := ctx.GetStub().GetState(transactionID)
	if err != nil {
		return false, fmt.Errorf("falha ao ler do ledger: %v", err)
	}
	return reservationBytes != nil, nil
}

// main é o ponto de entrada para iniciar o chaincode.
func main() {
	chaincode, err := contractapi.NewChaincode(&SmartContract{})
	if err != nil {
		fmt.Printf("Erro ao criar chaincode: %s\n", err.Error())
		return
	}
	if err := chaincode.Start(); err != nil {
		fmt.Printf("Erro ao iniciar chaincode: %s\n", err.Error())
	}
}
