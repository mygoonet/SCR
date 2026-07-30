package SCRP

import "os"

type DeliveryNote struct {
	Number           string `json:"number"`
	Date             string `json:"date"`
	Consignor        string `json:"consignor"`
	ConsignorAddress string `json:"consignorAddress"`
	Consignee        string `json:"consignee"`
	ConsigneeAddress string `json:"consigneeAddress"`
	Carrier          string `json:"carrier"`
	Driver           string `json:"driver"`
	DriverPhone      string `json:"driverPhone"`
	Truck            string `json:"truck"`
	Test             string `json:"test,omitempty"`
}

type Config struct {
	UserDataDir string
	ChromePath  string
	CertUser    string
}

func ConfigFromEnv() Config {
	return Config{
		UserDataDir: os.Getenv("USER_DATA_DIR"),
		ChromePath:  os.Getenv("CHROME_PATH"),
		CertUser:    os.Getenv("CERT_USER"),
	}
}


