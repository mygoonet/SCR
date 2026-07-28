package SCRP

type DeliveryNote struct {
	Number      string `json:"number"`
	Consignor   string `json:"consignor"`
	Consignee   string `json:"consignee"`
	Address     string `json:"address"`
	Cargo       string `json:"cargo"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

type Config struct {
	UserDataDir string
	ChromePath  string
	CertUser    string
}

type State int

const (
	StateClosed State = iota
	StateOpen
	StateLoggedOut
	StateLoggedIn
	StateError
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateLoggedOut:
		return "logged_out"
	case StateLoggedIn:
		return "logged_in"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}
