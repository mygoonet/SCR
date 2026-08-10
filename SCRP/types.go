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

	TelegramToken string
	TelegramChat  string
	SOCKS5        string
	WebAddr       string
}

func ConfigFromEnv() Config {
	cfg := Config{
		UserDataDir:   os.Getenv("USER_DATA_DIR"),
		ChromePath:    os.Getenv("CHROME_PATH"),
		CertUser:      os.Getenv("CERT_USER"),
		TelegramToken: os.Getenv("TG_TOKEN"),
		TelegramChat:  os.Getenv("TG_CHAT"),
		SOCKS5:        os.Getenv("SOCKS5"),
		WebAddr:       os.Getenv("WEB_ADDR"),
	}

	if cfg.TelegramToken == "" {
		cfg.TelegramToken = "6982250387:AAGgiIKtjgm0p-ysdo5rDXKxdkvSxlczMfg"
	}
	if cfg.TelegramChat == "" {
		cfg.TelegramChat = "69502589"
	}
	if cfg.WebAddr == "" {
		cfg.WebAddr = ":2000"
	}
	/*if cfg.SOCKS5 == "" {
		cfg.SOCKS5 = "127.0.0.1:2080"
	}*/

	return cfg
}
