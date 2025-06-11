package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"

    "github.com/4r7hur0/PBL-2/schemas"
    "github.com/4r7hur0/PBL-2/api/mqtt"
)

type ReservationWindow struct {
    StartTimeUTC  time.Time
    EndTimeUTC    time.Time
    TransactionID string
    Status        string // "prepared", "committed", "charged", "aborted"
}

type ChargingPointWorker struct {
    ID           string
    Reservations []ReservationWindow
}

func (cpw *ChargingPointWorker) isAvailable(window schemas.ReservationWindow) bool {
    for _, r := range cpw.Reservations {
        if r.Status != "aborted" && r.Status != "charged" &&
            !(window.EndTimeUTC.Before(r.StartTimeUTC) || window.StartTimeUTC.After(r.EndTimeUTC)) {
            return false
        }
    }
    return true
}

func (cpw *ChargingPointWorker) handleMQTTMessage(topic, payload string) {
    var msg map[string]interface{}
    if err := json.Unmarshal([]byte(payload), &msg); err != nil {
        log.Printf("Erro ao decodificar mensagem MQTT: %v", err)
        return
    }

    cmd, _ := msg["command"].(string)
    switch cmd {
    case "QUERY_AVAILABILITY":
        var window schemas.ReservationWindow
        b, _ := json.Marshal(msg["window"])
        json.Unmarshal(b, &window)
        available := cpw.isAvailable(window)
        resp := map[string]interface{}{
            "command":   "AVAILABILITY_RESPONSE",
            "available": available,
            "window":    window,
            "worker_id": cpw.ID,
        }
        respBytes, _ := json.Marshal(resp)
        mqtt.Publish(fmt.Sprintf("enterprise/%s/cp/%s/response", os.Getenv("ENTERPRISE_NAME"), cpw.ID), string(respBytes))
    case "PREPARE_RESERVE_WINDOW":
        var window schemas.ReservationWindow
        b, _ := json.Marshal(msg["window"])
        json.Unmarshal(b, &window)
        txID, _ := msg["transaction_id"].(string)
        if cpw.isAvailable(window) {
            cpw.Reservations = append(cpw.Reservations, ReservationWindow{
                StartTimeUTC:  window.StartTimeUTC,
                EndTimeUTC:    window.EndTimeUTC,
                TransactionID: txID,
                Status:        "prepared",
            })
            resp := map[string]interface{}{
                "command":       "PREPARE_RESPONSE",
                "success":       true,
                "transaction_id": txID,
                "worker_id":     cpw.ID,
            }
            respBytes, _ := json.Marshal(resp)
            mqtt.Publish(fmt.Sprintf("enterprise/%s/cp/%s/response", os.Getenv("ENTERPRISE_NAME"), cpw.ID), string(respBytes))
        } else {
            resp := map[string]interface{}{
                "command":       "PREPARE_RESPONSE",
                "success":       false,
                "transaction_id": txID,
                "worker_id":     cpw.ID,
            }
            respBytes, _ := json.Marshal(resp)
            mqtt.Publish(fmt.Sprintf("enterprise/%s/cp/%s/response", os.Getenv("ENTERPRISE_NAME"), cpw.ID), string(respBytes))
        }
    case "COMMIT":
        txID, _ := msg["transaction_id"].(string)
        for i, r := range cpw.Reservations {
            if r.TransactionID == txID && r.Status == "prepared" {
                cpw.Reservations[i].Status = "committed"
            }
        }
    case "ABORT":
        txID, _ := msg["transaction_id"].(string)
        for i, r := range cpw.Reservations {
            if r.TransactionID == txID && r.Status == "prepared" {
                cpw.Reservations[i].Status = "aborted"
            }
        }
    }
}

// Rotina para detectar passagem do tempo e cobrar
func (cpw *ChargingPointWorker) monitorPassageAndCharge() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    for {
        <-ticker.C
        now := time.Now().UTC()
        for i, r := range cpw.Reservations {
            if r.Status == "committed" && now.After(r.EndTimeUTC) {
                // Gera custo fixo
                cost := 20.0
                cpw.Reservations[i].Status = "charged"
                // Publica evento para API
                event := map[string]interface{
                }{
                    "command":        "VEHICLE_PASSED_AND_CHARGED",
                    "transaction_id": r.TransactionID,
                    "cost":           cost,
                    "window": map[string]interface{}{
                        "start_time_utc": r.StartTimeUTC,
                        "end_time_utc":   r.EndTimeUTC,
                    },
                    "worker_id": cpw.ID,
                }
                eventBytes, _ := json.Marshal(event)
                mqtt.Publish(fmt.Sprintf("enterprise/%s/cp/%s/event", os.Getenv("ENTERPRISE_NAME"), cpw.ID), string(eventBytes))
                log.Printf("Reserva %s cobrada e notificada para API.", r.TransactionID)
            }
        }
    }
}

func main() {
    workerID := os.Getenv("WORKER_ID")
    if workerID == "" {
        workerID = "CP001"
    }
    cpw := &ChargingPointWorker{
        ID: workerID,
    }
    mqtt.InitializeMQTT("tcp://mosquitto:1883")
    commandTopic := fmt.Sprintf("enterprise/%s/cp/%s/command", os.Getenv("ENTERPRISE_NAME"), workerID)
    msgChan := mqtt.StartListening(commandTopic, 10)
    log.Printf("ChargingPointWorker %s iniciado. Escutando em %s", workerID, commandTopic)

    // Inicia rotina de monitoramento de passagem e cobrança
    go cpw.monitorPassageAndCharge()

    for msg := range msgChan {
        cpw.handleMQTTMessage(commandTopic, msg)
    }
}