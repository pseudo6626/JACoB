package platform

type InputDeviceReport struct {
	Source        string `json:"source"`
	KeyboardCount int    `json:"keyboardCount"`
	MouseCount    int    `json:"mouseCount"`
	HIDCount      int    `json:"hidCount,omitempty"`
	Error         string `json:"error,omitempty"`
}

func ProbeInputDevices() InputDeviceReport { return probeInputDevices() }

func GameRunning() bool { return gameRunning() }
