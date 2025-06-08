package chaincode

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type smartContract struct {
	contractapi.Contract
}

type RouteSegmentAsset struct {
	City         string `json:"city"`
	StartTimeUTC string `json:"startTimeUTC"`
	EndTimeUTC   string `json:"endTimeUTC"`
}

type ChargingTransaction struct {
	TransactionID             string `json:"transactionId"`
	VeicleID                  string `json:"vehicleId"`
	Route                     []RouteSegmentAsset
	Status                    string  `json:"status"`
	Cost                      float64 `json:"cost"`
	EnergyConsumed            float64 `json:"energyConsumed"`
	ReservationTimeStampUTC   string  `json:"reservationTimeStampUTC"`
	ChargingStartTimeStampUTC string  `json:"chargingStartTimeStampUTC"`
	ChargingEndTimeStampUTC   string  `json:"chargingEndTimeStampUTC"`
	PaymantTimeStampUTC       string  `json:"paymentTimeStampUTC"`
}

func (s *smartContract) RegisterReserve(ctx contractapi.TransactionContextInterface, transactionID string, vehicleID string, routeJSON string) error {
	exists, err := s.transactionExists(ctx, transactionID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("transaction with ID %s already exists", transactionID)
	}

	var route []RouteSegmentAsset
	err = json.Unmarshal([]byte(routeJSON), &route)
	if err != nil {
		return fmt.Errorf("failed to parse route JSON: %v", err)
	}

	// cria o objeto de transação
	transaction := ChargingTransaction{
		TransactionID:           transactionID,
		VeicleID:                vehicleID,
		Route:                   route,
		Status:                  "RESERVED",
		Cost:                    0.0,
		EnergyConsumed:          0.0,
		ReservationTimeStampUTC: time.Now().UTC().Format(time.RFC3339),
	}

	assetBytes, err := json.Marshal(transaction)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	// grava o objeto de transação no ladger
	return ctx.GetStub().PutState(transactionID, assetBytes)
}

// Atualiza o status da transação para "CHARGING" e registra o horário de início do carregamento
func (s *smartContract) StartCharging(ctx contractapi.TransactionContextInterface, transactionID string) error {
	asset, err := s.getTransaction(ctx, transactionID)
	if err != nil {
		return err
	}

	// verifica se a transação está reservada
	if asset.Status != "RESERVED" {
		return fmt.Errorf("transaction with ID %s is not in RESERVED status", transactionID)
	}

	asset.Status = "CHARGING"
	asset.ChargingStartTimeStampUTC = time.Now().UTC().Format(time.RFC3339)

	assetBytes, err := json.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}
	// atualiza o estado da transação no ledger
	return ctx.GetStub().PutState(transactionID, assetBytes)
}

func (s *smartContract) EndCharging(ctx contractapi.TransactionContextInterface, transactionID string, cost float64, energyConsumed float64) error {
	asset, err := s.getTransaction(ctx, transactionID)
	if err != nil {
		return err
	}

	if asset.Status != "CHARGING" {
		return fmt.Errorf("transaction with ID %s is not in CHARGING status", transactionID)
	}

	asset.Status = "COMPLETED"
	asset.Cost = cost
	asset.EnergyConsumed = energyConsumed
	asset.ChargingEndTimeStampUTC = time.Now().UTC().Format(time.RFC3339)

	assetBytes, err := json.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}
	// atualiza o estado da transação no ledger
	return ctx.GetStub().PutState(transactionID, assetBytes)
}

func (s *smartContract) RegisterPayment(ctx contractapi.TransactionContextInterface, transactionID string) error {
	asset, err := s.getTransaction(ctx, transactionID)
	if err != nil {
		return err
	}

	if asset.Status != "COMPLETED" {
		return fmt.Errorf("transaction with ID %s is not in COMPLETED status", transactionID)
	}

	asset.Status = "PAID"
	asset.PaymantTimeStampUTC = time.Now().UTC().Format(time.RFC3339)

	assetBytes, err := json.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}
	// atualiza o estado da transação no ledger
	return ctx.GetStub().PutState(transactionID, assetBytes)
}

func (s *smartContract) QueryTransaction(ctx contractapi.TransactionContextInterface, transactionID string) (*ChargingTransaction, error) {
	return s.getTransaction(ctx, transactionID)
}

// Funções auxiliares:

// recupera uma transação do ledger
func (s *smartContract) getTransaction(ctx contractapi.TransactionContextInterface, transactionID string) (*ChargingTransaction, error) {
	assetBytes, err := ctx.GetStub().GetState(transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %v", err)
	}
	if assetBytes == nil {
		return nil, fmt.Errorf("transaction with ID %s does not exist", transactionID)
	}

	var transaction ChargingTransaction
	err = json.Unmarshal(assetBytes, &transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %v", err)
	}

	return &transaction, nil
}

// verifica se uma transação já existe
func (s *smartContract) transactionExists(ctx contractapi.TransactionContextInterface, transactionID string) (bool, error) {
	assetBytes, err := ctx.GetStub().GetState(transactionID)
	if err != nil {
		return false, fmt.Errorf("failed to read from world state: %v", err)
	}

	return assetBytes != nil, nil
}

func main() {
	chaincode, err := contractapi.NewChaincode(&smartContract{})
	if err != nil {
		fmt.Printf("Error creating smart contract: %v", err)
		return
	}
	if err := chaincode.Start(); err != nil {
		fmt.Printf("Error starting smart contract: %v", err)
	}
}
