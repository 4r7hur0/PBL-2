package schemas

import "time"

type ChargingResquest struct {
	EnterpriseName  string `json:"enterprise_name"`
	CarID           string `json:"car_id"`
	BatteryLevel    int    `json:"battery_level"`
	DischargeRate   string `json:"discharge_rate"`
	OriginCity      string `json:"origin_city"`
	DestinationCity string `json:"destination_city"`
}

type Routes struct {
	City      string    `json:"city"`
	TimeStamp time.Time `json:"timestamp"`
}

type Enterprises struct {
	Name string `json:"name"`
	City string `json:"city"`
}
