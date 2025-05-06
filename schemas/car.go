package schemas

type chargingResquest struct {
	EnterpriseName string   `json:"enterprise_name"`
	CarID          string   `json:"car_id"`
	BatteryLevel   int      `json:"battery_level"`
	DischaergeRate string   `json:"discharge_rate"`
	Route          []string `json:"route"`
}
